package modelerror

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"

	"github.com/tidwall/gjson"
)

// UpstreamInput contains bounded upstream diagnostics used only for client-safe
// classification. Raw details remain in operational logs.
type UpstreamInput struct {
	Status  int
	Body    []byte
	Message string
	Err     error
	Model   string
}

func ClassifyUpstream(input UpstreamInput) Descriptor {
	message := strings.TrimSpace(input.Message)
	if message == "" {
		message = ExtractMessage(input.Body)
	}
	code := strings.ToLower(strings.TrimSpace(extractCode(input.Body)))
	statusName := strings.ToLower(strings.TrimSpace(gjson.GetBytes(input.Body, "error.status").String()))
	normalized := normalizeDiagnostic(strings.Join([]string{message, code, statusName, string(limitBytes(input.Body, 8192))}, " "))
	params := Params{Model: input.Model}

	switch {
	case isContextTooLarge(normalized):
		return Descriptor{Code: CodeContextTooLarge, Params: params}
	case isPayloadTooLarge(input.Status, normalized):
		return Descriptor{Code: CodePayloadTooLarge, Params: params}
	case isContentPolicy(normalized):
		return Descriptor{Code: CodeContentPolicy, Params: params}
	case isModelNotFound(input.Status, normalized):
		return Descriptor{Code: CodeModelNotFound, Params: params}
	case isModelUnsupported(normalized):
		return Descriptor{Code: CodeModelUnsupported, Params: params}
	}

	if input.Err != nil {
		switch {
		case errors.Is(input.Err, context.DeadlineExceeded), errors.Is(input.Err, syscall.ETIMEDOUT):
			return Descriptor{Code: CodeUpstreamTimeout, Params: params}
		case errors.Is(input.Err, io.ErrUnexpectedEOF), errors.Is(input.Err, syscall.ECONNRESET), errors.Is(input.Err, syscall.ECONNABORTED), errors.Is(input.Err, syscall.EPIPE), errors.Is(input.Err, syscall.ECONNREFUSED):
			return Descriptor{Code: CodeUpstreamUnavailable, Params: params}
		}
		var netErr net.Error
		if errors.As(input.Err, &netErr) {
			if netErr.Timeout() {
				return Descriptor{Code: CodeUpstreamTimeout, Params: params}
			}
			return Descriptor{Code: CodeUpstreamUnavailable, Params: params}
		}
	}

	switch input.Status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return Descriptor{Code: CodeInvalidRequest, Params: params}
	case http.StatusUnauthorized:
		return Descriptor{Code: CodeUpstreamAuthFailed, Params: params}
	case http.StatusForbidden:
		return Descriptor{Code: CodeUpstreamForbidden, Params: params}
	case http.StatusNotFound:
		return Descriptor{Code: CodeModelNotFound, Params: params}
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return Descriptor{Code: CodeUpstreamTimeout, Params: params}
	case http.StatusTooManyRequests:
		return Descriptor{Code: CodeUpstreamRateLimited, Params: params}
	case 529:
		return Descriptor{Code: CodeUpstreamOverloaded, Params: params}
	case http.StatusServiceUnavailable:
		return Descriptor{Code: CodeUpstreamOverloaded, Params: params}
	default:
		if input.Status >= 500 {
			return Descriptor{Code: CodeUpstreamUnavailable, Params: params}
		}
	}
	return Descriptor{Code: CodeUpstreamBadResponse, Params: params}
}

// FromLegacy maps existing handler/middleware type+message pairs to stable
// descriptors so callers can migrate without changing protocol status/type.
// LegacyDescriptor classifies a legacy error while preserving an already
// branded custom message. This makes nested writers idempotent and keeps
// administrator-defined passthrough text after it has been sanitized once.
func LegacyDescriptor(status int, legacyCode, message string) Descriptor {
	descriptor := FromLegacy(status, legacyCode, message)
	if strings.HasPrefix(strings.TrimSpace(message), BrandPrefix) {
		return WithCustomMessage(descriptor, message)
	}
	return descriptor
}

