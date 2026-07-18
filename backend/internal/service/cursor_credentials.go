package service

import (
	"fmt"
	"strings"
)

const (
	CursorTransportAuto       = "auto"
	CursorTransportIDEChat    = "ide_chat"
	CursorTransportCloudAgent = "cloud_agent"
)

var cursorCredentialKeys = map[string]struct{}{
	"api_key":                  {},
	"dashboard_access_token":   {},
	"dashboard_refresh_token":  {},
	"cursor_transport_mode":    {},
	"cursor_machine_id":        {},
	"cursor_client_version":    {},
	"cursor_client_os_version": {},
	"cursor_config_version":    {},
	"cursor_upstream_model":    {},
	"cursor_model_params":      {},
	"_token_version":           {},
}

func NormalizeCursorTransportMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", CursorTransportAuto:
		return CursorTransportAuto
	case CursorTransportIDEChat:
		return CursorTransportIDEChat
	case CursorTransportCloudAgent:
		return CursorTransportCloudAgent
	default:
		return ""
	}
}

func CursorAccountTransportMode(account *Account) string {
	mode := CursorTransportAuto
	if account != nil {
		mode = NormalizeCursorTransportMode(cursorAccountSetting(account, "cursor_transport_mode"))
	}
	if mode == "" {
		return CursorTransportAuto
	}
	return mode
}

func CursorAccountUsesIDEChat(account *Account) bool {
	switch CursorAccountTransportMode(account) {
	case CursorTransportIDEChat:
		return true
	case CursorTransportCloudAgent:
		return false
	default:
		return account != nil && strings.TrimSpace(account.GetCredential("dashboard_access_token")) != ""
	}
}

// ValidateCursorAccountCredentials validates the two supported Cursor transports:
// the low-latency IDE chat session and the official Cloud Agents API.
func ValidateCursorAccountCredentials(accountType string, credentials map[string]any) error {
	if strings.TrimSpace(accountType) != AccountTypeAPIKey {
		return fmt.Errorf("cursor accounts must use type %q", AccountTypeAPIKey)
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

	modeRaw := credentialString(credentials, "cursor_transport_mode")
	mode := NormalizeCursorTransportMode(modeRaw)
	if mode == "" {
		return fmt.Errorf("cursor transport mode must be %q, %q, or %q", CursorTransportAuto, CursorTransportIDEChat, CursorTransportCloudAgent)
	}
	apiKey := strings.TrimSpace(credentialString(credentials, "api_key"))
	accessToken := strings.TrimSpace(credentialString(credentials, "dashboard_access_token"))
	switch mode {
	case CursorTransportIDEChat:
		if accessToken == "" {
			return fmt.Errorf("cursor IDE chat credentials require a Dashboard access token")
		}
	case CursorTransportCloudAgent:
		if apiKey == "" {
			return fmt.Errorf("cursor Cloud Agent credentials require a non-empty API key")
		}
	default:
		if apiKey == "" && accessToken == "" {
			return fmt.Errorf("cursor credentials require an IDE access token or Cloud Agent API key")
		}
	}

	if err := validateCursorCredentialText("API key", apiKey, 8192); err != nil {
		return err
	}
	for _, key := range []string{"dashboard_access_token", "dashboard_refresh_token"} {
		if err := validateCursorCredentialText(key, strings.TrimSpace(credentialString(credentials, key)), 65536); err != nil {
			return err
		}
	}
	for _, key := range []string{"cursor_machine_id", "cursor_client_version", "cursor_config_version"} {
		if err := validateCursorCredentialText(key, strings.TrimSpace(credentialString(credentials, key)), 1024); err != nil {
			return err
		}
	}
	return nil
}

func validateCursorCredentialText(label, value string, maxLength int) error {
	if value == "" {
		return nil
	}
	if len(value) > maxLength {
		return fmt.Errorf("cursor %s exceeds %d characters", label, maxLength)
	}
	if strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("cursor %s contains invalid control characters", label)
	}
	return nil
}

// NormalizeCursorCredentialExpiry is retained as a no-op for compatibility
// with older service call sites. Cloud Agents API keys do not carry a local
// cookie expiry value.
func NormalizeCursorCredentialExpiry(map[string]any) {}
