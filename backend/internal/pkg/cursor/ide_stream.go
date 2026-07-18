package cursor

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"google.golang.org/protobuf/encoding/protowire"
)

type ConnectFrame struct {
	Flags   byte
	Payload []byte
}

func (f ConnectFrame) Compressed() bool { return f.Flags&1 != 0 }
func (f ConnectFrame) EndStream() bool  { return f.Flags&2 != 0 }

type ConnectDecoder struct {
	buffer           []byte
	maxFrameSize     int
	maxBufferedBytes int
}

func NewConnectDecoder(maxFrameSize, maxBufferedBytes int) *ConnectDecoder {
	if maxFrameSize <= 0 {
		maxFrameSize = defaultIDEMaxFrameSize
	}
	if maxBufferedBytes <= 0 {
		maxBufferedBytes = defaultIDEMaxBufferedBytes
	}
	return &ConnectDecoder{maxFrameSize: maxFrameSize, maxBufferedBytes: maxBufferedBytes}
}

func (d *ConnectDecoder) Feed(chunk []byte) ([]ConnectFrame, error) {
	if len(chunk) > d.maxBufferedBytes-len(d.buffer) {
		return nil, protocolError("parse connect stream", fmt.Errorf("buffer exceeds %d bytes", d.maxBufferedBytes))
	}
	d.buffer = append(d.buffer, chunk...)
	frames := make([]ConnectFrame, 0, 1)
	for len(d.buffer) >= 5 {
		flags := d.buffer[0]
		if flags&^byte(3) != 0 {
			return nil, protocolError("parse connect stream", fmt.Errorf("unsupported frame flags 0x%02x", flags))
		}
		length := int(uint32(d.buffer[1])<<24 | uint32(d.buffer[2])<<16 | uint32(d.buffer[3])<<8 | uint32(d.buffer[4]))
		if length > d.maxFrameSize {
			return nil, protocolError("parse connect stream", fmt.Errorf("frame size %d exceeds %d bytes", length, d.maxFrameSize))
		}
		if length > d.maxBufferedBytes-5 {
			return nil, protocolError("parse connect stream", fmt.Errorf("frame cannot fit in %d-byte buffer", d.maxBufferedBytes))
		}
		if len(d.buffer) < 5+length {
			break
		}
		payload := append([]byte(nil), d.buffer[5:5+length]...)
		d.buffer = d.buffer[5+length:]
		if flags&1 != 0 {
			decoded, err := gunzipLimited(payload, d.maxFrameSize)
			if err != nil {
				return nil, protocolError("decompress connect frame", err)
			}
			payload = decoded
		}
		frames = append(frames, ConnectFrame{Flags: flags, Payload: payload})
	}
	if len(d.buffer) > d.maxBufferedBytes {
		return nil, protocolError("parse connect stream", fmt.Errorf("buffer exceeds %d bytes", d.maxBufferedBytes))
	}
	return frames, nil
}

func (d *ConnectDecoder) Finish() error {
	if len(d.buffer) != 0 {
		return protocolError("parse connect stream", fmt.Errorf("truncated frame with %d buffered bytes", len(d.buffer)))
	}
	return nil
}

func (s *IDEEventStream) Response() *http.Response {
	if s == nil {
		return nil
	}
	return s.response
}

