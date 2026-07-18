package kiro

import (
	"encoding/json"
	"strings"
)

// ResponsesStreamState assembles OpenAI Responses API SSE events from Kiro
// streaming callbacks. Event sequence:
//
//	response.created → response.in_progress →
//	  (response.output_item.added / response.content_part.added /
//	   response.output_text.delta / response.function_call_* ...) →
//	response.completed → [DONE]
type ResponsesStreamState struct {
	w      SSEWriter
	model  string
	respID string
	req    *ResponsesRequest

	fullText      strings.Builder
	reasoningText strings.Builder
	toolUses      []KiroToolUse

	messageItemID  string
	messageStarted bool
	outputIndex    int
	contentIndex   int
	started        bool
	createdAt      int64

	inputTokens  int
	outputTokens int
	credits      float64
}

// NewResponsesStreamState creates a stream state and emits response.created.
func NewResponsesStreamState(w SSEWriter, model, respID string, req *ResponsesRequest, createdAt int64) *ResponsesStreamState {
	s := &ResponsesStreamState{
		w:             w,
		model:         model,
		respID:        respID,
		req:           req,
		messageItemID: generateOutputItemID("msg"),
		createdAt:     createdAt,
	}
	initial := s.envelope("in_progress", nil)
	s.send("response.created", map[string]any{"type": "response.created", "response": initial})
	s.send("response.in_progress", map[string]any{"type": "response.in_progress", "response": initial})
	return s
}

func (s *ResponsesStreamState) envelope(status string, output []ResponseOutputItem) *ResponsesObject {
	obj := &ResponsesObject{
		ID:        s.respID,
		Object:    "response",
		CreatedAt: s.createdAt,
		Status:    status,
		Model:     s.model,
		Output:    output,
		Usage:     ResponsesUsage{},
	}
	if obj.Output == nil {
		obj.Output = []ResponseOutputItem{}
	}
	if s.req != nil {
		obj.PreviousResponseID = s.req.PreviousResponseID
		obj.Metadata = s.req.Metadata
	}
	return obj
}

func (s *ResponsesStreamState) send(event string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_ = s.w.WriteSSE(event, data)
	s.w.Flush()
}

// Callback returns a StreamCallback wired to emit Responses SSE events.
func (s *ResponsesStreamState) Callback() *StreamCallback {
	return &StreamCallback{
		OnText:         s.onText,
		OnToolUse:      s.onToolUse,
		OnComplete:     func(in, out int) { s.inputTokens = in; s.outputTokens = out },
		OnCredits:      func(c float64) { s.credits = c },
		OnContextUsage: func(float64) {},
	}
}

func (s *ResponsesStreamState) ensureMessageStarted() {
	if s.messageStarted {
		return
	}
	s.messageStarted = true
	s.send("response.output_item.added", map[string]any{
		"type":         "response.output_item.added",
		"output_index": s.outputIndex,
		"item": map[string]any{
			"id":      s.messageItemID,
			"type":    "message",
			"role":    "assistant",
			"status":  "in_progress",
			"content": []map[string]any{},
		},
	})
	s.send("response.content_part.added", map[string]any{
		"type":          "response.content_part.added",
		"item_id":       s.messageItemID,
		"output_index":  s.outputIndex,
		"content_index": s.contentIndex,
		"part":          map[string]any{"type": "output_text", "text": ""},
	})
}

func (s *ResponsesStreamState) onText(text string, isThinking bool) {
	if text == "" {
		return
	}
	if isThinking {
		_, _ = s.reasoningText.WriteString(text)
		return
	}
	_, _ = s.fullText.WriteString(text)
	s.started = true
	s.ensureMessageStarted()
	s.send("response.output_text.delta", map[string]any{
		"type":          "response.output_text.delta",
		"item_id":       s.messageItemID,
		"output_index":  s.outputIndex,
		"content_index": s.contentIndex,
		"delta":         text,
	})
}

