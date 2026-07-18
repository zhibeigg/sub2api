package kiro

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// modelAliases lists model names that need an explicit redirect — dated
// snapshots, cross-family legacy IDs (claude-3-*), and non-Anthropic fallbacks.
// Plain dash → dot version normalization is handled by claudeVersionPattern,
// so new versions (e.g. claude-opus-4-8) require no code changes.
type modelMapping struct {
	key   string
	value string
}

var modelAliases = []modelMapping{
	{"claude-sonnet-4-20250514", "claude-sonnet-4"},
	{"claude-3-5-sonnet", "claude-sonnet-4.5"},
	{"claude-3-opus", "claude-sonnet-4.5"},
	{"claude-3-sonnet", "claude-sonnet-4"},
	{"claude-3-haiku", "claude-haiku-4.5"},
	{"gpt-4-turbo", "claude-sonnet-4.5"},
	{"gpt-4o", "claude-sonnet-4.5"},
	{"gpt-4", "claude-sonnet-4.5"},
	{"gpt-3.5-turbo", "claude-sonnet-4.5"},
}

// claudeVersionPattern normalizes "claude-{family}-N-M" to "claude-{family}-N.M".
var claudeVersionPattern = regexp.MustCompile(`claude-(opus|sonnet|haiku)-(\d+)-(\d{1,2})\b`)

// ThinkingModePrompt is injected into the system priming when thinking is enabled.
const ThinkingModePrompt = `<thinking_mode>enabled</thinking_mode>
<max_thinking_length>200000</max_thinking_length>`

// DefaultThinkingSuffix is the model-name suffix that enables thinking mode.
const DefaultThinkingSuffix = "-thinking"

const minimalFallbackUserContent = "."
const toolResultsContinuationPrefix = "Tool results:"
const toolResultImagePlaceholder = "[Tool returned an image; the image is attached to this message.]"

const maxPayloadBytes = 900 * 1024

const truncationPlaceholder = "[Earlier conversation history was truncated to fit the model's input limit. Older messages and tool activity have been omitted.]"

const minRecentHistoryTurns = 4

const maxToolDescLen = 10237

// ParseModelAndThinking resolves a client-supplied model name to a Kiro model ID
// and reports whether thinking mode was requested via the configured suffix.
func ParseModelAndThinking(model string, thinkingSuffix string) (string, bool) {
	lower := strings.ToLower(model)
	thinking := false

	suffixLower := strings.ToLower(thinkingSuffix)
	if suffixLower != "" && strings.HasSuffix(lower, suffixLower) {
		thinking = true
		model = model[:len(model)-len(thinkingSuffix)]
		lower = strings.ToLower(model)
	}

	for _, m := range modelAliases {
		if strings.Contains(lower, m.key) {
			return m.value, thinking
		}
	}

	if claudeVersionPattern.MatchString(lower) {
		return claudeVersionPattern.ReplaceAllString(lower, "claude-$1-$2.$3"), thinking
	}

	if strings.HasPrefix(lower, "claude-") {
		return model, thinking
	}

	return model, thinking
}

// ResolveClaudeThinkingMode returns the mapped model and whether thinking should
// be enabled (via suffix or an explicit thinking config).
func ResolveClaudeThinkingMode(model string, thinkingCfg *ClaudeThinkingConfig, thinkingSuffix string) (string, bool) {
	actualModel, suffixThinking := ParseModelAndThinking(model, thinkingSuffix)
	return actualModel, suffixThinking || isClaudeThinkingRequested(thinkingCfg)
}

func isClaudeThinkingRequested(thinkingCfg *ClaudeThinkingConfig) bool {
	if thinkingCfg == nil {
		return false
	}
	kind := strings.ToLower(strings.TrimSpace(thinkingCfg.Type))
	return kind == "enabled" || kind == "adaptive"
}

// MapModel maps a client model name to the Kiro upstream model ID.
func MapModel(model string) string {
	mapped, _ := ParseModelAndThinking(model, DefaultThinkingSuffix)
	return mapped
}

// ==================== Claude -> Kiro ====================

