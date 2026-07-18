package opencode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

type StreamFrame struct {
	Event    string
	Data     []byte
	Semantic bool
}

type StreamTransformer struct {
	meta RequestMeta

	chatToAnth      *apicompat.ChatCompletionsToAnthropicStreamState
	chatToResponses *apicompat.ChatCompletionsToResponsesStreamState
	anthToResponses *apicompat.AnthropicEventToResponsesState
	responsesToChat *apicompat.ResponsesEventToChatState

	usage     Usage
	requestID string
	finished  bool
}

func NewStreamTransformer(meta RequestMeta) *StreamTransformer {
	t := &StreamTransformer{meta: meta}
	if meta.UpstreamProtocol == ProtocolChatCompletions {
		switch meta.InboundProtocol {
		case ProtocolMessages:
			t.chatToAnth = apicompat.NewChatCompletionsToAnthropicStreamState(meta.BillingModel)
		case ProtocolResponses:
			t.chatToResponses = apicompat.NewChatCompletionsToResponsesStreamState(meta.BillingModel)
		}
	}
	if meta.UpstreamProtocol == ProtocolMessages && meta.InboundProtocol != ProtocolMessages {
		t.anthToResponses = apicompat.NewAnthropicEventToResponsesState()
		if meta.InboundProtocol == ProtocolChatCompletions {
			t.responsesToChat = apicompat.NewResponsesEventToChatState()
		}
	}
	return t
}

func (t *StreamTransformer) Push(event string, data []byte) ([]StreamFrame, error) {
	if t == nil || t.finished {
		return nil, nil
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, nil
	}
	if bytes.Equal(data, []byte("[DONE]")) {
		return t.Finalize()
	}

	switch t.meta.UpstreamProtocol {
	case ProtocolChatCompletions:
		var chunk apicompat.ChatCompletionsChunk
		if err := json.Unmarshal(data, &chunk); err != nil {
			return nil, fmt.Errorf("decode chat completions stream chunk: %w", err)
		}
		if chunk.ID != "" {
			t.requestID = chunk.ID
		}
		if chunk.Usage != nil {
			t.usage = usageFromChat(chunk.Usage)
		}
		switch t.meta.InboundProtocol {
		case ProtocolChatCompletions:
			return []StreamFrame{{Data: append([]byte(nil), data...), Semantic: chatChunkSemantic(&chunk)}}, nil
		case ProtocolMessages:
			return anthropicFrames(apicompat.ChatCompletionsChunkToAnthropicEvents(&chunk, t.chatToAnth)), nil
		case ProtocolResponses:
			return responsesFrames(apicompat.ChatCompletionsChunkToResponsesEvents(&chunk, t.chatToResponses)), nil
		}
	case ProtocolMessages:
		var messageEvent apicompat.AnthropicStreamEvent
		if err := json.Unmarshal(data, &messageEvent); err != nil {
			return nil, fmt.Errorf("decode messages stream event: %w", err)
		}
		if messageEvent.Type == "" {
			messageEvent.Type = strings.TrimSpace(event)
		}
		captureAnthropicStreamMetadata(t, &messageEvent)
		switch t.meta.InboundProtocol {
		case ProtocolMessages:
			return []StreamFrame{{Event: messageEvent.Type, Data: append([]byte(nil), data...), Semantic: messageEvent.Type == "content_block_delta"}}, nil
		case ProtocolResponses:
			return responsesFrames(apicompat.AnthropicEventToResponsesEvents(&messageEvent, t.anthToResponses)), nil
		case ProtocolChatCompletions:
			responsesEvents := apicompat.AnthropicEventToResponsesEvents(&messageEvent, t.anthToResponses)
			return t.responsesEventsToChatFrames(responsesEvents), nil
		}
	}
	return nil, fmt.Errorf("unsupported OpenCode stream conversion %s -> %s", t.meta.UpstreamProtocol, t.meta.InboundProtocol)
}

