package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	cursorpkg "github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
	"github.com/google/uuid"
)

const (
	cursorDashboardAuthStateKey       = "cursor_dashboard_auth_state"
	cursorDashboardTokenExpiresAtKey  = "cursor_dashboard_token_expires_at"
	cursorDashboardLastVerifiedAtKey  = "cursor_dashboard_last_verified_at"
	cursorDashboardLastRefreshedAtKey = "cursor_dashboard_last_refreshed_at"
	cursorDashboardLastErrorCodeKey   = "cursor_dashboard_last_error_code"
)

var errCursorDashboardReauthRequired = errors.New("cursor dashboard session requires reauthorization")

type cursorDashboardLoginSession struct {
	AccountID int64
	Flow      *cursorpkg.DashboardLoginPKCE
	ExpiresAt time.Time
}

type CursorDashboardLoginStart struct {
	SessionID string    `json:"session_id"`
	AuthURL   string    `json:"auth_url"`
	ExpiresAt time.Time `json:"expires_at"`
}

type CursorDashboardLoginPoll struct {
	Status    string     `json:"status"`
	AccountID int64      `json:"account_id,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Message   string     `json:"message,omitempty"`
}

type CursorDashboardSessionInfo struct {
	State           string     `json:"state"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	LastVerifiedAt  *time.Time `json:"last_verified_at,omitempty"`
	LastRefreshedAt *time.Time `json:"last_refreshed_at,omitempty"`
	ErrorCode       string     `json:"error_code,omitempty"`
}

type CursorDashboardAuthService struct {
	accountRepo AccountRepository
	gateway     *CursorGatewayService
	refreshAPI  *OAuthRefreshAPI
	refresher   *CursorDashboardTokenRefresher
	cfg         *config.Config
	sessions    sync.Map
}

func NewCursorDashboardAuthService(accountRepo AccountRepository, gateway *CursorGatewayService, refreshAPI *OAuthRefreshAPI, cfg *config.Config) *CursorDashboardAuthService {
	service := &CursorDashboardAuthService{accountRepo: accountRepo, gateway: gateway, refreshAPI: refreshAPI, cfg: cfg}
	service.refresher = NewCursorDashboardTokenRefresher(gateway, cfg)
	if gateway != nil {
		gateway.SetDashboardAuthService(service)
	}
	return service
}

func (s *CursorDashboardAuthService) StartLogin(ctx context.Context, accountID int64) (*CursorDashboardLoginStart, error) {
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("get Cursor account: %w", err)
	}
	if account == nil || !account.IsCursorAPIKey() {
		return nil, fmt.Errorf("a Cursor API key account is required")
	}
	client, err := s.gateway.newDashboardAuthClient(ctx, account)
	if err != nil {
		return nil, err
	}
	flow, err := cursorpkg.GenerateDashboardLoginPKCE()
	if err != nil {
		return nil, err
	}
	authURL, err := client.BuildLoginURL(flow, "login")
	if err != nil {
		return nil, err
	}
	ttl := 5 * time.Minute
	if s.cfg != nil && s.cfg.Cursor.DashboardLoginSessionTTLMins > 0 {
		ttl = time.Duration(s.cfg.Cursor.DashboardLoginSessionTTLMins) * time.Minute
	}
	expiresAt := time.Now().UTC().Add(ttl)
	sessionID := uuid.NewString()
	s.sessions.Store(sessionID, &cursorDashboardLoginSession{AccountID: accountID, Flow: flow, ExpiresAt: expiresAt})
	s.cleanupExpiredSessions(time.Now().UTC())
	return &CursorDashboardLoginStart{SessionID: sessionID, AuthURL: authURL, ExpiresAt: expiresAt}, nil
}

