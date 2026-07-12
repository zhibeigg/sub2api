package service

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
)

const (
	kiroTokenCacheSkew            = 5 * time.Minute
	kiroRequestRefreshTimeout     = 8 * time.Second
	kiroTokenProviderLogComponent = "kiro_token_provider"
)

// KiroTokenCache reuses the shared token-cache interface.
type KiroTokenCache = GeminiTokenCache

// KiroTokenProvider resolves a fresh Kiro access token on the request path.
type KiroTokenProvider struct {
	accountRepo      AccountRepository
	tokenCache       KiroTokenCache
	refreshAPI       *OAuthRefreshAPI
	executor         OAuthRefreshExecutor
	refreshPolicy    ProviderRefreshPolicy
	tempUnschedCache TempUnschedCache
}

// NewKiroTokenProvider constructs a KiroTokenProvider.
func NewKiroTokenProvider(accountRepo AccountRepository, tokenCache KiroTokenCache) *KiroTokenProvider {
	return &KiroTokenProvider{
		accountRepo:   accountRepo,
		tokenCache:    tokenCache,
		refreshPolicy: AntigravityProviderRefreshPolicy(),
	}
}

func (p *KiroTokenProvider) SetRefreshAPI(api *OAuthRefreshAPI, executor OAuthRefreshExecutor) {
	p.refreshAPI = api
	p.executor = executor
}

func (p *KiroTokenProvider) SetRefreshPolicy(policy ProviderRefreshPolicy) {
	p.refreshPolicy = policy
}

func (p *KiroTokenProvider) SetTempUnschedCache(cache TempUnschedCache) {
	p.tempUnschedCache = cache
}

// GetAccessToken returns a valid access token, refreshing if needed.
func (p *KiroTokenProvider) GetAccessToken(ctx context.Context, account *Account) (string, error) {
	if account == nil {
		return "", errors.New("account is nil")
	}
	if account.Platform != PlatformKiro || account.Type != AccountTypeOAuth {
		return "", errors.New("not a kiro oauth account")
	}

	cacheKey := KiroTokenCacheKey(account)
	if p.tokenCache != nil {
		if token, err := p.tokenCache.GetAccessToken(ctx, cacheKey); err == nil && strings.TrimSpace(token) != "" {
			return token, nil
		}
	}

	// Scheduler selections can carry a credential snapshot that predates a
	// background token refresh. On a cache miss, reload the persisted account
	// before evaluating expiry or attempting another refresh; otherwise a
	// refresh failure can incorrectly reject a still-valid latest access token.
	account = p.reloadLatestAccount(ctx, account)

	expiresAt := account.GetCredentialAsTime("expires_at")
	needsRefresh := expiresAt == nil || time.Until(*expiresAt) <= kiroTokenRefreshSkew
	if needsRefresh && strings.TrimSpace(account.GetCredential("refresh_token")) == "" {
		if expiresAt == nil || !time.Now().Before(*expiresAt) {
			return "", errors.New("kiro access_token expired and refresh_token is missing")
		}
		needsRefresh = false
	}
	if needsRefresh && p.refreshAPI != nil && p.executor != nil {
		refreshCtx, cancel := context.WithTimeout(ctx, kiroRequestRefreshTimeout)
		defer cancel()
		result, err := p.refreshAPI.RefreshIfNeeded(refreshCtx, account, p.executor, kiroTokenRefreshSkew)
		if err != nil {
			// RefreshIfNeeded re-reads and may race with another credential update.
			// Re-read once more after the failed refresh so the fallback decision and
			// the token returned below are based on the latest persisted credentials.
			account = p.reloadLatestAccount(ctx, account)
			expiresAt = account.GetCredentialAsTime("expires_at")
			// Kiro access tokens can remain usable even after the locally recorded
			// expires_at and while the refresh endpoint rejects an outdated/rotated
			// refresh token. Kiro-Go sends the current access token and lets the
			// streaming upstream make the authoritative auth decision, so do the
			// same here instead of failing before the request reaches Kiro.
			if canUseKiroAccessTokenAfterRefreshFailure(account) {
				slog.Warn(kiroTokenProviderLogComponent+".refresh_failed_using_existing_token",
					"account_id", account.ID,
					"expires_at", expiresAt,
					"error", logredact.RedactText(err.Error()),
				)
			} else {
				p.markTempUnschedulable(account, err)
				if p.refreshPolicy.OnRefreshError == ProviderRefreshErrorReturn {
					return "", err
				}
			}
		} else if !result.LockHeld && result.Account != nil {
			account = result.Account
			expiresAt = account.GetCredentialAsTime("expires_at")
		}
	}

	accessToken := account.GetCredential("access_token")
	if strings.TrimSpace(accessToken) == "" {
		return "", errors.New("access_token not found in credentials")
	}

	if p.tokenCache != nil {
		latestAccount, isStale := CheckTokenVersion(ctx, account, p.accountRepo)
		if isStale && latestAccount != nil {
			account = latestAccount
			accessToken = latestAccount.GetCredential("access_token")
			expiresAt = latestAccount.GetCredentialAsTime("expires_at")
			if strings.TrimSpace(accessToken) == "" {
				return "", errors.New("access_token not found after version check")
			}
		}
		p.cacheAccessToken(ctx, cacheKey, accessToken, expiresAt)
	}

	return accessToken, nil
}