func ClaudeToKiro(req *ClaudeRequest, thinking bool) *KiroPayload {
	modelID := MapModel(req.Model)
	origin := "AI_EDITOR"

	systemPrompt := buildClaudeSystemPrompt(req.System, thinking)

	history := make([]KiroHistoryMessage, 0)
	var currentContent string
	var currentImages []KiroImage
	var currentToolResults []KiroToolResult

	for i, msg := range req.Messages {
		isLast := i == len(req.Messages)-1

		switch msg.Role {
		case "user":
			content, images, toolResults := extractClaudeUserContent(msg.Content)
			content = normalizeUserContent(content, len(images) > 0)

			if isLast {
				currentContent = content
				currentImages = images
				currentToolResults = toolResults
			} else {
				userMsg := KiroUserInputMessage{
					Content: content,
					ModelID: modelID,
					Origin:  origin,
				}
				if len(images) > 0 {
					userMsg.Images = images
				}
				if len(toolResults) > 0 {
					userMsg.UserInputMessageContext = &UserInputMessageContext{
						ToolResults: toolResults,
					}
				}
				history = append(history, KiroHistoryMessage{UserInputMessage: &userMsg})
			}
		case "assistant":
			content, toolUses := extractClaudeAssistantContent(msg.Content)
			history = append(history, KiroHistoryMessage{
				AssistantResponseMessage: &KiroAssistantResponseMessage{
					Content:  content,
					ToolUses: toolUses,
				},
			})
		}
	}

	history = trimLeadingAssistantHistory(history)

	if systemPrompt != "" {
		priming := []KiroHistoryMessage{
			{UserInputMessage: &KiroUserInputMessage{Content: systemPrompt, ModelID: modelID, Origin: origin}},
			{AssistantResponseMessage: &KiroAssistantResponseMessage{Content: "I will follow these instructions."}},
		}
		history = append(priming, history...)
	}

	currentToolResultIDs := collectToolResultIDs(currentToolResults)
	keepCurrentToolResults := currentToolResultsMatchLastAssistant(history, currentToolResultIDs)

	if keepCurrentToolResults {
		history = sanitizeKiroHistory(history, currentToolResultIDs)
	} else {
		history = sanitizeKiroHistory(history, nil)
	}

	finalContent := ""
	if currentContent != "" {
		finalContent = currentContent
	} else if len(currentImages) > 0 {
		finalContent = normalizeUserContent("", true)
	} else if len(currentToolResults) > 0 {
		finalContent = buildToolResultsContinuation(currentToolResults)
	} else {
		finalContent = minimalFallbackUserContent
	}

	kiroTools, toolNameMap := convertClaudeTools(req.Tools)

	payload := &KiroPayload{}
	payload.ToolNameMap = toolNameMap
	payload.ConversationState.ChatTriggerType = "MANUAL"
	payload.ConversationState.AgentTaskType = "vibe"
	payload.ConversationState.AgentContinuationId = uuid.New().String()
	payload.ConversationState.ConversationID = buildConversationID(modelID, systemPrompt, firstClaudeConversationAnchor(req.Messages))
	payload.ConversationState.CurrentMessage.UserInputMessage = KiroUserInputMessage{
		Content: finalContent,
		ModelID: modelID,
		Origin:  origin,
		Images:  currentImages,
	}

	var attachToolResults []KiroToolResult
	if keepCurrentToolResults {
		attachToolResults = currentToolResults
	}
	if len(kiroTools) > 0 || len(attachToolResults) > 0 {
		payload.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext = &UserInputMessageContext{
			Tools:       kiroTools,
			ToolResults: attachToolResults,
		}
	}

	if len(history) > 0 {
		payload.ConversationState.History = history
	}

	if req.MaxTokens > 0 || req.Temperature > 0 || req.TopP > 0 {
		payload.InferenceConfig = &InferenceConfig{
			MaxTokens:   req.MaxTokens,
			Temperature: req.Temperature,
			TopP:        req.TopP,
		}
	}

	truncatePayloadToLimit(payload, systemPrompt != "")
	return payload
}

func buildClaudeSystemPrompt(system any, thinking bool) string {
	systemPrompt := applyPromptFilters(extractSystemPrompt(system))
	if !thinking {
		return systemPrompt
	}
	if systemPrompt == "" {
		return ThinkingModePrompt
	}
	return ThinkingModePrompt + "\n\n" + systemPrompt
}

// CloneClaudeRequestForThinking returns a shallow request clone whose system
// prompt matches the thinking priming that ClaudeToKiro sends upstream.
// The original request and its structured system blocks are not mutated.
func CloneClaudeRequestForThinking(req *ClaudeRequest, thinking bool) *ClaudeRequest {
	if req == nil {
		return nil
	}

	cloned := *req
	if thinking {
		cloned.System = PrependThinkingSystem(req.System)
	}
	return &cloned
}

// PrependThinkingSystem returns a structured Claude system prompt with the
// Kiro thinking-mode priming inserted before the original system content.
func PrependThinkingSystem(system any) any {
	thinkingText := ThinkingModePrompt
	if hasClaudeSystemContent(system) {
		thinkingText += "\n"
	}
	thinkingBlock := map[string]any{
		"type": "text",
		"text": thinkingText,
	}

	switch value := system.(type) {
	case nil:
		return []any{thinkingBlock}
	case string:
		if value == "" {
			return []any{thinkingBlock}
		}
		return []any{
			thinkingBlock,
			map[string]any{
				"type": "text",
				"text": value,
			},
		}
	case []any:
		blocks := make([]any, 0, len(value)+1)
		blocks = append(blocks, thinkingBlock)
		blocks = append(blocks, value...)
		return blocks
	case []string:
		blocks := make([]any, 0, len(value)+1)
		blocks = append(blocks, thinkingBlock)
		for _, block := range value {
			blocks = append(blocks, map[string]any{
				"type": "text",
				"text": block,
			})
		}
		return blocks
	default:
		return []any{thinkingBlock}
	}
}

func hasClaudeSystemContent(system any) bool {
	switch value := system.(type) {
	case nil:
		return false
	case string:
		return value != ""
	case []any:
		return len(value) > 0
	case []string:
		return len(value) > 0
	default:
		return true
	}
}

