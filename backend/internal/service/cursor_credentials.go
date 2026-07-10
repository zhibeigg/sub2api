package service

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

var cursorCredentialKeys = map[string]struct{}{
	"cookie":                {},
	"cookie_expires_at":     {},
	"cursor_upstream_model": {},
	"cursor_referer":        {},
}

// ValidateCursorAccountCredentials validates the manually supplied Cursor
// documentation-chat session cookie. Cursor does not expose an OAuth or refresh
// flow for this endpoint, so an effective _vcrcs cookie is mandatory.
func ValidateCursorAccountCredentials(accountType string, credentials map[string]any) error {
	if strings.TrimSpace(accountType) != AccountTypeCookie {
		return fmt.Errorf("Cursor accounts must use type %q", AccountTypeCookie)
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

	cookie := strings.TrimSpace(credentialString(credentials, "cookie"))
	if cookie == "" {
		return fmt.Errorf("Cursor credentials require a cookie containing _vcrcs")
	}
	request := &http.Request{Header: make(http.Header)}
	request.Header.Set("Cookie", cookie)
	found := false
	for _, parsed := range request.Cookies() {
		if parsed.Name == "_vcrcs" && strings.TrimSpace(parsed.Value) != "" {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("Cursor cookie must contain a non-empty _vcrcs value")
	}
	if raw, ok := credentials["cookie_expires_at"]; ok && credentialReplacementPresent(raw) {
		if normalizeCursorCookieExpiry(raw) == "" {
			return fmt.Errorf("Cursor cookie_expires_at must be an RFC3339 time or Unix timestamp")
		}
	}
	return nil
}

// NormalizeCursorCredentialExpiry stores optional operator-provided expiry in a
// stable UTC RFC3339 representation. The value is advisory only.
func NormalizeCursorCredentialExpiry(credentials map[string]any) {
	if credentials == nil {
		return
	}
	if raw, ok := credentials["cookie_expires_at"]; ok {
		if normalized := normalizeCursorCookieExpiry(raw); normalized != "" {
			credentials["cookie_expires_at"] = normalized
		}
	}
}

func normalizeCursorCookieExpiry(value any) string {
	temp := &Account{Credentials: map[string]any{"expires_at": value}}
	expiresAt := temp.GetCredentialAsTime("expires_at")
	if expiresAt == nil || expiresAt.IsZero() {
		return ""
	}
	return expiresAt.UTC().Format(time.RFC3339)
}
