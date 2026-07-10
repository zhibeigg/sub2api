package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/provider/adobe/firefly"
	"github.com/stretchr/testify/require"
)

func TestAdobePlatformCapabilities(t *testing.T) {
	t.Parallel()
	require.NoError(t, ValidatePlatformAccountType(PlatformAdobe, AccountTypeOAuth))
	require.Error(t, ValidatePlatformAccountType(PlatformAdobe, AccountTypeAPIKey))
	require.True(t, PlatformSupportsImageGeneration(PlatformAdobe))
	require.True(t, PlatformSupportsVideoGeneration(PlatformAdobe))
	require.False(t, PlatformSupportsBatchImageGeneration(PlatformAdobe))
	require.False(t, PlatformSupportsUpstreamModelSync(PlatformAdobe))
	require.Equal(t, 1, DefaultAccountConcurrency(PlatformAdobe))
	require.False(t, IsMixedSchedulingCapablePlatform(PlatformAdobe))
	require.False(t, GroupPlatformSupportsMixedScheduling(PlatformAdobe))
	require.Contains(t, AllowedQuotaPlatforms, PlatformAdobe)
}

func TestAdobeAccountDefaultMappingUsesPublicCatalog(t *testing.T) {
	t.Parallel()
	account := &Account{Platform: PlatformAdobe, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "token"}}
	mapping := account.GetModelMapping()
	for _, modelID := range firefly.PublicModelIDs() {
		require.Equal(t, modelID, mapping[modelID])
		require.True(t, account.IsModelSupported(modelID))
	}
	require.False(t, account.IsModelSupported("claude-sonnet-4-6"))
}

type adobeFailureAccountRepoStub struct {
	AccountRepository
	rateLimitedUntil *time.Time
	tempUntil        *time.Time
	overloadUntil    *time.Time
}

func (s *adobeFailureAccountRepoStub) SetRateLimited(_ context.Context, _ int64, until time.Time) error {
	s.rateLimitedUntil = &until
	return nil
}

func (s *adobeFailureAccountRepoStub) SetTempUnschedulable(_ context.Context, _ int64, until time.Time, _ string) error {
	s.tempUntil = &until
	return nil
}

func (s *adobeFailureAccountRepoStub) SetOverloaded(_ context.Context, _ int64, until time.Time) error {
	s.overloadUntil = &until
	return nil
}

func TestHandleAdobeAccountFailure_OnlyMarksFailoverSafeErrors(t *testing.T) {
	repo := &adobeFailureAccountRepoStub{}
	svc := &OpenAIGatewayService{accountRepo: repo}

	svc.HandleAdobeAccountFailure(context.Background(), 7, &firefly.ProviderError{Kind: firefly.ErrorRequest, HTTPStatus: 400})
	require.Nil(t, repo.rateLimitedUntil)
	require.Nil(t, repo.tempUntil)
	require.Nil(t, repo.overloadUntil)

	svc.HandleAdobeAccountFailure(context.Background(), 7, &firefly.ProviderError{Kind: firefly.ErrorRateLimited, RetryAfter: 2 * time.Minute})
	require.NotNil(t, repo.rateLimitedUntil)
	require.WithinDuration(t, time.Now().Add(2*time.Minute), *repo.rateLimitedUntil, 2*time.Second)

	svc.HandleAdobeAccountFailure(context.Background(), 7, &firefly.ProviderError{Kind: firefly.ErrorAuth})
	require.NotNil(t, repo.tempUntil)

	svc.HandleAdobeAccountFailure(context.Background(), 7, &firefly.ProviderError{Kind: firefly.ErrorTemporary})
	require.NotNil(t, repo.overloadUntil)
}
