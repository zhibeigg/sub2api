package dto

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestAccountFromServiceShallow_RedactsSensitiveCredentials(t *testing.T) {
	src := &service.Account{
		ID:       42,
		Name:     "demo",
		Platform: "anthropic",
		Type:     "oauth",
		Credentials: map[string]any{
			"access_token":            "at-secret",
			"refresh_token":           "rt-secret",
			"id_token":                "id-secret",
			"api_key":                 "sk-secret",
			"quota_cookie":            "quota-cookie-secret",
			"dashboard_access_token":  "dashboard-at-secret",
			"dashboard_refresh_token": "dashboard-rt-secret",
			"base_url":                "https://api.example.com",
			"model_mapping":           map[string]any{"foo": "bar"},
		},
	}

	got := AccountFromServiceShallow(src)
	require.NotNil(t, got)

	// 敏感键不在 Credentials 里
	require.NotContains(t, got.Credentials, "access_token")
	require.NotContains(t, got.Credentials, "refresh_token")
	require.NotContains(t, got.Credentials, "id_token")
	require.NotContains(t, got.Credentials, "api_key")
	require.NotContains(t, got.Credentials, "quota_cookie")
	require.NotContains(t, got.Credentials, "dashboard_access_token")
	require.NotContains(t, got.Credentials, "dashboard_refresh_token")
	// 非敏感键保留
	require.Equal(t, "https://api.example.com", got.Credentials["base_url"])
	require.Equal(t, map[string]any{"foo": "bar"}, got.Credentials["model_mapping"])

	// 状态 map 标记敏感键存在
	require.True(t, got.CredentialsStatus["has_access_token"])
	require.True(t, got.CredentialsStatus["has_refresh_token"])
	require.True(t, got.CredentialsStatus["has_id_token"])
	require.True(t, got.CredentialsStatus["has_api_key"])
	require.True(t, got.CredentialsStatus["has_quota_cookie"])
	require.True(t, got.CredentialsStatus["has_dashboard_access_token"])
	require.True(t, got.CredentialsStatus["has_dashboard_refresh_token"])

	// JSON 序列化校验：响应体里不会出现敏感子串
	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "rt-secret")
	require.NotContains(t, string(raw), "at-secret")
	require.NotContains(t, string(raw), "sk-secret")
	require.NotContains(t, string(raw), "quota-cookie-secret")
	require.NotContains(t, string(raw), "id-secret")
	require.NotContains(t, string(raw), "dashboard-at-secret")
	require.NotContains(t, string(raw), "dashboard-rt-secret")
	// 状态标识应序列化进 JSON
	require.Contains(t, string(raw), "credentials_status")
	require.Contains(t, string(raw), "has_refresh_token")

	// 原始 service.Account 不应被改动
	require.Equal(t, "rt-secret", src.Credentials["refresh_token"])
}

func TestAccountFromServiceShallow_RedactsAdobeCredentials(t *testing.T) {
	src := &service.Account{
		ID:       99,
		Name:     "adobe",
		Platform: service.PlatformAdobe,
		Type:     service.AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "access-secret",
			"cookie":       "cookie-secret",
			"device_token": "device-token-secret",
			"device_id":    "device-id-secret",
			"password":     "password-secret",
			"expires_at":   "2030-01-01T00:00:00Z",
		},
	}
	got := AccountFromServiceShallow(src)
	for _, key := range []string{"access_token", "cookie", "device_token", "device_id", "password"} {
		require.NotContains(t, got.Credentials, key)
		require.True(t, got.CredentialsStatus["has_"+key])
	}
	require.Equal(t, "2030-01-01T00:00:00Z", got.Credentials["expires_at"])
	raw, err := json.Marshal(got)
	require.NoError(t, err)
	for _, secret := range []string{"access-secret", "cookie-secret", "device-token-secret", "device-id-secret", "password-secret"} {
		require.NotContains(t, string(raw), secret)
	}
}

func TestAccountFromServiceShallow_RedactsOllamaCloudManagedExtra(t *testing.T) {
	snapshot := map[string]any{
		"status":          service.OllamaCloudUsageStatusOK,
		"last_attempt_at": "2026-07-22T12:00:00Z",
		"next_refresh_at": "2026-07-22T13:00:00Z",
		"data":            map[string]any{"plan": "Pro"},
	}
	src := &service.Account{
		ID: 9, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "https://ollama.com", "api_key": "secret-key"},
		Extra: map[string]any{
			service.OllamaCloudUsageSessionExtraKey:     "ciphertext-secret",
			service.OllamaCloudUsageAutoRefreshExtraKey: true,
			service.OllamaCloudUsageSnapshotExtraKey:    snapshot,
			"ordinary":                                  "kept",
		},
	}

	got := AccountFromServiceShallow(src)
	require.NotContains(t, got.Extra, service.OllamaCloudUsageSessionExtraKey)
	require.NotContains(t, got.Extra, service.OllamaCloudUsageAutoRefreshExtraKey)
	require.NotContains(t, got.Extra, service.OllamaCloudUsageSnapshotExtraKey)
	require.Equal(t, "kept", got.Extra["ordinary"])
	require.NotNil(t, got.OllamaCloudUsage)
	require.True(t, got.OllamaCloudUsage.Configured)
	require.True(t, got.OllamaCloudUsage.AutoRefreshEnabled)
	require.Equal(t, "Pro", got.OllamaCloudUsage.Snapshot.Data.Plan)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "ciphertext-secret")
	require.NotContains(t, string(raw), "secret-key")
	require.Contains(t, src.Extra, service.OllamaCloudUsageSessionExtraKey)
}

func TestAccountFromServiceShallow_NilCredentialsOmitsStatus(t *testing.T) {
	src := &service.Account{ID: 1, Name: "n", Platform: "anthropic", Type: "oauth"}
	got := AccountFromServiceShallow(src)
	require.NotNil(t, got)
	require.Nil(t, got.Credentials)
	require.Nil(t, got.CredentialsStatus)
}
