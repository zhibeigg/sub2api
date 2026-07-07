package service

import (
	"context"
	"errors"
	"strings"
	"time"
)

const kiroTokenRefreshSkew = 2 * time.Minute

// KiroTokenRefresher implements TokenRefresher for Kiro OAuth accounts.
type KiroTokenRefresher struct {
	kiroOAuthService KiroOAuthTokenService
}

// NewKiroTokenRefresher constructs a KiroTokenRefresher.
func NewKiroTokenRefresher(kiroOAuthService KiroOAuthTokenService) *KiroTokenRefresher {
	return &KiroTokenRefresher{kiroOAuthService: kiroOAuthService}
}

func (r *KiroTokenRefresher) CacheKey(account *Account) string {
	return KiroTokenCacheKey(account)
}

func (r *KiroTokenRefresher) CanRefresh(account *Account) bool {
	return account != nil && account.Platform == PlatformKiro && account.Type == AccountTypeOAuth
}

func (r *KiroTokenRefresher) NeedsRefresh(account *Account, refreshWindow time.Duration) bool {
	if account == nil || strings.TrimSpace(account.GetCredential("refresh_token")) == "" {
		return false
	}
	expiresAt := account.GetCredentialAsTime("expires_at")
	if expiresAt == nil {
		return true
	}
	if refreshWindow < kiroTokenRefreshSkew {
		refreshWindow = kiroTokenRefreshSkew
	}
	return time.Until(*expiresAt) < refreshWindow
}

func (r *KiroTokenRefresher) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	if r == nil || r.kiroOAuthService == nil {
		return nil, errors.New("kiro oauth service is not configured")
	}
	tokenInfo, err := r.kiroOAuthService.RefreshAccountToken(ctx, account)
	if err != nil {
		return nil, err
	}
	newCredentials := r.kiroOAuthService.BuildAccountCredentials(tokenInfo)
	newCredentials = MergeCredentials(account.Credentials, newCredentials)
	return newCredentials, nil
}