func extractSystemPrompt(system any) string {
	if system == nil {
		return ""
	}
	if s, ok := system.(string); ok {
		return s
	}
	if blocks, ok := system.([]any); ok {
		var parts []string
		for _, b := range blocks {
			if block, ok := b.(map[string]any); ok {
				if text, ok := block["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func extractClaudeUserContent(content any) (string, []KiroImage, []KiroToolResult) {
	var text string
	var images []KiroImage
	var toolResults []KiroToolResult

	if s, ok := content.(string); ok {
		return s, nil, nil
	}

	if blocks, ok := content.([]any); ok {
		for _, b := range blocks {
			block, ok := b.(map[string]any)
			if !ok {
				continue
			}
			blockType, _ := block["type"].(string)
			switch blockType {
			case "text", "input_text":
				if t, ok := block["text"].(string); ok {
					text += t
				}
			case "image", "image_url", "input_image":
				if img := extractImageFromClaudeBlock(block); img != nil {
					images = append(images, *img)
				}
			case "tool_result":
				toolUseID, _ := block["tool_use_id"].(string)
				resultContent, resultImages := extractToolResultContent(block["content"])
				if len(resultImages) > 0 {
					images = append(images, resultImages...)
					if strings.TrimSpace(resultContent) == "" {
						resultContent = toolResultImagePlaceholder
					}
				}
				toolResults = append(toolResults, KiroToolResult{
					ToolUseID: toolUseID,
					Content:   []KiroResultContent{{Text: resultContent}},
					Status:    "success",
				})
			}
		}
	}
	return text, images, toolResults
}

func extractImageFromClaudeBlock(block map[string]any) *KiroImage {
	if source, ok := block["source"].(map[string]any); ok {
		if data, ok := source["data"].(string); ok {
			if img := parseDataURL(data); img != nil {
				return img
			}
			mediaType, _ := source["media_type"].(string)
			if mediaType == "" {
				mediaType, _ = source["mediaType"].(string)
			}
			if mediaType == "" {
				mediaType, _ = source["mime_type"].(string)
			}
			format := strings.TrimPrefix(strings.ToLower(mediaType), "image/")
			if img := parseBase64Image(data, format); img != nil {
				return img
			}
		}
		if url, ok := source["url"].(string); ok {
			if img := parseDataURL(url); img != nil {
				return img
			}
		}
	}
	if img := extractImageFromOpenAIPart(block); img != nil {
		return img
	}
	if data, ok := block["data"].(string); ok {
		if img := parseDataURL(data); img != nil {
			return img
		}
	}
	return nil
}

func extractToolResultContent(content any) (string, []KiroImage) {
	if s, ok := content.(string); ok {
		return s, nil
	}
	if blocks, ok := content.([]any); ok {
		var parts []string
		var images []KiroImage
		for _, b := range blocks {
			block, ok := b.(map[string]any)
			if !ok {
				continue
			}
			blockType, _ := block["type"].(string)
			switch blockType {
			case "image", "image_url", "input_image":
				if img := extractImageFromClaudeBlock(block); img != nil {
					images = append(images, *img)
					continue
				}
			}
			if text, ok := block["text"].(string); ok {
				parts = append(parts, text)
				continue
			}
			if img := extractImageFromClaudeBlock(block); img != nil {
				images = append(images, *img)
			}
		}
		return strings.Join(parts, ""), images
	}
	return "", nil
}

func extractClaudeAssistantContent(content any) (string, []KiroToolUse) {
	var text string
	var toolUses []KiroToolUse

	if s, ok := content.(string); ok {
		return s, nil
	}
	if blocks, ok := content.([]any); ok {
		for _, b := range blocks {
			block, ok := b.(map[string]any)
			if !ok {
				continue
			}
			blockType, _ := block["type"].(string)
			switch blockType {
			case "text":
				if t, ok := block["text"].(string); ok {
					text += t
				}
			case "tool_use":
				id, _ := block["id"].(string)
				name, _ := block["name"].(string)
				input, _ := block["input"].(map[string]any)
				if input == nil {
					input = make(map[string]any)
				}
				toolUses = append(toolUses, KiroToolUse{ToolUseID: id, Name: name, Input: input})
			}
		}
	}
	return text, toolUses
}

func convertClaudeTools(tools []ClaudeTool) ([]KiroToolWrapper, map[string]string) {
	if len(tools) == 0 {
		return nil, nil
	}
	result := make([]KiroToolWrapper, 0, len(tools))
	nameMap := make(map[string]string)
	for _, tool := range tools {
		desc := tool.Description
		if len(desc) > maxToolDescLen {
			desc = desc[:maxToolDescLen] + "..."
		}
		sanitized := shortenToolName(sanitizeToolName(tool.Name))
		if sanitized != tool.Name {
			nameMap[sanitized] = tool.Name
		}
		w := KiroToolWrapper{}
		w.ToolSpecification.Name = sanitized
		w.ToolSpecification.Description = normalizeToolDesc(desc, sanitized)
		w.ToolSpecification.InputSchema = InputSchema{JSON: ensureObjectSchema(tool.InputSchema)}
		result = append(result, w)
	}
	return result, nameMap
}

func ensureObjectSchema(schema any) any {
	m, ok := schema.(map[string]any)
	if !ok {
		return map[string]any{"type": "object"}
	}
	cleaned := cloneSchemaMap(m)
	cleanSchema(cleaned)
	if _, hasType := cleaned["type"]; !hasType {
		cleaned["type"] = "object"
	}
	return cleaned
}

func cloneSchemaMap(m map[string]any) map[string]any {
	cloned := make(map[string]any, len(m))
	for k, v := range m {
		cloned[k] = cloneSchemaValue(v)
	}
	return cloned
}

func cloneSchemaValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return cloneSchemaMap(val)
	case []any:
		cloned := make([]any, 0, len(val))
		for _, item := range val {
			cloned = append(cloned, cloneSchemaValue(item))
		}
		return cloned
	default:
		return v
	}
}

func cleanSchema(m map[string]any) {
	delete(m, "additionalProperties")
	if req, exists := m["required"]; exists {
		switch arr := req.(type) {
		case nil:
			delete(m, "required")
		case []any:
			if len(arr) == 0 {
				delete(m, "required")
			}
		case []string:
			if len(arr) == 0 {
				delete(m, "required")
			}
		default:
			delete(m, "required")
		}
	}
	for _, v := range m {
		switch val := v.(type) {
		case map[string]any:
			cleanSchema(val)
		case []any:
			for _, item := range val {
				if sub, ok := item.(map[string]any); ok {
					cleanSchema(sub)
				}
			}
		}
	}
}

func normalizeToolDesc(desc, name string) string {
	if strings.TrimSpace(desc) != "" {
		return desc
	}
	return "Tool: " + name
}

// sanitizeToolName normalizes a tool name to characters the Kiro API accepts
// (pure camelCase, no underscores or dashes).
func sanitizeToolName(name string) string {
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '_' || r == '-'
	})
	if len(parts) == 0 {
		return "tool"
	}
	var b strings.Builder
	for i, part := range parts {
		if part == "" {
			continue
		}
		if i == 0 {
			_, _ = b.WriteString(strings.ToLower(part[:1]) + part[1:])
		} else {
			_, _ = b.WriteString(strings.ToUpper(part[:1]) + part[1:])
		}
	}
	result := b.String()
	if result == "" {
		return "tool"
	}
	return result
}

