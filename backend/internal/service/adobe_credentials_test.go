package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/provider/adobe/ims"
	"github.com/stretchr/testify/require"
)

func TestValidateAdobeAccountCredentials(t *testing.T) {
	t.Parallel()
	valid := []map[string]any{
		{"access_token": "token"},
		{"cookie": "cookie"},
		{"device_token": "device-token", "device_id": "device-id"},
		{"access_token": "token", "password": "recovery-only"},
	}
	for _, credentials := range valid {
		require.NoError(t, ValidateAdobeAccountCredentials(AccountTypeOAuth, credentials))
	}
	require.Error(t, ValidateAdobeAccountCredentials(AccountTypeAPIKey, map[string]any{"access_token": "token"}))
	require.Error(t, ValidateAdobeAccountCredentials(AccountTypeOAuth, map[string]any{"password": "not-a-source"}))
	require.Error(t, ValidateAdobeAccountCredentials(AccountTypeOAuth, map[string]any{"device_token": "unpaired"}))
	require.Error(t, ValidateAdobeAccountCredentials(AccountTypeOAuth, map[string]any{"device_id": "unpaired"}))
}

func TestMergeAccountCredentialsKeepReplaceClear(t *testing.T) {
	t.Parallel()
	existing := map[string]any{
		"access_token":  "old-token",
		"cookie":        "old-cookie",
		"device_token":  "old-device-token",
		"device_id":     "old-device-id",
		"model_mapping": map[string]any{"nano-banana": "nano-banana"},
	}
	merged, err := MergeAccountCredentials(existing, map[string]any{
		"access_token":  "   ",
		"cookie":        "new-cookie",
		"model_mapping": map[string]any{"veo3": "veo3"},
	}, []string{"device_token", "device_id"})
	require.NoError(t, err)
	require.Equal(t, "old-token", merged["access_token"])
	require.Equal(t, "new-cookie", merged["cookie"])
	require.NotContains(t, merged, "device_token")
	require.NotContains(t, merged, "device_id")
	require.Equal(t, map[string]any{"veo3": "veo3"}, merged["model_mapping"])
}

func TestMergeAccountCredentialsRejectsReplaceAndClear(t *testing.T) {
	t.Parallel()
	_, err := MergeAccountCredentials(map[string]any{"cookie": "old"}, map[string]any{"cookie": "new"}, []string{"cookie"})
	require.Error(t, err)
	_, err = MergeAccountCredentials(nil, nil, []string{"not_sensitive"})
	require.Error(t, err)
}

func TestNormalizeAdobeCredentialExpiryFromJWT(t *testing.T) {
	t.Parallel()
	exp := time.Now().Add(time.Hour).Unix()
	payload, err := json.Marshal(map[string]any{"exp": exp})
	require.NoError(t, err)
	credentials := map[string]any{"access_token": "x." + base64.RawURLEncoding.EncodeToString(payload) + ".y"}
	NormalizeAdobeCredentialExpiry(credentials)
	require.Equal(t, time.Unix(exp, 0).UTC().Format(time.RFC3339), credentials["expires_at"])
}

func TestAdobeCreditsInfoDistinguishesUnknownZeroAndPositive(t *testing.T) {
	t.Parallel()
	unknown := AdobeCreditsInfoFromExtra(nil)
	require.False(t, unknown.Known)
	require.Equal(t, "unknown", unknown.State)
	require.Nil(t, unknown.Available)

	zero := AdobeCreditsInfoFromExtra(map[string]any{"adobe_credits_known": true, "adobe_credits": 0.0})
	require.True(t, zero.Known)
	require.Equal(t, "zero", zero.State)
	require.NotNil(t, zero.Available)
	require.Zero(t, *zero.Available)

	positive := AdobeCreditsInfoFromExtra(map[string]any{"adobe_credits_known": true, "adobe_credits": 12.5})
	require.True(t, positive.Known)
	require.Equal(t, "available", positive.State)
	require.Equal(t, 12.5, *positive.Available)
}

type adobeTokenRepoStub struct {
	account      *Account
	getByIDCalls int
}

func (r *adobeTokenRepoStub) GetByID(context.Context, int64) (*Account, error) {
	r.getByIDCalls++
	return r.account, nil
}

