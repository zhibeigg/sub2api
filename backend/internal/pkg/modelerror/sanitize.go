package modelerror

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

const maxClientMessageBytes = 2048

var (
	sensitiveQueryParamRegex       = regexp.MustCompile(`(?i)([?&](?:key|api_key|client_secret|access_token|refresh_token|id_token|session_token)=)[^&"\s]+`)
	sensitiveBearerTokenRegex      = regexp.MustCompile(`(?i)(\bbearer\s+)[A-Za-z0-9._~+/=-]+`)
	sensitiveInlineCredentialRegex = regexp.MustCompile(`(?i)(\b(?:api[_-]?key|access[_-]?token|refresh[_-]?token|id[_-]?token|session[_-]?token|client[_-]?secret|password|authorization)\b\s*[:=]\s*)(?:"[^"]*"|'[^']*'|[^\s,;&]+)`)
	privateAddressRegex            = regexp.MustCompile(`(?i)\b(?:(?:10|127)\.(?:\d{1,3}\.){2}\d{1,3}|192\.168\.(?:\d{1,3}\.)\d{1,3}|172\.(?:1[6-9]|2\d|3[01])\.(?:\d{1,3}\.)\d{1,3}|localhost)(?::\d{1,5})?\b`)
)

func SanitizeMessage(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}
	message = sensitiveQueryParamRegex.ReplaceAllString(message, `$1***`)
	message = sensitiveBearerTokenRegex.ReplaceAllString(message, `$1***`)
	message = sensitiveInlineCredentialRegex.ReplaceAllString(message, `$1***`)
	message = privateAddressRegex.ReplaceAllString(message, `[internal-address]`)
	message = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, message)
	message = strings.Join(strings.Fields(message), " ")
	return truncateUTF8Bytes(message, maxClientMessageBytes)
}

func truncateUTF8Bytes(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	end := maxBytes
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	if end <= 0 {
		return ""
	}
	return strings.TrimSpace(value[:end])
}

// TruncateUTF8Bytes safely limits WebSocket close reasons and other byte-bound
// protocol fields without producing invalid UTF-8.
func TruncateUTF8Bytes(value string, maxBytes int) string {
	return truncateUTF8Bytes(value, maxBytes)
}
