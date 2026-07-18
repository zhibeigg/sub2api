package opencode

import (
	"encoding/json"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

type Usage struct {
	InputTokens              int
	OutputTokens             int
	CacheReadInputTokens     int
	CacheCreationInputTokens int
}

type RequestMeta struct {
	InboundProtocol  Protocol
	UpstreamProtocol Protocol
	RequestedModel   string
	BillingModel     string
	UpstreamModel    string
	Stream           bool
	CustomTools      map[string]bool
	NamespaceTools   map[string]apicompat.NamespacedToolName
	HasToolSearch    bool
}

func InspectRequest(raw []byte, inbound Protocol) (model string, stream bool, err error) {
	var envelope struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return "", false, fmt.Errorf("invalid %s request: %w", inbound, err)
	}
	if envelope.Model == "" {
		return "", false, fmt.Errorf("model is required")
	}
	return envelope.Model, envelope.Stream, nil
}

func TransformRequest(raw []byte, meta RequestMeta) ([]byte, error) {
	switch meta.InboundProtocol {
	case ProtocolChatCompletions:
		var req apicompat.ChatCompletionsRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, fmt.Errorf("decode chat completions request: %w", err)
		}
		req.Model = meta.UpstreamModel
		meta.Stream = req.Stream
		switch meta.UpstreamProtocol {
		case ProtocolChatCompletions:
			return rewriteRequestEnvelope(raw, meta.UpstreamModel, req.Stream)
		case ProtocolMessages:
			responsesReq, err := apicompat.ChatCompletionsToResponses(&req)
			if err != nil {
				return nil, err
			}
			responsesReq.Stream = req.Stream
			messagesReq, err := apicompat.ResponsesToAnthropicRequest(responsesReq)
			if err != nil {
				return nil, err
			}
			messagesReq.Model = meta.UpstreamModel
			messagesReq.Stream = req.Stream
			return json.Marshal(messagesReq)
		}
	case ProtocolMessages:
		var req apicompat.AnthropicRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, fmt.Errorf("decode messages request: %w", err)
		}
		req.Model = meta.UpstreamModel
		meta.Stream = req.Stream
		switch meta.UpstreamProtocol {
		case ProtocolMessages:
			return rewriteRequestEnvelope(raw, meta.UpstreamModel, req.Stream)
		case ProtocolChatCompletions:
			chatReq, err := apicompat.AnthropicToChatCompletionsRequest(&req)
			if err != nil {
				return nil, err
			}
			chatReq.Model = meta.UpstreamModel
			chatReq.Stream = req.Stream
			return json.Marshal(chatReq)
		}
	case ProtocolResponses:
		var req apicompat.ResponsesRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, fmt.Errorf("decode responses request: %w", err)
		}
		req.Model = meta.UpstreamModel
		meta.Stream = req.Stream
		switch meta.UpstreamProtocol {
		case ProtocolChatCompletions:
			chatReq, err := apicompat.ResponsesToChatCompletionsRequest(&req)
			if err != nil {
				return nil, err
			}
			chatReq.Model = meta.UpstreamModel
			chatReq.Stream = req.Stream
			return json.Marshal(chatReq)
		case ProtocolMessages:
			messagesReq, err := apicompat.ResponsesToAnthropicRequest(&req)
			if err != nil {
				return nil, err
			}
			messagesReq.Model = meta.UpstreamModel
			messagesReq.Stream = req.Stream
			return json.Marshal(messagesReq)
		}
	}
	return nil, fmt.Errorf("unsupported OpenCode request conversion %s -> %s", meta.InboundProtocol, meta.UpstreamProtocol)
}

func PopulateResponseMetadata(raw []byte, meta *RequestMeta) error {
	if meta == nil || meta.InboundProtocol != ProtocolResponses {
		return nil
	}
	var req apicompat.ResponsesRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return err
	}
	tools, err := apicompat.EffectiveResponsesTools(&req)
	if err != nil {
		return err
	}
	meta.CustomTools = apicompat.CustomToolNames(tools)
	meta.NamespaceTools = apicompat.NamespaceToolNames(tools)
	meta.HasToolSearch = apicompat.HasToolSearchTool(tools)
	return nil
}