func (s *IDEEventStream) Next() (IDEEvent, error) {
	if len(s.pending) > 0 {
		event := s.pending[0]
		s.pending = s.pending[1:]
		if event.Type == IDEEventFinish || event.Type == IDEEventError {
			s.finished = true
		}
		return event, nil
	}
	if s.finished {
		return IDEEvent{}, io.EOF
	}
	buffer := make([]byte, 32<<10)
	for {
		n, err := s.response.Body.Read(buffer)
		if n > 0 {
			frames, decodeErr := s.decoder.Feed(buffer[:n])
			if decodeErr != nil {
				s.finished = true
				return IDEEvent{}, decodeErr
			}
			for _, frame := range frames {
				if frame.EndStream() {
					s.sawEndStream = true
				}
				for _, event := range parseIDEFrame(frame) {
					s.pending = append(s.pending, s.normalizeToolEvent(event)...)
				}
			}
			if len(s.pending) > 0 {
				event := s.pending[0]
				s.pending = s.pending[1:]
				if event.Type == IDEEventFinish {
					s.finished = true
				}
				return event, nil
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				s.finished = true
				return IDEEvent{}, transportError("read IDE stream", err)
			}
			if finishErr := s.decoder.Finish(); finishErr != nil {
				s.finished = true
				return IDEEvent{}, finishErr
			}
			s.finished = true
			if !s.sawEndStream {
				return IDEEvent{}, protocolError("read IDE stream", errors.New("stream ended before Connect end-stream envelope"))
			}
			return IDEEvent{}, io.EOF
		}
	}
}

func (s *IDEEventStream) Close() error {
	if s == nil || s.response == nil || s.response.Body == nil {
		return nil
	}
	s.finished = true
	return s.response.Body.Close()
}

func (s *IDEEventStream) normalizeToolEvent(event IDEEvent) []IDEEvent {
	if event.Type != IDEEventToolCall || event.ToolCall == nil {
		if event.Type == IDEEventFinish {
			if len(s.toolCalls) > 0 {
				return []IDEEvent{{Type: IDEEventError, Error: &IDEStreamError{Code: "incomplete_tool_call", Message: "stream ended with unfinished tool arguments"}}}
			}
			if s.finishReason != "" {
				event.FinishReason = s.finishReason
			}
		}
		return []IDEEvent{event}
	}
	if s.toolCalls == nil {
		s.toolCalls = make(map[string]*ideToolAccumulator)
	}
	if s.maxToolBytes <= 0 {
		s.maxToolBytes = defaultIDEMaxBufferedBytes
	}
	if s.maxToolCalls <= 0 {
		s.maxToolCalls = 64
	}
	key := event.ToolCall.ID
	if key == "" {
		key = event.ToolCall.Name
	}
	if key == "" {
		key = "anonymous"
	}
	accumulator := s.toolCalls[key]
	if accumulator == nil && event.ToolCall.ID != "" && event.ToolCall.Name != "" {
		if prior := s.toolCalls[event.ToolCall.Name]; prior != nil {
			delete(s.toolCalls, event.ToolCall.Name)
			accumulator = prior
			s.toolCalls[key] = prior
		}
	}
	if accumulator == nil {
		if len(s.toolCalls) >= s.maxToolCalls {
			return []IDEEvent{{Type: IDEEventError, Error: &IDEStreamError{Code: "tool_call_limit", Message: "too many unfinished tool calls"}}}
		}
		accumulator = &ideToolAccumulator{}
		s.toolCalls[key] = accumulator
	}
	if event.ToolCall.ID != "" {
		accumulator.id = event.ToolCall.ID
	}
	if event.ToolCall.Name != "" {
		accumulator.name = event.ToolCall.Name
	}
	if len(event.toolCallRawArgs) > s.maxToolBytes-s.toolBytes {
		return []IDEEvent{{Type: IDEEventError, Error: &IDEStreamError{Code: "tool_arguments_too_large", Message: "tool arguments exceed configured stream buffer limit"}}}
	}
	accumulator.rawArgs += event.toolCallRawArgs
	s.toolBytes += len(event.toolCallRawArgs)
	complete := event.toolCallHasLast && event.toolCallLast
	if !event.toolCallHasLast {
		complete = accumulator.rawArgs == "" || json.Valid([]byte(accumulator.rawArgs))
	}
	if !complete {
		return nil
	}
	delete(s.toolCalls, key)
	s.toolBytes -= len(accumulator.rawArgs)
	arguments := make(map[string]any)
	if accumulator.rawArgs != "" {
		decoder := json.NewDecoder(strings.NewReader(accumulator.rawArgs))
		decoder.UseNumber()
		if err := decoder.Decode(&arguments); err != nil {
			return []IDEEvent{{Type: IDEEventError, Error: &IDEStreamError{Code: "invalid_tool_arguments", Message: err.Error()}}}
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return []IDEEvent{{Type: IDEEventError, Error: &IDEStreamError{Code: "invalid_tool_arguments", Message: "multiple JSON values in tool arguments"}}}
		}
	}
	result := IDEEvent{Type: IDEEventToolCall, ToolCall: &Action{ID: accumulator.id, Name: accumulator.name, Arguments: arguments}}
	if event.toolCallHasLast && event.toolCallLast {
		s.finishReason = "tool_calls"
	}
	return []IDEEvent{result}
}

