package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type accountCapacityHTTPUpstreamStub struct {
	mu      sync.Mutex
	calls   int
	request []*http.Request
	proxy   []string
	do      func(call int, req *http.Request) (*http.Response, error)
}

func (s *accountCapacityHTTPUpstreamStub) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	return s.DoWithTLS(req, proxyURL, accountID, accountConcurrency, nil)
}

func (s *accountCapacityHTTPUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	s.mu.Lock()
	s.calls++
	call := s.calls
	s.request = append(s.request, req.Clone(req.Context()))
	s.proxy = append(s.proxy, proxyURL)
	do := s.do
	s.mu.Unlock()
	return do(call, req)
}

func (s *accountCapacityHTTPUpstreamStub) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func accountCapacityJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func accountCapacityTestConfig() *config.Config {
	return &config.Config{
		Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
			Enabled:           true,
			UpstreamHosts:     []string{"relay.example.com"},
			AllowInsecureHTTP: true,
		}},
		AccountCapacity: config.AccountCapacityConfig{
			UpstreamTimeoutSeconds: 1,
			SuccessCacheSeconds:    60,
			ErrorCacheSeconds:      30,
			StaleCacheSeconds:      300,
		},
	}
}

func poolCapacityTestAccount() *Account {
	return &Account{
		ID:          42,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 3,
		Credentials: map[string]any{
			"pool_mode": true,
			"base_url":  "https://relay.example.com/prefix/v1?ignored=true",
			"api_key":   "real-upstream-key",
			"header_overrides": map[string]any{
				"Authorization": "Bearer malicious-override",
			},
		},
	}
}

func TestParseSub2APIUsageResponseNormalizesSupportedModes(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.July, 21, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		body      string
		scope     string
		state     string
		remaining float64
		total     *float64
		used      *float64
	}{
		{
			name:      "quota",
			body:      `{"object":"sub2api.key_usage","schema_version":1,"mode":"quota_limited","isValid":true,"remaining":40,"unit":"USD","quota":{"limit":100,"used":60,"remaining":40,"unit":"USD"}}`,
			scope:     "quota",
			state:     AccountCapacityStateVerified,
			remaining: 40,
			total:     capacityFloat64Ptr(100),
			used:      capacityFloat64Ptr(60),
		},
		{
			name:      "exhausted quota tolerates billed overflow",
			body:      `{"object":"sub2api.key_usage","schema_version":1,"mode":"quota_limited","isValid":true,"remaining":0,"unit":"USD","quota":{"limit":100,"used":101,"remaining":0,"unit":"USD"}}`,
			scope:     "quota",
			state:     AccountCapacityStateVerified,
			remaining: 0,
			total:     capacityFloat64Ptr(100),
			used:      capacityFloat64Ptr(101),
		},
		{
			name:      "subscription chooses tightest window",
			body:      `{"object":"sub2api.key_usage","schema_version":1,"mode":"unrestricted","isValid":true,"remaining":5,"unit":"USD","subscription":{"daily_usage_usd":2,"daily_limit_usd":10,"weekly_usage_usd":5,"weekly_limit_usd":10,"monthly_usage_usd":20,"monthly_limit_usd":100}}`,
			scope:     "subscription_weekly",
			state:     AccountCapacityStateVerified,
			remaining: 5,
			total:     capacityFloat64Ptr(10),
			used:      capacityFloat64Ptr(5),
		},
		{
			name:      "legacy balance",
			body:      `{"mode":"unrestricted","isValid":true,"remaining":42.5,"unit":"USD","balance":42.5}`,
			scope:     "balance",
			state:     AccountCapacityStateVerified,
			remaining: 42.5,
		},
		{
			name:      "zero subscription limit is exhausted",
			body:      `{"object":"sub2api.key_usage","schema_version":1,"mode":"unrestricted","isValid":true,"remaining":0,"unit":"USD","subscription":{"daily_usage_usd":0,"daily_limit_usd":0}}`,
			scope:     "subscription_daily",
			state:     AccountCapacityStateVerified,
			remaining: 0,
			total:     capacityFloat64Ptr(0),
			used:      capacityFloat64Ptr(0),
		},
		{
			name:      "unlimited subscription",
			body:      `{"object":"sub2api.key_usage","schema_version":1,"mode":"unrestricted","isValid":true,"remaining":-1,"unit":"USD","subscription":{}}`,
			scope:     "subscription",
			state:     AccountCapacityStateUnlimited,
			remaining: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot, err := parseSub2APIUsageResponse([]byte(test.body), now)
			require.NoError(t, err)
			require.Equal(t, test.scope, snapshot.Scope)
			require.Equal(t, test.state, snapshot.State)
			require.Equal(t, AccountCapacityProviderSub2API, snapshot.Provider)
			require.True(t, snapshot.Authoritative)
			if test.state == AccountCapacityStateUnlimited {
				require.Nil(t, snapshot.Remaining)
			} else {
				require.NotNil(t, snapshot.Remaining)
				require.InDelta(t, test.remaining, *snapshot.Remaining, 1e-12)
			}
			if test.total != nil {
				require.InDelta(t, *test.total, *snapshot.Total, 1e-12)
			}
			if test.used != nil {
				require.InDelta(t, *test.used, *snapshot.Used, 1e-12)
			}
		})
	}
}

