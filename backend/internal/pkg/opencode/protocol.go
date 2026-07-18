package opencode

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
)

const (
	DefaultBaseURL = "https://opencode.ai/zen/go"
	ModelPrefix    = "opencode-go/"
)

type Protocol string

const (
	ProtocolChatCompletions Protocol = "chat_completions"
	ProtocolMessages        Protocol = "messages"
	ProtocolResponses       Protocol = "responses"
)

var ChatCompletionsModels = []string{
	"grok-4.5", "glm-5.2", "glm-5.1", "kimi-k3", "kimi-k2.7-code",
	"kimi-k2.6", "deepseek-v4-pro", "deepseek-v4-flash", "mimo-v2.5", "mimo-v2.5-pro",
}

var MessagesModels = []string{
	"minimax-m3", "minimax-m2.7", "minimax-m2.5", "qwen3.7-max", "qwen3.7-plus", "qwen3.6-plus",
}

type ModelResolution struct {
	RequestedModel string
	BillingModel   string
	UpstreamModel  string
	Protocol       Protocol
}

func ParseProtocol(value string) (Protocol, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(ProtocolChatCompletions), "openai", "chat", "chat-completions":
		return ProtocolChatCompletions, nil
	case string(ProtocolMessages), "anthropic", "claude":
		return ProtocolMessages, nil
	default:
		return "", fmt.Errorf("unsupported OpenCode upstream protocol %q", value)
	}
}

func NormalizeModelID(model string) string {
	model = strings.TrimSpace(model)
	if strings.HasPrefix(strings.ToLower(model), ModelPrefix) {
		return strings.TrimSpace(model[len(ModelPrefix):])
	}
	return model
}

// StripModelPrefix is retained as a descriptive alias for NormalizeModelID.
func StripModelPrefix(model string) string { return NormalizeModelID(model) }

func DefaultModelIDs() []string {
	models := make([]string, 0, len(ChatCompletionsModels)+len(MessagesModels))
	models = append(models, ChatCompletionsModels...)
	models = append(models, MessagesModels...)
	return models
}

func ProtocolForModel(model string, overrides map[string]string) (Protocol, bool) {
	rawModel := strings.TrimSpace(model)
	model = NormalizeModelID(rawModel)
	for key, value := range overrides {
		if strings.TrimSpace(key) != rawModel && NormalizeModelID(key) != model {
			continue
		}
		protocol, err := ParseProtocol(value)
		return protocol, err == nil
	}
	switch {
	case slices.Contains(ChatCompletionsModels, model):
		return ProtocolChatCompletions, true
	case slices.Contains(MessagesModels, model):
		return ProtocolMessages, true
	default:
		return "", false
	}
}

func ResolveModel(requestedModel, mappedModel string, rawOverrides any) (ModelResolution, error) {
	billingModel := StripModelPrefix(requestedModel)
	if billingModel == "" {
		return ModelResolution{}, fmt.Errorf("model is required")
	}
	upstreamModel := strings.TrimSpace(mappedModel)
	if upstreamModel == "" {
		upstreamModel = billingModel
	}
	upstreamModel = StripModelPrefix(upstreamModel)

	overrides, err := ParseModelProtocolOverrides(rawOverrides)
	if err != nil {
		return ModelResolution{}, err
	}
	for _, candidate := range []string{upstreamModel, billingModel, strings.TrimSpace(requestedModel)} {
		if value, ok := overrides[candidate]; ok {
			protocol, parseErr := ParseProtocol(value)
			if parseErr != nil {
				return ModelResolution{}, parseErr
			}
			return ModelResolution{RequestedModel: requestedModel, BillingModel: billingModel, UpstreamModel: upstreamModel, Protocol: protocol}, nil
		}
	}

	var protocol Protocol
	switch {
	case slices.Contains(ChatCompletionsModels, upstreamModel):
		protocol = ProtocolChatCompletions
	case slices.Contains(MessagesModels, upstreamModel):
		protocol = ProtocolMessages
	default:
		return ModelResolution{}, fmt.Errorf("unknown OpenCode model %q has no configured protocol", upstreamModel)
	}
	return ModelResolution{RequestedModel: requestedModel, BillingModel: billingModel, UpstreamModel: upstreamModel, Protocol: protocol}, nil
}

func ParseModelProtocolOverrides(raw any) (map[string]string, error) {
	out := make(map[string]string)
	if raw == nil {
		return out, nil
	}
	add := func(model, protocol string) error {
		model = StripModelPrefix(model)
		if model == "" {
			return nil
		}
		parsed, err := ParseProtocol(protocol)
		if err != nil {
			return err
		}
		out[model] = string(parsed)
		return nil
	}

	switch values := raw.(type) {
	case map[string]string:
		for key, value := range values {
			if err := add(key, value); err != nil {
				return nil, err
			}
		}
		return out, nil
	case map[string]any:
		for key, value := range values {
			switch typed := value.(type) {
			case string:
				if err := add(key, typed); err != nil {
					return nil, err
				}
			case []string:
				protocol, err := ParseProtocol(key)
				if err != nil {
					return nil, err
				}
				for _, model := range typed {
					if err := add(model, string(protocol)); err != nil {
						return nil, err
					}
				}
			case []any:
				protocol, err := ParseProtocol(key)
				if err != nil {
					return nil, err
				}
				for _, item := range typed {
					model, ok := item.(string)
					if !ok {
						return nil, fmt.Errorf("model_protocols.%s must contain only strings", key)
					}
					if err := add(model, string(protocol)); err != nil {
						return nil, err
					}
				}
			default:
				return nil, fmt.Errorf("model_protocols.%s has unsupported value type %T", key, value)
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("model_protocols must be an object, got %T", raw)
	}
}

func NormalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		raw = DefaultBaseURL
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid OpenCode base URL %q", raw)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("OpenCode base URL must use http or https")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("OpenCode base URL must not contain credentials, query, or fragment")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func Endpoint(baseURL string, protocol Protocol) (string, error) {
	baseURL, err := NormalizeBaseURL(baseURL)
	if err != nil {
		return "", err
	}
	var endpoint string
	switch protocol {
	case ProtocolChatCompletions:
		endpoint = "/v1/chat/completions"
	case ProtocolMessages:
		endpoint = "/v1/messages"
	case ProtocolResponses:
		endpoint = "/v1/responses"
	default:
		return "", fmt.Errorf("unsupported endpoint protocol %q", protocol)
	}
	if strings.HasSuffix(baseURL, "/v1") {
		endpoint = strings.TrimPrefix(endpoint, "/v1")
	}
	return baseURL + endpoint, nil
}

func ModelsEndpoint(baseURL string) (string, error) {
	baseURL, err := NormalizeBaseURL(baseURL)
	if err != nil {
		return "", err
	}
	if strings.HasSuffix(baseURL, "/v1") {
		return baseURL + "/models", nil
	}
	return baseURL + "/v1/models", nil
}
