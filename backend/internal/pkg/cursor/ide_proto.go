package cursor

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"google.golang.org/protobuf/encoding/protowire"
)

const (
	defaultIDEMaxFrameSize     = 16 << 20
	defaultIDEMaxBufferedBytes = 20 << 20
	defaultIDEMaxErrorBody     = 64 << 10
)

func appendVarint(dst []byte, field protowire.Number, value uint64) []byte {
	dst = protowire.AppendTag(dst, field, protowire.VarintType)
	return protowire.AppendVarint(dst, value)
}

func appendBytes(dst []byte, field protowire.Number, value []byte) []byte {
	dst = protowire.AppendTag(dst, field, protowire.BytesType)
	return protowire.AppendBytes(dst, value)
}

func appendString(dst []byte, field protowire.Number, value string) []byte {
	return appendBytes(dst, field, []byte(value))
}

func EncodeConnectFrame(payload []byte, compress bool) ([]byte, error) {
	flags := byte(0)
	if compress {
		var compressed bytes.Buffer
		writer := gzip.NewWriter(&compressed)
		if _, err := writer.Write(payload); err != nil {
			return nil, fmt.Errorf("compress connect frame: %w", err)
		}
		if err := writer.Close(); err != nil {
			return nil, fmt.Errorf("compress connect frame: %w", err)
		}
		payload = compressed.Bytes()
		flags = 1
	}
	if uint64(len(payload)) > uint64(^uint32(0)) {
		return nil, fmt.Errorf("connect frame is too large")
	}
	frame := make([]byte, 5+len(payload))
	frame[0] = flags
	frame[1] = byte(len(payload) >> 24)
	frame[2] = byte(len(payload) >> 16)
	frame[3] = byte(len(payload) >> 8)
	frame[4] = byte(len(payload))
	copy(frame[5:], payload)
	return frame, nil
}

func encodeIDEChatRequest(dialogue *Dialogue, options IDEChatOptions, uuidFn func() string, metadata []byte) ([]byte, error) {
	if _, err := validateDialogue(dialogue); err != nil {
		return nil, err
	}
	if strings.TrimSpace(options.Model) == "" {
		return nil, badRequest("encode IDE chat request", fmt.Errorf("model is required"))
	}
	mode := options.Mode
	if mode == "" {
		mode = IDEModeAsk
	}
	if mode != IDEModeAsk && mode != IDEModeAgent {
		return nil, badRequest("encode IDE chat request", fmt.Errorf("unsupported mode %q", mode))
	}
	conversationID := strings.TrimSpace(options.ConversationID)
	if conversationID == "" {
		conversationID = uuidFn()
	}

	var request []byte
	messageIDs := make([][]byte, 0, len(dialogue.Messages))
	for _, message := range dialogue.Messages {
		content, role, err := encodeIDEHistoryMessage(message)
		if err != nil {
			return nil, badRequest("encode IDE history", err)
		}
		if strings.TrimSpace(content) == "" {
			continue
		}
		messageID := uuidFn()
		var encoded []byte
		encoded = appendString(encoded, 1, content)
		encoded = appendVarint(encoded, 2, role)
		encoded = appendString(encoded, 13, messageID)
		if role == 1 {
			encoded = appendVarint(encoded, 47, modeEnum(mode))
		}
		request = appendBytes(request, 1, encoded)

		var id []byte
		id = appendString(id, 1, messageID)
		id = appendVarint(id, 3, role)
		messageIDs = append(messageIDs, id)
	}
	request = appendVarint(request, 2, 1)
	var instruction []byte
	if strings.TrimSpace(dialogue.System) != "" {
		instruction = appendString(instruction, 1, dialogue.System)
	}
	request = appendBytes(request, 3, instruction)
	request = appendVarint(request, 4, 1)
	var model []byte
	model = appendString(model, 1, options.Model)
	model = appendBytes(model, 4, nil)
	request = appendBytes(request, 5, model)
	request = appendString(request, 8, "")
	request = appendVarint(request, 13, 1)
	request = appendBytes(request, 15, encodeCursorSetting())
	request = appendVarint(request, 19, 1)
	request = appendString(request, 23, conversationID)
	request = appendBytes(request, 26, metadata)
	request = appendVarint(request, 27, boolVarint(mode == IDEModeAgent))
	if mode == IDEModeAgent && len(dialogue.Tools) > 0 {
		request = appendVarint(request, 29, 19) // ClientSideToolV2.MCP
		for _, tool := range dialogue.Tools {
			encodedTool, err := encodeMCPTool(tool)
			if err != nil {
				return nil, badRequest("encode IDE MCP tool", err)
			}
			request = appendBytes(request, 34, encodedTool)
		}
	}
	for _, id := range messageIDs {
		request = appendBytes(request, 30, id)
	}
	request = appendVarint(request, 35, 0)
	request = appendVarint(request, 38, 0)
	request = appendVarint(request, 46, modeEnum(mode))
	request = appendString(request, 47, "")
	request = appendVarint(request, 48, 0)
	request = appendVarint(request, 49, boolVarint(options.Thinking))
	request = appendVarint(request, 51, 0)
	request = appendVarint(request, 53, 1)
	request = appendString(request, 54, modeName(mode))

	var wrapper []byte
	wrapper = appendBytes(wrapper, 1, request)
	return wrapper, nil
}

