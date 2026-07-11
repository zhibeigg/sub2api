package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCursorPlatformCapabilities(t *testing.T) {
	t.Parallel()
	require.NoError(t, ValidatePlatformAccountType(PlatformCursor, AccountTypeAPIKey))
	require.Error(t, ValidatePlatformAccountType(PlatformCursor, "cookie"))
	require.Error(t, ValidatePlatformAccountType(PlatformCursor, AccountTypeOAuth))
	require.False(t, PlatformSupportsImageGeneration(PlatformCursor))
	require.False(t, PlatformSupportsVideoGeneration(PlatformCursor))
	require.False(t, PlatformSupportsBatchImageGeneration(PlatformCursor))
	require.True(t, PlatformSupportsUpstreamModelSync(PlatformCursor))
	require.True(t, IsMixedSchedulingCapablePlatform(PlatformCursor))
	require.Equal(t, 1, DefaultAccountConcurrency(PlatformCursor))
	require.Contains(t, AllowedQuotaPlatforms, PlatformCursor)
}

func TestCursorMixedSchedulingTargetsSupportedModelGroupPlatforms(t *testing.T) {
	t.Parallel()
	for _, platform := range []string{PlatformAnthropic, PlatformGemini, PlatformOpenAI, PlatformGrok} {
		require.True(t, GroupPlatformSupportsMixedScheduling(platform), platform)
		require.True(t, CursorSupportsGroupPlatform(platform), platform)
		require.Contains(t, MixedSchedulingCandidatePlatforms(platform), PlatformCursor, platform)
	}
	require.True(t, CursorSupportsGroupPlatform(PlatformCursor))
	require.False(t, CursorSupportsGroupPlatform(PlatformAdobe))
	require.NotContains(t, MixedSchedulingCandidatePlatforms(PlatformAdobe), PlatformCursor)

	account := &Account{
		Platform: PlatformCursor,
		Extra:    map[string]any{"mixed_scheduling": true},
	}
	require.True(t, account.IsMixedSchedulingEnabled())
}

func TestCursorAccountDefaultMapping(t *testing.T) {
	t.Parallel()
	account := &Account{Platform: PlatformCursor, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "cursor-key"}}
	mapping := account.GetModelMapping()
	require.Equal(t, "auto", mapping["cursor-agent"])
	require.Equal(t, "auto", mapping["cursor-chat"])
	require.Equal(t, "claude-4.7-opus-fast", mapping["claude-4.7-opus-fast"])
	require.Equal(t, "composer-2.5", mapping["composer-2.5"])
	require.Equal(t, "gpt-5.6-terra", mapping["gpt-5.6-terra"])
	require.Equal(t, "kimi-k2.7-code", mapping["kimi-k2.7-code"])
	require.Len(t, mapping, len(CursorModelCatalog)+2)
}

func TestValidateCursorAccountCredentials(t *testing.T) {
	t.Parallel()
	require.NoError(t, ValidateCursorAccountCredentials(AccountTypeAPIKey, map[string]any{
		"api_key": "cursor-key", "dashboard_access_token": "access", "dashboard_refresh_token": "refresh",
	}))
	require.NoError(t, ValidateCursorAccountCredentials(AccountTypeAPIKey, map[string]any{
		"cursor_transport_mode": CursorTransportIDEChat, "dashboard_access_token": "access", "dashboard_refresh_token": "refresh",
	}))
	require.NoError(t, ValidateCursorAccountCredentials(AccountTypeAPIKey, map[string]any{
		"cursor_transport_mode": CursorTransportCloudAgent, "api_key": "cursor-key",
	}))
	require.Error(t, ValidateCursorAccountCredentials(AccountTypeOAuth, map[string]any{"api_key": "cursor-key"}))
	require.Error(t, ValidateCursorAccountCredentials("cookie", map[string]any{"cookie": "_vcrcs=legacy"}))
	require.Error(t, ValidateCursorAccountCredentials(AccountTypeAPIKey, map[string]any{"api_key": ""}))
	require.Error(t, ValidateCursorAccountCredentials(AccountTypeAPIKey, map[string]any{"cursor_transport_mode": CursorTransportIDEChat, "api_key": "cursor-key"}))
	require.Error(t, ValidateCursorAccountCredentials(AccountTypeAPIKey, map[string]any{"cursor_transport_mode": CursorTransportCloudAgent, "dashboard_access_token": "access"}))
	require.Error(t, ValidateCursorAccountCredentials(AccountTypeAPIKey, map[string]any{"cursor_transport_mode": "invalid", "api_key": "cursor-key"}))
	require.Error(t, ValidateCursorAccountCredentials(AccountTypeAPIKey, map[string]any{"api_key": "cursor-key", "cookie": "legacy"}))
	require.Error(t, ValidateCursorAccountCredentials(AccountTypeAPIKey, map[string]any{"api_key": "cursor-key", "dashboard_access_token": "bad\nvalue"}))
}

func TestCursorAccountTransportMode(t *testing.T) {
	t.Parallel()
	cloud := &Account{Credentials: map[string]any{"api_key": "key"}}
	require.Equal(t, CursorTransportAuto, CursorAccountTransportMode(cloud))
	require.False(t, CursorAccountUsesIDEChat(cloud))

	ide := &Account{Credentials: map[string]any{"dashboard_access_token": "token"}}
	require.True(t, CursorAccountUsesIDEChat(ide))

	explicitCloud := &Account{Credentials: map[string]any{"dashboard_access_token": "token", "cursor_transport_mode": CursorTransportCloudAgent}}
	require.False(t, CursorAccountUsesIDEChat(explicitCloud))
}