func parseIDEFrame(frame ConnectFrame) []IDEEvent {
	if frame.EndStream() {
		streamErr, err := parseConnectEndStream(frame.Payload)
		if err != nil {
			return []IDEEvent{protocolEvent(err)}
		}
		if streamErr != nil {
			return []IDEEvent{{Type: IDEEventError, Error: streamErr}}
		}
		return []IDEEvent{{Type: IDEEventFinish, FinishReason: "stop"}}
	}
	return parseIDEResponse(frame.Payload)
}

func parseConnectEndStream(payload []byte) (*IDEStreamError, error) {
	var envelope struct {
		Error *struct {
			Code    string          `json:"code"`
			Message string          `json:"message"`
			Details json.RawMessage `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, fmt.Errorf("invalid Connect end-stream envelope: %w", err)
	}
	if envelope.Error == nil {
		return nil, nil
	}
	return (*IDEStreamError)(envelope.Error), nil
}

func parseIDEResponse(payload []byte) []IDEEvent {
	var events []IDEEvent
	for len(payload) > 0 {
		number, wireType, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return append(events, protocolEvent(protowire.ParseError(n)))
		}
		payload = payload[n:]
		switch wireType {
		case protowire.BytesType:
			value, size := protowire.ConsumeBytes(payload)
			if size < 0 {
				return append(events, protocolEvent(protowire.ParseError(size)))
			}
			payload = payload[size:]
			switch number {
			case 1:
				events = append(events, parseNativeToolCall(value)...)
			case 2:
				text, thinking := parseTextThinking(value, 0)
				if thinking != "" {
					events = append(events, IDEEvent{Type: IDEEventThinking, Thinking: thinking})
				}
				if text != "" {
					events = append(events, IDEEvent{Type: IDEEventText, Text: text})
				}
			case 12:
				usage := parseIDEUsage(value)
				events = append(events, IDEEvent{Type: IDEEventUsage, Usage: &usage})
			}
		default:
			size := protowire.ConsumeFieldValue(number, wireType, payload)
			if size < 0 {
				return append(events, protocolEvent(protowire.ParseError(size)))
			}
			payload = payload[size:]
		}
	}
	return events
}

func parseNativeToolCall(payload []byte) []IDEEvent {
	var id, name, rawArgs string
	var isLast, hasLast bool
	for len(payload) > 0 {
		number, wireType, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return []IDEEvent{protocolEvent(protowire.ParseError(n))}
		}
		payload = payload[n:]
		switch wireType {
		case protowire.BytesType:
			value, size := protowire.ConsumeBytes(payload)
			if size < 0 {
				return []IDEEvent{protocolEvent(protowire.ParseError(size))}
			}
			payload = payload[size:]
			switch number {
			case 3:
				id = string(value)
			case 9:
				name = string(value)
			case 10:
				rawArgs = string(value)
			case 27:
				mcpID, mcpName, mcpArgs := parseMCPToolCall(value)
				if id == "" {
					id = mcpID
				}
				if name == "" {
					name = mcpName
				}
				if rawArgs == "" {
					rawArgs = mcpArgs
				}
			}
		case protowire.VarintType:
			value, size := protowire.ConsumeVarint(payload)
			if size < 0 {
				return []IDEEvent{protocolEvent(protowire.ParseError(size))}
			}
			payload = payload[size:]
			if number == 11 {
				hasLast = true
				isLast = value != 0
			}
		default:
			size := protowire.ConsumeFieldValue(number, wireType, payload)
			if size < 0 {
				return []IDEEvent{protocolEvent(protowire.ParseError(size))}
			}
			payload = payload[size:]
		}
	}
	if id == "" && name == "" && rawArgs == "" {
		return nil
	}
	return []IDEEvent{{
		Type: IDEEventToolCall, ToolCall: &Action{ID: id, Name: name},
		toolCallRawArgs: rawArgs, toolCallLast: isLast, toolCallHasLast: hasLast,
	}}
}

func parseMCPToolCall(payload []byte) (id, name, rawArgs string) {
	for len(payload) > 0 {
		number, wireType, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return id, name, rawArgs
		}
		payload = payload[n:]
		if wireType != protowire.BytesType {
			size := protowire.ConsumeFieldValue(number, wireType, payload)
			if size < 0 {
				return id, name, rawArgs
			}
			payload = payload[size:]
			continue
		}
		value, size := protowire.ConsumeBytes(payload)
		if size < 0 {
			return id, name, rawArgs
		}
		payload = payload[size:]
		switch number {
		case 1, 9:
			if name == "" {
				name = string(value)
			}
		case 2, 10:
			if rawArgs == "" {
				rawArgs = string(value)
			}
		case 3:
			if id == "" {
				id = string(value)
			}
		}
	}
	return id, name, rawArgs
}

func parseTextThinking(payload []byte, depth int) (string, string) {
	if depth > 6 {
		return "", ""
	}
	var text, thinking string
	for len(payload) > 0 {
		number, wireType, n := protowire.ConsumeTag(payload)
		if n < 0 {
			break
		}
		payload = payload[n:]
		if wireType != protowire.BytesType {
			size := protowire.ConsumeFieldValue(number, wireType, payload)
			if size < 0 {
				break
			}
			payload = payload[size:]
			continue
		}
		value, size := protowire.ConsumeBytes(payload)
		if size < 0 {
			break
		}
		payload = payload[size:]
		switch number {
		case 1:
			if isReadableUTF8(value) {
				text += string(value)
			} else {
				nestedText, nestedThinking := parseTextThinking(value, depth+1)
				text += nestedText
				thinking += nestedThinking
			}
		case 25:
			if inner, ok := firstReadableFieldOne(value); ok {
				thinking += inner
			} else if isReadableUTF8(value) {
				thinking += string(value)
			}
		default:
			nestedText, nestedThinking := parseTextThinking(value, depth+1)
			text += nestedText
			thinking += nestedThinking
		}
	}
	return text, thinking
}

func firstReadableFieldOne(payload []byte) (string, bool) {
	for len(payload) > 0 {
		number, wireType, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return "", false
		}
		payload = payload[n:]
		if wireType == protowire.BytesType {
			value, size := protowire.ConsumeBytes(payload)
			if size < 0 {
				return "", false
			}
			payload = payload[size:]
			if number == 1 && isReadableUTF8(value) {
				return string(value), true
			}
			continue
		}
		size := protowire.ConsumeFieldValue(number, wireType, payload)
		if size < 0 {
			return "", false
		}
		payload = payload[size:]
	}
	return "", false
}

func isReadableUTF8(value []byte) bool {
	if len(value) == 0 || !utf8.Valid(value) {
		return false
	}
	for _, r := range string(value) {
		if r < 0x20 && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}
	return true
}

func parseIDEUsage(payload []byte) Usage {
	var usage Usage
	for len(payload) > 0 {
		number, wireType, n := protowire.ConsumeTag(payload)
		if n < 0 {
			break
		}
		payload = payload[n:]
		if wireType != protowire.VarintType {
			size := protowire.ConsumeFieldValue(number, wireType, payload)
			if size < 0 {
				break
			}
			payload = payload[size:]
			continue
		}
		value, size := protowire.ConsumeVarint(payload)
		if size < 0 {
			break
		}
		payload = payload[size:]
		switch number {
		case 1:
			usage.InputTokens = int(value)
		case 2:
			usage.OutputTokens = int(value)
		case 3:
			usage.CacheWriteTokens = int(value)
		case 4:
			usage.CacheReadTokens = int(value)
		case 5:
			usage.ReasoningTokens = int(value)
		case 6:
			usage.TotalTokens = int(value)
		}
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}
	return usage
}

// DecodeIDEAvailableModels decodes a Connect-Protobuf AvailableModels response.
// Cursor has used both repeated modelNames strings (field 1) and nested model
// messages (field 2); accepting both keeps discovery compatible across client
// protocol revisions without falling back to the JSON Connect codec.
func DecodeIDEAvailableModels(body []byte, maxFrameSize, maxBufferedBytes int) ([]string, error) {
	decoder := NewConnectDecoder(maxFrameSize, maxBufferedBytes)
	frames, err := decoder.Feed(body)
	if err != nil {
		return nil, err
	}
	if err := decoder.Finish(); err != nil {
		return nil, err
	}
	models := make([]string, 0)
	sawEndStream := false
	for _, frame := range frames {
		if frame.EndStream() {
			sawEndStream = true
			streamErr, envelopeErr := parseConnectEndStream(frame.Payload)
			if envelopeErr != nil {
				return nil, protocolError("decode IDE models response", envelopeErr)
			}
			if streamErr != nil {
				return nil, protocolError("decode IDE models response", errors.New(streamErr.Message))
			}
			continue
		}
		parsed, parseErr := parseIDEAvailableModelsPayload(frame.Payload)
		if parseErr != nil {
			return nil, parseErr
		}
		models = append(models, parsed...)
	}
	if !sawEndStream {
		return nil, protocolError("decode IDE models response", errors.New("response ended before Connect end-stream envelope"))
	}
	if len(models) == 0 {
		return nil, protocolError("decode IDE models response", errors.New("cursor returned no IDE models"))
	}
	return models, nil
}

func parseIDEAvailableModelsPayload(payload []byte) ([]string, error) {
	models := make([]string, 0)
	for len(payload) > 0 {
		number, wireType, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return nil, protocolError("decode IDE models response", protowire.ParseError(n))
		}
		payload = payload[n:]
		if wireType != protowire.BytesType {
			size := protowire.ConsumeFieldValue(number, wireType, payload)
			if size < 0 {
				return nil, protocolError("decode IDE models response", protowire.ParseError(size))
			}
			payload = payload[size:]
			continue
		}
		value, size := protowire.ConsumeBytes(payload)
		if size < 0 {
			return nil, protocolError("decode IDE models response", protowire.ParseError(size))
		}
		payload = payload[size:]
		switch number {
		case 1:
			if isReadableUTF8(value) {
				models = append(models, string(value))
			}
		case 2:
			if name := parseIDEAvailableModelName(value); name != "" {
				models = append(models, name)
			}
		}
	}
	return models, nil
}

func parseIDEAvailableModelName(payload []byte) string {
	for len(payload) > 0 {
		number, wireType, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return ""
		}
		payload = payload[n:]
		if wireType != protowire.BytesType {
			size := protowire.ConsumeFieldValue(number, wireType, payload)
			if size < 0 {
				return ""
			}
			payload = payload[size:]
			continue
		}
		value, size := protowire.ConsumeBytes(payload)
		if size < 0 {
			return ""
		}
		payload = payload[size:]
		if number == 1 && isReadableUTF8(value) {
			return string(value)
		}
	}
	return ""
}

func protocolEvent(err error) IDEEvent {
	return IDEEvent{Type: IDEEventError, Error: &IDEStreamError{Code: "protocol_error", Message: err.Error()}}
}
