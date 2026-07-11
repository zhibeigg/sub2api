package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadDefaultCursorConfig(t *testing.T) {
	resetViperWithJWTSecret(t)
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "https://api.cursor.com", cfg.Cursor.BaseURL)
	require.Equal(t, 120, cfg.Cursor.RequestTimeoutSeconds)
	require.Equal(t, 60, cfg.Cursor.StreamIdleTimeoutSeconds)
	require.Equal(t, "auto", cfg.Cursor.DefaultModel)
	require.Equal(t, 24000, cfg.Cursor.MaxHistoryTokens)
	require.Equal(t, 100, cfg.Cursor.MaxHistoryMessages)
	require.Equal(t, 86400, cfg.Cursor.ResponsesTTLSeconds)
	require.Contains(t, cfg.Security.URLAllowlist.UpstreamHosts, "api.cursor.com")
	require.NotContains(t, cfg.Security.URLAllowlist.UpstreamHosts, "*.cursor.com")
}