func TransformResponse(raw []byte, meta RequestMeta) ([]byte, Usage, string, error) {
	switch meta.UpstreamProtocol {
	case ProtocolChatCompletions:
		var upstream apicompat.ChatCompletionsResponse
		if err := json.Unmarshal(raw, &upstream); err != nil {
			return nil, Usage{}, "", fmt.Errorf("decode chat completions response: %w", err)
		}
		requestID := upstream.ID
		usage := usageFromChat(upstream.Usage)
		switch meta.InboundProtocol {
		case ProtocolChatCompletions:
			return raw, usage, requestID, nil
		case ProtocolMessages:
			converted := apicompat.ChatCompletionsResponseToAnthropic(&upstream, meta.BillingModel)
			body, err := json.Marshal(converted)
			return body, usageFromAnthropic(converted.Usage), converted.ID, err
		case ProtocolResponses:
			converted := apicompat.ChatCompletionsResponseToResponses(&upstream, meta.BillingModel, meta.CustomTools, meta.HasToolSearch, meta.NamespaceTools)
			body, err := json.Marshal(converted)
			return body, usageFromResponses(converted.Usage), converted.ID, err
		}
	case ProtocolMessages:
		var upstream apicompat.AnthropicResponse
		if err := json.Unmarshal(raw, &upstream); err != nil {
			return nil, Usage{}, "", fmt.Errorf("decode messages response: %w", err)
		}
		requestID := upstream.ID
		usage := usageFromAnthropic(upstream.Usage)
		switch meta.InboundProtocol {
		case ProtocolMessages:
			return raw, usage, requestID, nil
		case ProtocolResponses:
			converted := apicompat.AnthropicToResponsesResponse(&upstream)
			converted.Model = meta.BillingModel
			body, err := json.Marshal(converted)
			return body, usageFromResponses(converted.Usage), converted.ID, err
		case ProtocolChatCompletions:
			responsesResp := apicompat.AnthropicToResponsesResponse(&upstream)
			converted := apicompat.ResponsesToChatCompletions(responsesResp, meta.BillingModel)
			body, err := json.Marshal(converted)
			return body, usageFromChat(converted.Usage), converted.ID, err
		}
	}
	return nil, Usage{}, "", fmt.Errorf("unsupported OpenCode response conversion %s -> %s", meta.UpstreamProtocol, meta.InboundProtocol)
}

func rewriteRequestEnvelope(raw []byte, model string, stream bool) ([]byte, error) {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	modelJSON, _ := json.Marshal(model)
	streamJSON, _ := json.Marshal(stream)
	body["model"] = modelJSON
	body["stream"] = streamJSON
	return json.Marshal(body)
}

func usageFromChat(usage *apicompat.ChatUsage) Usage {
	if usage == nil {
		return Usage{}
	}
	out := Usage{InputTokens: usage.PromptTokens, OutputTokens: usage.CompletionTokens}
	if usage.PromptTokensDetails != nil {
		out.CacheReadInputTokens = usage.PromptTokensDetails.CachedTokens
		out.CacheCreationInputTokens = usage.PromptTokensDetails.CacheWriteTokens
		if out.CacheCreationInputTokens == 0 {
			out.CacheCreationInputTokens = usage.PromptTokensDetails.CacheCreationTokens
		}
	}
	return out
}

func usageFromAnthropic(usage apicompat.AnthropicUsage) Usage {
	return Usage{
		InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens,
		CacheReadInputTokens:     usage.CacheReadInputTokens,
		CacheCreationInputTokens: usage.CacheCreationInputTokens,
	}
}

func usageFromResponses(usage *apicompat.ResponsesUsage) Usage {
	if usage == nil {
		return Usage{}
	}
	out := Usage{InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens, CacheCreationInputTokens: usage.CacheCreationInputTokens}
	if usage.InputTokensDetails != nil {
		out.CacheReadInputTokens = usage.InputTokensDetails.CachedTokens
		if out.CacheCreationInputTokens == 0 {
			out.CacheCreationInputTokens = usage.InputTokensDetails.CacheWriteTokens
			if out.CacheCreationInputTokens == 0 {
				out.CacheCreationInputTokens = usage.InputTokensDetails.CacheCreationTokens
			}
		}
	}
	return out
}
