package cursor

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"strings"
)

const (
	maxInlineImageCount        = 20
	maxInlineImageBytes        = 5 << 20
	maxInlineImagesTotalBytes  = 6 << 20
	maxInlineImageBase64Length = ((maxInlineImageBytes + 2) / 3) * 4
)

type inlineImageBudget struct {
	count int
	bytes int
}

func (b *inlineImageBudget) add(image InlineImage) error {
	if b == nil {
		return nil
	}
	if b.count >= maxInlineImageCount {
		return fmt.Errorf("inline image count exceeds %d", maxInlineImageCount)
	}
	if len(image.Data) > maxInlineImageBytes {
		return fmt.Errorf("inline image exceeds %d MiB limit", maxInlineImageBytes>>20)
	}
	if b.bytes+len(image.Data) > maxInlineImagesTotalBytes {
		return fmt.Errorf("inline images exceed %d MiB total limit", maxInlineImagesTotalBytes>>20)
	}
	b.count++
	b.bytes += len(image.Data)
	return nil
}

func ParseRequest(protocol Protocol, data []byte) (*Dialogue, error) {
	switch protocol {
	case ProtocolAnthropic:
		return ParseAnthropic(data)
	case ProtocolOpenAIChat:
		return ParseOpenAIChat(data)
	case ProtocolResponses:
		return ParseResponses(data)
	default:
		return nil, &Error{Kind: ErrorBadRequest, Operation: "parse request", Err: fmt.Errorf("unsupported protocol %q", protocol)}
	}
}

type rawTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
	Parameters  json.RawMessage `json:"parameters"`
	Function    *struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type rawMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolCalls  []rawToolCall   `json:"tool_calls"`
	ToolCallID string          `json:"tool_call_id"`
}