func (r *adobeTokenRepoStub) Update(_ context.Context, account *Account) error {
	r.account = account
	return nil
}

func (r *adobeTokenRepoStub) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	if r.account.Extra == nil {
		r.account.Extra = make(map[string]any)
	}
	for key, value := range updates {
		r.account.Extra[key] = value
	}
	return nil
}

func TestAdobeTokenProviderUsesFreshTokenAndFiveMinuteSkew(t *testing.T) {
	t.Parallel()
	expiresAt := time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)
	account := &Account{ID: 91, Platform: PlatformAdobe, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "fresh-token", "expires_at": expiresAt}}
	repo := newAdobeTokenTestRepo(account)
	provider := NewAdobeTokenProvider(repo, nil, &config.Config{Adobe: config.AdobeConfig{TokenRefreshSkewSeconds: 300}})
	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "fresh-token", token)
	require.Zero(t, repo.getCalls.Load())
}

type adobeTokenTestRepo struct {
	account     *Account
	getCalls    atomic.Int32
	updateCalls atomic.Int32
	extraCalls  atomic.Int32
}

func newAdobeTokenTestRepo(account *Account) *adobeTokenTestRepo {
	return &adobeTokenTestRepo{account: account}
}

func (r *adobeTokenTestRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	r.getCalls.Add(1)
	if r.account == nil || r.account.ID != id {
		return nil, errors.New("account not found")
	}
	return r.account, nil
}

func (r *adobeTokenTestRepo) Update(ctx context.Context, account *Account) error {
	r.updateCalls.Add(1)
	return nil
}

func (r *adobeTokenTestRepo) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	r.extraCalls.Add(1)
	return nil
}

func TestAdobeTokenProviderUnknownExpirySemantics(t *testing.T) {
	t.Parallel()

	t.Run("access token without refresh source remains usable", func(t *testing.T) {
		account := &Account{ID: 92, Platform: PlatformAdobe, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "opaque-token"}}
		repo := newAdobeTokenTestRepo(account)
		provider := NewAdobeTokenProvider(repo, nil, &config.Config{})
		token, err := provider.GetAccessToken(context.Background(), account)
		require.NoError(t, err)
		require.Equal(t, "opaque-token", token)
		require.Zero(t, repo.getCalls.Load())
	})

	t.Run("unknown expiry with cookie refreshes", func(t *testing.T) {
		account := &Account{ID: 93, Platform: PlatformAdobe, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "opaque-token", "cookie": "refresh-cookie"}}
		repo := newAdobeTokenTestRepo(account)
		provider := NewAdobeTokenProvider(repo, nil, &config.Config{})
		var refreshCalls atomic.Int32
		provider.refreshViaCookie = func(_ context.Context, cookie string, _ ims.RefreshOptions) (*ims.FullResult, error) {
			refreshCalls.Add(1)
			require.Equal(t, "refresh-cookie", cookie)
			return &ims.FullResult{AccessToken: "refreshed-token", ExpiresAt: time.Now().Add(time.Hour).Unix(), Credits: 7}, nil
		}

		token, err := provider.GetAccessToken(context.Background(), account)
		require.NoError(t, err)
		require.Equal(t, "refreshed-token", token)
		require.Equal(t, int32(1), refreshCalls.Load())
		require.Equal(t, "refreshed-token", account.GetCredential("access_token"))
		require.NotNil(t, account.GetCredentialAsTime("expires_at"))
		require.Equal(t, 7.0, account.Extra["adobe_credits"])
		require.Equal(t, int32(1), repo.updateCalls.Load())
		require.Equal(t, int32(1), repo.extraCalls.Load())
	})
}