// ForceRefreshAccessToken refreshes a Kiro token after the upstream rejects the
// cached token with HTTP 401. It bypasses the normal expiry check and returns the
// freshly persisted token so the gateway can retry once on the same account.
func (p *KiroTokenProvider) ForceRefreshAccessToken(ctx context.Context, account *Account) (string, error) {
	if account == nil {
		return "", errors.New("account is nil")
	}
	if account.Platform != PlatformKiro || account.Type != AccountTypeOAuth {
		return "", errors.New("not a kiro oauth account")
	}
	if p.refreshAPI == nil || p.executor == nil {
		return "", errors.New("kiro token refresh is not configured")
	}
	if strings.TrimSpace(account.GetCredential("refresh_token")) == "" {
		return "", errors.New("kiro refresh_token is missing")
	}

	cacheKey := KiroTokenCacheKey(account)
	if p.tokenCache != nil {
		_ = p.tokenCache.DeleteAccessToken(ctx, cacheKey)
	}

	refreshCtx, cancel := context.WithTimeout(ctx, kiroRequestRefreshTimeout)
	defer cancel()
	result, err := p.refreshAPI.ForceRefresh(refreshCtx, account, p.executor)
	if err != nil {
		p.markTempUnschedulable(account, err)
		return "", err
	}

	refreshedAccount := account
	if result != nil && result.Account != nil {
		refreshedAccount = result.Account
	}
	if result != nil && result.LockHeld {
		if p.accountRepo == nil {
			return "", errors.New("kiro token refresh is already in progress")
		}
		latestAccount, readErr := p.accountRepo.GetByID(ctx, account.ID)
		if readErr != nil || latestAccount == nil {
			return "", errors.New("kiro token refresh is already in progress")
		}
		refreshedAccount = latestAccount
		if refreshedAccount.GetCredential("access_token") == account.GetCredential("access_token") {
			return "", errors.New("kiro token refresh is already in progress")
		}
	}

	accessToken := strings.TrimSpace(refreshedAccount.GetCredential("access_token"))
	if accessToken == "" {
		return "", errors.New("access_token not found after forced refresh")
	}
	if p.tokenCache != nil {
		p.cacheAccessToken(ctx, cacheKey, accessToken, refreshedAccount.GetCredentialAsTime("expires_at"))
	}
	return accessToken, nil
}

func (p *KiroTokenProvider) reloadLatestAccount(ctx context.Context, account *Account) *Account {
	if p == nil || p.accountRepo == nil || account == nil {
		return account
	}
	latestAccount, err := p.accountRepo.GetByID(ctx, account.ID)
	if err != nil {
		slog.Warn(kiroTokenProviderLogComponent+".latest_account_reload_failed",
			"account_id", account.ID,
			"error", logredact.RedactText(err.Error()),
		)
		return account
	}
	if latestAccount == nil {
		return account
	}
	return latestAccount
}

func (p *KiroTokenProvider) cacheAccessToken(ctx context.Context, cacheKey, accessToken string, expiresAt *time.Time) {
	if p == nil || p.tokenCache == nil || strings.TrimSpace(accessToken) == "" {
		return
	}

	ttl := 30 * time.Minute
	if expiresAt != nil {
		until := time.Until(*expiresAt)
		// Do not cache tokens inside the safety window. A cache hit bypasses the
		// expiry check above, so caching here until the literal expiry would prevent
		// the request path from refreshing a token that Kiro may already reject.
		if until <= kiroTokenCacheSkew {
			return
		}
		ttl = until - kiroTokenCacheSkew
	}
	if ttl <= 0 {
		return
	}
	_ = p.tokenCache.SetAccessToken(ctx, cacheKey, accessToken, ttl)
}

func canUseKiroAccessTokenAfterRefreshFailure(account *Account) bool {
	return account != nil && strings.TrimSpace(account.GetCredential("access_token")) != ""
}

func (p *KiroTokenProvider) markTempUnschedulable(account *Account, refreshErr error) {
	if p == nil || p.accountRepo == nil || account == nil {
		return
	}
	now := time.Now()
	until := now.Add(tokenRefreshTempUnschedDuration)
	redactedErr := "unknown error"
	if refreshErr != nil {
		redactedErr = logredact.RedactText(refreshErr.Error())
	}
	if isNonRetryableRefreshError(refreshErr) {
		if err := p.accountRepo.SetError(context.Background(), account.ID, "kiro token refresh failed (non-retryable): "+redactedErr); err != nil {
			slog.Warn(kiroTokenProviderLogComponent+".set_error_status_failed", "account_id", account.ID, "error", err)
		}
		return
	}
	reason := "kiro token refresh failed on request path: " + redactedErr
	bgCtx := context.Background()
	if err := p.accountRepo.SetTempUnschedulable(bgCtx, account.ID, until, reason); err != nil {
		slog.Warn(kiroTokenProviderLogComponent+".set_temp_unschedulable_failed", "account_id", account.ID, "error", err)
		return
	}
	if p.tempUnschedCache != nil {
		state := &TempUnschedState{
			UntilUnix:       until.Unix(),
			TriggeredAtUnix: now.Unix(),
			ErrorMessage:    "token_refresh_failed: " + reason,
		}
		if err := p.tempUnschedCache.SetTempUnsched(bgCtx, account.ID, state); err != nil {
			slog.Warn(kiroTokenProviderLogComponent+".temp_unsched_cache_set_failed", "account_id", account.ID, "error", err)
		}
	}
}

// KiroTokenCacheKey returns the token-cache key for a Kiro account.
func KiroTokenCacheKey(account *Account) string {
	if account == nil {
		return "kiro:account:0"
	}
	return "kiro:account:" + strconv.FormatInt(account.ID, 10)
}