func shortenToolName(name string) string {
	if len(name) <= 64 {
		return name
	}
	if strings.HasPrefix(name, "mcp__") {
		lastIdx := strings.LastIndex(name, "__")
		if lastIdx > 5 {
			shortened := "mcp__" + name[lastIdx+2:]
			if len(shortened) <= 64 {
				return shortened
			}
		}
	}
	return name[:64]
}

// ==================== Kiro -> Claude ====================

func KiroToClaudeResponse(content, thinkingContent string, includeEmptyThinkingBlock bool, toolUses []KiroToolUse, inputTokens, outputTokens int, model string) *ClaudeResponse {
	blocks := make([]ClaudeContentBlock, 0)

	if thinkingContent != "" || includeEmptyThinkingBlock {
		blocks = append(blocks, ClaudeContentBlock{Type: "thinking", Thinking: thinkingContent})
	}
	if content != "" {
		blocks = append(blocks, ClaudeContentBlock{Type: "text", Text: content})
	}
	for _, tu := range toolUses {
		blocks = append(blocks, ClaudeContentBlock{
			Type:  "tool_use",
			ID:    tu.ToolUseID,
			Name:  tu.Name,
			Input: tu.Input,
		})
	}

	stopReason := "end_turn"
	if len(toolUses) > 0 {
		stopReason = "tool_use"
	}

	return &ClaudeResponse{
		ID:         "msg_" + uuid.New().String(),
		Type:       "message",
		Role:       "assistant",
		Content:    blocks,
		Model:      model,
		StopReason: stopReason,
		Usage:      ClaudeUsage{InputTokens: inputTokens, OutputTokens: outputTokens},
	}
}

// ==================== OpenAI -> Kiro ====================

func OpenAIToKiro(req *OpenAIRequest, thinking bool) *KiroPayload {
	modelID := MapModel(req.Model)
	origin := "AI_EDITOR"

	var systemPrompt string
	var nonSystemMessages []OpenAIMessage
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			if s := extractOpenAIMessageText(msg.Content); s != "" {
				systemPrompt += s + "\n"
			}
		} else {
			nonSystemMessages = append(nonSystemMessages, msg)
		}
	}

	systemPrompt = applyPromptFilters(systemPrompt)
	if thinking {
		if systemPrompt == "" {
			systemPrompt = ThinkingModePrompt
		} else {
			systemPrompt = ThinkingModePrompt + "\n\n" + systemPrompt
		}
	}

	history := make([]KiroHistoryMessage, 0)
	var currentContent string
	var currentImages []KiroImage
	var currentToolResults []KiroToolResult

	for i, msg := range nonSystemMessages {
		isLast := i == len(nonSystemMessages)-1
		switch msg.Role {
		case "user":
			content, images := extractOpenAIUserContent(msg.Content)
			content = normalizeUserContent(content, len(images) > 0)
			if isLast {
				currentContent = content
				currentImages = images
			} else {
				history = append(history, KiroHistoryMessage{
					UserInputMessage: &KiroUserInputMessage{Content: content, ModelID: modelID, Origin: origin, Images: images},
				})
			}
		case "assistant":
			content := extractOpenAIMessageText(msg.Content)
			var toolUses []KiroToolUse
			for _, tc := range msg.ToolCalls {
				var input map[string]any
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
				if input == nil {
					input = make(map[string]any)
				}
				toolUses = append(toolUses, KiroToolUse{ToolUseID: tc.ID, Name: tc.Function.Name, Input: input})
			}
			history = append(history, KiroHistoryMessage{
				AssistantResponseMessage: &KiroAssistantResponseMessage{Content: content, ToolUses: toolUses},
			})
		case "tool":
			cleanText, toolImages := extractOpenAIUserContent(msg.Content)
			var content string
			if len(toolImages) > 0 {
				currentImages = append(currentImages, toolImages...)
				content = strings.TrimSpace(cleanText)
				if content == "" {
					content = toolResultImagePlaceholder
				}
			} else {
				content = extractOpenAIMessageText(msg.Content)
			}
			currentToolResults = append(currentToolResults, KiroToolResult{
				ToolUseID: msg.ToolCallID,
				Content:   []KiroResultContent{{Text: content}},
				Status:    "success",
			})
			nextIdx := i + 1
			if nextIdx >= len(nonSystemMessages) || nonSystemMessages[nextIdx].Role != "tool" {
				if !isLast {
					history = append(history, KiroHistoryMessage{
						UserInputMessage: &KiroUserInputMessage{
							ModelID:                 modelID,
							Origin:                  origin,
							Images:                  currentImages,
							UserInputMessageContext: &UserInputMessageContext{ToolResults: currentToolResults},
						},
					})
					currentToolResults = nil
					currentImages = nil
				}
			}
		}
	}

	if systemPrompt != "" {
		priming := []KiroHistoryMessage{
			{UserInputMessage: &KiroUserInputMessage{Content: strings.TrimSpace(systemPrompt), ModelID: modelID, Origin: origin}},
			{AssistantResponseMessage: &KiroAssistantResponseMessage{Content: "I will follow these instructions."}},
		}
		history = append(priming, history...)
	}

	currentToolResultIDs := collectToolResultIDs(currentToolResults)
	keepCurrentToolResults := currentToolResultsMatchLastAssistant(history, currentToolResultIDs)
	if keepCurrentToolResults {
		history = sanitizeKiroHistory(history, currentToolResultIDs)
	} else {
		history = sanitizeKiroHistory(history, nil)
	}

	finalContent := currentContent
	if finalContent == "" {
		if len(currentImages) > 0 {
			finalContent = normalizeUserContent("", true)
		} else if len(currentToolResults) > 0 {
			finalContent = buildToolResultsContinuation(currentToolResults)
		} else {
			finalContent = minimalFallbackUserContent
		}
	}

	kiroTools := convertOpenAITools(req.Tools)

	payload := &KiroPayload{}
	payload.ConversationState.ChatTriggerType = "MANUAL"
	payload.ConversationState.ConversationID = buildConversationID(modelID, systemPrompt, firstOpenAIConversationAnchor(nonSystemMessages))
	payload.ConversationState.CurrentMessage.UserInputMessage = KiroUserInputMessage{
		Content: finalContent,
		ModelID: modelID,
		Origin:  origin,
		Images:  currentImages,
	}

	var attachToolResults []KiroToolResult
	if keepCurrentToolResults {
		attachToolResults = currentToolResults
	}
	if len(kiroTools) > 0 || len(attachToolResults) > 0 {
		payload.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext = &UserInputMessageContext{
			Tools:       kiroTools,
			ToolResults: attachToolResults,
		}
	}

	if len(history) > 0 {
		payload.ConversationState.History = history
	}

	if req.MaxTokens > 0 || req.Temperature > 0 || req.TopP > 0 {
		payload.InferenceConfig = &InferenceConfig{MaxTokens: req.MaxTokens, Temperature: req.Temperature, TopP: req.TopP}
	}

	truncatePayloadToLimit(payload, systemPrompt != "")
	return payload
}

