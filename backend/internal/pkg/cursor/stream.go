package cursor

import (
	"fmt"
	"strings"
)

type Event interface {
	cursorEvent()
}

type TextDelta struct {
	Delta string
}

func (TextDelta) cursorEvent() {}

type ToolCall struct {
	Index     int
	ID        string
	Name      string
	Arguments map[string]any
}

func (ToolCall) cursorEvent() {}

type Finish struct {
	Reason string
	Usage  Usage
}

func (Finish) cursorEvent() {}

type EventHandler func(Event) error

type Aggregator struct {
	handler   EventHandler
	text      strings.Builder
	cleanText strings.Builder
	pending   string
	usage     Usage
	finished  bool
	toolCalls []ToolCall
}

func NewAggregator(handler EventHandler) *Aggregator {
	return &Aggregator{handler: handler}
}

func (a *Aggregator) HandleSSE(event SSEEvent) error {
	if a == nil {
		return fmt.Errorf("cursor: nil aggregator")
	}
	if a.finished {
		return protocolError("aggregate stream", fmt.Errorf("event received after finish"))
	}
	if usage := event.EventUsage(); usage != nil {
		a.usage = *usage
		if a.usage.TotalTokens == 0 {
			a.usage.TotalTokens = a.usage.InputTokens + a.usage.OutputTokens
		}
	}
	switch event.Type {
	case "text-delta", "text_delta", "response.output_text.delta":
		if event.Delta == "" {
			return nil
		}
		a.text.WriteString(event.Delta)
		a.pending += event.Delta
		return a.processPending(false)
	case "finish", "message-finish", "message_finish", "done", "response.completed":
		return a.finish(event.FinishReason)
	default:
		if event.FinishReason != "" {
			return a.finish(event.FinishReason)
		}
		return nil
	}
}

func (a *Aggregator) processPending(final bool) error {
	const marker = "```json"
	for a.pending != "" {
		markerAt := strings.Index(a.pending, marker)
		if markerAt < 0 {
			emitLength := len(a.pending)
			if !final {
				emitLength -= trailingMarkerPrefixLength(a.pending, marker)
			}
			if emitLength <= 0 {
				return nil
			}
			if err := a.emitText(a.pending[:emitLength]); err != nil {
				return err
			}
			a.pending = a.pending[emitLength:]
			continue
		}
		if markerAt > 0 {
			if err := a.emitText(a.pending[:markerAt]); err != nil {
				return err
			}
			a.pending = a.pending[markerAt:]
			continue
		}

		contentStart := len(marker)
		for contentStart < len(a.pending) && (a.pending[contentStart] == ' ' || a.pending[contentStart] == '\t') {
			contentStart++
		}
		if strings.HasPrefix(a.pending[contentStart:], "action") {
			contentStart += len("action")
		}
		for contentStart < len(a.pending) && strings.ContainsRune("\r\n \t", rune(a.pending[contentStart])) {
			contentStart++
		}
		closeAt := findFenceOutsideJSONString(a.pending, contentStart)
		if closeAt < 0 && !final {
			return nil
		}
		candidateEnd, blockEnd := len(a.pending), len(a.pending)
		if closeAt >= 0 {
			candidateEnd, blockEnd = closeAt, closeAt+3
		}
		candidate := strings.TrimSpace(a.pending[contentStart:candidateEnd])
		if !looksLikeAction(candidate) {
			if err := a.emitText(a.pending[:blockEnd]); err != nil {
				return err
			}
			a.pending = a.pending[blockEnd:]
			continue
		}
		action, err := parseActionJSON(candidate)
		if err != nil {
			return protocolError("aggregate tool calls", err)
		}
		if err := a.emitToolCall(action); err != nil {
			return err
		}
		a.pending = a.pending[blockEnd:]
	}
	return nil
}

func trailingMarkerPrefixLength(value, marker string) int {
	max := len(marker) - 1
	if len(value) < max {
		max = len(value)
	}
	for length := max; length > 0; length-- {
		if strings.HasSuffix(value, marker[:length]) {
			return length
		}
	}
	return 0
}

func (a *Aggregator) emitText(text string) error {
	if text == "" {
		return nil
	}
	a.cleanText.WriteString(text)
	return a.emit(TextDelta{Delta: text})
}

func (a *Aggregator) emitToolCall(action Action) error {
	index := len(a.toolCalls)
	id := action.ID
	if id == "" {
		id = fmt.Sprintf("call_%d", index+1)
	}
	call := ToolCall{Index: index, ID: id, Name: action.Name, Arguments: action.Arguments}
	a.toolCalls = append(a.toolCalls, call)
	return a.emit(call)
}

func (a *Aggregator) finish(reason string) error {
	if err := a.processPending(true); err != nil {
		return err
	}
	if reason == "" {
		if len(a.toolCalls) > 0 {
			reason = "tool_calls"
		} else {
			reason = "stop"
		}
	}
	a.finished = true
	return a.emit(Finish{Reason: reason, Usage: a.usage})
}

func (a *Aggregator) emit(event Event) error {
	if a.handler == nil {
		return nil
	}
	return a.handler(event)
}

func (a *Aggregator) Text() string {
	if a == nil {
		return ""
	}
	return a.text.String()
}

func (a *Aggregator) CleanText() string {
	if a == nil {
		return ""
	}
	return strings.TrimSpace(a.cleanText.String() + a.pending)
}

func (a *Aggregator) Usage() Usage {
	if a == nil {
		return Usage{}
	}
	return a.usage
}

func (a *Aggregator) ToolCalls() []ToolCall {
	if a == nil {
		return nil
	}
	return append([]ToolCall(nil), a.toolCalls...)
}
