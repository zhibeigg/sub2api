package modelerror

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newWriterTestContext(t *testing.T, locale Locale) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "req-model-error-1")
	ctx = WithLocale(ctx, locale, "en")
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(ctx)
	return c, recorder
}

func TestProtocolWritersPreserveEnvelopeAndHeaders(t *testing.T) {
	tests := []struct {
		name  string
		write func(*gin.Context)
	}{
		{
			name: "anthropic",
			write: func(c *gin.Context) {
				WriteAnthropicDescriptorWithCode(c, http.StatusTooManyRequests, "rate_limit_error", "subscription_concurrency_limit_exceeded", Descriptor{Code: CodeConcurrencyLimit})
			},
		},
		{
			name: "chat completions",
			write: func(c *gin.Context) {
				WriteOpenAIDescriptor(c, http.StatusBadRequest, "invalid_request_error", "context_length_exceeded", Descriptor{Code: CodeContextTooLarge})
			},
		},
		{
			name: "responses",
			write: func(c *gin.Context) {
				WriteResponsesDescriptorWithType(c, http.StatusBadGateway, "upstream_error", "upstream_error", Descriptor{Code: CodeUpstreamBadResponse})
			},
		},
		{
			name: "google",
			write: func(c *gin.Context) {
				WriteGoogleDescriptor(c, http.StatusServiceUnavailable, Descriptor{Code: CodeUpstreamUnavailable})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, recorder := newWriterTestContext(t, LocaleChinese)
			test.write(c)

			require.NotEqual(t, http.StatusOK, recorder.Code)
			require.Equal(t, "zh-CN", recorder.Header().Get("Content-Language"))
			require.Equal(t, "req-model-error-1", recorder.Header().Get(HeaderRequestID))
			require.NotEmpty(t, recorder.Header().Get(HeaderErrorCode))
			require.Contains(t, recorder.Header().Values("Vary"), "Accept-Language")

			var body map[string]any
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
			errorObject, ok := body["error"].(map[string]any)
			require.True(t, ok)
			message, _ := errorObject["message"].(string)
			require.Contains(t, message, BrandPrefix)
			require.Contains(t, message, "请")
		})
	}
}

func TestLegacyDescriptorKeepsBrandedCustomMessage(t *testing.T) {
	message := "[PokeAPI] 管理员提示 api_key=secret-value"
	descriptor := LegacyDescriptor(http.StatusBadRequest, "upstream_error", message)
	presentation := Present(context.Background(), descriptor)
	require.Equal(t, BrandPrefix, presentation.Message[:len(BrandPrefix)])
	require.NotContains(t, presentation.Message, "secret-value")
	require.Equal(t, 1, strings.Count(presentation.Message, BrandPrefix))
}