func TestParseSub2APIUsageResponseRejectsAmbiguousOrInconsistentPayloads(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.July, 21, 12, 0, 0, 0, time.UTC)
	for _, body := range []string{
		`{"mode":"unrestricted","isValid":true,"remaining":10,"unit":"USD"}`,
		`{"object":"other.usage","schema_version":1,"mode":"unrestricted","isValid":true,"remaining":10,"unit":"USD","balance":10}`,
		`{"object":"sub2api.key_usage","schema_version":1,"mode":"quota_limited","isValid":true,"remaining":41,"unit":"USD","quota":{"limit":100,"used":60,"remaining":40,"unit":"USD"}}`,
		`{"object":"sub2api.key_usage","schema_version":1,"mode":"unrestricted","isValid":true,"remaining":10,"unit":"USD","balance":10}{"extra":true}`,
	} {
		_, err := parseSub2APIUsageResponse([]byte(body), now)
		require.Error(t, err, body)
	}
}

func TestAccountCapacityServiceUsesSafeRequestAndCache(t *testing.T) {
	t.Parallel()
	stub := &accountCapacityHTTPUpstreamStub{do: func(call int, req *http.Request) (*http.Response, error) {
		remaining := "10"
		if call > 1 {
			remaining = "8"
		}
		return accountCapacityJSONResponse(http.StatusOK, `{"object":"sub2api.key_usage","schema_version":1,"mode":"unrestricted","isValid":true,"remaining":`+remaining+`,"unit":"USD","balance":`+remaining+`}`), nil
	}}
	service := NewAccountCapacityService(stub, accountCapacityTestConfig(), nil)
	account := poolCapacityTestAccount()

	first, err := service.GetPoolBalance(context.Background(), account, false)
	require.NoError(t, err)
	require.Equal(t, AccountCapacityStateVerified, first.State)
	require.InDelta(t, 10, *first.Remaining, 1e-12)

	cached, err := service.GetPoolBalance(context.Background(), account, false)
	require.NoError(t, err)
	require.InDelta(t, 10, *cached.Remaining, 1e-12)
	require.Equal(t, 1, stub.callCount())

	forced, err := service.GetPoolBalance(context.Background(), account, true)
	require.NoError(t, err)
	require.InDelta(t, 8, *forced.Remaining, 1e-12)
	require.Equal(t, 2, stub.callCount())

	require.Len(t, stub.request, 2)
	request := stub.request[0]
	require.Equal(t, http.MethodGet, request.Method)
	require.Equal(t, "/prefix/v1/usage", request.URL.Path)
	require.Empty(t, request.URL.RawQuery)
	require.Equal(t, "Bearer real-upstream-key", request.Header.Get("Authorization"))
	require.Equal(t, "application/json", request.Header.Get("Accept"))
	require.True(t, HTTPUpstreamRedirectsDisabled(request.Context()))
	require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(request.Context()))
}

func TestAccountCapacityServiceMatchesCCSwitchUsageProbeWhenAllowlistDisabled(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name         string
		baseURL      string
		expectedPath string
	}{
		{name: "root base URL", baseURL: "https://relay.example.com", expectedPath: "/v1/usage"},
		{name: "versioned base URL", baseURL: "https://relay.example.com/v1", expectedPath: "/v1/usage"},
		{name: "prefixed versioned base URL", baseURL: "https://relay.example.com/prefix/v1", expectedPath: "/prefix/v1/usage"},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &accountCapacityHTTPUpstreamStub{do: func(_ int, _ *http.Request) (*http.Response, error) {
				return accountCapacityJSONResponse(http.StatusOK, `{"mode":"unrestricted","isValid":true,"remaining":46.00897575,"unit":"USD","balance":46.00897575,"daily_usage":1.2,"usage":{},"model_stats":{}}`), nil
			}}
			cfg := accountCapacityTestConfig()
			cfg.Security.URLAllowlist.Enabled = false
			cfg.Security.URLAllowlist.AllowInsecureHTTP = false
			service := NewAccountCapacityService(stub, cfg, nil)
			account := poolCapacityTestAccount()
			account.Credentials["base_url"] = test.baseURL

			snapshot, err := service.GetPoolBalance(context.Background(), account, true)
			require.NoError(t, err)
			require.Equal(t, AccountCapacityStateVerified, snapshot.State)
			require.True(t, snapshot.Authoritative)
			require.Equal(t, "balance", snapshot.Scope)
			require.Equal(t, "USD", snapshot.Unit)
			require.InDelta(t, 46.00897575, *snapshot.Remaining, 1e-12)
			require.Len(t, stub.request, 1)
			require.Equal(t, http.MethodGet, stub.request[0].Method)
			require.Equal(t, test.expectedPath, stub.request[0].URL.Path)
			require.Equal(t, "Bearer real-upstream-key", stub.request[0].Header.Get("Authorization"))
			require.Equal(t, "application/json", stub.request[0].Header.Get("Accept"))
		})
	}
}