func encodeIDEHistoryMessage(message DialogueMessage) (string, uint64, error) {
	var content strings.Builder
	_, _ = content.WriteString(message.Text)
	for _, call := range message.ToolCalls {
		if strings.TrimSpace(call.Name) == "" {
			return "", 0, fmt.Errorf("tool call name is required")
		}
		args, err := json.Marshal(call.Arguments)
		if err != nil {
			return "", 0, fmt.Errorf("tool call %q arguments: %w", call.Name, err)
		}
		if content.Len() > 0 {
			_, _ = content.WriteString("\n\n")
		}
		_, _ = content.WriteString("[tool_call id=")
		_, _ = content.WriteString(call.ID)
		_, _ = content.WriteString(" name=")
		_, _ = content.WriteString(call.Name)
		_, _ = content.WriteString("]\n")
		_, _ = content.Write(args)
	}
	switch message.Role {
	case "user":
		return content.String(), 1, nil
	case "assistant":
		return content.String(), 2, nil
	case "tool":
		prefix := "[tool_result"
		if message.IsError {
			prefix = "[tool_error"
		}
		if message.ToolCallID != "" {
			prefix += " id=" + message.ToolCallID
		}
		return prefix + "]\n" + content.String(), 1, nil
	default:
		return "", 0, fmt.Errorf("unsupported dialogue role %q", message.Role)
	}
}

func encodeMCPTool(tool ToolDefinition) ([]byte, error) {
	if strings.TrimSpace(tool.Name) == "" {
		return nil, fmt.Errorf("tool name is required")
	}
	schema := tool.InputSchema
	if len(schema) == 0 {
		schema = json.RawMessage(`{"type":"object"}`)
	}
	var value any
	if err := json.Unmarshal(schema, &value); err != nil {
		return nil, fmt.Errorf("tool %q schema: %w", tool.Name, err)
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("tool %q schema: %w", tool.Name, err)
	}
	var encoded []byte
	encoded = appendString(encoded, 1, tool.Name)
	encoded = appendString(encoded, 2, tool.Description)
	encoded = appendString(encoded, 3, string(canonical))
	encoded = appendString(encoded, 4, "custom")
	return encoded, nil
}

func encodeCursorSetting() []byte {
	var nested []byte
	nested = appendBytes(nested, 1, nil)
	nested = appendBytes(nested, 2, nil)
	var setting []byte
	setting = appendString(setting, 1, `cursor\aisettings`)
	setting = appendBytes(setting, 3, nil)
	setting = appendBytes(setting, 6, nested)
	setting = appendVarint(setting, 8, 1)
	setting = appendVarint(setting, 9, 1)
	return setting
}

func modeEnum(mode IDEMode) uint64 {
	if mode == IDEModeAgent {
		return 2
	}
	return 1
}

func modeName(mode IDEMode) string {
	if mode == IDEModeAgent {
		return "agent"
	}
	return "Ask"
}

func boolVarint(value bool) uint64 {
	if value {
		return 1
	}
	return 0
}

func gunzipLimited(payload []byte, max int) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()
	data, err := io.ReadAll(io.LimitReader(reader, int64(max)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > max {
		return nil, fmt.Errorf("decompressed frame exceeds %d bytes", max)
	}
	return data, nil
}
