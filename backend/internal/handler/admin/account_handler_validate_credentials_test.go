package admin

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type validateCredentialsUpstream struct {
	err error
}

func (s *validateCredentialsUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	return s.DoWithTLS(req, proxyURL, accountID, accountConcurrency, nil)
}

func (s *validateCredentialsUpstream) DoWithTLS(_ *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"apiKeyName":"admin-test","userEmail":"cursor@example.com"}`)),
	}, nil
}

func setupValidateCredentialsRouter(upstream service.HTTPUpstream) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	gateway := service.NewCursorGatewayService(upstream, nil, nil, nil, &config.Config{Cursor: config.CursorConfig{
		BaseURL:                  "https://api.cursor.com",
		DefaultModel:             "auto",
		RequestTimeoutSeconds:    10,
		StreamIdleTimeoutSeconds: 10,
	}})
	testService := service.NewAccountTestService(nil, nil, nil, nil, nil, nil, &config.Config{}, nil)
	testService.SetCursorGatewayService(gateway)
	handler := NewAccountHandler(nil, nil, nil, nil, nil, nil, nil, nil, testService, nil, nil, nil, nil, nil)
	router.POST("/api/v1/admin/accounts/validate-credentials", handler.ValidateCredentials)
	return router
}

type validateOpenCodeCredentialsUpstream struct {
	mu              sync.Mutex
	paths           []string
	inferenceStatus int
	inferenceBody   string
}

func (s *validateOpenCodeCredentialsUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	s.mu.Lock()
	s.paths = append(s.paths, req.URL.Path)
	s.mu.Unlock()
	if strings.HasSuffix(req.URL.Path, "/v1/models") {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"object":"list","data":[{"id":"kimi-k3"}]}`)),
		}, nil
	}
	status := s.inferenceStatus
	if status == 0 {
		status = http.StatusOK
	}
	body := s.inferenceBody
	if body == "" {
		body = `{"id":"chat-validation","object":"chat.completion","model":"kimi-k3","choices":[{"index":0,"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}

func (s *validateOpenCodeCredentialsUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

func (s *validateOpenCodeCredentialsUpstream) snapshotPaths() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.paths...)
}

func setupValidateOpenCodeCredentialsRouter(upstream service.HTTPUpstream) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	gateway := service.NewOpenCodeGatewayService(upstream, nil, &config.Config{}, nil)
	testService := service.NewAccountTestService(nil, nil, nil, nil, nil, nil, &config.Config{}, nil)
	testService.SetOpenCodeGatewayService(gateway)
	handler := NewAccountHandler(nil, nil, nil, nil, nil, nil, nil, nil, testService, nil, nil, nil, nil, nil)
	router.POST("/api/v1/admin/accounts/validate-credentials", handler.ValidateCredentials)
	return router
}

func performValidateCredentialsRequest(router http.Handler, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/validate-credentials", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestAccountHandlerValidateCredentialsRequestValidation(t *testing.T) {
	router := setupValidateCredentialsRouter(&validateCredentialsUpstream{})

	for _, body := range []string{
		`{}`,
		`{"platform":"openai","type":"oauth","credentials":{"access_token":"secret"}}`,
		`{"platform":"adobe","type":"cookie","credentials":{"cookie":"secret"}}`,
		`{"platform":"cursor","type":"cookie","credentials":{"cookie":"_vcrcs=legacy"}}`,
		`{"platform":"cursor","type":"apikey","credentials":{"api_key":""}}`,
	} {
		recorder := performValidateCredentialsRequest(router, body)
		require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
		require.NotContains(t, recorder.Body.String(), "secret")
	}
}

func TestAccountHandlerValidateCredentialsCursorSuccessDoesNotEchoAPIKey(t *testing.T) {
	router := setupValidateCredentialsRouter(&validateCredentialsUpstream{})
	apiKey := "cursor-handler-secret"
	recorder := performValidateCredentialsRequest(router, `{"platform":"cursor","type":"apikey","credentials":{"api_key":"`+apiKey+`"}}`)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"success":true`)
	require.Contains(t, recorder.Body.String(), `"platform":"cursor"`)
	require.Contains(t, recorder.Body.String(), `"summary":"cursor@example.com"`)
	require.NotContains(t, recorder.Body.String(), apiKey)
}

