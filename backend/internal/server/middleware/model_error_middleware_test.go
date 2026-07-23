package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/modelerror"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func modelErrorTestRouter(handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), ctxkey.RequestID, "req-middleware-1")
		ctx = service.WithEndpointProtocol(ctx, service.EndpointProtocolOpenAIChatCompletions)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.Use(ModelErrorLocale(&config.Config{Gateway: config.GatewayConfig{ModelErrorDefaultLocale: "en"}}))
	router.Use(ModelErrorMetadata())
	router.GET("/v1/chat/completions", handler)
	return router
}

func TestModelErrorMiddlewareDoesNotMarkSuccessfulResponseAsLocalizedError(t *testing.T) {
	router := modelErrorTestRouter(func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	req.Header.Set("Accept-Language", "zh-CN")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "req-middleware-1", recorder.Header().Get(modelerror.HeaderRequestID))
	require.Empty(t, recorder.Header().Get(modelerror.HeaderErrorCode))
	require.Empty(t, recorder.Header().Get("Content-Language"))
	require.Empty(t, recorder.Header().Values("Vary"))
}

func TestModelErrorMiddlewareLocalizesOnlyErrorResponse(t *testing.T) {
	router := modelErrorTestRouter(func(c *gin.Context) {
		modelerror.WriteOpenAI(c, http.StatusBadRequest, "invalid_request_error", "invalid request")
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	req.Header.Set("Accept-Language", "zh-CN, en;q=0.5")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Equal(t, "zh-CN", recorder.Header().Get("Content-Language"))
	require.Equal(t, "POKE_INVALID_REQUEST", recorder.Header().Get(modelerror.HeaderErrorCode))
	require.Equal(t, "req-middleware-1", recorder.Header().Get(modelerror.HeaderRequestID))
	require.Contains(t, recorder.Body.String(), "[PokeAPI]")
	require.Contains(t, recorder.Body.String(), "模型请求")
}