func FromLegacy(status int, legacyCode, message string) Descriptor {
	code := strings.ToUpper(strings.TrimSpace(legacyCode))
	normalized := normalizeDiagnostic(message)

	switch code {
	case "API_KEY_REQUIRED", "AUTHORIZATION_REQUIRED", "UNAUTHORIZED":
		return Descriptor{Code: CodeAuthRequired}
	case "INVALID_API_KEY", "INVALID_TOKEN", "INVALID_AUTH_HEADER", "EMPTY_TOKEN":
		return Descriptor{Code: CodeAuthInvalid}
	case "API_KEY_DISABLED", "USER_INACTIVE", "TOKEN_REVOKED":
		return Descriptor{Code: CodeAuthDisabled}
	case "API_KEY_EXPIRED", "TOKEN_EXPIRED":
		return Descriptor{Code: CodeAuthExpired}
	case "API_KEY_AUTH_OVERLOADED":
		return Descriptor{Code: CodeAuthUnavailable}
	case "API_KEY_QUOTA_EXHAUSTED", "INSUFFICIENT_QUOTA":
		return Descriptor{Code: CodeQuotaExhausted}
	case "INSUFFICIENT_BALANCE":
		return Descriptor{Code: CodeBalanceInsufficient}
	case "SUBSCRIPTION_NOT_FOUND":
		return Descriptor{Code: CodeSubscriptionRequired}
	case "USAGE_LIMIT_EXCEEDED":
		return Descriptor{Code: CodeUsageLimitExceeded}
	case "GROUP_NOT_ALLOWED", "GROUP_DISABLED", "GROUP_DELETED", "GROUP_NOT_FOUND", "GROUP_NOT_ASSIGNED":
		return Descriptor{Code: CodeGroupUnavailable}
	case "ENDPOINT_NOT_ALLOWED":
		return Descriptor{Code: CodeEndpointNotAllowed}
	case "SUBSCRIPTION_CONCURRENCY_LIMIT_EXCEEDED":
		return Descriptor{Code: CodeConcurrencyLimit, Params: Params{Scope: "subscription"}}
	case "MODEL_NOT_FOUND":
		return Descriptor{Code: CodeModelNotFound}
	case "MODEL_UNSUPPORTED", "COMPACT_NOT_SUPPORTED":
		return Descriptor{Code: CodeModelUnsupported}
	case "INVALID_AUTH_RATE_LIMITED", "RATE_LIMIT_ERROR", "RATE_LIMIT_EXCEEDED":
		if strings.Contains(normalized, "concurrency") {
			return Descriptor{Code: CodeConcurrencyLimit}
		}
		if strings.Contains(normalized, "upstream") {
			return Descriptor{Code: CodeUpstreamRateLimited}
		}
		return Descriptor{Code: CodeRateLimited}
	case "CONTENT_POLICY", "CONTENT_POLICY_VIOLATION", "CYBER_POLICY", "SESSION_BLOCKED", "PROMPT_GUARD_BLOCKED", "PERMISSION_ERROR":
		if code != "PERMISSION_ERROR" || strings.Contains(normalized, "policy") || strings.Contains(normalized, "moderation") || strings.Contains(normalized, "cyber") {
			return Descriptor{Code: CodeContentPolicy}
		}
	case "OVERLOADED_ERROR":
		return Descriptor{Code: CodeUpstreamOverloaded}
	case "TIMEOUT_ERROR":
		return Descriptor{Code: CodeUpstreamTimeout}
	}

	switch {
	case strings.Contains(normalized, "upstream authentication") || strings.Contains(normalized, "upstream websocket authentication"):
		return Descriptor{Code: CodeUpstreamAuthFailed}
	case strings.Contains(normalized, "upstream access forbidden") || strings.Contains(normalized, "upstream access denied"):
		return Descriptor{Code: CodeUpstreamForbidden}
	case strings.Contains(normalized, "upstream rate limit") || strings.Contains(normalized, "upstream service is rate-limited"):
		return Descriptor{Code: CodeUpstreamRateLimited}
	case strings.Contains(normalized, "upstream service overloaded") || strings.Contains(normalized, "upstream model service is overloaded"):
		return Descriptor{Code: CodeUpstreamOverloaded}
	case strings.Contains(normalized, "upstream service temporarily unavailable") || strings.Contains(normalized, "upstream websocket proxy failed") || strings.Contains(normalized, "no available account"):
		return Descriptor{Code: CodeUpstreamUnavailable}
	case strings.Contains(normalized, "upstream request failed") || strings.Contains(normalized, "upstream gone") || strings.Contains(normalized, "upstream returned") && strings.Contains(normalized, "invalid"):
		return Descriptor{Code: CodeUpstreamBadResponse}
	case strings.Contains(normalized, "api key is required") || strings.Contains(normalized, "api key required"):
		return Descriptor{Code: CodeAuthRequired}
	case strings.Contains(normalized, "invalid api key"):
		return Descriptor{Code: CodeAuthInvalid}
	case strings.Contains(normalized, "api key is disabled") || strings.Contains(normalized, "api key 已停用"):
		return Descriptor{Code: CodeAuthDisabled}
	case strings.Contains(normalized, "api key has expired") || strings.Contains(normalized, "api key 已过期"):
		return Descriptor{Code: CodeAuthExpired}
	case strings.Contains(normalized, "authentication is temporarily unavailable"):
		return Descriptor{Code: CodeAuthUnavailable}
	case strings.Contains(normalized, "no active subscription"):
		return Descriptor{Code: CodeSubscriptionRequired}
	case (strings.Contains(normalized, "daily usage limit") || strings.Contains(normalized, "weekly usage limit") || strings.Contains(normalized, "monthly usage limit")) && strings.Contains(normalized, "exceeded"):
		return Descriptor{Code: CodeUsageLimitExceeded}
	case strings.Contains(normalized, "所属分组") || strings.Contains(normalized, "group") && (strings.Contains(normalized, "deleted") || strings.Contains(normalized, "disabled") || strings.Contains(normalized, "not allowed") || strings.Contains(normalized, "not assigned")):
		return Descriptor{Code: CodeGroupUnavailable}
	case strings.Contains(normalized, "api is not supported for this platform") || strings.Contains(normalized, "only available for") && strings.Contains(normalized, "group") || strings.Contains(normalized, "not enabled") && (strings.Contains(normalized, "image") || strings.Contains(normalized, "video") || strings.Contains(normalized, "api") || strings.Contains(normalized, "endpoint")):
		return Descriptor{Code: CodeEndpointNotAllowed}
	case strings.Contains(normalized, "model is required") || strings.Contains(normalized, "missing model"):
		return Descriptor{Code: CodeModelRequired}
	case isContextTooLarge(normalized):
		return Descriptor{Code: CodeContextTooLarge}
	case isPayloadTooLarge(status, normalized):
		return Descriptor{Code: CodePayloadTooLarge}
	case isContentPolicy(normalized):
		return Descriptor{Code: CodeContentPolicy}
	case strings.Contains(normalized, "concurrency"):
		return Descriptor{Code: CodeConcurrencyLimit}
	case strings.Contains(normalized, "too many pending") || strings.Contains(normalized, "queue") && strings.Contains(normalized, "full"):
		return Descriptor{Code: CodeQueueFull}
	case strings.Contains(normalized, "model") && strings.Contains(normalized, "not supported"):
		return Descriptor{Code: CodeModelUnsupported}
	case strings.Contains(normalized, "model") && strings.Contains(normalized, "not found"):
		return Descriptor{Code: CodeModelNotFound}
	case strings.Contains(normalized, "balance") && strings.Contains(normalized, "insufficient"):
		return Descriptor{Code: CodeBalanceInsufficient}
	case strings.Contains(normalized, "quota") && (strings.Contains(normalized, "exhaust") || strings.Contains(normalized, "used up")):
		return Descriptor{Code: CodeQuotaExhausted}
	}

	if code == "UPSTREAM_ERROR" {
		return Descriptor{Code: CodeUpstreamBadResponse}
	}

	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return Descriptor{Code: CodeInvalidRequest}
	case http.StatusUnauthorized:
		return Descriptor{Code: CodeAuthInvalid}
	case http.StatusForbidden:
		return Descriptor{Code: CodePermissionDenied}
	case http.StatusNotFound:
		return Descriptor{Code: CodeModelNotFound}
	case http.StatusRequestEntityTooLarge:
		return Descriptor{Code: CodePayloadTooLarge}
	case http.StatusTooManyRequests:
		return Descriptor{Code: CodeRateLimited}
	case http.StatusServiceUnavailable:
		return Descriptor{Code: CodeServiceUnavailable}
	default:
		if status >= 500 {
			return Descriptor{Code: CodeInternalError}
		}
	}
	return Descriptor{Code: CodeInvalidRequest}
}

