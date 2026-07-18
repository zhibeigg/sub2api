package opencode

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEndpointNormalization(t *testing.T) {
	t.Parallel()

	endpoint, err := Endpoint("https://opencode.ai/zen/go/", ProtocolChatCompletions)
	require.NoError(t, err)
	require.Equal(t, "https://opencode.ai/zen/go/v1/chat/completions", endpoint)

	endpoint, err = Endpoint("https://relay.example.com/v1/", ProtocolMessages)
	require.NoError(t, err)
	require.Equal(t, "https://relay.example.com/v1/messages", endpoint)

	models, err := ModelsEndpoint("")
	require.NoError(t, err)
	require.Equal(t, DefaultBaseURL+"/v1/models", models)
}

func TestResolveModelPrefixMappingAndOverrides(t *testing.T) {
	t.Parallel()

	resolved, err := ResolveModel("opencode-go/grok-4.5", "", nil)
	require.NoError(t, err)
	require.Equal(t, "grok-4.5", resolved.BillingModel)
	require.Equal(t, ProtocolChatCompletions, resolved.Protocol)

	resolved, err = ResolveModel("opencode-go/alias", "minimax-m3", map[string]string{"minimax-m3": "openai"})
	require.NoError(t, err)
	require.Equal(t, "minimax-m3", resolved.UpstreamModel)
	require.Equal(t, ProtocolChatCompletions, resolved.Protocol)

	resolved, err = ResolveModel("custom", "custom", map[string]string{"custom": "anthropic"})
	require.NoError(t, err)
	require.Equal(t, ProtocolMessages, resolved.Protocol)

	_, err = ResolveModel("custom", "custom", map[string]string{"custom": "unknown"})
	require.ErrorContains(t, err, "unsupported OpenCode upstream protocol")

	_, err = ResolveModel("unknown", "unknown", nil)
	require.ErrorContains(t, err, "has no configured protocol")

	protocol, ok := ProtocolForModel("opencode-go/custom", map[string]string{"opencode-go/custom": "anthropic"})
	require.True(t, ok)
	require.Equal(t, ProtocolMessages, protocol)
}

func TestDefaultModelIDs(t *testing.T) {
	t.Parallel()

	require.Equal(t, []string{
		"grok-4.5", "glm-5.2", "glm-5.1", "kimi-k3", "kimi-k2.7-code", "kimi-k2.6",
		"deepseek-v4-pro", "deepseek-v4-flash", "mimo-v2.5", "mimo-v2.5-pro",
		"minimax-m3", "minimax-m2.7", "minimax-m2.5", "qwen3.7-max", "qwen3.7-plus", "qwen3.6-plus",
	}, DefaultModelIDs())
}
