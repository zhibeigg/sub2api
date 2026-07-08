package service

import (
	"context"
	"net/http"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/google/uuid"
)

// Kiro interactive-login session lifetimes.
const (
	kiroDeviceSessionTTL = 15 * time.Minute
	kiroSSOSessionTTL    = 10 * time.Minute
	kiroSSOImportTimeout = 2 * time.Minute
)

// kiroLoginSession holds the transient state for an in-flight interactive login.
type kiroLoginSession struct {
	kind         string // "builderid" | "iamsso"
	clientID     string
	clientSecret string
	deviceCode   string // builderid
	codeVerifier string // iamsso
	state        string // iamsso
	redirectURI  string // iamsso
	region       string
	interval     int
	proxyURL     string
	expiresAt    time.Time
}

// kiroSessionStore is a TTL-bounded in-memory store for interactive login
// sessions. It runs a background reaper started lazily on first Set.
type kiroSessionStore struct {
	mu       sync.Mutex
	sessions map[string]*kiroLoginSession
}

func newKiroSessionStore() *kiroSessionStore {
	return &kiroSessionStore{sessions: make(map[string]*kiroLoginSession)}
}

func (s *kiroSessionStore) set(id string, sess *kiroLoginSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[id] = sess
	s.reapLocked()
}

func (s *kiroSessionStore) get(id string) (*kiroLoginSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return nil, false
	}
	if time.Now().After(sess.expiresAt) {
		delete(s.sessions, id)
		return nil, false
	}
	return sess, true
}

func (s *kiroSessionStore) delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

func (s *kiroSessionStore) reapLocked() {
	now := time.Now()
	for id, sess := range s.sessions {
		if now.After(sess.expiresAt) {
			delete(s.sessions, id)
		}
	}
}

