//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
)

type kiroTokenProviderRepoStub struct {
	refreshAPIAccountRepo
	tempUnschedCalls int
	setErrorCalls    int
}

func (r *kiroTokenProviderRepoStub) SetTempUnschedulable(_ context.Context, _ int64, _ time.Time, _ string) error {
	r.tempUnschedCalls++
	return nil
}

func (r *kiroTokenProviderRepoStub) SetError(_ context.Context, _ int64, _ string) error {
	r.setErrorCalls++
	return nil
}

func TestKiroTokenProvider_RefreshFailureUsesUnexpiredAccessToken(t *testing.T) {
	expiresAt := time.Now().Add(time.Minute)
	account := &Account{
		ID:       143,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":  "still-valid-access-token",
			"refresh_token": "rejected-refresh-token",
			"expires_at":    expiresAt.Format(time.RFC3339),
		},
	}
	repo := &kiroTokenProviderRepoStub{}
	repo.account = account
	cache := &refreshAPICacheStub{lockResult: true}
	executor := &refreshAPIExecutorStub{
		needsRefresh: true,
		err:          errors.New("OAuth 401: Invalid bearer token"),
	}
	provider := NewKiroTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), executor)

	token, err := provider.GetAccessToken(context.Background(), account)

	require.NoError(t, err)
	require.Equal(t, "still-valid-access-token", token)
	require.Equal(t, 1, executor.refreshCalls)
	require.Zero(t, repo.tempUnschedCalls)
	require.Zero(t, repo.setErrorCalls)
}

func TestKiroTokenProvider_ReloadsLatestAccountBeforeRefreshFallback(t *testing.T) {
	staleExpiresAt := time.Now().Add(-time.Minute)
	latestExpiresAt := time.Now().Add(time.Minute)
	staleAccount := &Account{
		ID:       143,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":  "stale-expired-access-token",
			"refresh_token": "stale-refresh-token",
			"expires_at":    staleExpiresAt.Format(time.RFC3339),
		},
	}
	latestAccount := &Account{
		ID:       143,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":  "latest-still-valid-access-token",
			"refresh_token": "rejected-refresh-token",
			"expires_at":    latestExpiresAt.Format(time.RFC3339),
		},
	}
	repo := &kiroTokenProviderRepoStub{}
	repo.account = latestAccount
	cache := &refreshAPICacheStub{lockResult: true}
	executor := &refreshAPIExecutorStub{
		needsRefresh: true,
		err:          errors.New("OAuth 401: Invalid bearer token"),
	}
	provider := NewKiroTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), executor)

	token, err := provider.GetAccessToken(context.Background(), staleAccount)

	require.NoError(t, err)
	require.Equal(t, "latest-still-valid-access-token", token)
	require.Equal(t, 1, executor.refreshCalls)
	require.Zero(t, repo.tempUnschedCalls)
	require.Zero(t, repo.setErrorCalls)
}

func TestKiroTokenProvider_RefreshFailureDefersExpiredTokenDecisionToUpstream(t *testing.T) {
	expiresAt := time.Now().Add(-time.Minute)
	account := &Account{
		ID:       143,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":  "locally-expired-access-token",
			"refresh_token": "rejected-refresh-token",
			"expires_at":    expiresAt.Format(time.RFC3339),
		},
	}
	repo := &kiroTokenProviderRepoStub{}
	repo.account = account
	cache := &refreshAPICacheStub{lockResult: true}
	executor := &refreshAPIExecutorStub{
		needsRefresh: true,
		err:          errors.New("OAuth 401: Invalid bearer token"),
	}
	provider := NewKiroTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), executor)

	token, err := provider.GetAccessToken(context.Background(), account)

	require.NoError(t, err)
	require.Equal(t, "locally-expired-access-token", token)
	require.Equal(t, 1, executor.refreshCalls)
	require.Zero(t, repo.tempUnschedCalls)
	require.Zero(t, repo.setErrorCalls)
}

func TestKiroTokenProvider_RefreshFailureStillRejectsMissingAccessToken(t *testing.T) {
	expiresAt := time.Now().Add(-time.Minute)
	account := &Account{
		ID:       143,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"refresh_token": "rejected-refresh-token",
			"expires_at":    expiresAt.Format(time.RFC3339),
		},
	}
	repo := &kiroTokenProviderRepoStub{}
	repo.account = account
	cache := &refreshAPICacheStub{lockResult: true}
	executor := &refreshAPIExecutorStub{
		needsRefresh: true,
		err:          errors.New("OAuth 401: Invalid bearer token"),
	}
	provider := NewKiroTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), executor)

	token, err := provider.GetAccessToken(context.Background(), account)

	require.Error(t, err)
	require.Empty(t, token)
	require.Equal(t, 1, executor.refreshCalls)
	require.Equal(t, 1, repo.tempUnschedCalls)
	require.Zero(t, repo.setErrorCalls)
}

func TestKiroTokenProvider_DoesNotCacheTokenInsideSafetyWindow(t *testing.T) {
	expiresAt := time.Now().Add(4 * time.Minute)
	account := &Account{
		ID:       143,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":  "near-expiry-access-token",
			"refresh_token": "refresh-token",
			"expires_at":    expiresAt.Format(time.RFC3339),
		},
	}
	repo := &kiroTokenProviderRepoStub{}
	repo.account = account
	cache := &refreshAPICacheStub{}
	provider := NewKiroTokenProvider(repo, cache)

	token, err := provider.GetAccessToken(context.Background(), account)

	require.NoError(t, err)
	require.Equal(t, "near-expiry-access-token", token)
	require.Zero(t, cache.setCalls)
}

func TestKiroGateway_Upstream401ForcesRefreshAndRetriesSameAccount(t *testing.T) {
	expiresAt := time.Now().Add(30 * time.Minute)
	account := &Account{
		ID:       143,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":  "rejected-access-token",
			"refresh_token": "refresh-token",
			"expires_at":    expiresAt.Format(time.RFC3339),
		},
	}
	repo := &kiroTokenProviderRepoStub{}
	repo.account = account
	cache := &refreshAPICacheStub{accessToken: "rejected-access-token", lockResult: true}
	executor := &refreshAPIExecutorStub{
		credentials: map[string]any{
			"access_token":  "refreshed-access-token",
			"refresh_token": "rotated-refresh-token",
			"expires_at":    time.Now().Add(time.Hour).Format(time.RFC3339),
		},
	}
	provider := NewKiroTokenProvider(repo, cache)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), executor)

	calls := 0
	gateway := &KiroGatewayService{
		tokenProvider: provider,
		callKiroAPI: func(_ context.Context, cred *kiro.Credential, _ *kiro.KiroPayload, _ *kiro.StreamCallback) error {
			calls++
			if cred.AccessToken == "rejected-access-token" {
				return &kiro.APIError{StatusCode: 401, Endpoint: "Kiro IDE", Body: "Unauthorized"}
			}
			require.Equal(t, "refreshed-access-token", cred.AccessToken)
			return nil
		},
	}
	cred := &kiro.Credential{AccessToken: "rejected-access-token"}

	err := gateway.callKiroWithAuthRetry(context.Background(), account, cred, &kiro.KiroPayload{}, &kiro.StreamCallback{})

	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.Equal(t, 1, executor.refreshCalls)
	require.Equal(t, 1, cache.deleteCalls)
	require.Equal(t, "refreshed-access-token", cache.lastSetToken)
	require.Equal(t, "refreshed-access-token", cred.AccessToken)
}