func TestAccountHandlerValidateCredentialsOpenCodeRunsRealInference(t *testing.T) {
	upstream := &validateOpenCodeCredentialsUpstream{}
	router := setupValidateOpenCodeCredentialsRouter(upstream)
	apiKey := "opencode-handler-secret"

	recorder := performValidateCredentialsRequest(router, `{"platform":"opencode","type":"apikey","credentials":{"api_key":"`+apiKey+`","base_url":"https://opencode.ai/zen/go","model_protocols":{"kimi-k3":"chat_completions"}},"model_id":"kimi-k3"}`)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, []string{"/zen/go/v1/models", "/zen/go/v1/chat/completions"}, upstream.snapshotPaths())
	require.Contains(t, recorder.Body.String(), `"success":true`)
	require.Contains(t, recorder.Body.String(), `"summary":"Authenticated inference request succeeded with kimi-k3"`)
	require.NotContains(t, recorder.Body.String(), apiKey)
}

func TestAccountHandlerValidateCredentialsOpenCodeRejectsCatalogOnlyKey(t *testing.T) {
	upstream := &validateOpenCodeCredentialsUpstream{
		inferenceStatus: http.StatusUnauthorized,
		inferenceBody:   `{"type":"error","error":{"type":"AuthError","message":"Request blocked by upstream provider."}}`,
	}
	router := setupValidateOpenCodeCredentialsRouter(upstream)
	apiKey := "blocked-opencode-secret"

	recorder := performValidateCredentialsRequest(router, `{"platform":"opencode","type":"apikey","credentials":{"api_key":"`+apiKey+`","base_url":"https://opencode.ai/zen/go","model_protocols":{"kimi-k3":"chat_completions"}},"model_id":"kimi-k3"}`)

	require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
	require.Equal(t, []string{"/zen/go/v1/models", "/zen/go/v1/chat/completions"}, upstream.snapshotPaths())
	require.Contains(t, recorder.Body.String(), "OpenCode Go inference access was rejected by the upstream provider")
	require.NotContains(t, recorder.Body.String(), apiKey)
	require.NotContains(t, recorder.Body.String(), "Request blocked by upstream provider")
}

func TestAccountHandlerValidateCredentialsSanitizesUpstreamErrors(t *testing.T) {
	router := setupValidateCredentialsRouter(&validateCredentialsUpstream{err: errors.New("upstream response body: cursor-leaked-secret")})
	recorder := performValidateCredentialsRequest(router, `{"platform":"cursor","type":"apikey","credentials":{"api_key":"cursor-request-secret"}}`)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "Credential validation failed")
	require.NotContains(t, recorder.Body.String(), "leaked-secret")
	require.NotContains(t, recorder.Body.String(), "request-secret")
}

func TestAccountRequestBindingsAcceptCursorAPIKeyType(t *testing.T) {
	gin.SetMode(gin.TestMode)

	createRecorder := httptest.NewRecorder()
	createContext, _ := gin.CreateTestContext(createRecorder)
	createContext.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"cursor","platform":"cursor","type":"apikey","credentials":{"api_key":"cursor-key"}}`))
	createContext.Request.Header.Set("Content-Type", "application/json")
	var createRequest CreateAccountRequest
	require.NoError(t, createContext.ShouldBindJSON(&createRequest))
	require.Equal(t, service.AccountTypeAPIKey, createRequest.Type)

	updateRecorder := httptest.NewRecorder()
	updateContext, _ := gin.CreateTestContext(updateRecorder)
	updateContext.Request = httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"type":"apikey"}`))
	updateContext.Request.Header.Set("Content-Type", "application/json")
	var updateRequest UpdateAccountRequest
	require.NoError(t, updateContext.ShouldBindJSON(&updateRequest))
	require.Equal(t, service.AccountTypeAPIKey, updateRequest.Type)
}
