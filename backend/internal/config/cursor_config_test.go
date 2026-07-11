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
	require.Equal(t, "https://api2.cursor.sh", cfg.Cursor.DashboardBaseURL)
	require.Equal(t, "https://cursor.com", cfg.Cursor.DashboardAuthWebsiteURL)
	require.True(t, cfg.Cursor.DashboardMaintenanceEnabled)
	require.Equal(t, 30, cfg.Cursor.DashboardMaintenanceIntervalMins)
	require.Equal(t, 360, cfg.Cursor.DashboardProbeIntervalMins)
	require.Equal(t, 1272, cfg.Cursor.DashboardRefreshBeforeExpiryHours)
	require.Equal(t, 5, cfg.Cursor.DashboardLoginSessionTTLMins)
	require.Equal(t, 120, cfg.Cursor.RequestTimeoutSeconds)
	require.Equal(t, 60, cfg.Cursor.StreamIdleTimeoutSeconds)
	require.Equal(t, "auto", cfg.Cursor.DefaultModel)
	require.Equal(t, 24000, cfg.Cursor.MaxHistoryTokens)
	require.Equal(t, 100, cfg.Cursor.MaxHistoryMessages)
	require.Equal(t, 86400, cfg.Cursor.ResponsesTTLSeconds)
	require.Contains(t, cfg.Security.URLAllowlist.UpstreamHosts, "api.cursor.com")
	require.Contains(t, cfg.Security.URLAllowlist.UpstreamHosts, "api2.cursor.sh")
	require.NotContains(t, cfg.Security.URLAllowlist.UpstreamHosts, "*.cursor.com")
}