func extractOpenAIUserContent(content any) (string, []KiroImage) {
	if s, ok := content.(string); ok {
		return s, nil
	}
	var text string
	var images []KiroImage
	if part, ok := content.(map[string]any); ok {
		if t, ok := extractOpenAITextPart(part); ok {
			text += t
		}
		if img := extractImageFromOpenAIPart(part); img != nil {
			images = append(images, *img)
		}
	}
	if parts, ok := content.([]any); ok {
		for _, p := range parts {
			part, ok := p.(map[string]any)
			if !ok {
				continue
			}
			if t, ok := extractOpenAITextPart(part); ok {
				text += t
			}
			if img := extractImageFromOpenAIPart(part); img != nil {
				images = append(images, *img)
			}
		}
	}
	if len(images) > 0 {
		text = sanitizeImagePlaceholders(text)
	}
	return text, images
}

func extractOpenAIMessageText(content any) string {
	if content == nil {
		return ""
	}
	if s, ok := content.(string); ok {
		return s
	}
	if text, _ := extractOpenAIUserContent(content); strings.TrimSpace(text) != "" {
		return text
	}
	switch v := content.(type) {
	case map[string]any:
		if nested, ok := v["content"]; ok {
			if nestedText := extractOpenAIMessageText(nested); strings.TrimSpace(nestedText) != "" {
				return nestedText
			}
		}
		if raw, err := json.Marshal(v); err == nil {
			return string(raw)
		}
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			partText := extractOpenAIMessageText(item)
			if strings.TrimSpace(partText) != "" {
				parts = append(parts, partText)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "")
		}
		if raw, err := json.Marshal(v); err == nil {
			return string(raw)
		}
	default:
		if raw, err := json.Marshal(v); err == nil {
			return string(raw)
		}
	}
	return ""
}

func collectToolResultIDs(toolResults []KiroToolResult) map[string]bool {
	if len(toolResults) == 0 {
		return nil
	}
	ids := make(map[string]bool, len(toolResults))
	for _, tr := range toolResults {
		if id := strings.TrimSpace(tr.ToolUseID); id != "" {
			ids[id] = true
		}
	}
	return ids
}

func currentToolResultsMatchLastAssistant(history []KiroHistoryMessage, currentToolResultIDs map[string]bool) bool {
	if len(currentToolResultIDs) == 0 || len(history) == 0 {
		return false
	}
	last := history[len(history)-1]
	if last.AssistantResponseMessage == nil || len(last.AssistantResponseMessage.ToolUses) == 0 {
		return false
	}
	for _, tu := range last.AssistantResponseMessage.ToolUses {
		if !currentToolResultIDs[tu.ToolUseID] {
			return false
		}
	}
	return true
}

var pollutedToolCallTextPattern = regexp.MustCompile(`\[Called tool [^\]]*\]`)
var blankLinesPattern = regexp.MustCompile(`\n{3,}`)

func stripPollutedToolCallText(content string) string {
	if !strings.Contains(content, "[Called tool ") {
		return content
	}
	cleaned := pollutedToolCallTextPattern.ReplaceAllString(content, "")
	cleaned = blankLinesPattern.ReplaceAllString(cleaned, "\n\n")
	return strings.TrimSpace(cleaned)
}