func (s *CursorDashboardAuthService) PollLogin(ctx context.Context, sessionID string) (*CursorDashboardLoginPoll, error) {
	sessionID = strings.TrimSpace(sessionID)
	value, ok := s.sessions.Load(sessionID)
	if !ok {
		return &CursorDashboardLoginPoll{Status: "expired", Message: "login session does not exist or has expired"}, nil
	}
	session, ok := value.(*cursorDashboardLoginSession)
	if !ok || session == nil {
		s.sessions.Delete(sessionID)
		return &CursorDashboardLoginPoll{Status: "expired"}, nil
	}
	if !time.Now().UTC().Before(session.ExpiresAt) {
		s.sessions.Delete(sessionID)
		return &CursorDashboardLoginPoll{Status: "expired", AccountID: session.AccountID}, nil
	}
	account, err := s.accountRepo.GetByID(ctx, session.AccountID)
	if err != nil {
		return nil, fmt.Errorf("get Cursor account: %w", err)
	}
	if account == nil || !account.IsCursorAPIKey() {
		s.sessions.Delete(sessionID)
		return nil, fmt.Errorf("cursor account is unavailable")
	}
	client, err := s.gateway.newDashboardAuthClient(ctx, account)
	if err != nil {
		return nil, err
	}
	polled, err := client.PollLogin(ctx, session.Flow)
	if err != nil {
		return nil, err
	}
	if polled.Pending {
		return &CursorDashboardLoginPoll{Status: "pending", AccountID: account.ID, ExpiresAt: &session.ExpiresAt}, nil
	}
	metadata, err := cursorpkg.ParseDashboardTokenMetadata(polled.AccessToken)
	if err != nil || !metadata.ExpiresAt.After(time.Now().UTC()) {
		s.sessions.Delete(sessionID)
		return nil, fmt.Errorf("cursor returned an invalid or expired Dashboard token")
	}
	dashboardClient, err := s.gateway.newDashboardClient(ctx, account, polled.AccessToken)
	if err != nil {
		return nil, err
	}
	usage, err := dashboardClient.FetchUsage(ctx)
	if err != nil {
		return nil, fmt.Errorf("verify Cursor Dashboard session: %w", err)
	}

	credentials := shallowCopyMap(account.Credentials)
	credentials["dashboard_access_token"] = polled.AccessToken
	credentials["dashboard_refresh_token"] = polled.RefreshToken
	// Cursor binds IDE chat request checksums to the UUID used by the PKCE
	// login flow. Model discovery does not enforce this binding, so omitting it
	// produces a misleading state where AvailableModels works but chat is
	// rejected as an unsupported client.
	credentials["cursor_machine_id"] = session.Flow.UUID
	credentials["_token_version"] = time.Now().UnixMilli()
	if err := persistAccountCredentials(ctx, s.accountRepo, account, credentials); err != nil {
		return nil, fmt.Errorf("persist Cursor Dashboard session: %w", err)
	}
	account.Credentials = credentials
	s.gateway.PrewarmIDEModelCatalog(account)
	now := time.Now().UTC()
	updates := cursorPlanUsageSnapshotUpdates(cursorPlanUsageFromDashboard(usage, now))
	if updates == nil {
		updates = map[string]any{}
	}
	updates[cursorDashboardAuthStateKey] = "connected"
	updates[cursorDashboardTokenExpiresAtKey] = metadata.ExpiresAt.Format(time.RFC3339)
	updates[cursorDashboardLastVerifiedAtKey] = now.Format(time.RFC3339)
	updates[cursorDashboardLastRefreshedAtKey] = now.Format(time.RFC3339)
	updates[cursorDashboardLastErrorCodeKey] = nil
	if err := s.accountRepo.UpdateExtra(ctx, account.ID, updates); err != nil {
		return nil, fmt.Errorf("persist Cursor Dashboard metadata: %w", err)
	}
	s.sessions.Delete(sessionID)
	return &CursorDashboardLoginPoll{Status: "connected", AccountID: account.ID, ExpiresAt: &metadata.ExpiresAt}, nil
}