func TestAccountCapacityServiceRespectsEnabledURLAllowlist(t *testing.T) {
	t.Parallel()
	stub := &accountCapacityHTTPUpstreamStub{do: func(_ int, _ *http.Request) (*http.Response, error) {
		t.Fatal("unexpected upstream request")
		return nil, nil
	}}
	cfg := accountCapacityTestConfig()
	cfg.Security.URLAllowlist.UpstreamHosts = []string{"other.example.com"}
	service := NewAccountCapacityService(stub, cfg, nil)

	snapshot, err := service.GetPoolBalance(context.Background(), poolCapacityTestAccount(), true)
	require.NoError(t, err)
	require.Equal(t, AccountCapacityStateUnknown, snapshot.State)
	require.Equal(t, "invalid_base_url", snapshot.MessageCode)
	require.Zero(t, stub.callCount())
}

func TestAccountCapacityServiceKeepsRecentSuccessAsStale(t *testing.T) {
	t.Parallel()
	stub := &accountCapacityHTTPUpstreamStub{do: func(call int, _ *http.Request) (*http.Response, error) {
		if call == 1 {
			return accountCapacityJSONResponse(http.StatusOK, `{"object":"sub2api.key_usage","schema_version":1,"mode":"unrestricted","isValid":true,"remaining":12,"unit":"USD","balance":12}`), nil
		}
		return accountCapacityJSONResponse(http.StatusTooManyRequests, `{}`), nil
	}}
	cfg := accountCapacityTestConfig()
	cfg.AccountCapacity.SuccessCacheSeconds = 1
	service := NewAccountCapacityService(stub, cfg, nil)
	current := time.Date(2026, time.July, 21, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return current }
	account := poolCapacityTestAccount()

	verified, err := service.GetPoolBalance(context.Background(), account, false)
	require.NoError(t, err)
	require.Equal(t, AccountCapacityStateVerified, verified.State)

	current = current.Add(2 * time.Second)
	stale, err := service.GetPoolBalance(context.Background(), account, false)
	require.NoError(t, err)
	require.Equal(t, AccountCapacityStateStale, stale.State)
	require.False(t, stale.Authoritative)
	require.Equal(t, "upstream_rate_limited", stale.MessageCode)
	require.InDelta(t, 12, *stale.Remaining, 1e-12)
	require.Equal(t, 2, stub.callCount())

	again, err := service.GetPoolBalance(context.Background(), account, false)
	require.NoError(t, err)
	require.Equal(t, AccountCapacityStateStale, again.State)
	require.Equal(t, 2, stub.callCount(), "error cache should suppress repeated probes")
}

func TestAccountCapacityServiceRejectsUnsafeCredentialRoutes(t *testing.T) {
	t.Parallel()
	stub := &accountCapacityHTTPUpstreamStub{do: func(_ int, _ *http.Request) (*http.Response, error) {
		t.Fatal("unexpected upstream request")
		return nil, nil
	}}
	service := NewAccountCapacityService(stub, accountCapacityTestConfig(), nil)

	nativeBedrock := &Account{
		ID:       1,
		Platform: PlatformAnthropic,
		Type:     AccountTypeBedrock,
		Credentials: map[string]any{
			"pool_mode": true,
			"base_url":  "https://bedrock.example.com/v1",
			"api_key":   "must-not-be-sent",
		},
	}
	snapshot, err := service.GetPoolBalance(context.Background(), nativeBedrock, false)
	require.NoError(t, err)
	require.Equal(t, AccountCapacityStateUnsupported, snapshot.State)
	require.Equal(t, "native_bedrock_balance_unsupported", snapshot.MessageCode)

	proxyID := int64(99)
	missingProxy := poolCapacityTestAccount()
	missingProxy.ProxyID = &proxyID
	snapshot, err = service.GetPoolBalance(context.Background(), missingProxy, false)
	require.NoError(t, err)
	require.Equal(t, AccountCapacityStateUnknown, snapshot.State)
	require.Equal(t, "proxy_unavailable", snapshot.MessageCode)
	require.Zero(t, stub.callCount())
}