func WithCustomMessage(descriptor Descriptor, message string) Descriptor {
	descriptor.CustomMessage = message
	return descriptor
}

func ExtractMessage(body []byte) string {
	for _, path := range []string{"error.message", "response.error.message", "error.detail", "detail", "message"} {
		if message := strings.TrimSpace(gjson.GetBytes(body, path).String()); message != "" {
			return message
		}
	}
	if value := gjson.GetBytes(body, "error"); value.Type == gjson.String {
		return strings.TrimSpace(value.String())
	}
	return ""
}

func extractCode(body []byte) string {
	for _, path := range []string{"error.code", "response.error.code", "code"} {
		if code := strings.TrimSpace(gjson.GetBytes(body, path).String()); code != "" {
			return code
		}
	}
	return ""
}

func normalizeDiagnostic(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer("_", " ", "-", " ", "\n", " ", "\r", " ", "\t", " ").Replace(value)
	return strings.Join(strings.Fields(value), " ")
}

func isPayloadTooLarge(status int, normalized string) bool {
	return status == http.StatusRequestEntityTooLarge || strings.Contains(normalized, "payload too large") || strings.Contains(normalized, "request body too large") || strings.Contains(normalized, "request entity too large")
}

func isContextTooLarge(normalized string) bool {
	if normalized == "" {
		return false
	}
	if strings.Contains(normalized, "context too large") || strings.Contains(normalized, "context length exceeded") || strings.Contains(normalized, "maximum context length") || strings.Contains(normalized, "max context length") {
		return true
	}
	exceeded := strings.Contains(normalized, "exceed") || strings.Contains(normalized, "too long") || strings.Contains(normalized, "too many tokens")
	return exceeded && (strings.Contains(normalized, "context window") || strings.Contains(normalized, "context length") || strings.Contains(normalized, "token limit"))
}

func isContentPolicy(normalized string) bool {
	return strings.Contains(normalized, "content policy") || strings.Contains(normalized, "cyber policy") || strings.Contains(normalized, "cyber-security") || strings.Contains(normalized, "security policy") || strings.Contains(normalized, "safety policy") || strings.Contains(normalized, "policy violation") || strings.Contains(normalized, "blocked by policy") || strings.Contains(normalized, "moderation blocked") || strings.Contains(normalized, "flagged for")
}

func isModelNotFound(status int, normalized string) bool {
	if status != http.StatusNotFound || !strings.Contains(normalized, "model") {
		return false
	}
	return strings.Contains(normalized, "not found") || strings.Contains(normalized, "unknown model") || strings.Contains(normalized, "does not exist")
}

func isModelUnsupported(normalized string) bool {
	return strings.Contains(normalized, "model") && (strings.Contains(normalized, "not supported") || strings.Contains(normalized, "unsupported model") || strings.Contains(normalized, "not available for"))
}

func limitBytes(value []byte, max int) []byte {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max]
}