func (s *ResponsesStreamState) closeMessage(text string) {
	s.send("response.content_part.done", map[string]any{
		"type":          "response.content_part.done",
		"item_id":       s.messageItemID,
		"output_index":  s.outputIndex,
		"content_index": s.contentIndex,
		"part":          map[string]any{"type": "output_text", "text": text},
	})
	s.send("response.output_item.done", map[string]any{
		"type":         "response.output_item.done",
		"output_index": s.outputIndex,
		"item": map[string]any{
			"id":      s.messageItemID,
			"type":    "message",
			"role":    "assistant",
			"status":  "completed",
			"content": []map[string]any{{"type": "output_text", "text": text}},
		},
	})
	s.messageStarted = false
	s.outputIndex++
}

func (s *ResponsesStreamState) onToolUse(tu KiroToolUse) {
	if s.messageStarted {
		s.closeMessage(s.fullText.String())
	}
	s.toolUses = append(s.toolUses, tu)
	args, _ := json.Marshal(tu.Input)
	fcID := generateOutputItemID("fc")
	s.send("response.output_item.added", map[string]any{
		"type":         "response.output_item.added",
		"output_index": s.outputIndex,
		"item": map[string]any{
			"id":        fcID,
			"type":      "function_call",
			"status":    "in_progress",
			"call_id":   tu.ToolUseID,
			"name":      tu.Name,
			"arguments": "",
		},
	})
	s.send("response.function_call_arguments.delta", map[string]any{
		"type":         "response.function_call_arguments.delta",
		"item_id":      fcID,
		"output_index": s.outputIndex,
		"delta":        string(args),
	})
	s.send("response.output_item.done", map[string]any{
		"type":         "response.output_item.done",
		"output_index": s.outputIndex,
		"item": map[string]any{
			"id":        fcID,
			"type":      "function_call",
			"status":    "completed",
			"call_id":   tu.ToolUseID,
			"name":      tu.Name,
			"arguments": string(args),
		},
	})
	s.outputIndex++
	s.started = true
}

// Finish flushes any open message, emits response.completed and [DONE], and
// returns the completed ResponsesObject (for optional persistence) plus token
// counts and accumulated credits.
func (s *ResponsesStreamState) Finish(thinking bool, estimatedInputTokens int) (*ResponsesObject, int, int, float64) {
	finalContent, _ := extractThinkingFromContent(s.fullText.String())
	if s.messageStarted {
		s.closeMessage(finalContent)
	}

	inputTokens := s.inputTokens
	if inputTokens <= 0 {
		inputTokens = estimatedInputTokens
	}
	outputTokens := s.outputTokens
	if outputTokens <= 0 {
		outputTokens = estimateResponsesOutputTokens(finalContent, s.reasoningText.String(), s.toolUses)
	}

	obj := BuildResponsesObject(s.respID, s.model, finalContent, s.toolUses, inputTokens, outputTokens, s.req)
	obj.CreatedAt = s.createdAt
	s.send("response.completed", map[string]any{"type": "response.completed", "response": obj})
	_ = s.w.WriteSSE("", []byte("[DONE]"))
	s.w.Flush()
	return obj, inputTokens, outputTokens, s.credits
}

// EmitFailed sends a response.failed event (used when the upstream errors after
// the stream has started).
func (s *ResponsesStreamState) EmitFailed(message string) {
	s.send("response.failed", map[string]any{
		"type": "response.failed",
		"response": map[string]any{
			"id":     s.respID,
			"status": "failed",
			"error":  map[string]string{"type": "server_error", "message": message},
		},
	})
}

// Started reports whether any output has been emitted (used to decide failover).
func (s *ResponsesStreamState) Started() bool { return s.started }

// estimateResponsesOutputTokens is a rough token estimate for output content
// when the upstream did not report exact counts (~4 chars/token).
func estimateResponsesOutputTokens(content, reasoning string, toolUses []KiroToolUse) int {
	chars := len(content) + len(reasoning)
	for _, tu := range toolUses {
		args, _ := json.Marshal(tu.Input)
		chars += len(tu.Name) + len(args)
	}
	tokens := chars / 4
	if tokens < 1 && (chars > 0 || len(toolUses) > 0) {
		tokens = 1
	}
	return tokens
}
