package kiro

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// Aggregator collects streaming callback output so a non-streaming response can
// be assembled after CallKiroAPI returns. It is also used by the SSE writers to
// track final usage.
type Aggregator struct {
	Text         strings.Builder
	Thinking     strings.Builder
	ToolUses     []KiroToolUse
	InputTokens  int
	OutputTokens int
}

// Callback returns a StreamCallback that feeds this aggregator.
func (a *Aggregator) Callback() *StreamCallback {
	return &StreamCallback{
		OnText: func(text string, isThinking bool) {
			if isThinking {
				_, _ = a.Thinking.WriteString(text)
			} else {
				_, _ = a.Text.WriteString(text)
			}
		},
		OnToolUse: func(tu KiroToolUse) {
			a.ToolUses = append(a.ToolUses, tu)
		},
		OnComplete: func(in, out int) {
			a.InputTokens = in
			a.OutputTokens = out
		},
	}
}

// SSEWriter is the minimal sink for server-sent events (a *gin.Context wrapper,
// http.ResponseWriter, etc. can satisfy it via a small adapter).
type SSEWriter interface {
	WriteSSE(event string, data []byte) error
	Flush()
}

// ClaudeStreamState assembles Anthropic Messages SSE from streaming callbacks.
// Event ordering: message_start → (content_block_start/delta/stop per block) →
// message_delta (stop_reason + usage) → message_stop.
type ClaudeStreamState struct {
	w             SSEWriter
	model         string
	messageID     string
	started       bool
	blockIndex    int
	blockOpen     bool
	openBlockType string // "thinking" | "text"
	toolUses      []KiroToolUse
	usage         ClaudeUsage
	writeErr      error
}

// NewClaudeStreamState creates a Claude SSE assembler.
func NewClaudeStreamState(w SSEWriter, model string) *ClaudeStreamState {
	return &ClaudeStreamState{w: w, model: model, messageID: "msg_" + uuid.New().String(), blockIndex: -1}
}

func (s *ClaudeStreamState) ensureStarted() {
	if s.started {
		return
	}
	s.started = true
	startUsage := s.usage
	startUsage.OutputTokens = 0
	start := map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            s.messageID,
			"type":          "message",
			"role":          "assistant",
			"model":         s.model,
			"content":       []any{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage":         startUsage,
		},
	}
	s.emit("message_start", start)
}

func (s *ClaudeStreamState) closeBlock() {
	if s.blockOpen {
		s.emit("content_block_stop", map[string]any{"type": "content_block_stop", "index": s.blockIndex})
		s.blockOpen = false
		s.openBlockType = ""
	}
}

// OnText streams a text or thinking delta.
func (s *ClaudeStreamState) OnText(text string, isThinking bool) {
	if text == "" {
		return
	}
	s.ensureStarted()
	wantType := "text"
	deltaType := "text_delta"
	deltaField := "text"
	if isThinking {
		wantType = "thinking"
		deltaType = "thinking_delta"
		deltaField = "thinking"
	}
	if !s.blockOpen || s.openBlockType != wantType {
		s.closeBlock()
		s.blockIndex++
		s.blockOpen = true
		s.openBlockType = wantType
		blockStart := map[string]any{"type": wantType}
		if isThinking {
			blockStart["thinking"] = ""
		} else {
			blockStart["text"] = ""
		}
		s.emit("content_block_start", map[string]any{
			"type":          "content_block_start",
			"index":         s.blockIndex,
			"content_block": blockStart,
		})
	}
	s.emit("content_block_delta", map[string]any{
		"type":  "content_block_delta",
		"index": s.blockIndex,
		"delta": map[string]any{"type": deltaType, deltaField: text},
	})
}

// OnToolUse buffers tool uses; they are emitted as complete blocks at finish so
// the input JSON is well-formed.
func (s *ClaudeStreamState) OnToolUse(tu KiroToolUse) {
	s.toolUses = append(s.toolUses, tu)
}

// OnComplete records final upstream usage. KiroGatewayService may replace it
// with cache-aware usage after applying local token fallbacks.
func (s *ClaudeStreamState) OnComplete(in, out int) {
	s.usage.InputTokens = in
	s.usage.OutputTokens = out
}

// SetUsage replaces the usage payload emitted to Anthropic clients. Callers use
// it before streaming starts for estimated prompt usage and again before Finish
// for the final cache-aware token split.
func (s *ClaudeStreamState) SetUsage(usage ClaudeUsage) {
	s.usage = usage
}

// Err returns the first SSE serialization or write error observed by the state.
func (s *ClaudeStreamState) Err() error {
	if s == nil {
		return nil
	}
	return s.writeErr
}

// Finish writes tool_use blocks (if any), message_delta and message_stop.
func (s *ClaudeStreamState) Finish() {
	s.ensureStarted()
	s.closeBlock()

	for _, tu := range s.toolUses {
		s.blockIndex++
		inputJSON, _ := json.Marshal(tu.Input)
		s.emit("content_block_start", map[string]any{
			"type":  "content_block_start",
			"index": s.blockIndex,
			"content_block": map[string]any{
				"type":  "tool_use",
				"id":    tu.ToolUseID,
				"name":  tu.Name,
				"input": map[string]any{},
			},
		})
		s.emit("content_block_delta", map[string]any{
			"type":  "content_block_delta",
			"index": s.blockIndex,
			"delta": map[string]any{"type": "input_json_delta", "partial_json": string(inputJSON)},
		})
		s.emit("content_block_stop", map[string]any{"type": "content_block_stop", "index": s.blockIndex})
	}

	stopReason := "end_turn"
	if len(s.toolUses) > 0 {
		stopReason = "tool_use"
	}
	s.emit("message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": stopReason, "stop_sequence": nil},
		"usage": s.usage,
	})
	s.emit("message_stop", map[string]any{"type": "message_stop"})
}

