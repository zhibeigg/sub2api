package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadDefaultCursorConfig(t *testing.T) {
	resetViperWithJWTSecret(t)
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "https://cursor.com", cfg.Cursor.BaseURL)
	require.Equal(t, 120, cfg.Cursor.RequestTimeoutSeconds)
	require.Equal(t, 60, cfg.Cursor.StreamIdleTimeoutSeconds)
	require.Equal(t, "google/gemini-3-flash", cfg.Cursor.DefaultModel)
	require.Equal(t, "https://cursor.com/docs", cfg.Cursor.Referer)
	require.Equal(t, 24000, cfg.Cursor.MaxHistoryTokens)
	require.Equal(t, 100, cfg.Cursor.MaxHistoryMessages)
	require.Equal(t, 0, cfg.Cursor.MaxAutoContinue)
	require.Equal(t, 86400, cfg.Cursor.ResponsesTTLSeconds)
	require.Equal(t, 1024, cfg.Cursor.ToolDescriptionMaxLength)
	require.Contains(t, cfg.Security.URLAllowlist.UpstreamHosts, "cursor.com")
}