func (s *CursorDashboardAuthService) FetchDashboardUsage(ctx context.Context, account *Account) (*CursorDashboardUsageResult, error) {
	if account == nil || !account.IsCursorAPIKey() {
		return nil, fmt.Errorf("a Cursor API key account is required")
	}
	accessToken := strings.TrimSpace(account.GetCredential("dashboard_access_token"))
	if accessToken == "" {
		return nil, fmt.Errorf("cursor Dashboard access token is missing")
	}
	client, err := s.gateway.newDashboardClient(ctx, account, accessToken)
	if err != nil {
		return nil, err
	}
	usage, err := client.FetchUsage(ctx)
	if err == nil {
		s.markVerified(ctx, account, accessToken, false)
		return &CursorDashboardUsageResult{Usage: usage}, nil
	}
	if !cursorpkg.IsKind(err, cursorpkg.ErrorUnauthorized) {
		s.markError(ctx, account.ID, "network_error")
		return nil, err
	}
	refreshedAccount, refreshErr := s.forceRefresh(ctx, account)
	if refreshErr != nil {
		s.markError(ctx, account.ID, "reauth_required")
		return nil, refreshErr
	}
	newAccessToken := strings.TrimSpace(refreshedAccount.GetCredential("dashboard_access_token"))
	retryClient, err := s.gateway.newDashboardClient(ctx, refreshedAccount, newAccessToken)
	if err != nil {
		return nil, err
	}
	usage, err = retryClient.FetchUsage(ctx)
	if err != nil {
		if cursorpkg.IsKind(err, cursorpkg.ErrorUnauthorized) {
			s.markError(ctx, account.ID, "reauth_required")
			return nil, errCursorDashboardReauthRequired
		}
		return nil, err
	}
	s.markVerified(ctx, refreshedAccount, newAccessToken, true)
	return &CursorDashboardUsageResult{Usage: usage}, nil
}

func (s *CursorDashboardAuthService) RefreshIfNeeded(ctx context.Context, account *Account) (*Account, bool, error) {
	if account == nil || !s.refresher.CanRefresh(account) || !s.refresher.NeedsRefresh(account, 0) {
		return account, false, nil
	}
	if s.refreshAPI == nil {
		credentials, err := s.refresher.Refresh(ctx, account)
		if err != nil {
			return account, false, err
		}
		if err := persistAccountCredentials(ctx, s.accountRepo, account, credentials); err != nil {
			return account, false, err
		}
		account.Credentials = credentials
		return account, true, nil
	}
	result, err := s.refreshAPI.RefreshIfNeeded(ctx, account, s.refresher, 0)
	if err != nil {
		return account, false, err
	}
	if result.Account != nil {
		account = result.Account
	}
	return account, result.Refreshed, nil
}

func (s *CursorDashboardAuthService) forceRefresh(ctx context.Context, account *Account) (*Account, error) {
	if strings.TrimSpace(account.GetCredential("dashboard_refresh_token")) == "" {
		return account, errCursorDashboardReauthRequired
	}
	if s.refreshAPI == nil {
		credentials, err := s.refresher.Refresh(ctx, account)
		if err != nil {
			return account, err
		}
		if err := persistAccountCredentials(ctx, s.accountRepo, account, credentials); err != nil {
			return account, err
		}
		account.Credentials = credentials
		return account, nil
	}
	result, err := s.refreshAPI.ForceRefresh(ctx, account, s.refresher)
	if err != nil {
		return account, err
	}
	if result.Account != nil {
		account = result.Account
	}
	fresh, err := s.accountRepo.GetByID(ctx, account.ID)
	if err == nil && fresh != nil {
		account = fresh
	}
	return account, nil
}

func (s *CursorDashboardAuthService) SessionInfo(account *Account) *CursorDashboardSessionInfo {
	return cursorDashboardSessionInfoFromAccount(account)
}

func cursorDashboardSessionInfoFromAccount(account *Account) *CursorDashboardSessionInfo {
	if account == nil {
		return &CursorDashboardSessionInfo{State: "missing"}
	}
	if strings.TrimSpace(account.GetCredential("dashboard_access_token")) == "" {
		return &CursorDashboardSessionInfo{State: "missing"}
	}
	state, _ := account.Extra[cursorDashboardAuthStateKey].(string)
	state = strings.TrimSpace(state)
	if state == "" {
		state = "connected"
	}
	errorCode, _ := account.Extra[cursorDashboardLastErrorCodeKey].(string)
	return &CursorDashboardSessionInfo{
		State:           state,
		ExpiresAt:       extraTimePointer(account.Extra[cursorDashboardTokenExpiresAtKey]),
		LastVerifiedAt:  extraTimePointer(account.Extra[cursorDashboardLastVerifiedAtKey]),
		LastRefreshedAt: extraTimePointer(account.Extra[cursorDashboardLastRefreshedAtKey]),
		ErrorCode:       strings.TrimSpace(errorCode),
	}
}

