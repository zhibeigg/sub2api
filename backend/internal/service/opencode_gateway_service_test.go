package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openCodeHTTPUpstreamStub struct {
	HTTPUpstream
	do func(*http.Request, string, int64, int) (*http.Response, error)
}

func (s *openCodeHTTPUpstreamStub) Do(req *http.Request, proxyURL string, accountID int64, concurrency int) (*http.Response, error) {
	return s.do(req, proxyURL, accountID, concurrency)
}

type openCodeProxyRepositoryStub struct {
	ProxyRepository
	proxy *Proxy
	err   error
}

func (s *openCodeProxyRepositoryStub) GetByID(context.Context, int64) (*Proxy, error) {
	return s.proxy, s.err
}

type openCodeRateLimitRepositoryStub struct {
	AccountRepository
	mu        sync.Mutex
	accountID int64
	resetAt   time.Time
	calls     int
}

func (s *openCodeRateLimitRepositoryStub) SetRateLimited(_ context.Context, accountID int64, resetAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accountID = accountID
	s.resetAt = resetAt
	s.calls++
	return nil
}

func (s *openCodeRateLimitRepositoryStub) snapshot() (int, int64, time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls, s.accountID, s.resetAt
}

func TestOpenCodeForwardURLAuthenticationProxyAndProtocolOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)
	proxyID := int64(9)
	var gotRequest *http.Request
	var gotProxy string
	var gotAccountID int64
	var gotConcurrency int
	upstream := &openCodeHTTPUpstreamStub{do: func(req *http.Request, proxyURL string, accountID int64, concurrency int) (*http.Response, error) {
		gotRequest, gotProxy, gotAccountID, gotConcurrency = req, proxyURL, accountID, concurrency
		return openCodeResponse(http.StatusOK, `{"id":"msg-1","type":"message","role":"assistant","model":"grok-4.5","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":1}}`, nil), nil
	}}
	service := NewOpenCodeGatewayService(upstream, &openCodeProxyRepositoryStub{proxy: &Proxy{ID: proxyID, Protocol: "http", Host: "127.0.0.1", Port: 8080}}, &config.Config{}, nil)
	account := &Account{
		ID: 42, Platform: PlatformOpenCode, Type: AccountTypeAPIKey, ProxyID: &proxyID, Concurrency: 7,
		Credentials: map[string]any{
			"api_key": "secret", "base_url": "https://relay.example.com/opencode/",
			"model_protocols": map[string]any{"grok-4.5": "anthropic"},
		},
	}
	recorder, c := openCodeTestContext()
	result, err := service.ForwardChatCompletions(t.Context(), c, account, []byte(`{"model":"opencode-go/grok-4.5","messages":[{"role":"user","content":"hi"}]}`))
	require.NoError(t, err)
	require.Equal(t, "https://relay.example.com/opencode/v1/messages", gotRequest.URL.String())
	require.Empty(t, gotRequest.Header.Get("Authorization"))
	require.Equal(t, "secret", gotRequest.Header.Get("X-Api-Key"))
	require.Equal(t, "2023-06-01", gotRequest.Header.Get("Anthropic-Version"))
	require.Equal(t, "application/json", gotRequest.Header.Get("Content-Type"))
	require.Equal(t, "application/json", gotRequest.Header.Get("Accept"))
	require.Equal(t, "http://127.0.0.1:8080", gotProxy)
	require.Equal(t, int64(42), gotAccountID)
	require.Equal(t, 7, gotConcurrency)
	require.Equal(t, "opencode-go/grok-4.5", result.Model)
	require.Equal(t, "opencode-go/grok-4.5", result.BillingModel)
	require.Equal(t, "/v1/messages", result.UpstreamEndpoint)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"object":"chat.completion"`)
}

func TestOpenCodeModelMappingThenProtocolOverride(t *testing.T) {
	var gotPath string
	var gotBody string
	var gotAuthorization string
	var gotAPIKey string
	var gotAnthropicVersion string
	upstream := &openCodeHTTPUpstreamStub{do: func(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		gotPath = req.URL.Path
		gotAuthorization = req.Header.Get("Authorization")
		gotAPIKey = req.Header.Get("X-Api-Key")
		gotAnthropicVersion = req.Header.Get("Anthropic-Version")
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		gotBody = string(body)
		return openCodeResponse(http.StatusOK, `{"id":"chat-1","object":"chat.completion","model":"custom-upstream","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`, nil), nil
	}}
	service := NewOpenCodeGatewayService(upstream, nil, &config.Config{}, nil)
	account := openCodeAccount(map[string]any{
		"base_url": "https://relay.example.com", "model_mapping": map[string]any{"alias": "custom-upstream"},
		"model_protocols": map[string]any{"custom-upstream": "openai"},
	})
	_, c := openCodeTestContext()
	result, err := service.ForwardMessages(t.Context(), c, account, []byte(`{"model":"opencode-go/alias","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`))
	require.NoError(t, err)
	require.Equal(t, "/v1/chat/completions", gotPath)
	require.Equal(t, "Bearer key", gotAuthorization)
	require.Empty(t, gotAPIKey)
	require.Empty(t, gotAnthropicVersion)
	require.Contains(t, gotBody, `"model":"custom-upstream"`)
	require.Equal(t, "opencode-go/alias", result.BillingModel)
	require.Equal(t, "custom-upstream", result.UpstreamModel)
}