func narrateToolResults(toolResults []KiroToolResult, names map[string]string) string {
	if len(toolResults) == 0 {
		return ""
	}
	parts := make([]string, 0, len(toolResults))
	for _, tr := range toolResults {
		var texts []string
		for _, c := range tr.Content {
			if strings.TrimSpace(c.Text) != "" {
				texts = append(texts, c.Text)
			}
		}
		body := strings.Join(texts, "\n")
		if strings.TrimSpace(body) == "" {
			body = "(no output)"
		}
		if name := names[tr.ToolUseID]; name != "" {
			parts = append(parts, fmt.Sprintf("[%s] %s", name, body))
		} else {
			parts = append(parts, body)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return toolResultsContinuationPrefix + "\n\n" + strings.Join(parts, "\n\n")
}

func joinHistoryText(existing, narrated string) string {
	existing = strings.TrimSpace(existing)
	narrated = strings.TrimSpace(narrated)
	switch {
	case existing != "" && narrated != "":
		return existing + "\n\n" + narrated
	case narrated != "":
		return narrated
	default:
		return existing
	}
}

func sanitizeKiroHistory(history []KiroHistoryMessage, currentToolResultIDs map[string]bool) []KiroHistoryMessage {
	if len(history) == 0 {
		return history
	}

	toolNames := make(map[string]string)
	for i := range history {
		if a := history[i].AssistantResponseMessage; a != nil {
			for _, tu := range a.ToolUses {
				if tu.ToolUseID != "" && tu.Name != "" {
					toolNames[tu.ToolUseID] = tu.Name
				}
			}
		}
	}

	activeIdx := -1
	if len(currentToolResultIDs) > 0 {
		last := history[len(history)-1]
		if last.AssistantResponseMessage != nil && len(last.AssistantResponseMessage.ToolUses) > 0 {
			allCovered := true
			for _, tu := range last.AssistantResponseMessage.ToolUses {
				if !currentToolResultIDs[tu.ToolUseID] {
					allCovered = false
					break
				}
			}
			if allCovered {
				activeIdx = len(history) - 1
			}
		}
	}

	for i := range history {
		msg := &history[i]
		if msg.AssistantResponseMessage != nil {
			if msg.AssistantResponseMessage.Content != "" {
				msg.AssistantResponseMessage.Content = stripPollutedToolCallText(msg.AssistantResponseMessage.Content)
			}
		}
		if msg.AssistantResponseMessage != nil && len(msg.AssistantResponseMessage.ToolUses) > 0 {
			if i == activeIdx {
				continue
			}
			msg.AssistantResponseMessage.ToolUses = nil
		}
		if msg.UserInputMessage != nil && msg.UserInputMessage.UserInputMessageContext != nil {
			ctx := msg.UserInputMessage.UserInputMessageContext
			if len(ctx.ToolResults) > 0 {
				narrated := narrateToolResults(ctx.ToolResults, toolNames)
				msg.UserInputMessage.Content = joinHistoryText(msg.UserInputMessage.Content, narrated)
				ctx.ToolResults = nil
			}
			ctx.Tools = nil
			if len(ctx.Tools) == 0 && len(ctx.ToolResults) == 0 {
				msg.UserInputMessage.UserInputMessageContext = nil
			}
		}
		if msg.UserInputMessage != nil && strings.TrimSpace(msg.UserInputMessage.Content) == "" && len(msg.UserInputMessage.Images) == 0 {
			msg.UserInputMessage.Content = minimalFallbackUserContent
		}
	}

	cleaned := history[:0:0]
	for i := range history {
		msg := history[i]
		if msg.AssistantResponseMessage != nil && len(msg.AssistantResponseMessage.ToolUses) == 0 {
			c := strings.TrimSpace(msg.AssistantResponseMessage.Content)
			if c == "" || c == minimalFallbackUserContent {
				continue
			}
		}
		if msg.UserInputMessage != nil && len(cleaned) > 0 {
			last := cleaned[len(cleaned)-1]
			if last.UserInputMessage != nil &&
				strings.TrimSpace(last.UserInputMessage.Content) == strings.TrimSpace(msg.UserInputMessage.Content) &&
				strings.TrimSpace(msg.UserInputMessage.Content) != "" &&
				len(msg.UserInputMessage.Images) == 0 {
				continue
			}
		}
		cleaned = append(cleaned, msg)
	}

	return trimLeadingAssistantHistory(cleaned)
}

func truncatePayloadToLimit(payload *KiroPayload, hasPriming bool) {
	if payload == nil {
		return
	}
	if payloadByteSize(payload) <= maxPayloadBytes {
		return
	}

	history := payload.ConversationState.History
	primingCount := 0
	if hasPriming && len(history) >= 2 {
		primingCount = 2
	}
	priming := history[:primingCount]
	conversation := history[primingCount:]

	placeholderEntry := KiroHistoryMessage{
		UserInputMessage: &KiroUserInputMessage{
			Content: truncationPlaceholder,
			ModelID: currentMessageModelID(payload),
			Origin:  "AI_EDITOR",
		},
	}

	entrySizes := make([]int, len(conversation))
	for i := range conversation {
		entrySizes[i] = historyEntryByteSize(conversation[i])
	}

	payload.ConversationState.History = priming
	baseSize := payloadByteSize(payload) + historyEntryByteSize(placeholderEntry)

	keepFrom := len(conversation)
	running := baseSize
	for i := len(conversation) - 1; i >= 0; i-- {
		running += entrySizes[i]
		kept := len(conversation) - i
		if running > maxPayloadBytes && kept > minRecentHistoryTurns {
			break
		}
		keepFrom = i
	}

	tail := conversation[keepFrom:]
	tail = dropLeadingAssistant(tail)

	rebuilt := make([]KiroHistoryMessage, 0, len(priming)+1+len(tail))
	rebuilt = append(rebuilt, priming...)
	if keepFrom > 0 {
		rebuilt = append(rebuilt, placeholderEntry)
	}
	rebuilt = append(rebuilt, tail...)
	payload.ConversationState.History = rebuilt

	if payloadByteSize(payload) > maxPayloadBytes {
		truncateCurrentMessage(payload)
	}
}

func historyEntryByteSize(entry KiroHistoryMessage) int {
	raw, err := json.Marshal(entry)
	if err != nil {
		return 0
	}
	return len(raw) + 1
}

func dropLeadingAssistant(tail []KiroHistoryMessage) []KiroHistoryMessage {
	for len(tail) > 0 && tail[0].AssistantResponseMessage != nil {
		tail = tail[1:]
	}
	return tail
}

func payloadByteSize(payload *KiroPayload) int {
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0
	}
	return len(raw)
}

func currentMessageModelID(payload *KiroPayload) string {
	return payload.ConversationState.CurrentMessage.UserInputMessage.ModelID
}

func truncateCurrentMessage(payload *KiroPayload) {
	cur := &payload.ConversationState.CurrentMessage.UserInputMessage
	overhead := payloadByteSize(payload) - len(cur.Content)
	budget := maxPayloadBytes - overhead
	if budget < 0 {
		budget = 0
	}
	if len(cur.Content) > budget {
		if budget == 0 {
			cur.Content = minimalFallbackUserContent
			return
		}
		cur.Content = cur.Content[:budget]
	}
}

func buildToolResultsContinuation(toolResults []KiroToolResult) string {
	if len(toolResults) == 0 {
		return minimalFallbackUserContent
	}
	parts := make([]string, 0, len(toolResults))
	for _, tr := range toolResults {
		if len(tr.Content) == 0 {
			continue
		}
		for _, c := range tr.Content {
			if strings.TrimSpace(c.Text) != "" {
				parts = append(parts, c.Text)
			}
		}
	}
	if len(parts) == 0 {
		return minimalFallbackUserContent
	}
	joined := toolResultsContinuationPrefix + "\n\n" + strings.Join(parts, "\n\n")
	if len(joined) > 4000 {
		return joined[:4000]
	}
	return joined
}

func trimLeadingAssistantHistory(history []KiroHistoryMessage) []KiroHistoryMessage {
	idx := 0
	for idx < len(history) && history[idx].AssistantResponseMessage != nil {
		idx++
	}
	if idx == 0 {
		return history
	}
	if idx >= len(history) {
		return nil
	}
	return history[idx:]
}

func firstClaudeConversationAnchor(messages []ClaudeMessage) string {
	for _, msg := range messages {
		if msg.Role != "user" {
			continue
		}
		text, _, toolResults := extractClaudeUserContent(msg.Content)
		if strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
		if len(toolResults) > 0 {
			continue
		}
	}
	return ""
}

func firstOpenAIConversationAnchor(messages []OpenAIMessage) string {
	for _, msg := range messages {
		if msg.Role != "user" {
			continue
		}
		text := extractOpenAIMessageText(msg.Content)
		if strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func buildConversationID(modelID, systemPrompt, anchor string) string {
	anchor = strings.TrimSpace(anchor)
	if isSyntheticConversationAnchor(anchor) {
		return uuid.New().String()
	}
	seed := strings.Join([]string{modelID, strings.TrimSpace(systemPrompt), anchor}, "\n")
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(seed)).String()
}

func isSyntheticConversationAnchor(anchor string) bool {
	if strings.TrimSpace(anchor) == "" {
		return true
	}
	normalized := strings.ToLower(strings.Join(strings.Fields(anchor), " "))
	switch normalized {
	case ".", "begin conversation", "please analyze the attached image.", strings.ToLower(minimalFallbackUserContent):
		return true
	default:
		return false
	}
}

func extractOpenAITextPart(part map[string]any) (string, bool) {
	partType, _ := part["type"].(string)
	switch partType {
	case "text", "input_text":
		if t, ok := part["text"].(string); ok {
			return t, true
		}
	}
	if t, ok := part["text"].(string); ok {
		return t, true
	}
	return "", false
}

func extractImageFromOpenAIPart(part map[string]any) *KiroImage {
	partType, _ := part["type"].(string)
	if partType != "" {
		switch partType {
		case "image", "image_url", "input_image", "file", "input_file":
		default:
			return nil
		}
	}
	if fileObj, ok := part["file"].(map[string]any); ok {
		if img := extractImageFromOpenAIPart(fileObj); img != nil {
			return img
		}
	}
	if sourceObj, ok := part["source"].(map[string]any); ok {
		if img := extractImageFromOpenAIPart(sourceObj); img != nil {
			return img
		}
	}
	if raw, ok := part["mime"].(string); ok && !strings.HasPrefix(strings.ToLower(raw), "image/") {
		return nil
	}
	if raw, ok := part["media_type"].(string); ok && !strings.HasPrefix(strings.ToLower(raw), "image/") {
		return nil
	}
	if raw, ok := part["mime_type"].(string); ok && !strings.HasPrefix(strings.ToLower(raw), "image/") {
		return nil
	}
	if raw, ok := part["url"].(string); ok {
		if img := parseDataURL(raw); img != nil {
			return img
		}
	}
	if raw, ok := part["b64_json"].(string); ok {
		if img := parseBase64Image(raw, "png"); img != nil {
			return img
		}
	}
	if raw, ok := part["image_url"]; ok {
		switch v := raw.(type) {
		case string:
			if img := parseDataURL(v); img != nil {
				return img
			}
		case map[string]any:
			if u, ok := v["url"].(string); ok {
				if img := parseDataURL(u); img != nil {
					return img
				}
			}
		}
	}
	if raw, ok := part["image_base64"].(string); ok {
		if img := parseBase64Image(raw, "png"); img != nil {
			return img
		}
	}
	if raw, ok := part["data"].(string); ok {
		if img := parseDataURL(raw); img != nil {
			return img
		}
		if img := parseBase64Image(raw, "png"); img != nil {
			return img
		}
	}
	return nil
}

var imagePlaceholderPattern = regexp.MustCompile(`\[Image\s+\d+\]`)

func sanitizeImagePlaceholders(text string) string {
	cleaned := imagePlaceholderPattern.ReplaceAllString(text, "")
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	return strings.TrimSpace(cleaned)
}

func normalizeUserContent(text string, hasImages bool) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" && hasImages {
		return "Please analyze the attached image."
	}
	return trimmed
}