type rawToolCall struct {
	ID        string          `json:"id"`
	CallID    string          `json:"call_id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Input     json.RawMessage `json:"input"`
	Function  *struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

func ParseAnthropic(data []byte) (*Dialogue, error) {
	var req struct {
		System     json.RawMessage `json:"system"`
		Messages   []rawMessage    `json:"messages"`
		Tools      []rawTool       `json:"tools"`
		ToolChoice json.RawMessage `json:"tool_choice"`
	}
	if err := decodeStrictEnough(data, &req); err != nil {
		return nil, badRequest("parse anthropic request", err)
	}
	d := &Dialogue{}
	imageBudget := &inlineImageBudget{}
	var err error
	if len(req.System) > 0 && string(req.System) != "null" {
		d.System, err = parseTextContent(req.System, false)
		if err != nil {
			return nil, badRequest("parse anthropic system", err)
		}
	}
	for _, msg := range req.Messages {
		if msg.Role == "system" || msg.Role == "developer" {
			text, textErr := parseTextContent(msg.Content, true)
			if textErr != nil {
				return nil, badRequest("parse anthropic system message", textErr)
			}
			if strings.TrimSpace(text) != "" {
				if strings.TrimSpace(d.System) != "" {
					d.System += "\n\n"
				}
				d.System += text
			}
			continue
		}
		converted, convErr := parseAnthropicMessage(msg, imageBudget)
		if convErr != nil {
			return nil, badRequest("parse anthropic message", convErr)
		}
		d.Messages = append(d.Messages, converted...)
	}
	d.Tools, err = normalizeTools(req.Tools)
	if err != nil {
		return nil, badRequest("parse anthropic tools", err)
	}
	d.ToolChoice = parseToolChoice(req.ToolChoice)
	return validateDialogue(d)
}

func parseAnthropicMessage(msg rawMessage, imageBudget *inlineImageBudget) ([]DialogueMessage, error) {
	if msg.Role != "user" && msg.Role != "assistant" {
		return nil, fmt.Errorf("unsupported role %q", msg.Role)
	}
	if text, ok := rawString(msg.Content); ok {
		return []DialogueMessage{{Role: msg.Role, Text: text}}, nil
	}
	var blocks []struct {
		Type      string          `json:"type"`
		Text      string          `json:"text"`
		ID        string          `json:"id"`
		Name      string          `json:"name"`
		Input     json.RawMessage `json:"input"`
		ToolUseID string          `json:"tool_use_id"`
		Content   json.RawMessage `json:"content"`
		IsError   bool            `json:"is_error"`
		Source    *struct {
			Type      string `json:"type"`
			MediaType string `json:"media_type"`
			Data      string `json:"data"`
		} `json:"source"`
	}
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		return nil, fmt.Errorf("content must be text or blocks: %w", err)
	}
	current := DialogueMessage{Role: msg.Role}
	var result []DialogueMessage
	for _, block := range blocks {
		switch block.Type {
		case "text", "input_text":
			current.Text += block.Text
		case "thinking", "redacted_thinking":
			// Cursor receives a flattened conversation transcript rather than native
			// Anthropic thinking blocks. Ignore prior assistant reasoning so a
			// follow-up request can replay responses emitted by this gateway.
			continue
		case "server_tool_use", "web_search_tool_result", "web_fetch_tool_result", "code_execution_tool_result", "bash_code_execution_tool_result", "text_editor_code_execution_tool_result":
			// Anthropic executes these tools on its own infrastructure. Cursor Cloud
			// cannot replay their native blocks, but the following visible assistant
			// text still preserves the useful result in conversation history.
			continue
		case "tool_use":
			if msg.Role != "assistant" {
				return nil, fmt.Errorf("tool_use is only valid for assistant messages")
			}
			action, err := actionFromRaw(block.ID, block.Name, block.Input)
			if err != nil {
				return nil, err
			}
			current.ToolCalls = append(current.ToolCalls, action)
		case "tool_result":
			if msg.Role != "user" {
				return nil, fmt.Errorf("tool_result is only valid for user messages")
			}
			if dialogueMessageHasContent(current) {
				result = append(result, current)
				current = DialogueMessage{Role: msg.Role}
			}
			text, err := parseTextContent(block.Content, false)
			if err != nil {
				return nil, fmt.Errorf("tool result: %w", err)
			}
			result = append(result, DialogueMessage{Role: "tool", Text: text, ToolCallID: block.ToolUseID, IsError: block.IsError})
		case "image":
			if msg.Role != "user" {
				return nil, fmt.Errorf("image content is only valid for user messages")
			}
			image, err := parseAnthropicInlineImage(block.Source)
			if err != nil {
				return nil, err
			}
			if err := imageBudget.add(image); err != nil {
				return nil, err
			}
			current.Images = append(current.Images, image)
		case "image_url", "input_image", "audio", "input_audio", "file", "input_file", "document":
			return nil, fmt.Errorf("unsupported multimodal content type %q", block.Type)
		default:
			return nil, fmt.Errorf("unsupported content type %q", block.Type)
		}
	}
	if dialogueMessageHasContent(current) {
		result = append(result, current)
	}
	return result, nil
}

func parseAnthropicInlineImage(source *struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}) (InlineImage, error) {
	if source == nil {
		return InlineImage{}, fmt.Errorf("image source is required")
	}
	if source.Type != "base64" {
		return InlineImage{}, fmt.Errorf("unsupported remote image source; use base64 image data")
	}
	mediaType, err := normalizeImageMIMEType(source.MediaType)
	if err != nil {
		return InlineImage{}, err
	}
	encoded := strings.TrimSpace(source.Data)
	if encoded == "" {
		return InlineImage{}, fmt.Errorf("image data is required")
	}
	if len(encoded) > maxInlineImageBase64Length {
		return InlineImage{}, fmt.Errorf("inline image exceeds %d MiB limit", maxInlineImageBytes>>20)
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return InlineImage{}, fmt.Errorf("invalid base64 image data")
	}
	if len(decoded) == 0 {
		return InlineImage{}, fmt.Errorf("image data is empty")
	}
	if len(decoded) > maxInlineImageBytes {
		return InlineImage{}, fmt.Errorf("inline image exceeds %d MiB limit", maxInlineImageBytes>>20)
	}
	if !inlineImageMatchesMIMEType(mediaType, decoded) {
		return InlineImage{}, fmt.Errorf("image data does not match media_type")
	}
	return InlineImage{MIMEType: mediaType, Data: decoded}, nil
}

func normalizeImageMIMEType(value string) (string, error) {
	mediaType, parameters, err := mime.ParseMediaType(strings.TrimSpace(value))
	if err != nil || len(parameters) > 0 {
		return "", fmt.Errorf("image media_type must be one of image/png, image/jpeg, image/gif, or image/webp")
	}
	switch strings.ToLower(mediaType) {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return strings.ToLower(mediaType), nil
	default:
		return "", fmt.Errorf("image media_type must be one of image/png, image/jpeg, image/gif, or image/webp")
	}
}

func inlineImageMatchesMIMEType(mediaType string, data []byte) bool {
	switch mediaType {
	case "image/png":
		return bytes.HasPrefix(data, []byte{'\x89', 'P', 'N', 'G', '\r', '\n', '\x1a', '\n'})
	case "image/jpeg":
		return bytes.HasPrefix(data, []byte{'\xff', '\xd8', '\xff'})
	case "image/gif":
		return bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a"))
	case "image/webp":
		return len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP"))
	default:
		return false
	}
}

func dialogueMessageHasContent(message DialogueMessage) bool {
	return message.Text != "" || len(message.Images) > 0 || len(message.ToolCalls) > 0
}

func ParseOpenAIChat(data []byte) (*Dialogue, error) {
	var req struct {
		Messages   []rawMessage    `json:"messages"`
		Tools      []rawTool       `json:"tools"`
		ToolChoice json.RawMessage `json:"tool_choice"`
	}
	if err := decodeStrictEnough(data, &req); err != nil {
		return nil, badRequest("parse openai chat request", err)
	}
	d := &Dialogue{}
	for _, msg := range req.Messages {
		text, err := parseTextContent(msg.Content, false)
		if err != nil {
			return nil, badRequest("parse openai chat message", err)
		}
		switch msg.Role {
		case "system", "developer":
			if text != "" {
				if d.System != "" {
					d.System += "\n\n"
				}
				d.System += text
			}
		case "user", "assistant":
			converted := DialogueMessage{Role: msg.Role, Text: text}
			for _, tc := range msg.ToolCalls {
				name, args := tc.Name, tc.Arguments
				if tc.Function != nil {
					name, args = tc.Function.Name, tc.Function.Arguments
				}
				action, actionErr := actionFromRaw(tc.ID, name, args)
				if actionErr != nil {
					return nil, badRequest("parse openai tool call", actionErr)
				}
				converted.ToolCalls = append(converted.ToolCalls, action)
			}
			d.Messages = append(d.Messages, converted)
		case "tool":
			d.Messages = append(d.Messages, DialogueMessage{Role: "tool", Text: text, ToolCallID: msg.ToolCallID})
		default:
			return nil, badRequest("parse openai chat message", fmt.Errorf("unsupported role %q", msg.Role))
		}
	}
	var err error
	d.Tools, err = normalizeTools(req.Tools)
	if err != nil {
		return nil, badRequest("parse openai tools", err)
	}
	d.ToolChoice = parseToolChoice(req.ToolChoice)
	return validateDialogue(d)
}

func ParseResponses(data []byte) (*Dialogue, error) {
	var req struct {
		Instructions json.RawMessage `json:"instructions"`
		Input        json.RawMessage `json:"input"`
		Tools        []rawTool       `json:"tools"`
		ToolChoice   json.RawMessage `json:"tool_choice"`
	}
	if err := decodeStrictEnough(data, &req); err != nil {
		return nil, badRequest("parse responses request", err)
	}
	d := &Dialogue{}
	var err error
	if len(req.Instructions) > 0 && string(req.Instructions) != "null" {
		d.System, err = parseTextContent(req.Instructions, false)
		if err != nil {
			return nil, badRequest("parse responses instructions", err)
		}
	}
	if text, ok := rawString(req.Input); ok {
		d.Messages = append(d.Messages, DialogueMessage{Role: "user", Text: text})
	} else {
		var items []struct {
			Type      string          `json:"type"`
			Role      string          `json:"role"`
			Content   json.RawMessage `json:"content"`
			ID        string          `json:"id"`
			CallID    string          `json:"call_id"`
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
			Output    json.RawMessage `json:"output"`
		}
		if err := json.Unmarshal(req.Input, &items); err != nil {
			return nil, badRequest("parse responses input", err)
		}
		for _, item := range items {
			switch item.Type {
			case "message", "":
				text, textErr := parseTextContent(item.Content, false)
				if textErr != nil {
					return nil, badRequest("parse responses message", textErr)
				}
				role := item.Role
				if role == "developer" || role == "system" {
					if d.System != "" {
						d.System += "\n\n"
					}
					d.System += text
					continue
				}
				if role != "user" && role != "assistant" {
					return nil, badRequest("parse responses message", fmt.Errorf("unsupported role %q", role))
				}
				d.Messages = append(d.Messages, DialogueMessage{Role: role, Text: text})
			case "function_call":
				action, actionErr := actionFromRaw(firstNonEmpty(item.CallID, item.ID), item.Name, item.Arguments)
				if actionErr != nil {
					return nil, badRequest("parse responses function call", actionErr)
				}
				d.Messages = append(d.Messages, DialogueMessage{Role: "assistant", ToolCalls: []Action{action}})
			case "function_call_output":
				text, textErr := parseTextContent(item.Output, false)
				if textErr != nil {
					return nil, badRequest("parse responses function output", textErr)
				}
				d.Messages = append(d.Messages, DialogueMessage{Role: "tool", Text: text, ToolCallID: item.CallID})
			case "input_image", "image", "input_audio", "audio", "input_file", "file":
				return nil, badRequest("parse responses input", fmt.Errorf("unsupported multimodal input type %q", item.Type))
			default:
				return nil, badRequest("parse responses input", fmt.Errorf("unsupported input type %q", item.Type))
			}
		}
	}
	d.Tools, err = normalizeTools(req.Tools)
	if err != nil {
		return nil, badRequest("parse responses tools", err)
	}
	d.ToolChoice = parseToolChoice(req.ToolChoice)
	return validateDialogue(d)
}

func parseTextContent(raw json.RawMessage, allowEmpty bool) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		if allowEmpty {
			return "", nil
		}
		return "", nil
	}
	if text, ok := rawString(raw); ok {
		return text, nil
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", fmt.Errorf("content must be a string or part array: %w", err)
	}
	var result strings.Builder
	for _, part := range parts {
		switch part.Type {
		case "text", "input_text", "output_text":
			if _, err := result.WriteString(part.Text); err != nil {
				return "", fmt.Errorf("append text content: %w", err)
			}
		case "image", "image_url", "input_image", "output_image", "audio", "input_audio", "file", "input_file", "document":
			return "", fmt.Errorf("unsupported multimodal content type %q", part.Type)
		default:
			return "", fmt.Errorf("unsupported content type %q", part.Type)
		}
	}
	return result.String(), nil
}

func isAnthropicServerToolType(toolType string) bool {
	for _, prefix := range []string{
		"web_search_",
		"web_fetch_",
		"code_execution_",
		"bash_code_execution_",
		"text_editor_code_execution_",
	} {
		if strings.HasPrefix(toolType, prefix) {
			return true
		}
	}
	return false
}

func normalizeTools(raw []rawTool) ([]ToolDefinition, error) {
	result := make([]ToolDefinition, 0, len(raw))
	for _, tool := range raw {
		if isAnthropicServerToolType(tool.Type) {
			// Server tools are executed by Anthropic itself and have no client-side
			// input schema to expose to Cursor. Ignore their declarations instead of
			// rejecting an otherwise compatible conversation request.
			continue
		}
		name, description, schema := tool.Name, tool.Description, tool.InputSchema
		if tool.Function != nil {
			name, description, schema = tool.Function.Name, tool.Function.Description, tool.Function.Parameters
		} else if len(schema) == 0 {
			schema = tool.Parameters
		}
		if tool.Type != "" && tool.Type != "function" && tool.Type != "custom" {
			return nil, fmt.Errorf("unsupported tool type %q", tool.Type)
		}
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("tool name is required")
		}
		if len(schema) == 0 || string(schema) == "null" {
			schema = json.RawMessage(`{"type":"object"}`)
		}
		var object map[string]any
		if err := json.Unmarshal(schema, &object); err != nil {
			return nil, fmt.Errorf("tool %q schema: %w", name, err)
		}
		result = append(result, ToolDefinition{Name: name, Description: description, InputSchema: append(json.RawMessage(nil), schema...)})
	}
	return result, nil
}

func parseToolChoice(raw json.RawMessage) ToolChoice {
	if text, ok := rawString(raw); ok {
		return ToolChoice{Mode: text}
	}
	var value struct {
		Type     string `json:"type"`
		Name     string `json:"name"`
		Function *struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if json.Unmarshal(raw, &value) != nil {
		return ToolChoice{}
	}
	if value.Function != nil && value.Name == "" {
		value.Name = value.Function.Name
	}
	return ToolChoice{Mode: value.Type, Name: value.Name}
}

func actionFromRaw(id, name string, raw json.RawMessage) (Action, error) {
	if strings.TrimSpace(name) == "" {
		return Action{}, fmt.Errorf("tool call name is required")
	}
	if text, ok := rawString(raw); ok {
		raw = json.RawMessage(text)
	}
	if len(raw) == 0 || string(raw) == "null" {
		return Action{}, fmt.Errorf("tool call %q arguments are required", name)
	}
	var arguments map[string]any
	if err := json.Unmarshal(raw, &arguments); err != nil {
		return Action{}, fmt.Errorf("tool call %q arguments: %w", name, err)
	}
	return Action{ID: id, Name: name, Arguments: arguments}, nil
}

func validateDialogue(d *Dialogue) (*Dialogue, error) {
	if d == nil || (strings.TrimSpace(d.System) == "" && len(d.Messages) == 0) {
		return nil, badRequest("validate dialogue", fmt.Errorf("request contains no text conversation"))
	}
	return d, nil
}

func decodeStrictEnough(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return fmt.Errorf("invalid trailing JSON: %w", err)
	}
	return nil
}

func rawString(raw json.RawMessage) (string, bool) {
	var text string
	if len(raw) == 0 || json.Unmarshal(raw, &text) != nil {
		return "", false
	}
	return text, true
}

func badRequest(operation string, err error) *Error {
	return &Error{Kind: ErrorBadRequest, StatusCode: 400, Operation: operation, Err: err}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
