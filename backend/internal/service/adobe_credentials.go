package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/provider/adobe/ims"
)

var adobeCredentialKeys = map[string]struct{}{
	"access_token": {},
	"cookie":       {},
	"device_token": {},
	"device_id":    {},
	"password":     {},
	"expires_at":   {},
}

// MergeAccountCredentials applies the account update protocol. Sensitive blank
// strings mean keep, non-blank values replace, and clearCredentials explicitly
// removes sensitive keys. The returned map never aliases its inputs.
func MergeAccountCredentials(existing, incoming map[string]any, clearCredentials []string) (map[string]any, error) {
	clearSet := make(map[string]struct{}, len(clearCredentials))
	for _, rawKey := range clearCredentials {
		key := strings.TrimSpace(rawKey)
		if !IsSensitiveCredentialKey(key) {
			return nil, fmt.Errorf("clear_credentials contains unsupported key %q", rawKey)
		}
		clearSet[key] = struct{}{}
	}

	normalizedIncoming := make(map[string]any, len(incoming))
	for key, value := range incoming {
		if _, clear := clearSet[key]; clear {
			if IsSensitiveCredentialKey(key) && credentialReplacementPresent(value) {
				return nil, fmt.Errorf("credential %q cannot be replaced and cleared in the same request", key)
			}
			continue
		}
		if IsSensitiveCredentialKey(key) && !credentialReplacementPresent(value) {
			continue
		}
		normalizedIncoming[key] = value
	}

	out := MergePreservingSensitiveCreds(existing, normalizedIncoming)
	for key := range clearSet {
		delete(out, key)
	}
	return out, nil
}

func credentialReplacementPresent(value any) bool {
	if value == nil {
		return false
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) != ""
	}
	return true
}

func ValidateAdobeAccountCredentials(accountType string, credentials map[string]any) error {
	if strings.TrimSpace(accountType) != AccountTypeOAuth {
		return fmt.Errorf("adobe accounts must use type %q", AccountTypeOAuth)
	}
	for key := range credentials {
		if _, allowed := adobeCredentialKeys[key]; allowed {
			continue
		}
		// Common non-secret routing metadata is allowed alongside Adobe credentials.
		switch key {
		case "model_mapping", "header_overrides", "temp_unschedulable_enabled", "temp_unschedulable_rules":
		default:
			return fmt.Errorf("credential %q is not supported for Adobe accounts", key)
		}
	}

	hasAccessToken := strings.TrimSpace(credentialString(credentials, "access_token")) != ""
	hasCookie := strings.TrimSpace(credentialString(credentials, "cookie")) != ""
	hasDeviceToken := strings.TrimSpace(credentialString(credentials, "device_token")) != ""
	hasDeviceID := strings.TrimSpace(credentialString(credentials, "device_id")) != ""
	if hasDeviceToken != hasDeviceID {
		return fmt.Errorf("adobe device_token and device_id must be provided or cleared together")
	}
	if !hasAccessToken && !hasCookie && (!hasDeviceToken || !hasDeviceID) {
		return fmt.Errorf("adobe credentials require access_token, cookie, or a complete device_token/device_id pair")
	}
	if raw, ok := credentials["expires_at"]; ok && credentialReplacementPresent(raw) {
		if normalizeAdobeExpiresAt(raw) == "" {
			return fmt.Errorf("adobe expires_at must be an RFC3339 time or Unix timestamp")
		}
	}
	return nil
}

func NormalizeAdobeCredentialExpiry(credentials map[string]any) {
	if credentials == nil {
		return
	}
	if raw, ok := credentials["expires_at"]; ok {
		if normalized := normalizeAdobeExpiresAt(raw); normalized != "" {
			credentials["expires_at"] = normalized
		}
		return
	}
	if token := strings.TrimSpace(credentialString(credentials, "access_token")); token != "" {
		if expiry := ims.ExtractJWTExpiry(token); expiry > 0 {
			credentials["expires_at"] = time.Unix(expiry, 0).UTC().Format(time.RFC3339)
		}
	}
}

func normalizeAdobeExpiresAt(value any) string {
	temp := &Account{Credentials: map[string]any{"expires_at": value}}
	expiresAt := temp.GetCredentialAsTime("expires_at")
	if expiresAt == nil || expiresAt.IsZero() {
		return ""
	}
	return expiresAt.UTC().Format(time.RFC3339)
}

func credentialString(credentials map[string]any, key string) string {
	if credentials == nil {
		return ""
	}
	return (&Account{Credentials: credentials}).GetCredential(key)
}
