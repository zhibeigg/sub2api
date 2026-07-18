package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type ChatwootSettings struct {
	Enabled                  bool
	BaseURL                  string
	WebsiteToken             string
	IdentityValidationSecret string
}

type ChatwootIdentity struct {
	Identifier     string `json:"identifier"`
	IdentifierHash string `json:"identifier_hash"`
}

func (s *SettingService) chatwootConfigFallback() ChatwootSettings {
	if s == nil || s.cfg == nil {
		return ChatwootSettings{}
	}
	baseURL, _ := config.NormalizeChatwootBaseURL(s.cfg.Chatwoot.BaseURL)
	return ChatwootSettings{
		Enabled:                  s.cfg.Chatwoot.Enabled,
		BaseURL:                  baseURL,
		WebsiteToken:             strings.TrimSpace(s.cfg.Chatwoot.WebsiteToken),
		IdentityValidationSecret: strings.TrimSpace(s.cfg.Chatwoot.IdentityValidationSecret),
	}
}

func (s *SettingService) resolveChatwootSettings(values map[string]string) ChatwootSettings {
	result := s.chatwootConfigFallback()
	if raw, ok := values[SettingKeyChatwootEnabled]; ok {
		result.Enabled = raw == "true"
	}
	if raw, ok := values[SettingKeyChatwootBaseURL]; ok {
		result.BaseURL, _ = config.NormalizeChatwootBaseURL(raw)
	}
	if raw, ok := values[SettingKeyChatwootWebsiteToken]; ok {
		result.WebsiteToken = strings.TrimSpace(raw)
	}
	if raw, ok := values[SettingKeyChatwootIdentityValidationSecret]; ok {
		result.IdentityValidationSecret = strings.TrimSpace(raw)
	}
	return result
}

func (s *SettingService) GetChatwootSettings(ctx context.Context) (ChatwootSettings, error) {
	if cached, _ := s.chatwootSettingsCache.Load().(*ChatwootSettings); cached != nil {
		return *cached, nil
	}
	values, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingKeyChatwootEnabled,
		SettingKeyChatwootBaseURL,
		SettingKeyChatwootWebsiteToken,
		SettingKeyChatwootIdentityValidationSecret,
	})
	if err != nil {
		return ChatwootSettings{}, fmt.Errorf("get chatwoot settings: %w", err)
	}
	result := s.resolveChatwootSettings(values)
	s.chatwootSettingsCache.Store(&result)
	return result, nil
}

func (s *SettingService) BuildChatwootIdentity(ctx context.Context, userID int64) (*ChatwootIdentity, error) {
	settings, err := s.GetChatwootSettings(ctx)
	if err != nil {
		return nil, err
	}
	if !settings.Enabled || settings.BaseURL == "" || settings.WebsiteToken == "" {
		return nil, infraerrors.ServiceUnavailable("CHATWOOT_DISABLED", "Chatwoot is not enabled or fully configured")
	}
	if settings.IdentityValidationSecret == "" {
		return nil, infraerrors.ServiceUnavailable("CHATWOOT_IDENTITY_SECRET_MISSING", "Chatwoot identity validation secret is not configured")
	}
	identifier := "sub2api-user-" + strconv.FormatInt(userID, 10)
	mac := hmac.New(sha256.New, []byte(settings.IdentityValidationSecret))
	_, _ = mac.Write([]byte(identifier))
	return &ChatwootIdentity{Identifier: identifier, IdentifierHash: hex.EncodeToString(mac.Sum(nil))}, nil
}

type DynamicCSPSources map[string][]string

func (s *SettingService) GetDynamicCSPSources(ctx context.Context) (DynamicCSPSources, error) {
	settings, err := s.GetPublicSettings(ctx)
	if err != nil {
		return nil, err
	}
	sources := DynamicCSPSources{"frame-src": {}}
	seen := map[string]map[string]struct{}{}
	add := func(directive, origin string) {
		if origin == "" {
			return
		}
		if seen[directive] == nil {
			seen[directive] = map[string]struct{}{}
		}
		if _, ok := seen[directive][origin]; ok {
			return
		}
		seen[directive][origin] = struct{}{}
		sources[directive] = append(sources[directive], origin)
	}
	add("frame-src", extractOriginFromURL(settings.HomeContent))
	if settings.PurchaseSubscriptionEnabled {
		add("frame-src", extractOriginFromURL(settings.PurchaseSubscriptionURL))
	}
	for _, item := range parseCustomMenuItemURLs(settings.CustomMenuItems) {
		add("frame-src", extractOriginFromURL(item))
	}
	if settings.ChatwootEnabled {
		origin := extractOriginFromURL(settings.ChatwootBaseURL)
		add("script-src", origin)
		add("frame-src", origin)
		add("connect-src", origin)
		if u, parseErr := url.Parse(origin); parseErr == nil && u.Host != "" {
			switch u.Scheme {
			case "https":
				add("connect-src", "wss://"+u.Host)
			case "http":
				add("connect-src", "ws://"+u.Host)
			}
		}
	}
	return sources, nil
}