func TestOpenCodeUnknownProtocolFailsClosed(t *testing.T) {
	service := NewOpenCodeGatewayService(&openCodeHTTPUpstreamStub{do: func(*http.Request, string, int64, int) (*http.Response, error) {
		t.Fatal("upstream must not be called")
		return nil, nil
	}}, nil, &config.Config{}, nil)
	account := openCodeAccount(map[string]any{"model_protocols": map[string]any{"grok-4.5": "future_protocol"}})
	_, c := openCodeTestContext()
	_, err := service.ForwardChatCompletions(t.Context(), c, account, []byte(`{"model":"grok-4.5","messages":[{"role":"user","content":"hi"}]}`))
	var failure *UpstreamFailoverError
	require.ErrorAs(t, err, &failure)
	require.Equal(t, NextAccountStop, failure.NextAccountAction)
	require.Equal(t, GatewayFailureScopeRequest, failure.Scope)
}

func TestOpenCodeFetchModelsUsesBearerGET(t *testing.T) {
	var method, target, auth string
	upstream := &openCodeHTTPUpstreamStub{do: func(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		method, target, auth = req.Method, req.URL.String(), req.Header.Get("Authorization")
		return openCodeResponse(http.StatusOK, `{"object":"list","data":[]}`, http.Header{"X-Request-Id": []string{"req-models"}}), nil
	}}
	service := NewOpenCodeGatewayService(upstream, nil, &config.Config{}, nil)
	account := openCodeAccount(map[string]any{"base_url": "https://relay.example.com/v1"})
	recorder, c := openCodeTestContext()
	result, err := service.FetchModels(t.Context(), c, account)
	require.NoError(t, err)
	require.Equal(t, http.MethodGet, method)
	require.Equal(t, "https://relay.example.com/v1/models", target)
	require.Equal(t, "Bearer key", auth)
	require.Equal(t, "req-models", result.RequestID)
	require.JSONEq(t, `{"object":"list","data":[]}`, recorder.Body.String())
}