var dataURLPattern = regexp.MustCompile(`^data:image/([a-zA-Z0-9+.-]+)(;[a-zA-Z0-9=._:+-]+)*;base64,(.+)$`)

func parseDataURL(url string) *KiroImage {
	cleaned := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(url, "\n", ""), "\r", ""))
	if strings.Contains(cleaned, "[Image") {
		return nil
	}
	matches := dataURLPattern.FindStringSubmatch(cleaned)
	if len(matches) == 4 {
		return parseBase64Image(matches[3], matches[1])
	}
	if len(matches) != 3 {
		return nil
	}
	return parseBase64Image(matches[2], matches[1])
}

func parseBase64Image(data, format string) *KiroImage {
	format = strings.ToLower(format)
	if format == "jpg" {
		format = "jpeg"
	}
	if _, err := base64.StdEncoding.DecodeString(data); err != nil {
		if _, errRaw := base64.RawStdEncoding.DecodeString(data); errRaw != nil {
			if _, errURL := base64.URLEncoding.DecodeString(data); errURL != nil {
				if _, errRawURL := base64.RawURLEncoding.DecodeString(data); errRawURL != nil {
					return nil
				}
			}
		}
	}
	if format == "" {
		format = "png"
	}
	img := &KiroImage{Format: format}
	img.Source.Bytes = data
	return img
}

