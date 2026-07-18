package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenCodePlatformCapabilities(t *testing.T) {
	t.Parallel()

	capabilities, ok := GetPlatformCapabilities(PlatformOpenCode)
	require.True(t, ok)
	require.Equal(t, PlatformOpenCodeDisplayName, capabilities.DisplayName)
	require.NoError(t, ValidatePlatformAccountType(PlatformOpenCode, AccountTypeAPIKey))
	require.Error(t, ValidatePlatformAccountType(PlatformOpenCode, AccountTypeOAuth))
	require.True(t, PlatformSupportsUpstreamModelSync(PlatformOpenCode))
	require.True(t, PlatformSupportsMixedScheduling(PlatformOpenCode))
	require.Equal(t, 1, DefaultAccountConcurrency(PlatformOpenCode))
	require.Contains(t, AllowedQuotaPlatforms, PlatformOpenCode)
}

func TestOpenCodeMixedSchedulingFoundation(t *testing.T) {
	t.Parallel()

	for _, platform := range []string{PlatformAnthropic, PlatformOpenAI} {
		require.Contains(t, MixedSchedulingCandidatePlatforms(platform), PlatformOpenCode, platform)
	}
	for _, platform := range []string{PlatformGemini, PlatformGrok} {
		require.NotContains(t, MixedSchedulingCandidatePlatforms(platform), PlatformOpenCode, platform)
	}

	account := &Account{
		Platform: PlatformOpenCode,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{"mixed_scheduling": true},
	}
	require.True(t, account.IsMixedSchedulingEnabled())
}

func TestOpenCodeAccountCredentials(t *testing.T) {
	t.Parallel()

	credentials := map[string]any{
		"api_key":            " opencode-key ",
		"base_url":           "https://relay.example.com/opencode/",
		"quota_cookie":       "session=secret",
		"quota_workspace_id": " workspace-1 ",
		"model_mapping":      map[string]any{"claude-sonnet": "upstream-sonnet"},
		"model_protocols":    map[string]any{"claude-sonnet": "messages", "gpt": "chat_completions"},
	}
	account := &Account{Platform: PlatformOpenCode, Type: AccountTypeAPIKey, Credentials: credentials}

	require.NoError(t, ValidateOpenCodeAccountCredentials(account.Type, credentials))
	require.Equal(t, "opencode-key", account.GetOpenCodeAPIKey())
	require.Equal(t, "https://relay.example.com/opencode", account.GetOpenCodeBaseURL())
	require.Equal(t, "session=secret", account.GetOpenCodeQuotaCookie())
	require.Equal(t, "workspace-1", account.GetOpenCodeQuotaWorkspaceID())
	require.Equal(t, "upstream-sonnet", account.GetModelMapping()["claude-sonnet"])
	require.Equal(t, "messages", account.GetModelProtocol("claude-sonnet"))
	require.Equal(t, "chat_completions", account.GetModelProtocols()["gpt"])
}

func TestOpenCodeAccountCredentialValidation(t *testing.T) {
	t.Parallel()

	require.NoError(t, ValidateOpenCodeAccountCredentials(AccountTypeAPIKey, map[string]any{"api_key": "key"}))
	require.Error(t, ValidateOpenCodeAccountCredentials(AccountTypeOAuth, map[string]any{"api_key": "key"}))
	require.Error(t, ValidateOpenCodeAccountCredentials(AccountTypeAPIKey, map[string]any{}))
	require.Error(t, ValidateOpenCodeAccountCredentials(AccountTypeAPIKey, map[string]any{"api_key": "key", "quota_cookie": 1}))
	require.Error(t, ValidateOpenCodeAccountCredentials(AccountTypeAPIKey, map[string]any{"api_key": "key", "model_protocols": []string{"openai"}}))
}

func TestOpenCodeDefaultBaseURL(t *testing.T) {
	t.Parallel()

	account := &Account{Platform: PlatformOpenCode, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "key"}}
	require.Equal(t, DefaultOpenCodeBaseURL, account.GetBaseURL())
	require.Equal(t, DefaultOpenCodeBaseURL, account.GetOpenCodeBaseURL())
}
