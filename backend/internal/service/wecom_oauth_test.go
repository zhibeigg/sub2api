//go:build unit

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

const (
	testWeComInstanceID = "wxpay-instance-42"
	testWeComCorpID     = "ww1234567890abcdef"
	testWeComSecret     = "wecom-secret-sensitive-value"
)

var testWeComApp = WeComOAuthAppCredentials{
	InstanceID: testWeComInstanceID,
	CorpID:     testWeComCorpID,
	Secret:     testWeComSecret,
}

func TestWeComOAuthClient_InternalMemberConvertsToOpenID(t *testing.T) {
	cache := newWeComMemoryCache()
	var tokenCalls atomic.Int32
	var convertCalls atomic.Int32

	client, server := newWeComHTTPTestClient(t, cache, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/gettoken":
			tokenCalls.Add(1)
			require.Equal(t, testWeComCorpID, r.URL.Query().Get("corpid"))
			require.Equal(t, testWeComSecret, r.URL.Query().Get("corpsecret"))
			writeWeComJSON(t, w, map[string]any{"errcode": 0, "access_token": "token-internal", "expires_in": 7200})
		case "/cgi-bin/auth/getuserinfo":
			require.Equal(t, "token-internal", r.URL.Query().Get("access_token"))
			require.Equal(t, "oauth-code-internal", r.URL.Query().Get("code"))
			writeWeComJSON(t, w, map[string]any{"errcode": 0, "UserId": "member-1001"})
		case "/cgi-bin/user/convert_to_openid":
			convertCalls.Add(1)
			require.Equal(t, http.MethodPost, r.Method)
			require.Equal(t, "token-internal", r.URL.Query().Get("access_token"))
			var request struct {
				UserID string `json:"userid"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
			require.Equal(t, "member-1001", request.UserID)
			writeWeComJSON(t, w, map[string]any{"errcode": 0, "openid": "openid-converted"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	identity, err := client.ResolveOpenID(context.Background(), testWeComApp, "oauth-code-internal")
	require.NoError(t, err)
	require.Equal(t, WeComOAuthIdentity{OpenID: "openid-converted"}, identity)
	require.Equal(t, int32(1), tokenCalls.Load())
	require.Equal(t, int32(1), convertCalls.Load())

	encoded, err := json.Marshal(identity)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "member-1001")
	require.NotContains(t, string(encoded), testWeComSecret)
	require.NotContains(t, string(encoded), "token-internal")
}

func TestWeComOAuthClient_ExternalVisitorUsesOpenIDDirectly(t *testing.T) {
	cache := newWeComMemoryCache()
	cache.seedAccessToken(weComOAuthCacheScope(testWeComApp), "cached-token")
	var convertCalls atomic.Int32

	client, server := newWeComHTTPTestClient(t, cache, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/auth/getuserinfo":
			require.Equal(t, "cached-token", r.URL.Query().Get("access_token"))
			writeWeComJSON(t, w, map[string]any{"errcode": 0, "OpenId": "openid-external"})
		case "/cgi-bin/user/convert_to_openid":
			convertCalls.Add(1)
			writeWeComJSON(t, w, map[string]any{"errcode": 0, "openid": "unexpected"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	identity, err := client.ResolveOpenID(context.Background(), testWeComApp, "oauth-code-external")
	require.NoError(t, err)
	require.Equal(t, "openid-external", identity.OpenID)
	require.Zero(t, convertCalls.Load())
}

func TestWeComOAuthClient_RejectsAmbiguousOrEmptyIdentity(t *testing.T) {
	tests := []struct {
		name     string
		response map[string]any
	}{
		{name: "both empty", response: map[string]any{"errcode": 0}},
		{name: "both present", response: map[string]any{"errcode": 0, "UserId": "member", "OpenId": "openid"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cache := newWeComMemoryCache()
			cache.seedAccessToken(weComOAuthCacheScope(testWeComApp), "cached-token")
			client, server := newWeComHTTPTestClient(t, cache, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/cgi-bin/auth/getuserinfo" {
					http.NotFound(w, r)
					return
				}
				writeWeComJSON(t, w, test.response)
			}))
			defer server.Close()

			_, err := client.ResolveOpenID(context.Background(), testWeComApp, "oauth-code")
			require.Error(t, err)
			require.Equal(t, "WECOM_OAUTH_IDENTITY_INVALID", infraerrors.Reason(err))
		})
	}
}

func TestWeComOAuthClient_MapsUpstreamErrorWithoutSensitiveMessage(t *testing.T) {
	cache := newWeComMemoryCache()
	cache.seedAccessToken(weComOAuthCacheScope(testWeComApp), "access-token-sensitive")
	upstreamMessage := "bad request: " + testWeComSecret + " https://qyapi.weixin.qq.com/cgi-bin/auth/getuserinfo?access_token=access-token-sensitive"

	client, server := newWeComHTTPTestClient(t, cache, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeWeComJSON(t, w, map[string]any{"errcode": 50001, "errmsg": upstreamMessage})
	}))
	defer server.Close()

	_, err := client.ResolveOpenID(context.Background(), testWeComApp, "oauth-code-sensitive")
	require.Error(t, err)
	require.Equal(t, "WECOM_UPSTREAM_ERROR", infraerrors.Reason(err))
	assertWeComErrorRedacted(t, err, upstreamMessage, testWeComSecret, "access-token-sensitive", "oauth-code-sensitive", "qyapi.weixin.qq.com", "access_token=")
}

func TestWeComOAuthClient_CacheHitAvoidsTokenRequest(t *testing.T) {
	cache := newWeComMemoryCache()
	var tokenCalls atomic.Int32
	var userInfoCalls atomic.Int32

	client, server := newWeComHTTPTestClient(t, cache, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/gettoken":
			tokenCalls.Add(1)
			writeWeComJSON(t, w, map[string]any{"errcode": 0, "access_token": "cached-after-first-call", "expires_in": 7200})
		case "/cgi-bin/auth/getuserinfo":
			userInfoCalls.Add(1)
			writeWeComJSON(t, w, map[string]any{"errcode": 0, "OpenId": "openid-cache-hit"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	for range 2 {
		identity, err := client.ResolveOpenID(context.Background(), testWeComApp, "oauth-code")
		require.NoError(t, err)
		require.Equal(t, "openid-cache-hit", identity.OpenID)
	}
	require.Equal(t, int32(1), tokenCalls.Load())
	require.Equal(t, int32(2), userInfoCalls.Load())
}

func TestWeComOAuthClient_TokenInvalidRefreshesOnlyOnce(t *testing.T) {
	t.Run("refresh succeeds", func(t *testing.T) {
		cache := newWeComMemoryCache()
		cache.seedAccessToken(weComOAuthCacheScope(testWeComApp), "stale-token")
		var tokenCalls atomic.Int32
		var userInfoCalls atomic.Int32

		client, server := newWeComHTTPTestClient(t, cache, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/cgi-bin/gettoken":
				tokenCalls.Add(1)
				writeWeComJSON(t, w, map[string]any{"errcode": 0, "access_token": "fresh-token", "expires_in": 7200})
			case "/cgi-bin/auth/getuserinfo":
				userInfoCalls.Add(1)
				switch r.URL.Query().Get("access_token") {
				case "stale-token":
					writeWeComJSON(t, w, map[string]any{"errcode": 42001, "errmsg": "expired"})
				case "fresh-token":
					writeWeComJSON(t, w, map[string]any{"errcode": 0, "OpenId": "openid-after-refresh"})
				default:
					writeWeComJSON(t, w, map[string]any{"errcode": 40014, "errmsg": "invalid"})
				}
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()

		identity, err := client.ResolveOpenID(context.Background(), testWeComApp, "oauth-code")
		require.NoError(t, err)
		require.Equal(t, "openid-after-refresh", identity.OpenID)
		require.Equal(t, int32(1), tokenCalls.Load())
		require.Equal(t, int32(2), userInfoCalls.Load())
		require.Equal(t, 1, cache.accessTokenDeletes())
	})

	t.Run("second rejection is returned without another refresh", func(t *testing.T) {
		cache := newWeComMemoryCache()
		cache.seedAccessToken(weComOAuthCacheScope(testWeComApp), "stale-token")
		var tokenCalls atomic.Int32
		var userInfoCalls atomic.Int32

		client, server := newWeComHTTPTestClient(t, cache, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/cgi-bin/gettoken":
				tokenCalls.Add(1)
				writeWeComJSON(t, w, map[string]any{"errcode": 0, "access_token": "fresh-but-rejected", "expires_in": 7200})
			case "/cgi-bin/auth/getuserinfo":
				userInfoCalls.Add(1)
				writeWeComJSON(t, w, map[string]any{"errcode": 40014, "errmsg": "invalid token"})
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()

		_, err := client.ResolveOpenID(context.Background(), testWeComApp, "oauth-code")
		require.Error(t, err)
		require.Equal(t, "WECOM_UPSTREAM_ERROR", infraerrors.Reason(err))
		require.Equal(t, int32(1), tokenCalls.Load())
		require.Equal(t, int32(2), userInfoCalls.Load())
		require.Equal(t, 1, cache.accessTokenDeletes())
	})
}

func TestWeComOAuthClient_SingleflightPreventsTokenStampede(t *testing.T) {
	const callers = 12
	cache := newWeComMemoryCache()
	cache.accessGetNotify = make(chan struct{}, callers*3)
	tokenEntered := make(chan struct{})
	releaseToken := make(chan struct{})
	var tokenOnce sync.Once
	var tokenCalls atomic.Int32

	client, server := newWeComHTTPTestClient(t, cache, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/gettoken":
			tokenCalls.Add(1)
			tokenOnce.Do(func() { close(tokenEntered) })
			<-releaseToken
			writeWeComJSON(t, w, map[string]any{"errcode": 0, "access_token": "singleflight-token", "expires_in": 7200})
		case "/cgi-bin/auth/getuserinfo":
			writeWeComJSON(t, w, map[string]any{"errcode": 0, "OpenId": "openid-singleflight"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	start := make(chan struct{})
	var ready sync.WaitGroup
	ready.Add(callers)
	results := make(chan error, callers)
	for range callers {
		go func() {
			ready.Done()
			<-start
			identity, err := client.ResolveOpenID(context.Background(), testWeComApp, "oauth-code")
			if err == nil && identity.OpenID != "openid-singleflight" {
				err = infraerrors.New(http.StatusInternalServerError, "TEST_UNEXPECTED_IDENTITY", "unexpected identity")
			}
			results <- err
		}()
	}
	ready.Wait()
	close(start)
	<-tokenEntered
	for range callers {
		<-cache.accessGetNotify
	}
	close(releaseToken)

	for range callers {
		require.NoError(t, <-results)
	}
	require.Equal(t, int32(1), tokenCalls.Load())
}

func TestWeComOAuthClient_BuildJSConfigUsesTicketCacheAndFixedSignatureVector(t *testing.T) {
	cache := newWeComMemoryCache()
	cache.seedAccessToken(weComOAuthCacheScope(testWeComApp), "ticket-access-token")
	var ticketCalls atomic.Int32

	client, server := newWeComHTTPTestClient(t, cache, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cgi-bin/get_jsapi_ticket" {
			http.NotFound(w, r)
			return
		}
		ticketCalls.Add(1)
		require.Equal(t, "ticket-access-token", r.URL.Query().Get("access_token"))
		writeWeComJSON(t, w, map[string]any{"errcode": 0, "ticket": "ticket-fixed-vector", "expires_in": 7200})
	}))
	defer server.Close()

	client.now = func() time.Time { return time.Unix(1712345678, 0) }
	client.nonce = func() (string, error) { return "nonce-fixed-vector", nil }
	pageURL := "https://merchant.example/pay?order=42#payment-panel"

	for range 2 {
		config, err := client.BuildJSConfig(context.Background(), testWeComApp, pageURL)
		require.NoError(t, err)
		require.Equal(t, testWeComCorpID, config.AppID)
		require.Equal(t, int64(1712345678), config.Timestamp)
		require.Equal(t, "nonce-fixed-vector", config.NonceStr)
		require.Equal(t, "b484bc9d968560ecfd05671430ff1e37edfbb829", config.Signature)

		encoded, err := json.Marshal(config)
		require.NoError(t, err)
		assertWeComErrorRedacted(t, stringError(encoded), testWeComSecret, "ticket-fixed-vector", "ticket-access-token")
	}
	require.Equal(t, int32(1), ticketCalls.Load())
}

func TestWeComOAuthClient_ResponseSizeLimitAndSensitiveBody(t *testing.T) {
	cache := newWeComMemoryCache()
	bodyMarker := "sensitive-response-marker-" + testWeComSecret
	client, server := newWeComHTTPTestClient(t, cache, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, bodyMarker+strings.Repeat("x", weComMaxResponseBodyBytes))
	}))
	defer server.Close()

	_, err := client.ResolveOpenID(context.Background(), testWeComApp, "sensitive-oauth-code")
	require.Error(t, err)
	require.Equal(t, "WECOM_UPSTREAM_RESPONSE_TOO_LARGE", infraerrors.Reason(err))
	assertWeComErrorRedacted(t, err, bodyMarker, testWeComSecret, "sensitive-oauth-code", server.URL, "qyapi.weixin.qq.com", "corpsecret")
}

func TestWeComOAuthCacheScopeUsesOnlyInstanceAndDigest(t *testing.T) {
	normalized, err := normalizeWeComOAuthApp(testWeComApp)
	require.NoError(t, err)
	scope := weComOAuthCacheScope(normalized)

	require.True(t, strings.HasPrefix(scope, testWeComInstanceID+":"))
	require.NotContains(t, scope, testWeComCorpID)
	require.NotContains(t, scope, testWeComSecret)
	require.Len(t, strings.TrimPrefix(scope, testWeComInstanceID+":"), sha256HexLength)

	changedSecret := normalized
	changedSecret.Secret = "another-secret"
	require.NotEqual(t, scope, weComOAuthCacheScope(changedSecret))
	changedCorpID := normalized
	changedCorpID.CorpID = "wwfedcba0987654321"
	require.NotEqual(t, scope, weComOAuthCacheScope(changedCorpID))
}

func TestNewWeComOAuthClientEnforcesHTTPPolicy(t *testing.T) {
	injected := &http.Client{Timeout: time.Minute}
	client, ok := NewWeComOAuthClient(newWeComMemoryCache(), injected).(*weComOAuthClient)
	require.True(t, ok)
	require.Equal(t, weComHTTPTimeout, client.httpClient.Timeout)
	require.NotNil(t, client.httpClient.CheckRedirect)
	require.ErrorIs(t, client.httpClient.CheckRedirect(nil, nil), http.ErrUseLastResponse)
}

func TestWeComCacheTTLUsesSafetyWindowAndMaximum(t *testing.T) {
	require.Equal(t, 115*time.Minute, weComCacheTTL(7200))
	require.Equal(t, 50*time.Second, weComCacheTTL(100))
	require.Equal(t, 115*time.Minute, weComCacheTTL(86400))
	require.Zero(t, weComCacheTTL(0))
}

const sha256HexLength = 64

type weComRewriteTransport struct {
	target    *url.URL
	transport http.RoundTripper
}

func (t weComRewriteTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clonedURL := *request.URL
	clonedURL.Scheme = t.target.Scheme
	clonedURL.Host = t.target.Host
	clone.URL = &clonedURL
	clone.Host = t.target.Host
	return t.transport.RoundTrip(clone)
}

func newWeComHTTPTestClient(t *testing.T, cache *weComMemoryCache, handler http.Handler) (*weComOAuthClient, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	target, err := url.Parse(server.URL)
	require.NoError(t, err)
	injected := &http.Client{
		Transport: weComRewriteTransport{target: target, transport: http.DefaultTransport},
	}
	client, ok := NewWeComOAuthClient(cache, injected).(*weComOAuthClient)
	require.True(t, ok)
	return client, server
}

func writeWeComJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(value))
}

func assertWeComErrorRedacted(t *testing.T, err error, secrets ...string) {
	t.Helper()
	message := err.Error()
	for _, secret := range secrets {
		if secret != "" {
			require.NotContains(t, message, secret)
		}
	}
}

type stringError []byte

func (e stringError) Error() string { return string(e) }

type weComMemoryCache struct {
	mu                sync.Mutex
	accessTokens      map[string]string
	tickets           map[string]string
	accessDeleteCount int
	ticketDeleteCount int
	accessGetNotify   chan struct{}
}

func newWeComMemoryCache() *weComMemoryCache {
	return &weComMemoryCache{
		accessTokens: make(map[string]string),
		tickets:      make(map[string]string),
	}
}

func (c *weComMemoryCache) GetAccessToken(_ context.Context, scope string) (string, bool, error) {
	c.mu.Lock()
	value, found := c.accessTokens[scope]
	notify := c.accessGetNotify
	c.mu.Unlock()
	if notify != nil {
		notify <- struct{}{}
	}
	return value, found, nil
}

func (c *weComMemoryCache) SetAccessToken(_ context.Context, scope, value string, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.accessTokens[scope] = value
	return nil
}

func (c *weComMemoryCache) DeleteAccessToken(_ context.Context, scope string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.accessTokens, scope)
	c.accessDeleteCount++
	return nil
}

func (c *weComMemoryCache) GetJSAPITicket(_ context.Context, scope string) (string, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	value, found := c.tickets[scope]
	return value, found, nil
}

func (c *weComMemoryCache) SetJSAPITicket(_ context.Context, scope, value string, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tickets[scope] = value
	return nil
}

func (c *weComMemoryCache) DeleteJSAPITicket(_ context.Context, scope string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.tickets, scope)
	c.ticketDeleteCount++
	return nil
}

func (c *weComMemoryCache) seedAccessToken(scope, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.accessTokens[scope] = value
}

func (c *weComMemoryCache) accessTokenDeletes() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.accessDeleteCount
}