// Callback binds this state to a StreamCallback.
func (s *ClaudeStreamState) Callback() *StreamCallback {
	return &StreamCallback{
		OnText:     s.OnText,
		OnToolUse:  s.OnToolUse,
		OnComplete: s.OnComplete,
	}
}

func (s *ClaudeStreamState) emit(event string, payload any) {
	if s == nil || s.writeErr != nil {
		return
	}
	data, err := json.Marshal(payload)
	if err != nil {
		s.writeErr = err
		return
	}
	if s.w == nil {
		s.writeErr = fmt.Errorf("kiro: nil Claude SSE writer")
		return
	}
	if err := s.w.WriteSSE(event, data); err != nil {
		s.writeErr = err
		return
	}
	s.w.Flush()
}

// ==================== OpenAI SSE ====================

// OpenAIStreamState assembles OpenAI chat.completion.chunk SSE.
type OpenAIStreamState struct {
	w            SSEWriter
	model        string
	id           string
	created      int64
	roleSent     bool
	toolUses     []KiroToolUse
	inputTokens  int
	outputTokens int
	writeErr     error
}

// NewOpenAIStreamState creates an OpenAI SSE assembler.
func NewOpenAIStreamState(w SSEWriter, model string) *OpenAIStreamState {
	return &OpenAIStreamState{w: w, model: model, id: "chatcmpl-" + uuid.New().String(), created: nowUnix()}
}

// OnText streams a content delta. Thinking deltas are surfaced as reasoning_content.
func (s *OpenAIStreamState) OnText(text string, isThinking bool) {
	if text == "" {
		return
	}
	delta := map[string]any{}
	if !s.roleSent {
		delta["role"] = "assistant"
		s.roleSent = true
	}
	if isThinking {
		delta["reasoning_content"] = text
	} else {
		delta["content"] = text
	}
	s.emitChunk(delta, nil)
}

// OnToolUse buffers tool calls for emission at finish.
func (s *OpenAIStreamState) OnToolUse(tu KiroToolUse) {
	s.toolUses = append(s.toolUses, tu)
}

// OnComplete records final usage.
func (s *OpenAIStreamState) OnComplete(in, out int) {
	s.inputTokens = in
	s.outputTokens = out
}

// Err returns the first SSE serialization or write error observed by the state.
func (s *OpenAIStreamState) Err() error {
	if s == nil {
		return nil
	}
	return s.writeErr
}

// Finish emits tool calls (if any), the finish_reason chunk and the [DONE] marker.
func (s *OpenAIStreamState) Finish() {
	finishReason := "stop"
	if len(s.toolUses) > 0 {
		finishReason = "tool_calls"
		toolCalls := make([]map[string]any, len(s.toolUses))
		for i, tu := range s.toolUses {
			args, _ := json.Marshal(tu.Input)
			toolCalls[i] = map[string]any{
				"index": i,
				"id":    tu.ToolUseID,
				"type":  "function",
				"function": map[string]string{
					"name":      tu.Name,
					"arguments": string(args),
				},
			}
		}
		delta := map[string]any{}
		if !s.roleSent {
			delta["role"] = "assistant"
			s.roleSent = true
		}
		delta["tool_calls"] = toolCalls
		s.emitChunk(delta, nil)
	}
	s.emitChunk(map[string]any{}, &finishReason)
	s.emit("", []byte("[DONE]"))
}

// Callback binds this state to a StreamCallback.
func (s *OpenAIStreamState) Callback() *StreamCallback {
	return &StreamCallback{
		OnText:     s.OnText,
		OnToolUse:  s.OnToolUse,
		OnComplete: s.OnComplete,
	}
}

func (s *OpenAIStreamState) emitChunk(delta map[string]any, finishReason *string) {
	if s == nil || s.writeErr != nil {
		return
	}
	choice := map[string]any{
		"index": 0,
		"delta": delta,
	}
	if finishReason != nil {
		choice["finish_reason"] = *finishReason
	} else {
		choice["finish_reason"] = nil
	}
	chunk := map[string]any{
		"id":      s.id,
		"object":  "chat.completion.chunk",
		"created": s.created,
		"model":   s.model,
		"choices": []any{choice},
	}
	data, err := json.Marshal(chunk)
	if err != nil {
		if s.writeErr == nil {
			s.writeErr = err
		}
		return
	}
	s.emit("", data)
}

func (s *OpenAIStreamState) emit(event string, data []byte) {
	if s == nil || s.writeErr != nil {
		return
	}
	if s.w == nil {
		s.writeErr = fmt.Errorf("kiro: nil OpenAI SSE writer")
		return
	}
	if err := s.w.WriteSSE(event, data); err != nil {
		s.writeErr = err
		return
	}
	s.w.Flush()
}

// FormatSSE renders a single SSE frame. When event is empty, only a data line is
// written (OpenAI style); otherwise an event line precedes it (Anthropic style).
func FormatSSE(event string, data []byte) string {
	var b strings.Builder
	if event != "" {
		_, _ = b.WriteString("event: ")
		_, _ = b.WriteString(event)
		_, _ = b.WriteString("\n")
	}
	_, _ = b.WriteString("data: ")
	_, _ = b.Write(data)
	_, _ = b.WriteString("\n\n")
	return b.String()
}

// DebugString is a helper for logging a payload compactly.
func DebugString(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(data)
}
