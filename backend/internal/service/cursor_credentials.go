package service

import (
	"fmt"
	"strings"
)

var cursorCredentialKeys = map[string]struct{}{
	"api_key":                 {},
	"dashboard_access_token":  {},
	"dashboard_refresh_token": {},
	"cursor_upstream_model":   {},
	"cursor_model_params":     {},
	"_token_version":          {},
}

// ValidateCursorAccountCredentials validates a Cursor Cloud Agents API key.
// User API keys and service-account API keys are both accepted; the live /v1/me
// probe is responsible for checking authorization and revocation status.
func ValidateCursorAccountCredentials(accountType string, credentials map[string]any) error {
	if strings.TrimSpace(accountType) != AccountTypeAPIKey {
		return fmt.Errorf("Cursor accounts must use type %q", AccountTypeAPIKey)
	}
	for key := range credentials {
		if _, allowed := cursorCredentialKeys[key]; allowed {
			continue
		}
		switch key {
		case "model_mapping", "header_overrides", "temp_unschedulable_enabled", "temp_unschedulable_rules":
		default:
			return fmt.Errorf("credential %q is not supported for Cursor accounts", key)
		}
	}

	apiKey := strings.TrimSpace(credentialString(credentials, "api_key"))
	if apiKey == "" {
		return fmt.Errorf("Cursor credentials require a non-empty API key")
	}
	if len(apiKey) > 8192 {
		return fmt.Errorf("Cursor API key exceeds 8192 characters")
	}
	if strings.ContainsAny(apiKey, "\r\n\x00") {
		return fmt.Errorf("Cursor API key contains invalid control characters")
	}
	for _, key := range []string{"dashboard_access_token", "dashboard_refresh_token"} {
		value := strings.TrimSpace(credentialString(credentials, key))
		if value == "" {
			continue
		}
		if len(value) > 65536 {
			return fmt.Errorf("Cursor %s exceeds 65536 characters", key)
		}
		if strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("Cursor %s contains invalid control characters", key)
		}
	}
	return nil
}

// NormalizeCursorCredentialExpiry is retained as a no-op for compatibility
// with older service call sites. Cloud Agents API keys do not carry a local
// cookie expiry value.
func NormalizeCursorCredentialExpiry(map[string]any) {}
