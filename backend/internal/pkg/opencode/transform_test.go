package opencode

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

func TestNonStreamingConversionMatrix(t *testing.T) {
	t.Parallel()

	requests := map[Protocol][]byte{
		ProtocolChatCompletions: []byte(`{"model":"alias","messages":[{"role":"user","content":"hello"}]}`),
		ProtocolMessages:        []byte(`{"model":"alias","max_tokens":32,"messages":[{"role":"user","content":"hello"}]}`),
		ProtocolResponses:       []byte(`{"model":"alias","input":"hello"}`),
	}
	upstreamResponses := map[Protocol][]byte{
		ProtocolChatCompletions: []byte(`{"id":"chat-1","object":"chat.completion","created":1,"model":"grok-4.5","choices":[{"index":0,"message":{"role":"assistant","content":"hello back"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":2,"total_tokens":6}}`),
		ProtocolMessages:        []byte(`{"id":"msg-1","type":"message","role":"assistant","model":"minimax-m3","content":[{"type":"text","text":"hello back"}],"stop_reason":"end_turn","usage":{"input_tokens":4,"output_tokens":2}}`),
	}

	for _, inbound := range []Protocol{ProtocolChatCompletions, ProtocolMessages, ProtocolResponses} {
		for _, upstream := range []Protocol{ProtocolChatCompletions, ProtocolMessages} {
			inbound, upstream := inbound, upstream
			t.Run(string(inbound)+"_via_"+string(upstream), func(t *testing.T) {
				meta := RequestMeta{
					InboundProtocol: inbound, UpstreamProtocol: upstream,
					RequestedModel: "alias", BillingModel: "alias", UpstreamModel: map[Protocol]string{
						ProtocolChatCompletions: "grok-4.5", ProtocolMessages: "minimax-m3",
					}[upstream],
				}
				require.NoError(t, PopulateResponseMetadata(requests[inbound], &meta))
				convertedRequest, err := TransformRequest(requests[inbound], meta)
				require.NoError(t, err)
				assertTargetRequest(t, upstream, convertedRequest, meta.UpstreamModel)

				convertedResponse, usage, requestID, err := TransformResponse(upstreamResponses[upstream], meta)
				require.NoError(t, err)
				require.Equal(t, 4, usage.InputTokens)
				require.Equal(t, 2, usage.OutputTokens)
				require.NotEmpty(t, requestID)
				assertInboundResponse(t, inbound, convertedResponse)
			})
		}
	}
}

func assertTargetRequest(t *testing.T, protocol Protocol, body []byte, model string) {
	t.Helper()
	switch protocol {
	case ProtocolChatCompletions:
		var request apicompat.ChatCompletionsRequest
		require.NoError(t, json.Unmarshal(body, &request))
		require.Equal(t, model, request.Model)
		require.NotEmpty(t, request.Messages)
	case ProtocolMessages:
		var request apicompat.AnthropicRequest
		require.NoError(t, json.Unmarshal(body, &request))
		require.Equal(t, model, request.Model)
		require.NotEmpty(t, request.Messages)
	}
}

func assertInboundResponse(t *testing.T, protocol Protocol, body []byte) {
	t.Helper()
	switch protocol {
	case ProtocolChatCompletions:
		var response apicompat.ChatCompletionsResponse
		require.NoError(t, json.Unmarshal(body, &response))
		require.NotEmpty(t, response.Choices)
	case ProtocolMessages:
		var response apicompat.AnthropicResponse
		require.NoError(t, json.Unmarshal(body, &response))
		require.NotEmpty(t, response.Content)
	case ProtocolResponses:
		var response apicompat.ResponsesResponse
		require.NoError(t, json.Unmarshal(body, &response))
		require.NotEmpty(t, response.Output)
	}
}

func TestStreamConversions(t *testing.T) {
	t.Parallel()

	t.Run("chat_to_messages", func(t *testing.T) {
		transformer := NewStreamTransformer(RequestMeta{InboundProtocol: ProtocolMessages, UpstreamProtocol: ProtocolChatCompletions, BillingModel: "alias"})
		frames, err := transformer.Push("", []byte(`{"id":"chat-1","model":"grok-4.5","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}]}`))
		require.NoError(t, err)
		require.True(t, containsSemanticFrame(frames))
		final, err := transformer.Finalize()
		require.NoError(t, err)
		require.True(t, containsEvent(final, "message_stop"))
	})

	t.Run("messages_to_chat", func(t *testing.T) {
		transformer := NewStreamTransformer(RequestMeta{InboundProtocol: ProtocolChatCompletions, UpstreamProtocol: ProtocolMessages, BillingModel: "alias"})
		_, err := transformer.Push("message_start", []byte(`{"type":"message_start","message":{"id":"msg-1","type":"message","role":"assistant","model":"minimax-m3","content":[],"stop_reason":"","usage":{"input_tokens":3,"output_tokens":0}}}`))
		require.NoError(t, err)
		_, err = transformer.Push("content_block_start", []byte(`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`))
		require.NoError(t, err)
		frames, err := transformer.Push("content_block_delta", []byte(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`))
		require.NoError(t, err)
		require.True(t, containsSemanticFrame(frames))
		final, err := transformer.Finalize()
		require.NoError(t, err)
		require.Equal(t, "[DONE]", string(final[len(final)-1].Data))
	})

	t.Run("chat_to_responses", func(t *testing.T) {
		transformer := NewStreamTransformer(RequestMeta{InboundProtocol: ProtocolResponses, UpstreamProtocol: ProtocolChatCompletions, BillingModel: "alias"})
		frames, err := transformer.Push("", []byte(`{"id":"chat-1","model":"grok-4.5","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}]}`))
		require.NoError(t, err)
		require.True(t, containsEvent(frames, "response.output_text.delta"))
	})
}

func containsSemanticFrame(frames []StreamFrame) bool {
	for _, frame := range frames {
		if frame.Semantic {
			return true
		}
	}
	return false
}

func containsEvent(frames []StreamFrame, event string) bool {
	for _, frame := range frames {
		if frame.Event == event {
			return true
		}
	}
	return false
}
