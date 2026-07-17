package admin

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
