package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadDefaultOpenCodeConfig(t *testing.T) {
	resetViperWithJWTSecret(t)

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "https://opencode.ai/zen/go", cfg.OpenCode.BaseURL)
	require.Equal(t, 600, cfg.OpenCode.InferenceTimeoutSeconds)
	require.Equal(t, 300, cfg.OpenCode.QuotaCacheTTLSeconds)
	require.Equal(t, 1800, cfg.OpenCode.QuotaStaleTTLSeconds)
	require.Equal(t, 15, cfg.OpenCode.QuotaRequestTimeoutSeconds)
	require.Contains(t, cfg.Security.URLAllowlist.UpstreamHosts, "opencode.ai")
}

func TestLoadOpenCodeConfigFromEnv(t *testing.T) {
	resetViperWithJWTSecret(t)
	t.Setenv("OPENCODE_BASE_URL", "https://relay.example.com/opencode/")
	t.Setenv("OPENCODE_INFERENCE_TIMEOUT_SECONDS", "900")
	t.Setenv("OPENCODE_QUOTA_CACHE_TTL_SECONDS", "60")
	t.Setenv("OPENCODE_QUOTA_STALE_TTL_SECONDS", "600")
	t.Setenv("OPENCODE_QUOTA_REQUEST_TIMEOUT_SECONDS", "20")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "https://relay.example.com/opencode", cfg.OpenCode.BaseURL)
	require.Equal(t, 900, cfg.OpenCode.InferenceTimeoutSeconds)
	require.Equal(t, 60, cfg.OpenCode.QuotaCacheTTLSeconds)
	require.Equal(t, 600, cfg.OpenCode.QuotaStaleTTLSeconds)
	require.Equal(t, 20, cfg.OpenCode.QuotaRequestTimeoutSeconds)
}

func TestValidateOpenCodeConfig(t *testing.T) {
	resetViperWithJWTSecret(t)
	cfg, err := Load()
	require.NoError(t, err)

	cfg.OpenCode.QuotaStaleTTLSeconds = cfg.OpenCode.QuotaCacheTTLSeconds - 1
	require.ErrorContains(t, cfg.Validate(), "opencode.quota_stale_ttl_seconds")

	cfg.OpenCode.QuotaStaleTTLSeconds = cfg.OpenCode.QuotaCacheTTLSeconds
	cfg.OpenCode.BaseURL = "not-a-url"
	require.ErrorContains(t, cfg.Validate(), "opencode.base_url")
}
