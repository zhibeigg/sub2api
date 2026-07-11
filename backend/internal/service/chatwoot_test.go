package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type chatwootSettingRepoStub struct {
	values map[string]string
}

func (s *chatwootSettingRepoStub) Get(context.Context, string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *chatwootSettingRepoStub) GetValue(context.Context, string) (string, error) {
	panic("unexpected GetValue call")
}

func (s *chatwootSettingRepoStub) Set(context.Context, string, string) error {
	panic("unexpected Set call")
}

func (s *chatwootSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}

func (s *chatwootSettingRepoStub) SetMultiple(_ context.Context, values map[string]string) error {
	if s.values == nil {
		s.values = make(map[string]string, len(values))
	}
	for key, value := range values {
		s.values[key] = value
	}
	return nil
}

func (s *chatwootSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	result := make(map[string]string, len(s.values))
	for key, value := range s.values {
		result[key] = value
	}
	return result, nil
}

func (s *chatwootSettingRepoStub) Delete(context.Context, string) error {
	panic("unexpected Delete call")
}

func TestNormalizeChatwootBaseURL(t *testing.T) {
	normalized, err := config.NormalizeChatwootBaseURL(" HTTPS://CHAT.EXAMPLE.COM/support/ ")
	require.NoError(t, err)
	require.Equal(t, "https://chat.example.com/support", normalized)

	for _, raw := range []string{
		"javascript:alert(1)",
		"https://user:pass@chat.example.com",
		"https://chat.example.com?token=secret",
		"https://chat.example.com/#fragment",
		"https://chat.example.com/../admin",
	} {
		_, normalizeErr := config.NormalizeChatwootBaseURL(raw)
		require.Error(t, normalizeErr, raw)
	}
}

func TestResolveChatwootSettingsDatabaseValuesOverrideConfig(t *testing.T) {
	svc := NewSettingService(&chatwootSettingRepoStub{}, &config.Config{
		Chatwoot: config.ChatwootConfig{
			Enabled:                  true,
			BaseURL:                  "https://app.chatwoot.com",
			WebsiteToken:             "config-token",
			IdentityValidationSecret: "config-secret",
		},
	})

	resolved := svc.resolveChatwootSettings(map[string]string{
		SettingKeyChatwootEnabled:                  "false",
		SettingKeyChatwootBaseURL:                  "",
		SettingKeyChatwootWebsiteToken:             "",
		SettingKeyChatwootIdentityValidationSecret: "",
	})

	require.False(t, resolved.Enabled)
	require.Empty(t, resolved.BaseURL)
	require.Empty(t, resolved.WebsiteToken)
	require.Empty(t, resolved.IdentityValidationSecret)
}

func TestBuildChatwootIdentityUsesStableHMAC(t *testing.T) {
	const secret = "identity-secret"
	repo := &chatwootSettingRepoStub{values: map[string]string{
		SettingKeyChatwootEnabled:                  "true",
		SettingKeyChatwootBaseURL:                  "https://chat.example.com/",
		SettingKeyChatwootWebsiteToken:             "website-token",
		SettingKeyChatwootIdentityValidationSecret: secret,
	}}
	svc := NewSettingService(repo, &config.Config{})

	identity, err := svc.BuildChatwootIdentity(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, "sub2api-user-42", identity.Identifier)

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(identity.Identifier))
	require.Equal(t, hex.EncodeToString(mac.Sum(nil)), identity.IdentifierHash)
}

func TestBuildChatwootIdentityRequiresValidationSecret(t *testing.T) {
	svc := NewSettingService(&chatwootSettingRepoStub{values: map[string]string{
		SettingKeyChatwootEnabled:      "true",
		SettingKeyChatwootBaseURL:      "https://chat.example.com",
		SettingKeyChatwootWebsiteToken: "website-token",
	}}, &config.Config{})

	_, err := svc.BuildChatwootIdentity(context.Background(), 42)
	require.Error(t, err)
	require.Contains(t, err.Error(), "identity validation secret")
}

func TestPublicChatwootSettingsAndDynamicCSP(t *testing.T) {
	repo := &chatwootSettingRepoStub{values: map[string]string{
		SettingKeyChatwootEnabled:                  "true",
		SettingKeyChatwootBaseURL:                  "https://chat.example.com/support/",
		SettingKeyChatwootWebsiteToken:             "website-token",
		SettingKeyChatwootIdentityValidationSecret: "must-not-be-public",
	}}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.ChatwootEnabled)
	require.Equal(t, "https://chat.example.com/support", settings.ChatwootBaseURL)
	require.Equal(t, "website-token", settings.ChatwootWebsiteToken)

	sources, err := svc.GetDynamicCSPSources(context.Background())
	require.NoError(t, err)
	require.Contains(t, sources["script-src"], "https://chat.example.com")
	require.Contains(t, sources["frame-src"], "https://chat.example.com")
	require.Contains(t, sources["connect-src"], "https://chat.example.com")
	require.Contains(t, sources["connect-src"], "wss://chat.example.com")
}

func TestPublicChatwootSettingsDisableIncompleteConfiguration(t *testing.T) {
	svc := NewSettingService(&chatwootSettingRepoStub{values: map[string]string{
		SettingKeyChatwootEnabled: "true",
		SettingKeyChatwootBaseURL: "https://chat.example.com",
	}}, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.False(t, settings.ChatwootEnabled)

	sources, err := svc.GetDynamicCSPSources(context.Background())
	require.NoError(t, err)
	require.Empty(t, sources["script-src"])
}

func TestBuildSystemSettingsUpdatesValidatesChatwootRequirements(t *testing.T) {
	svc := NewSettingService(&chatwootSettingRepoStub{}, &config.Config{})

	_, err := svc.buildSystemSettingsUpdates(context.Background(), &SystemSettings{
		ChatwootEnabled:      true,
		ChatwootWebsiteToken: "website-token",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "base URL is required")

	_, err = svc.buildSystemSettingsUpdates(context.Background(), &SystemSettings{
		ChatwootEnabled: true,
		ChatwootBaseURL: "https://chat.example.com",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "website token is required")

	_, err = svc.buildSystemSettingsUpdates(context.Background(), &SystemSettings{
		ChatwootBaseURL: "https://chat.example.com?invalid=true",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "query and fragment are not allowed")
}
