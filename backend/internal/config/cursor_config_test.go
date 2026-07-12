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
	require.Equal(t, "https://api2.cursor.sh", cfg.Cursor.ChatBaseURL)
	require.Equal(t, "auto", cfg.Cursor.DefaultTransportMode)
	require.Equal(t, "3.11.13", cfg.Cursor.ClientVersion)
	require.False(t, cfg.Cursor.GhostMode)
	require.False(t, cfg.Cursor.NewOnboardingCompleted)
	require.Equal(t, 8*1024*1024, cfg.Cursor.MaxFrameBytes)
	require.Equal(t, 16*1024*1024, cfg.Cursor.MaxBufferedBytes)
	require.Equal(t, 60, cfg.Cursor.ResponseHeaderTimeoutSeconds)
	require.Equal(t, 60, cfg.Cursor.IDEStreamIdleTimeoutSeconds)
	require.True(t, cfg.Cursor.AgentRPCEnabled)
	require.True(t, cfg.Cursor.AgentCloudFallbackEnabled)
	require.Equal(t, 300, cfg.Cursor.AgentModelCacheTTLSeconds)
	require.Equal(t, 1800, cfg.Cursor.AgentModelStaleTTLSeconds)
	require.Equal(t, 5, cfg.Cursor.AgentModelProbeTimeoutSeconds)
	require.True(t, cfg.Cursor.AgentModelPrewarmEnabled)
	require.Equal(t, 3, cfg.Cursor.AgentModelPrewarmConcurrency)
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
