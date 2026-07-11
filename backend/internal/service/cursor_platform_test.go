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

func TestCursorMixedSchedulingTargetsAnthropicMessagesOnly(t *testing.T) {
	t.Parallel()
	require.Contains(t, MixedSchedulingCandidatePlatforms(PlatformAnthropic), PlatformCursor)
	require.NotContains(t, MixedSchedulingCandidatePlatforms(PlatformGemini), PlatformCursor)
	require.NotContains(t, MixedSchedulingCandidatePlatforms(PlatformOpenAI), PlatformCursor)

	account := &Account{
		Platform: PlatformCursor,
		Extra:    map[string]any{"mixed_scheduling": true},
	}
	require.True(t, account.IsMixedSchedulingEnabled())
}

func TestCursorAccountDefaultMapping(t *testing.T) {
	t.Parallel()
	account := &Account{Platform: PlatformCursor, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "cursor-key"}}
	require.Equal(t, "auto", account.GetModelMapping()["cursor-agent"])
	require.Equal(t, "auto", account.GetModelMapping()["cursor-chat"])
}

func TestValidateCursorAccountCredentials(t *testing.T) {
	t.Parallel()
	require.NoError(t, ValidateCursorAccountCredentials(AccountTypeAPIKey, map[string]any{"api_key": "cursor-key"}))
	require.Error(t, ValidateCursorAccountCredentials(AccountTypeOAuth, map[string]any{"api_key": "cursor-key"}))
	require.Error(t, ValidateCursorAccountCredentials("cookie", map[string]any{"cookie": "_vcrcs=legacy"}))
	require.Error(t, ValidateCursorAccountCredentials(AccountTypeAPIKey, map[string]any{"api_key": ""}))
	require.Error(t, ValidateCursorAccountCredentials(AccountTypeAPIKey, map[string]any{"api_key": "cursor-key", "cookie": "legacy"}))
}