func (s *CursorDashboardAuthService) markVerified(ctx context.Context, account *Account, token string, refreshed bool) {
	now := time.Now().UTC()
	updates := map[string]any{
		cursorDashboardAuthStateKey:      "connected",
		cursorDashboardLastVerifiedAtKey: now.Format(time.RFC3339),
		cursorDashboardLastErrorCodeKey:  nil,
	}
	if metadata, err := cursorpkg.ParseDashboardTokenMetadata(token); err == nil {
		updates[cursorDashboardTokenExpiresAtKey] = metadata.ExpiresAt.Format(time.RFC3339)
	}
	if refreshed {
		updates[cursorDashboardLastRefreshedAtKey] = now.Format(time.RFC3339)
	}
	_ = s.accountRepo.UpdateExtra(ctx, account.ID, updates)
}

func (s *CursorDashboardAuthService) markError(ctx context.Context, accountID int64, code string) {
	state := "error"
	if code == "reauth_required" {
		state = "reauth_required"
	}
	_ = s.accountRepo.UpdateExtra(ctx, accountID, map[string]any{
		cursorDashboardAuthStateKey:     state,
		cursorDashboardLastErrorCodeKey: code,
	})
}

func (s *CursorDashboardAuthService) cleanupExpiredSessions(now time.Time) {
	s.sessions.Range(func(key, value any) bool {
		session, ok := value.(*cursorDashboardLoginSession)
		if !ok || session == nil || !now.Before(session.ExpiresAt) {
			s.sessions.Delete(key)
		}
		return true
	})
}

type CursorDashboardTokenRefresher struct {
	gateway *CursorGatewayService
	cfg     *config.Config
}

func NewCursorDashboardTokenRefresher(gateway *CursorGatewayService, cfg *config.Config) *CursorDashboardTokenRefresher {
	return &CursorDashboardTokenRefresher{gateway: gateway, cfg: cfg}
}

func (r *CursorDashboardTokenRefresher) CacheKey(account *Account) string {
	if account == nil {
		return "cursor-dashboard:unknown"
	}
	return fmt.Sprintf("cursor-dashboard:%d", account.ID)
}

func (r *CursorDashboardTokenRefresher) RefreshTokenCredentialKey() string {
	return "dashboard_refresh_token"
}

func (r *CursorDashboardTokenRefresher) CanRefresh(account *Account) bool {
	return account != nil && account.IsCursorAPIKey() && strings.TrimSpace(account.GetCredential("dashboard_refresh_token")) != ""
}

func (r *CursorDashboardTokenRefresher) NeedsRefresh(account *Account, _ time.Duration) bool {
	if !r.CanRefresh(account) {
		return false
	}
	metadata, err := cursorpkg.ParseDashboardTokenMetadata(account.GetCredential("dashboard_access_token"))
	if err != nil {
		return true
	}
	window := 1272 * time.Hour
	if r.cfg != nil && r.cfg.Cursor.DashboardRefreshBeforeExpiryHours > 0 {
		window = time.Duration(r.cfg.Cursor.DashboardRefreshBeforeExpiryHours) * time.Hour
	}
	return time.Now().UTC().Add(window).After(metadata.ExpiresAt)
}

func (r *CursorDashboardTokenRefresher) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	if !r.CanRefresh(account) {
		return nil, fmt.Errorf("cursor Dashboard refresh token is missing")
	}
	client, err := r.gateway.newDashboardAuthClient(ctx, account)
	if err != nil {
		return nil, err
	}
	result, err := client.RefreshAccessToken(ctx, account.GetCredential("dashboard_refresh_token"))
	if err != nil {
		return nil, err
	}
	if result.ShouldLogout {
		return nil, errCursorDashboardReauthRequired
	}
	credentials := shallowCopyMap(account.Credentials)
	credentials["dashboard_access_token"] = result.AccessToken
	credentials["dashboard_refresh_token"] = result.RefreshToken
	return credentials, nil
}