// KiroDeviceLoginResult is returned when a device-code login starts.
type KiroDeviceLoginResult struct {
	SessionID       string `json:"session_id"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	Interval        int    `json:"interval"`
	ExpiresIn       int    `json:"expires_in"`
}

// KiroAuthURLResult is returned when an IAM SSO authorize URL is generated.
type KiroAuthURLResult struct {
	SessionID string `json:"session_id"`
	AuthURL   string `json:"auth_url"`
	State     string `json:"state"`
	ExpiresIn int    `json:"expires_in"`
}

// KiroDevicePollResult reports device-poll progress.
type KiroDevicePollResult struct {
	Status    string         `json:"status"` // "pending" | "completed"
	TokenInfo *KiroTokenInfo `json:"-"`
}

// StartBuilderIDLogin begins the AWS Builder ID device-code flow.
func (s *KiroOAuthService) StartBuilderIDLogin(ctx context.Context, region string, proxyID *int64) (*KiroDeviceLoginResult, error) {
	proxyURL, err := s.proxyURLByID(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	auth, err := kiro.StartBuilderIDLogin(ctx, region, proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "KIRO_BUILDERID_START_FAILED", "%v", err)
	}
	sessionID := uuid.NewString()
	s.sessions.set(sessionID, &kiroLoginSession{
		kind:         "builderid",
		clientID:     auth.ClientID,
		clientSecret: auth.ClientSecret,
		deviceCode:   auth.DeviceCode,
		region:       auth.Region,
		interval:     auth.Interval,
		proxyURL:     proxyURL,
		expiresAt:    time.Now().Add(kiroDeviceSessionTTL),
	})
	return &KiroDeviceLoginResult{
		SessionID:       sessionID,
		UserCode:        auth.UserCode,
		VerificationURI: auth.VerificationURI,
		Interval:        auth.Interval,
		ExpiresIn:       auth.ExpiresIn,
	}, nil
}

// PollBuilderIDLogin performs a single device-token poll. Returns status
// "pending" until authorized, then "completed" with a KiroTokenInfo.
func (s *KiroOAuthService) PollBuilderIDLogin(ctx context.Context, sessionID string) (*KiroDevicePollResult, error) {
	sess, ok := s.sessions.get(sessionID)
	if !ok || sess.kind != "builderid" {
		return nil, infraerrors.New(http.StatusBadRequest, "KIRO_BUILDERID_SESSION_NOT_FOUND", "login session not found or expired")
	}
	poll, err := kiro.PollDeviceToken(ctx, sess.region, sess.clientID, sess.clientSecret, sess.deviceCode, sess.proxyURL)
	if err != nil {
		s.sessions.delete(sessionID)
		return nil, infraerrors.Newf(http.StatusBadGateway, "KIRO_BUILDERID_POLL_FAILED", "%v", err)
	}
	if poll.Status == "pending" || poll.Status == "slow_down" {
		return &KiroDevicePollResult{Status: "pending"}, nil
	}
	s.sessions.delete(sessionID)
	info := newKiroTokenInfoFromLogin(poll.AccessToken, poll.RefreshToken, poll.ExpiresIn, sess.clientID, sess.clientSecret, "idc", "BuilderId", sess.region)
	return &KiroDevicePollResult{Status: "completed", TokenInfo: info}, nil
}

// StartIAMSSOLogin generates the IAM Identity Center authorize URL (PKCE).
func (s *KiroOAuthService) StartIAMSSOLogin(ctx context.Context, startURL, region string, proxyID *int64) (*KiroAuthURLResult, error) {
	proxyURL, err := s.proxyURLByID(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	res, err := kiro.StartIAMSSOLogin(ctx, startURL, region, proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "KIRO_IAMSSO_START_FAILED", "%v", err)
	}
	sessionID := uuid.NewString()
	s.sessions.set(sessionID, &kiroLoginSession{
		kind:         "iamsso",
		clientID:     res.ClientID,
		clientSecret: res.ClientSecret,
		codeVerifier: res.CodeVerifier,
		state:        res.State,
		redirectURI:  res.RedirectURI,
		region:       res.Region,
		proxyURL:     proxyURL,
		expiresAt:    time.Now().Add(kiroSSOSessionTTL),
	})
	return &KiroAuthURLResult{
		SessionID: sessionID,
		AuthURL:   res.AuthorizeURL,
		State:     res.State,
		ExpiresIn: res.ExpiresIn,
	}, nil
}

// CompleteIAMSSOLogin exchanges the callback (URL or raw code) for tokens.
func (s *KiroOAuthService) CompleteIAMSSOLogin(ctx context.Context, sessionID, callbackURL string) (*KiroTokenInfo, error) {
	sess, ok := s.sessions.get(sessionID)
	if !ok || sess.kind != "iamsso" {
		return nil, infraerrors.New(http.StatusBadRequest, "KIRO_IAMSSO_SESSION_NOT_FOUND", "login session not found or expired")
	}
	defer s.sessions.delete(sessionID)

	code, err := kiro.ParseAuthCodeCallback(callbackURL, sess.state)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadRequest, "KIRO_IAMSSO_INVALID_CALLBACK", "%v", err)
	}
	tok, err := kiro.ExchangeAuthCode(ctx, sess.region, sess.clientID, sess.clientSecret, code, sess.codeVerifier, sess.redirectURI, sess.proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "KIRO_IAMSSO_EXCHANGE_FAILED", "%v", err)
	}
	return newKiroTokenInfoFromLogin(tok.AccessToken, tok.RefreshToken, tok.ExpiresIn, sess.clientID, sess.clientSecret, "idc", "", sess.region), nil
}

// ImportFromSSOToken runs the SSO-token device-approval flow and returns tokens.
func (s *KiroOAuthService) ImportFromSSOToken(ctx context.Context, bearerToken, region string, proxyID *int64) (*KiroTokenInfo, error) {
	proxyURL, err := s.proxyURLByID(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	importCtx, cancel := context.WithTimeout(ctx, kiroSSOImportTimeout)
	defer cancel()

	res, err := kiro.ImportFromSSOToken(importCtx, bearerToken, region, proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "KIRO_SSO_IMPORT_FAILED", "%v", err)
	}
	return newKiroTokenInfoFromLogin(res.AccessToken, res.RefreshToken, res.ExpiresIn, res.ClientID, res.ClientSecret, "idc", "", res.Region), nil
}

func newKiroTokenInfoFromLogin(accessToken, refreshToken string, expiresIn int, clientID, clientSecret, authMethod, provider, region string) *KiroTokenInfo {
	var expiresAt int64
	if expiresIn > 0 {
		expiresAt = time.Now().Unix() + int64(expiresIn)
	}
	return &KiroTokenInfo{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		AuthMethod:   authMethod,
		Provider:     provider,
		Region:       region,
	}
}

func (s *KiroOAuthService) proxyURLByID(ctx context.Context, proxyID *int64) (string, error) {
	if proxyID == nil {
		return "", nil
	}
	if s.proxyRepo == nil {
		return "", infraerrors.New(http.StatusBadRequest, "KIRO_OAUTH_PROXY_NOT_AVAILABLE", "proxy repository is not available")
	}
	proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
	if err != nil {
		return "", infraerrors.Newf(http.StatusBadRequest, "KIRO_OAUTH_PROXY_NOT_FOUND", "proxy not found: %v", err)
	}
	if proxy == nil {
		return "", nil
	}
	return proxy.URL(), nil
}
