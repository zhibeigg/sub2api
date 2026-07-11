package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type cursorDashboardAuthHandlerRepo struct {
	service.AccountRepository
	account *service.Account
}

func (r *cursorDashboardAuthHandlerRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	if r.account != nil && r.account.ID == id {
		return r.account, nil
	}
	return nil, service.ErrAccountNotFound
}

func TestCursorDashboardAuthHandlerStartReturnsOnlyPublicSessionFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{Cursor: config.CursorConfig{
		BaseURL:                 "https://api.cursor.com",
		DashboardBaseURL:        "https://api2.cursor.sh",
		DashboardAuthWebsiteURL: "https://cursor.com",
	}}
	repo := &cursorDashboardAuthHandlerRepo{account: &service.Account{
		ID:          42,
		Platform:    service.PlatformCursor,
		Type:        service.AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "cursor-cloud-key"},
	}}
	gateway := service.NewCursorGatewayService(nil, nil, nil, nil, cfg)
	handler := NewCursorDashboardAuthHandler(service.NewCursorDashboardAuthService(repo, gateway, nil, cfg))
	router := gin.New()
	router.POST("/api/v1/admin/cursor/dashboard-auth/start", handler.Start)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/admin/cursor/dashboard-auth/start",
		strings.NewReader(`{"account_id":42}`),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	require.Contains(t, body, `"session_id"`)
	require.Contains(t, body, `"auth_url"`)
	require.Contains(t, body, `"expires_at"`)
	require.Contains(t, body, "loginDeepControl")
	require.NotContains(t, body, "cursor-cloud-key")
	require.NotContains(t, body, "code_verifier")
	require.NotContains(t, body, "refresh_token")
	require.NotContains(t, body, "access_token")
}

func TestCursorDashboardAuthHandlerRejectsInvalidRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewCursorDashboardAuthHandler(nil)
	router := gin.New()
	router.POST("/start", handler.Start)
	router.POST("/poll", handler.Poll)

	for _, testCase := range []struct {
		path string
		body string
	}{
		{path: "/start", body: `{"account_id":0}`},
		{path: "/start", body: `{}`},
		{path: "/poll", body: `{"session_id":""}`},
		{path: "/poll", body: `{}`},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, testCase.path, strings.NewReader(testCase.body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusBadRequest, recorder.Code, testCase.path+" "+testCase.body)
	}
}