func (t *StreamTransformer) Finalize() ([]StreamFrame, error) {
	if t == nil || t.finished {
		return nil, nil
	}
	t.finished = true
	var frames []StreamFrame
	switch {
	case t.meta.UpstreamProtocol == ProtocolChatCompletions && t.meta.InboundProtocol == ProtocolChatCompletions:
		frames = append(frames, StreamFrame{Data: []byte("[DONE]")})
	case t.meta.UpstreamProtocol == ProtocolChatCompletions && t.meta.InboundProtocol == ProtocolMessages:
		frames = append(frames, anthropicFrames(apicompat.FinalizeChatCompletionsAnthropicStream(t.chatToAnth))...)
	case t.meta.UpstreamProtocol == ProtocolChatCompletions && t.meta.InboundProtocol == ProtocolResponses:
		frames = append(frames, responsesFrames(apicompat.FinalizeChatCompletionsResponsesStream(t.chatToResponses))...)
	case t.meta.UpstreamProtocol == ProtocolMessages && t.meta.InboundProtocol == ProtocolMessages:
		// Anthropic streams terminate with message_stop and do not require [DONE].
	case t.meta.UpstreamProtocol == ProtocolMessages && t.meta.InboundProtocol == ProtocolResponses:
		frames = append(frames, responsesFrames(apicompat.FinalizeAnthropicResponsesStream(t.anthToResponses))...)
	case t.meta.UpstreamProtocol == ProtocolMessages && t.meta.InboundProtocol == ProtocolChatCompletions:
		responsesEvents := apicompat.FinalizeAnthropicResponsesStream(t.anthToResponses)
		frames = append(frames, t.responsesEventsToChatFrames(responsesEvents)...)
		frames = append(frames, chatFrames(apicompat.FinalizeResponsesChatStream(t.responsesToChat))...)
		frames = append(frames, StreamFrame{Data: []byte("[DONE]")})
	}
	return frames, nil
}

func (t *StreamTransformer) Usage() Usage      { return t.usage }
func (t *StreamTransformer) RequestID() string { return t.requestID }

func captureAnthropicStreamMetadata(t *StreamTransformer, event *apicompat.AnthropicStreamEvent) {
	if event.Message != nil {
		if event.Message.ID != "" {
			t.requestID = event.Message.ID
		}
		t.usage = usageFromAnthropic(event.Message.Usage)
	}
	if event.Usage != nil {
		usage := usageFromAnthropic(*event.Usage)
		if usage.InputTokens != 0 {
			t.usage.InputTokens = usage.InputTokens
		}
		if usage.OutputTokens != 0 {
			t.usage.OutputTokens = usage.OutputTokens
		}
		if usage.CacheReadInputTokens != 0 {
			t.usage.CacheReadInputTokens = usage.CacheReadInputTokens
		}
		if usage.CacheCreationInputTokens != 0 {
			t.usage.CacheCreationInputTokens = usage.CacheCreationInputTokens
		}
	}
}

func (t *StreamTransformer) responsesEventsToChatFrames(events []apicompat.ResponsesStreamEvent) []StreamFrame {
	var frames []StreamFrame
	for i := range events {
		frames = append(frames, chatFrames(apicompat.ResponsesEventToChatChunks(&events[i], t.responsesToChat))...)
	}
	return frames
}

func anthropicFrames(events []apicompat.AnthropicStreamEvent) []StreamFrame {
	frames := make([]StreamFrame, 0, len(events))
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			continue
		}
		frames = append(frames, StreamFrame{Event: event.Type, Data: data, Semantic: event.Type == "content_block_delta"})
	}
	return frames
}

func responsesFrames(events []apicompat.ResponsesStreamEvent) []StreamFrame {
	frames := make([]StreamFrame, 0, len(events))
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			continue
		}
		frames = append(frames, StreamFrame{Event: event.Type, Data: data, Semantic: strings.HasSuffix(event.Type, ".delta")})
	}
	return frames
}

func chatFrames(chunks []apicompat.ChatCompletionsChunk) []StreamFrame {
	frames := make([]StreamFrame, 0, len(chunks))
	for i := range chunks {
		data, err := json.Marshal(chunks[i])
		if err != nil {
			continue
		}
		frames = append(frames, StreamFrame{Data: data, Semantic: chatChunkSemantic(&chunks[i])})
	}
	return frames
}

func chatChunkSemantic(chunk *apicompat.ChatCompletionsChunk) bool {
	if chunk == nil {
		return false
	}
	for _, choice := range chunk.Choices {
		if choice.Delta.Content != nil && *choice.Delta.Content != "" {
			return true
		}
		if choice.Delta.ReasoningContent != nil && *choice.Delta.ReasoningContent != "" {
			return true
		}
		if len(choice.Delta.ToolCalls) > 0 {
			return true
		}
	}
	return false
}