func TestAdobeTokenProviderDeviceThenCookieFallback(t *testing.T) {
	t.Parallel()
	account := &Account{ID: 94, Platform: PlatformAdobe, Type: AccountTypeOAuth, Credentials: map[string]any{
		"access_token": "expired-token", "expires_at": time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
		"device_token": "device-token", "device_id": "device-id", "cookie": "cookie",
	}}
	repo := newAdobeTokenTestRepo(account)
	provider := NewAdobeTokenProvider(repo, nil, &config.Config{})
	var deviceCalls atomic.Int32
	var cookieCalls atomic.Int32
	provider.refreshViaDevice = func(_ context.Context, deviceToken, deviceID string, _ ims.RefreshOptions) (*ims.FullResult, error) {
		deviceCalls.Add(1)
		require.Equal(t, "device-token", deviceToken)
		require.Equal(t, "device-id", deviceID)
		return nil, errors.New("device expired")
	}
	provider.refreshViaCookie = func(_ context.Context, cookie string, _ ims.RefreshOptions) (*ims.FullResult, error) {
		cookieCalls.Add(1)
		require.Equal(t, "cookie", cookie)
		return &ims.FullResult{AccessToken: "cookie-token", ExpiresAt: time.Now().Add(time.Hour).Unix(), Credits: -1}, nil
	}

	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "cookie-token", token)
	require.Equal(t, int32(1), deviceCalls.Load())
	require.Equal(t, int32(1), cookieCalls.Load())
}

func TestAdobeTokenProviderForceRefreshWithoutSourceIsExplicit(t *testing.T) {
	t.Parallel()
	account := &Account{ID: 95, Platform: PlatformAdobe, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "access-only"}}
	repo := newAdobeTokenTestRepo(account)
	provider := NewAdobeTokenProvider(repo, nil, &config.Config{})
	_, err := provider.ForceRefresh(context.Background(), account)
	require.ErrorContains(t, err, "no refresh source")
}

func TestAdobeTokenProviderInvalidationReloadsAccessOnlyCredential(t *testing.T) {
	t.Parallel()
	stale := &Account{ID: 951, Platform: PlatformAdobe, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "stale-token"}}
	current := &Account{ID: stale.ID, Platform: PlatformAdobe, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "replacement-token"}}
	repo := newAdobeTokenTestRepo(current)
	provider := NewAdobeTokenProvider(repo, nil, &config.Config{})
	invalidator := NewCompositeTokenCacheInvalidator(nil)
	invalidator.SetAdobeTokenProvider(provider)
	require.NoError(t, invalidator.InvalidateToken(context.Background(), stale))

	token, err := provider.GetAccessToken(context.Background(), stale)
	require.NoError(t, err)
	require.Equal(t, "replacement-token", token)
	require.Equal(t, "replacement-token", stale.GetCredential("access_token"))
	require.Equal(t, int32(1), repo.getCalls.Load())
	require.Zero(t, repo.updateCalls.Load())
}

func TestAdobeTokenProviderRefreshSingleflight(t *testing.T) {
	account := &Account{ID: 96, Platform: PlatformAdobe, Type: AccountTypeOAuth, Credentials: map[string]any{
		"access_token": "expired-token", "expires_at": time.Now().Add(-time.Minute).UTC().Format(time.RFC3339), "cookie": "cookie",
	}}
	repo := newAdobeTokenTestRepo(account)
	provider := NewAdobeTokenProvider(repo, nil, &config.Config{})
	var refreshCalls atomic.Int32
	refreshEntered := make(chan struct{})
	releaseRefresh := make(chan struct{})
	var enterOnce sync.Once
	provider.refreshViaCookie = func(_ context.Context, _ string, _ ims.RefreshOptions) (*ims.FullResult, error) {
		refreshCalls.Add(1)
		enterOnce.Do(func() { close(refreshEntered) })
		<-releaseRefresh
		return &ims.FullResult{AccessToken: "singleflight-token", ExpiresAt: time.Now().Add(time.Hour).Unix(), Credits: -1}, nil
	}

	const callers = 12
	start := make(chan struct{})
	var ready sync.WaitGroup
	var done sync.WaitGroup
	ready.Add(callers)
	done.Add(callers)
	errs := make(chan error, callers)
	for range callers {
		go func() {
			defer done.Done()
			ready.Done()
			<-start
			token, err := provider.GetAccessToken(context.Background(), account)
			if err == nil && token != "singleflight-token" {
				err = errors.New("unexpected refreshed token")
			}
			errs <- err
		}()
	}
	ready.Wait()
	close(start)
	<-refreshEntered
	time.Sleep(25 * time.Millisecond)
	close(releaseRefresh)
	done.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, int32(1), refreshCalls.Load())
	require.Equal(t, int32(1), repo.getCalls.Load())
	require.Equal(t, int32(1), repo.updateCalls.Load())
	require.Equal(t, int32(1), repo.extraCalls.Load())
}
