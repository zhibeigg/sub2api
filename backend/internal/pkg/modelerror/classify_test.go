package modelerror

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClassifyUpstream(t *testing.T) {
	tests := []struct {
		name  string
		input UpstreamInput
		want  Code
	}{
		{name: "context", input: UpstreamInput{Status: 400, Body: []byte(`{"error":{"message":"maximum context length exceeded"}}`)}, want: CodeContextTooLarge},
		{name: "model not found", input: UpstreamInput{Status: 404, Body: []byte(`{"error":{"message":"model gpt-x not found"}}`)}, want: CodeModelNotFound},
		{name: "unsupported model", input: UpstreamInput{Status: 400, Message: "model is not supported for this account"}, want: CodeModelUnsupported},
		{name: "policy", input: UpstreamInput{Status: 400, Message: "request blocked by content policy"}, want: CodeContentPolicy},
		{name: "rate limit", input: UpstreamInput{Status: 429}, want: CodeUpstreamRateLimited},
		{name: "overloaded", input: UpstreamInput{Status: 529}, want: CodeUpstreamOverloaded},
		{name: "timeout status", input: UpstreamInput{Status: 504}, want: CodeUpstreamTimeout},
		{name: "timeout error", input: UpstreamInput{Err: context.DeadlineExceeded}, want: CodeUpstreamTimeout},
		{name: "network", input: UpstreamInput{Err: errors.New("connection failed")}, want: CodeUpstreamBadResponse},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, ClassifyUpstream(test.input).Code)
		})
	}
}

func TestFromLegacy(t *testing.T) {
	require.Equal(t, CodeAuthInvalid, FromLegacy(http.StatusUnauthorized, "INVALID_API_KEY", "Invalid API key").Code)
	require.Equal(t, CodeModelRequired, FromLegacy(http.StatusBadRequest, "invalid_request_error", "model is required").Code)
	require.Equal(t, CodeConcurrencyLimit, FromLegacy(http.StatusTooManyRequests, "rate_limit_error", "Concurrency limit exceeded").Code)
	require.Equal(t, CodeUpstreamRateLimited, ClassifyUpstream(UpstreamInput{Status: http.StatusTooManyRequests}).Code)
}

func TestSanitizeMessage(t *testing.T) {
	input := "Bearer abc.def api_key=secret https://example.test?access_token=sensitive-value connect 10.1.2.3:443\nretry"
	output := SanitizeMessage(input)
	require.NotContains(t, output, "abc.def")
	require.NotContains(t, output, "secret")
	require.NotContains(t, output, "sensitive-value")
	require.NotContains(t, output, "10.1.2.3")
	require.Contains(t, output, "***")
	require.NotContains(t, output, "\n")
}