func convertOpenAITools(tools []OpenAITool) []KiroToolWrapper {
	if len(tools) == 0 {
		return nil
	}
	result := make([]KiroToolWrapper, 0, len(tools))
	for _, tool := range tools {
		if tool.Type != "function" {
			continue
		}
		desc := tool.Function.Description
		if len(desc) > maxToolDescLen {
			desc = desc[:maxToolDescLen] + "..."
		}
		name := shortenToolName(tool.Function.Name)
		if strings.TrimSpace(name) == "" {
			continue
		}
		wrapper := KiroToolWrapper{}
		wrapper.ToolSpecification.Name = name
		wrapper.ToolSpecification.Description = normalizeToolDesc(desc, name)
		wrapper.ToolSpecification.InputSchema = InputSchema{JSON: ensureObjectSchema(tool.Function.Parameters)}
		result = append(result, wrapper)
	}
	return result
}

// ==================== Kiro -> OpenAI ====================

func KiroToOpenAIResponse(content string, toolUses []KiroToolUse, inputTokens, outputTokens int, model string) *OpenAIResponse {
	msg := OpenAIMessage{Role: "assistant"}
	finishReason := "stop"

	if len(toolUses) > 0 {
		msg.Content = nil
		msg.ToolCalls = make([]ToolCall, len(toolUses))
		for i, tu := range toolUses {
			args, _ := json.Marshal(tu.Input)
			msg.ToolCalls[i] = ToolCall{ID: tu.ToolUseID, Type: "function"}
			msg.ToolCalls[i].Function.Name = tu.Name
			msg.ToolCalls[i].Function.Arguments = string(args)
		}
		finishReason = "tool_calls"
	} else {
		msg.Content = content
	}

	return &OpenAIResponse{
		ID:      "chatcmpl-" + uuid.New().String(),
		Object:  "chat.completion",
		Created: nowUnix(),
		Model:   model,
		Choices: []OpenAIChoice{{Index: 0, Message: msg, FinishReason: finishReason}},
		Usage: OpenAIUsage{
			PromptTokens:     inputTokens,
			CompletionTokens: outputTokens,
			TotalTokens:      inputTokens + outputTokens,
		},
	}
}

// ExtractThinkingFromContent is the exported form of extractThinkingFromContent,
// used by callers that aggregate non-streaming Kiro output.
func ExtractThinkingFromContent(content string) (string, string) {
	return extractThinkingFromContent(content)
}

// extractThinkingFromContent pulls out <thinking>...</thinking> spans, returning
// the cleaned content and the concatenated reasoning text.
func extractThinkingFromContent(content string) (string, string) {
	var reasoning string
	result := content
	for {
		start := strings.Index(result, "<thinking>")
		if start == -1 {
			break
		}
		rest := result[start:]
		end := strings.Index(rest, "</thinking>")
		if end == -1 {
			break
		}
		end += start
		reasoning += result[start+len("<thinking>") : end]
		result = result[:start] + result[end+len("</thinking>"):]
	}
	return strings.TrimSpace(result), reasoning
}

// KiroToOpenAIResponseWithReasoning builds a chat.completion including thinking
// output, formatted per thinkingFormat ("reasoning_content" | "thinking" |
// "think"). Returns a generic map so the reasoning field can be emitted without
// polluting the strongly-typed OpenAIResponse.
func KiroToOpenAIResponseWithReasoning(content, reasoningContent string, toolUses []KiroToolUse, inputTokens, outputTokens int, model, thinkingFormat string) map[string]any {
	finishReason := "stop"
	message := map[string]any{"role": "assistant"}

	if len(toolUses) > 0 {
		message["content"] = nil
		toolCalls := make([]map[string]any, len(toolUses))
		for i, tu := range toolUses {
			args, _ := json.Marshal(tu.Input)
			toolCalls[i] = map[string]any{
				"id":   tu.ToolUseID,
				"type": "function",
				"function": map[string]string{
					"name":      tu.Name,
					"arguments": string(args),
				},
			}
		}
		message["tool_calls"] = toolCalls
		finishReason = "tool_calls"
	} else if reasoningContent != "" {
		switch thinkingFormat {
		case "thinking":
			message["content"] = "<thinking>" + reasoningContent + "</thinking>" + content
		case "think":
			message["content"] = "<think>" + reasoningContent + "</think>" + content
		default: // "reasoning_content"
			message["content"] = content
			message["reasoning_content"] = reasoningContent
		}
	} else {
		message["content"] = content
	}

	return map[string]any{
		"id":      "chatcmpl-" + uuid.New().String(),
		"object":  "chat.completion",
		"created": nowUnix(),
		"model":   model,
		"choices": []map[string]any{{
			"index":         0,
			"message":       message,
			"finish_reason": finishReason,
		}},
		"usage": map[string]int{
			"prompt_tokens":     inputTokens,
			"completion_tokens": outputTokens,
			"total_tokens":      inputTokens + outputTokens,
		},
	}
}

func nowUnix() int64 {
	return time.Now().Unix()
}
