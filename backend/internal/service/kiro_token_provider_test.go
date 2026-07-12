//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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

func TestKiroTokenProvider_RefreshFailureStillRejectsExpiredAccessToken(t *testing.T) {
	expiresAt := time.Now().Add(-time.Minute)
	account := &Account{
		ID:       143,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":  "expired-access-token",
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
