package firefly

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type ErrorKind string

const (
	ErrorAuth            ErrorKind = "auth"
	ErrorRateLimited     ErrorKind = "rate_limited"
	ErrorQuota           ErrorKind = "quota_exhausted"
	ErrorNotEntitled     ErrorKind = "not_entitled"
	ErrorProviderBlocked ErrorKind = "provider_blocked"
	ErrorContentPolicy   ErrorKind = "content_policy"
	ErrorTemporary       ErrorKind = "temporary"
	ErrorRequest         ErrorKind = "request"
)

type ProviderError struct {
	Kind       ErrorKind
	HTTPStatus int
	Code       string
	Message    string
	Retryable  bool
	RetryAfter time.Duration
}

func (e *ProviderError) Error() string {
	if e.HTTPStatus > 0 {
		return fmt.Sprintf("Adobe upstream error (%d): %s", e.HTTPStatus, e.Message)
	}
	return "Adobe upstream error: " + e.Message
}

func classifyError(status int, headers map[string]string, body []byte) error {
	access := strings.ToLower(headers["x-access-error"])
	text := strings.ToLower(string(body))
	retryAfter := parseRetryAfter(headers["retry-after"])
	mk := func(kind ErrorKind, code, msg string, retry bool) *ProviderError {
		return &ProviderError{Kind: kind, HTTPStatus: status, Code: code, Message: msg, Retryable: retry, RetryAfter: retryAfter}
	}
	if (status == 401 || status == 403) && (access == "taste_exhausted" || access == "quota_exhausted") {
		return mk(ErrorQuota, access, "quota exhausted", false)
	}
	if (status == 401 || status == 403) && access == "blocked_by_3p_model_provider" {
		return mk(ErrorProviderBlocked, access, "model provider rejected the request", true)
	}
	if (status == 401 || status == 403) && (access == "user_not_entitled" || access == "not_entitled") {
		return mk(ErrorNotEntitled, access, "account is not entitled to this capability", false)
	}
	isHTML := strings.Contains(text, "<html") || strings.Contains(text, "<!doctype html")
	if status == 401 || status == 403 {
		if isHTML {
			return mk(ErrorTemporary, "gateway_challenge", "upstream gateway challenge", true)
		}
		return mk(ErrorAuth, "authentication_failed", "access token was rejected", false)
	}
	if status == 429 || strings.Contains(text, "backpressure_limited") || strings.Contains(text, "worker is overloaded") {
		return mk(ErrorRateLimited, "rate_limited", "upstream is rate limited", true)
	}
	if status == 408 || strings.Contains(text, "timeout_error") || strings.Contains(text, "system under load") {
		return mk(ErrorTemporary, "upstream_busy", "upstream is busy", true)
	}
	if status == 451 {
		return mk(ErrorContentPolicy, "region_or_policy_rejected", "request was rejected by content policy", false)
	}
	if status == 422 && strings.Contains(text, "invalid usage for") {
		return mk(ErrorContentPolicy, "invalid_usage", "prompt or payload was rejected", false)
	}
	if strings.Contains(text, "content_policy") || strings.Contains(text, "moderation") || strings.Contains(text, "unsafe") {
		return mk(ErrorContentPolicy, "content_policy", "request was rejected by content policy", false)
	}
	if status >= 500 {
		return mk(ErrorTemporary, "upstream_failure", "upstream service failure", true)
	}
	if status == 413 {
		return mk(ErrorRequest, "payload_too_large", "request payload is too large", false)
	}
	return mk(ErrorRequest, "invalid_request", "upstream rejected the request", false)
}

func IsAuthError(err error) bool {
	var e *ProviderError
	return errors.As(err, &e) && e.Kind == ErrorAuth
}
func IsRetryableError(err error) bool { var e *ProviderError; return errors.As(err, &e) && e.Retryable }
func StatusCode(err error) int {
	var e *ProviderError
	if errors.As(err, &e) {
		return e.HTTPStatus
	}
	return 0
}
func parseRetryAfter(v string) time.Duration {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n <= 0 {
		return 0
	}
	return time.Duration(n) * time.Second
}
func shouldTryNextCandidate(err error) bool {
	var e *ProviderError
	if !errors.As(err, &e) {
		return false
	}
	return e.Kind == ErrorRequest || e.Code == "invalid_usage"
}