func TestOpenCodeStreamTracksFirstTokenUsageAndRequestID(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"id":"chat-stream","model":"grok-4.5","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chat-stream","model":"grok-4.5","choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chat-stream","model":"grok-4.5","choices":[],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &openCodeHTTPUpstreamStub{do: func(*http.Request, string, int64, int) (*http.Response, error) {
		return openCodeResponse(http.StatusOK, stream, http.Header{"Content-Type": []string{"text/event-stream"}}), nil
	}}
	service := NewOpenCodeGatewayService(upstream, nil, &config.Config{}, nil)
	recorder, c := openCodeTestContext()
	result, err := service.ForwardMessages(t.Context(), c, openCodeAccount(nil), []byte(`{"model":"grok-4.5","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	require.NoError(t, err)
	require.True(t, result.Stream)
	require.NotNil(t, result.FirstTokenMs)
	require.Equal(t, "chat-stream", result.RequestID)
	require.Equal(t, 5, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.Contains(t, recorder.Body.String(), "event: content_block_delta")
	require.Contains(t, recorder.Body.String(), `"text":"hello"`)
}

func TestOpenCodeErrorClassification(t *testing.T) {
	tests := []struct {
		name          string
		status        int
		stage         GatewayFailureStage
		scope         GatewayFailureScope
		next          NextAccountAction
		credential    bool
		clientStatus  int
		clientMessage string
	}{
		{name: "unauthorized", status: 401, stage: GatewayFailureStageAccountAuth, scope: GatewayFailureScopeAccount, next: NextAccountRetry, credential: true, clientStatus: http.StatusServiceUnavailable, clientMessage: "OpenCode Go account credentials are unavailable"},
		{name: "forbidden", status: 403, stage: GatewayFailureStageAccountAuth, scope: GatewayFailureScopeAccount, next: NextAccountRetry, credential: true, clientStatus: http.StatusServiceUnavailable, clientMessage: "OpenCode Go account credentials are unavailable"},
		{name: "rate_limit", status: 429, stage: GatewayFailureStageInference, scope: GatewayFailureScopeAccount, next: NextAccountRetry, clientStatus: http.StatusTooManyRequests, clientMessage: "OpenCode Go upstream rate limit exceeded, please retry later"},
		{name: "server_error", status: 503, stage: GatewayFailureStageInference, scope: GatewayFailureScopeProvider, next: NextAccountRetry, clientStatus: http.StatusServiceUnavailable},
		{name: "bad_request", status: 400, stage: GatewayFailureStageInference, scope: GatewayFailureScopeRequest, next: NextAccountStop, clientStatus: http.StatusBadRequest, clientMessage: "boom"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			upstream := &openCodeHTTPUpstreamStub{do: func(*http.Request, string, int64, int) (*http.Response, error) {
				return openCodeResponse(test.status, `{"error":"boom"}`, http.Header{"X-Debug": []string{"kept"}}), nil
			}}
			service := NewOpenCodeGatewayService(upstream, nil, &config.Config{}, nil)
			_, c := openCodeTestContext()
			_, err := service.ForwardChatCompletions(t.Context(), c, openCodeAccount(nil), []byte(`{"model":"grok-4.5","messages":[{"role":"user","content":"hi"}]}`))
			var failure *UpstreamFailoverError
			require.ErrorAs(t, err, &failure)
			require.Equal(t, test.status, failure.StatusCode)
			require.Equal(t, test.stage, failure.Stage)
			require.Equal(t, test.scope, failure.Scope)
			require.Equal(t, test.next, failure.NextAccountAction)
			require.Equal(t, test.credential, failure.IsCredentialFailure())
			require.Equal(t, test.clientStatus, failure.ClientStatusCode)
			require.Equal(t, test.clientMessage, failure.ClientMessage)
			require.JSONEq(t, `{"error":"boom"}`, string(failure.ResponseBody))
			require.Equal(t, "kept", failure.ResponseHeaders.Get("X-Debug"))
			require.Equal(t, test.status, c.GetInt(OpsUpstreamStatusCodeKey))
			require.Equal(t, "boom", c.GetString(OpsUpstreamErrorMessageKey))
			require.JSONEq(t, `{"error":"boom"}`, c.GetString(OpsUpstreamErrorDetailKey))
			eventsValue, ok := c.Get(OpsUpstreamErrorsKey)
			require.True(t, ok)
			events, ok := eventsValue.([]*OpsUpstreamErrorEvent)
			require.True(t, ok)
			require.Len(t, events, 1)
			require.Equal(t, test.status, events[0].UpstreamStatusCode)
			require.Equal(t, string(test.stage), events[0].Stage)
			require.Equal(t, string(test.scope), events[0].Scope)
			require.Equal(t, "boom", events[0].Message)
			require.Equal(t, `{"error":"boom"}`, events[0].UpstreamResponseBody)
		})
	}
}

func TestOpenCodeRequestErrorRedactsEmbeddedCredentials(t *testing.T) {
	upstream := &openCodeHTTPUpstreamStub{do: func(*http.Request, string, int64, int) (*http.Response, error) {
		return openCodeResponse(http.StatusBadRequest, `{"error":{"message":"invalid api_key=secret-key Bearer abc.def access_token: token-value"}}`, nil), nil
	}}
	gateway := NewOpenCodeGatewayService(upstream, nil, &config.Config{}, nil)
	_, c := openCodeTestContext()

	_, err := gateway.ForwardChatCompletions(t.Context(), c, openCodeAccount(nil), []byte(`{"model":"kimi-k3","messages":[{"role":"user","content":"hi"}]}`))

	var failure *UpstreamFailoverError
	require.ErrorAs(t, err, &failure)
	require.Equal(t, http.StatusBadRequest, failure.ClientStatusCode)
	require.Contains(t, failure.ClientMessage, "api_key=***")
	require.Contains(t, failure.ClientMessage, "Bearer ***")
	require.Contains(t, failure.ClientMessage, "access_token: ***")
	require.NotContains(t, failure.ClientMessage, "secret-key")
	require.NotContains(t, failure.ClientMessage, "abc.def")
	require.NotContains(t, failure.ClientMessage, "token-value")

	upstreamMessage := c.GetString(OpsUpstreamErrorMessageKey)
	upstreamDetail := c.GetString(OpsUpstreamErrorDetailKey)
	require.NotContains(t, upstreamMessage, "secret-key")
	require.NotContains(t, upstreamMessage, "abc.def")
	require.NotContains(t, upstreamMessage, "token-value")
	require.NotContains(t, upstreamDetail, "secret-key")
	require.NotContains(t, upstreamDetail, "abc.def")
	require.NotContains(t, upstreamDetail, "token-value")
}

func TestOpenCode429PersistsCrossRequestCooldown(t *testing.T) {
	tests := []struct {
		name         string
		retryAfter   string
		wantCooldown time.Duration
	}{
		{name: "retry_after", retryAfter: "2", wantCooldown: 2 * time.Second},
		{name: "default", wantCooldown: time.Duration(defaultRateLimit429CooldownSeconds) * time.Second},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &openCodeRateLimitRepositoryStub{}
			rateLimitService := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			headers := make(http.Header)
			if test.retryAfter != "" {
				headers.Set("Retry-After", test.retryAfter)
			}
			upstream := &openCodeHTTPUpstreamStub{do: func(*http.Request, string, int64, int) (*http.Response, error) {
				return openCodeResponse(http.StatusTooManyRequests, `{"error":{"message":"quota reached"}}`, headers), nil
			}}
			gateway := NewOpenCodeGatewayService(upstream, nil, &config.Config{}, rateLimitService)
			account := openCodeAccount(nil)
			account.ID = 88
			_, c := openCodeTestContext()
			startedAt := time.Now()

			_, err := gateway.ForwardChatCompletions(t.Context(), c, account, []byte(`{"model":"kimi-k3","messages":[{"role":"user","content":"hi"}]}`))

			var failure *UpstreamFailoverError
			require.ErrorAs(t, err, &failure)
			require.Equal(t, http.StatusTooManyRequests, failure.StatusCode)
			require.Equal(t, GatewayFailureScopeAccount, failure.Scope)
			require.Equal(t, NextAccountRetry, failure.NextAccountAction)
			require.Equal(t, "OpenCode Go upstream rate limit exceeded, please retry later", failure.ClientMessage)
			require.Equal(t, test.retryAfter, failure.ResponseHeaders.Get("Retry-After"))
			require.Eventually(t, func() bool {
				calls, _, _ := repo.snapshot()
				return calls == 1
			}, time.Second, 10*time.Millisecond)
			calls, accountID, resetAt := repo.snapshot()
			require.Equal(t, 1, calls)
			require.Equal(t, account.ID, accountID)
			actualCooldown := resetAt.Sub(startedAt)
			require.GreaterOrEqual(t, actualCooldown, test.wantCooldown-500*time.Millisecond)
			require.LessOrEqual(t, actualCooldown, test.wantCooldown+time.Second)
			require.Equal(t, http.StatusTooManyRequests, c.GetInt(OpsUpstreamStatusCodeKey))
			require.Equal(t, "quota reached", c.GetString(OpsUpstreamErrorMessageKey))
			require.JSONEq(t, `{"error":{"message":"quota reached"}}`, c.GetString(OpsUpstreamErrorDetailKey))
		})
	}
}

func TestOpenCodeNetworkErrorIsFailover(t *testing.T) {
	upstream := &openCodeHTTPUpstreamStub{do: func(*http.Request, string, int64, int) (*http.Response, error) {
		return nil, errors.New("dial failed")
	}}
	service := NewOpenCodeGatewayService(upstream, nil, &config.Config{}, nil)
	_, c := openCodeTestContext()
	_, err := service.ForwardChatCompletions(t.Context(), c, openCodeAccount(nil), []byte(`{"model":"grok-4.5","messages":[{"role":"user","content":"hi"}]}`))
	var failure *UpstreamFailoverError
	require.ErrorAs(t, err, &failure)
	require.Equal(t, http.StatusBadGateway, failure.StatusCode)
	require.Equal(t, NextAccountRetry, failure.NextAccountAction)
	require.Contains(t, string(failure.ResponseBody), "dial failed")
}

func TestOpenCodeCountTokensIsLocal(t *testing.T) {
	service := NewOpenCodeGatewayService(nil, nil, &config.Config{}, nil)
	recorder, c := openCodeTestContext()
	result, err := service.CountTokens(t.Context(), c, nil, []byte(`{"model":"opencode-go/grok-4.5","messages":[{"role":"user","content":"hello"}]}`))
	require.NoError(t, err)
	require.Positive(t, result.Usage.InputTokens)
	require.Equal(t, "grok-4.5", result.BillingModel)
	require.Contains(t, recorder.Body.String(), "input_tokens")
}

func openCodeAccount(credentials map[string]any) *Account {
	merged := map[string]any{"api_key": "key", "base_url": "https://relay.example.com"}
	for key, value := range credentials {
		merged[key] = value
	}
	return &Account{ID: 1, Platform: PlatformOpenCode, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: merged}
}

func openCodeTestContext() (*httptest.ResponseRecorder, *gin.Context) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	return recorder, c
}

func openCodeResponse(status int, body string, headers http.Header) *http.Response {
	if headers == nil {
		headers = make(http.Header)
	}
	return &http.Response{StatusCode: status, Header: headers, Body: io.NopCloser(strings.NewReader(body))}
}
