package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

func makeCursorDashboardJWT(t *testing.T, expiresAt time.Time) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"sub": "auth-user", "exp": expiresAt.Unix()})
	require.NoError(t, err)
	return strings.Join([]string{
		base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)),
		base64.RawURLEncoding.EncodeToString(payload),
		"signature",
	}, ".")
}

func TestCursorDashboardTokenRefresherUsesDesktopRefreshWindow(t *testing.T) {
	refresher := NewCursorDashboardTokenRefresher(nil, &config.Config{Cursor: config.CursorConfig{DashboardRefreshBeforeExpiryHours: 1272}})
	account := &Account{Platform: PlatformCursor, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"dashboard_access_token":  makeCursorDashboardJWT(t, time.Now().Add(1273*time.Hour)),
		"dashboard_refresh_token": "refresh",
	}}
	require.False(t, refresher.NeedsRefresh(account, time.Minute))
	account.Credentials["dashboard_access_token"] = makeCursorDashboardJWT(t, time.Now().Add(1271*time.Hour))
	require.True(t, refresher.NeedsRefresh(account, time.Minute))
	require.Equal(t, "dashboard_refresh_token", refresher.RefreshTokenCredentialKey())
}

func TestCursorDashboardSessionInfoMissingOverridesStaleExtra(t *testing.T) {
	account := &Account{Platform: PlatformCursor, Type: AccountTypeAPIKey, Credentials: map[string]any{}, Extra: map[string]any{
		cursorDashboardAuthStateKey: "connected",
	}}
	info := cursorDashboardSessionInfoFromAccount(account)
	require.Equal(t, "missing", info.State)
}

func TestCursorDashboardMaintenanceClassifiesExplicitRevocation(t *testing.T) {
	require.Equal(t, "reauth_required", classifyCursorDashboardMaintenanceError(errCursorDashboardReauthRequired))
	require.Equal(t, "refresh_error", classifyCursorDashboardMaintenanceError(errors.New("temporary refresh failure")))
}

func TestCursorDashboardSessionInfoReadsMetadata(t *testing.T) {
	expiresAt := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	verifiedAt := time.Now().UTC().Truncate(time.Second)
	account := &Account{Platform: PlatformCursor, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"dashboard_access_token": "token",
	}, Extra: map[string]any{
		cursorDashboardAuthStateKey:      "reauth_required",
		cursorDashboardTokenExpiresAtKey: expiresAt.Format(time.RFC3339),
		cursorDashboardLastVerifiedAtKey: verifiedAt.Format(time.RFC3339),
		cursorDashboardLastErrorCodeKey:  "reauth_required",
	}}
	info := cursorDashboardSessionInfoFromAccount(account)
	require.Equal(t, "reauth_required", info.State)
	require.Equal(t, expiresAt, *info.ExpiresAt)
	require.Equal(t, verifiedAt, *info.LastVerifiedAt)
	require.Equal(t, "reauth_required", info.ErrorCode)
}

type cursorDashboardLoginUpstream struct {
	token string
}

func (u cursorDashboardLoginUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	header := http.Header{"Content-Type": []string{"application/json"}}
	switch req.URL.Path {
	case "/auth/poll":
		body := `{"accessToken":"` + u.token + `","refreshToken":"refresh","authId":"auth"}`
		return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(body))}, nil
	case "/aiserver.v1.DashboardService/GetCurrentPeriodUsage":
		return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(`{"enabled":true,"planUsage":{"totalPercentUsed":1}}`))}, nil
	default:
		return &http.Response{StatusCode: http.StatusNotFound, Header: header, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	}
}

func (u cursorDashboardLoginUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}

func TestCursorDashboardAuthServiceStartAndPollPersistsTokensWithoutReturningThem(t *testing.T) {
	account := Account{ID: 77, Platform: PlatformCursor, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{"api_key": "key"}, Extra: map[string]any{}}
	repo := &cursorDashboardAccountRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}}}
	cfg := &config.Config{Cursor: config.CursorConfig{
		BaseURL:                      "https://api.cursor.com",
		DashboardBaseURL:             "https://api2.cursor.sh",
		DashboardAuthWebsiteURL:      "https://cursor.com",
		DashboardLoginSessionTTLMins: 5,
		RequestTimeoutSeconds:        5,
	}}
	token := makeCursorDashboardJWT(t, time.Now().Add(60*24*time.Hour))
	gateway := NewCursorGatewayService(cursorDashboardLoginUpstream{token: token}, nil, nil, nil, cfg)
	svc := NewCursorDashboardAuthService(repo, gateway, nil, cfg)

	start, err := svc.StartLogin(context.Background(), account.ID)
	require.NoError(t, err)
	require.NotEmpty(t, start.SessionID)
	require.Contains(t, start.AuthURL, "loginDeepControl")
	require.NotContains(t, start.AuthURL, "verifier")

	poll, err := svc.PollLogin(context.Background(), start.SessionID)
	require.NoError(t, err)
	require.Equal(t, "connected", poll.Status)
	require.Equal(t, account.ID, poll.AccountID)
	require.NotContains(t, poll.Message, token)
	require.Equal(t, token, repo.updatedCredentials["dashboard_access_token"])
	require.Equal(t, "refresh", repo.updatedCredentials["dashboard_refresh_token"])
	require.Equal(t, "connected", repo.updatedExtra[cursorDashboardAuthStateKey])
}
