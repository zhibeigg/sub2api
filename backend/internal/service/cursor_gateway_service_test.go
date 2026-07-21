package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	cursorpkg "github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type cursorGatewayUpstreamStub struct {
	mu                       sync.Mutex
	requests                 []*http.Request
	bodies                   []string
	outputs                  []string
	accountIDs               []int64
	concurrencies            []int
	nextAgent                int
	streamStatus             int
	streamWithoutResult      bool
	runPollCount             int
	dashboardRequiresRefresh bool
	dashboardUsageCalls      int
	cleanupCh                chan string
}

func (s *cursorGatewayUpstreamStub) Do(req *http.Request, _ string, accountID int64, accountConcurrency int) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, req)
	s.bodies = append(s.bodies, string(body))
	s.accountIDs = append(s.accountIDs, accountID)
	s.concurrencies = append(s.concurrencies, accountConcurrency)

	header := http.Header{"Content-Type": []string{"application/json"}}
	switch {
	case req.Method == http.MethodGet && req.URL.Path == "/v1/me":
		return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(`{"apiKeyName":"test-key","userEmail":"cursor@example.com"}`))}, nil
	case req.Method == http.MethodGet && req.URL.Path == "/v1/models":
		return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(`{"items":[{"id":"default","displayName":"Default"},{"id":"model-b","displayName":"B"},{"id":"model-a","displayName":"A"}]}`))}, nil
	case req.Method == http.MethodPost && req.URL.Path == "/aiserver.v1.DashboardService/GetCurrentPeriodUsage":
		s.dashboardUsageCalls++
		if s.dashboardRequiresRefresh && req.Header.Get("Authorization") == "Bearer old-access" {
			return &http.Response{StatusCode: http.StatusUnauthorized, Header: header, Body: io.NopCloser(strings.NewReader(`{"error":"expired"}`))}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(`{"enabled":true,"planUsage":{"totalPercentUsed":1,"autoPercentUsed":0,"apiPercentUsed":1}}`))}, nil
	case req.Method == http.MethodPost && req.URL.Path == "/oauth/token":
		return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(`{"access_token":"new-access","refresh_token":"new-refresh"}`))}, nil
	case req.Method == http.MethodPost && req.URL.Path == "/v1/agents":
		s.nextAgent++
		id := s.nextAgent
		response := `{"agent":{"id":"agent-` + string(rune('0'+id)) + `","status":"RUNNING"},"run":{"id":"run-` + string(rune('0'+id)) + `","agentId":"agent-` + string(rune('0'+id)) + `","status":"RUNNING"}}`
		return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(response))}, nil
	case req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/stream"):
		if s.streamStatus != 0 {
			return &http.Response{StatusCode: s.streamStatus, Header: header, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"run_failed","message":"run failed"}}`))}, nil
		}
		if s.streamWithoutResult {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("event: done\ndata: {}\n\n"))}, nil
		}
		output := "hello"
		if len(s.outputs) > 0 {
			output = s.outputs[0]
			s.outputs = s.outputs[1:]
		}
		sse := "event: interaction_update\ndata: {\"type\":\"text_delta\",\"text\":\"" + output + "\",\"usage\":{\"inputTokens\":7,\"outputTokens\":3,\"cacheWriteTokens\":2,\"cacheReadTokens\":5,\"reasoningTokens\":1}}\n\n" +
			"event: result\ndata: {\"status\":\"FINISHED\",\"text\":\"" + output + "\"}\n\n"
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(sse))}, nil
	case req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/runs/"):
		s.runPollCount++
		if s.runPollCount == 1 {
			return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(`{"id":"run-1","agentId":"agent-1","status":"RUNNING"}`))}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(`{"id":"run-1","agentId":"agent-1","status":"FINISHED","result":"polled","usage":{"inputTokens":11,"outputTokens":4,"cacheWriteTokens":3,"cacheReadTokens":6,"reasoningTokens":2,"totalTokens":24}}`))}, nil
	case req.Method == http.MethodDelete || strings.HasSuffix(req.URL.Path, "/cancel"):
		if s.cleanupCh != nil {
			action := "delete"
			if strings.HasSuffix(req.URL.Path, "/cancel") {
				action = "cancel"
			}
			select {
			case s.cleanupCh <- action:
			default:
			}
		}
		return &http.Response{StatusCode: http.StatusNoContent, Header: header, Body: io.NopCloser(strings.NewReader(""))}, nil
	default:
		return &http.Response{StatusCode: http.StatusNotFound, Header: header, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"not found"}}`))}, nil
	}
}

func (s *cursorGatewayUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

func (s *cursorGatewayUpstreamStub) snapshot() ([]*http.Request, []string, []int64, []int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*http.Request(nil), s.requests...), append([]string(nil), s.bodies...), append([]int64(nil), s.accountIDs...), append([]int(nil), s.concurrencies...)
}

func newCursorGatewayTestContext(t *testing.T, path, body string, apiKeyID int64) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	c.Set("api_key", &APIKey{ID: apiKeyID, UserID: 9})
	return c, recorder
}

func newCursorGatewayForTest(upstream HTTPUpstream, redisClient *redis.Client) *CursorGatewayService {
	return NewCursorGatewayService(upstream, nil, nil, redisClient, &config.Config{Cursor: config.CursorConfig{
		BaseURL: "https://api.cursor.com", DashboardBaseURL: "https://api2.cursor.sh", DefaultModel: "auto", RequestTimeoutSeconds: 10, StreamIdleTimeoutSeconds: 10, ResponsesTTLSeconds: 60,
	}})
}

func cursorAPIKeyAccount() *Account {
	return &Account{ID: 1, Platform: PlatformCursor, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{"api_key": "cursor-key"}}
}

func TestOpenAIGatewayRoutesMixedScheduledCursorAcrossCompatibleProtocols(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		body    string
		forward func(context.Context, *OpenAIGatewayService, *gin.Context, *Account, []byte) (*OpenAIForwardResult, error)
	}{
		{
			name: "chat_completions",
			path: "/v1/chat/completions",
			body: `{"model":"grok-4.5","stream":false,"messages":[{"role":"user","content":"hi"}]}`,
			forward: func(ctx context.Context, svc *OpenAIGatewayService, c *gin.Context, account *Account, body []byte) (*OpenAIForwardResult, error) {
				return svc.ForwardAsChatCompletions(ctx, c, account, body, "", "")
			},
		},
		{
			name: "responses",
			path: "/v1/responses",
			body: `{"model":"grok-4.5","stream":false,"store":false,"input":"hi"}`,
			forward: func(ctx context.Context, svc *OpenAIGatewayService, c *gin.Context, account *Account, body []byte) (*OpenAIForwardResult, error) {
				return svc.Forward(ctx, c, account, body)
			},
		},
		{
			name: "anthropic_messages",
			path: "/v1/messages",
			body: `{"model":"grok-4.5","stream":false,"max_tokens":128,"messages":[{"role":"user","content":"hi"}]}`,
			forward: func(ctx context.Context, svc *OpenAIGatewayService, c *gin.Context, account *Account, body []byte) (*OpenAIForwardResult, error) {
				return svc.ForwardAsAnthropic(ctx, c, account, body, "", "")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			upstream := &cursorGatewayUpstreamStub{outputs: []string{"cursor answer"}}
			cursorGateway := newCursorGatewayForTest(upstream, nil)
			openAIGateway := &OpenAIGatewayService{}
			openAIGateway.SetCursorGatewayService(cursorGateway)
			account := cursorAPIKeyAccount()
			account.Credentials["model_mapping"] = map[string]string{"grok-4.5": "grok-4.5"}
			c, recorder := newCursorGatewayTestContext(t, tt.path, tt.body, 28)

			result, err := tt.forward(context.Background(), openAIGateway, c, account, []byte(tt.body))
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, 7, result.Usage.InputTokens)
			require.Equal(t, 3, result.Usage.OutputTokens)
			require.NotContains(t, recorder.Body.String(), "api_key not found in credentials")

			requests, bodies, accountIDs, concurrencies := upstream.snapshot()
			require.NotEmpty(t, requests)
			require.Equal(t, http.MethodPost, requests[0].Method)
			require.Equal(t, "/v1/agents", requests[0].URL.Path)
			require.Equal(t, "Bearer cursor-key", requests[0].Header.Get("Authorization"))
			require.Contains(t, bodies[0], "grok-4.5")
			require.Equal(t, int64(1), accountIDs[0])
			require.Equal(t, 1, concurrencies[0])
		})
	}
}

func TestCursorMixedSchedulingRequiresConfiguredGateway(t *testing.T) {
	t.Parallel()
	body := []byte(`{"model":"grok-4.5","stream":false,"messages":[{"role":"user","content":"hi"}]}`)
	c, _ := newCursorGatewayTestContext(t, "/v1/chat/completions", string(body), 28)

	_, err := (&OpenAIGatewayService{}).ForwardAsChatCompletions(context.Background(), c, cursorAPIKeyAccount(), body, "", "")
	require.ErrorContains(t, err, "Cursor gateway service is not configured")
}

func TestCursorGatewayFetchDashboardUsageRefreshesExpiredToken(t *testing.T) {
	upstream := &cursorGatewayUpstreamStub{dashboardRequiresRefresh: true}
	svc := newCursorGatewayForTest(upstream, nil)
	account := cursorAPIKeyAccount()
	account.Credentials["dashboard_access_token"] = "old-access"
	account.Credentials["dashboard_refresh_token"] = "old-refresh"

	result, err := svc.FetchDashboardUsage(context.Background(), account)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Usage)
	require.Equal(t, "new-access", result.RefreshedAccessToken)
	require.Equal(t, "new-refresh", result.RefreshedRefreshToken)
	require.Equal(t, 2, upstream.dashboardUsageCalls)

	requests, bodies, _, _ := upstream.snapshot()
	require.Len(t, requests, 3)
	require.Equal(t, "Bearer old-access", requests[0].Header.Get("Authorization"))
	require.Equal(t, "{}", bodies[0])
	require.Equal(t, "/oauth/token", requests[1].URL.Path)
	require.Contains(t, bodies[1], `"refresh_token":"old-refresh"`)
	require.Equal(t, "Bearer new-access", requests[2].Header.Get("Authorization"))
}

func TestCursorGatewayForwardAnthropicCloudAgent(t *testing.T) {
	cleanupCh := make(chan string, 2)
	upstream := &cursorGatewayUpstreamStub{outputs: []string{"hello"}, cleanupCh: cleanupCh}
	svc := newCursorGatewayForTest(upstream, nil)
	body := `{"model":"cursor-chat","stream":false,"messages":[{"role":"user","content":"hi"}]}`
	c, recorder := newCursorGatewayTestContext(t, "/v1/messages", body, 3)

	result, err := svc.Forward(context.Background(), c, cursorAPIKeyAccount(), []byte(body))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"text":"hello"`)
	require.Equal(t, 7, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
	require.Equal(t, 2, result.Usage.CacheCreationInputTokens)
	require.Equal(t, 5, result.Usage.CacheReadInputTokens)
	require.Contains(t, recorder.Body.String(), `"cache_creation_input_tokens":2`)
	require.Contains(t, recorder.Body.String(), `"cache_read_input_tokens":5`)
	require.Equal(t, "auto", result.UpstreamModel)

	select {
	case action := <-cleanupCh:
		require.Equal(t, "delete", action)
	case <-time.After(time.Second):
		t.Fatal("completed agent was not deleted")
	}
	requests, bodies, _, _ := upstream.snapshot()
	require.GreaterOrEqual(t, len(requests), 3)
	require.Equal(t, "Bearer cursor-key", requests[0].Header.Get("Authorization"))
	require.Equal(t, "/v1/agents", requests[0].URL.Path)
	require.Contains(t, bodies[0], "Conversation transcript")
	require.Contains(t, bodies[0], "hi")
}

func TestCursorGatewayForwardAnthropicFollowUpWithThinkingHistory(t *testing.T) {
	upstream := &cursorGatewayUpstreamStub{outputs: []string{"follow-up answer"}}
	svc := newCursorGatewayForTest(upstream, nil)
	body := `{"model":"claude-fable-5","stream":false,"messages":[{"role":"user","content":[{"type":"text","text":"first"}]},{"role":"assistant","content":[{"type":"thinking","thinking":"private reasoning","signature":""},{"type":"text","text":"visible answer"}]},{"role":"user","content":[{"type":"text","text":"follow up"}]}]}`
	c, recorder := newCursorGatewayTestContext(t, "/v1/messages", body, 3)

	result, err := svc.Forward(context.Background(), c, cursorAPIKeyAccount(), []byte(body))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"text":"follow-up answer"`)
	require.NotNil(t, result)

	requests, bodies, _, _ := upstream.snapshot()
	require.NotEmpty(t, requests)
	require.Equal(t, "/v1/agents", requests[0].URL.Path)
	require.Contains(t, bodies[0], "visible answer")
	require.Contains(t, bodies[0], "follow up")
	require.NotContains(t, bodies[0], "private reasoning")
}

func TestCursorGatewayForwardAnthropicIgnoresServerWebSearchTool(t *testing.T) {
	upstream := &cursorGatewayUpstreamStub{outputs: []string{"answer without server search"}}
	svc := newCursorGatewayForTest(upstream, nil)
	body := `{"model":"claude-fable-5","stream":false,"tools":[{"type":"web_search_20250305","name":"web_search","max_uses":5}],"messages":[{"role":"user","content":"answer normally"}]}`
	c, recorder := newCursorGatewayTestContext(t, "/v1/messages", body, 3)

	result, err := svc.Forward(context.Background(), c, cursorAPIKeyAccount(), []byte(body))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"text":"answer without server search"`)
	require.NotNil(t, result)

	requests, bodies, _, _ := upstream.snapshot()
	require.NotEmpty(t, requests)
	require.Equal(t, "/v1/agents", requests[0].URL.Path)
	require.NotContains(t, bodies[0], "web_search_20250305")
}

func TestCursorGatewayPollsRunWhenStreamEndsBeforeResult(t *testing.T) {
	cleanupCh := make(chan string, 1)
	upstream := &cursorGatewayUpstreamStub{streamWithoutResult: true, cleanupCh: cleanupCh}
	svc := newCursorGatewayForTest(upstream, nil)
	body := `{"model":"cursor-chat","stream":false,"messages":[{"role":"user","content":"hi"}]}`
	c, recorder := newCursorGatewayTestContext(t, "/v1/messages", body, 3)

	result, err := svc.Forward(context.Background(), c, cursorAPIKeyAccount(), []byte(body))
	require.NoError(t, err)
	require.Contains(t, recorder.Body.String(), `"text":"polled"`)
	require.Equal(t, 11, result.Usage.InputTokens)
	require.Equal(t, 4, result.Usage.OutputTokens)
	require.Equal(t, 3, result.Usage.CacheCreationInputTokens)
	require.Equal(t, 6, result.Usage.CacheReadInputTokens)
	select {
	case <-cleanupCh:
	case <-time.After(time.Second):
		t.Fatal("polled agent was not deleted")
	}
}

func TestCursorGatewayCancelsRunAndDeletesAgentOnStreamFailure(t *testing.T) {
	cleanupCh := make(chan string, 2)
	upstream := &cursorGatewayUpstreamStub{streamStatus: http.StatusInternalServerError, cleanupCh: cleanupCh}
	svc := newCursorGatewayForTest(upstream, nil)
	body := `{"model":"cursor-chat","stream":false,"messages":[{"role":"user","content":"hi"}]}`
	c, _ := newCursorGatewayTestContext(t, "/v1/messages", body, 3)

	_, err := svc.Forward(context.Background(), c, cursorAPIKeyAccount(), []byte(body))
	require.Error(t, err)

	actions := map[string]bool{}
	deadline := time.After(time.Second)
	for len(actions) < 2 {
		select {
		case action := <-cleanupCh:
			actions[action] = true
		case <-deadline:
			t.Fatalf("cleanup actions missing: %#v", actions)
		}
	}
	require.True(t, actions["cancel"])
	require.True(t, actions["delete"])
}

func TestCursorResponsesPreviousResponseIsOwnerBound(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	upstream := &cursorGatewayUpstreamStub{outputs: []string{"first", "second"}}
	svc := newCursorGatewayForTest(upstream, redisClient)
	account := cursorAPIKeyAccount()

	firstBody := `{"model":"cursor-chat","input":"one","store":true}`
	firstContext, firstRecorder := newCursorGatewayTestContext(t, "/v1/responses", firstBody, 10)
	firstResult, err := svc.ForwardResponses(context.Background(), firstContext, account, []byte(firstBody))
	require.NoError(t, err)
	require.Contains(t, firstRecorder.Body.String(), `"status":"completed"`)

	secondBody := `{"model":"cursor-chat","input":"two","previous_response_id":"` + firstResult.RequestID + `"}`
	secondContext, _ := newCursorGatewayTestContext(t, "/v1/responses", secondBody, 10)
	_, err = svc.ForwardResponses(context.Background(), secondContext, account, []byte(secondBody))
	require.NoError(t, err)

	requests, bodies, _, _ := upstream.snapshot()
	var createBodies []string
	for i, req := range requests {
		if req.Method == http.MethodPost && req.URL.Path == "/v1/agents" {
			createBodies = append(createBodies, bodies[i])
		}
	}
	require.Len(t, createBodies, 2)
	require.Contains(t, createBodies[1], "first")
	require.Contains(t, createBodies[1], "two")

	otherContext, _ := newCursorGatewayTestContext(t, "/v1/responses", secondBody, 11)
	_, err = svc.ForwardResponses(context.Background(), otherContext, account, []byte(secondBody))
	require.Error(t, err)
}

func TestCursorEndpointRejectsUntrustedHost(t *testing.T) {
	_, err := cursorEndpoint("https://cursor.com")
	require.ErrorContains(t, err, "api.cursor.com")
	endpoint, err := cursorEndpoint("https://api.cursor.com/")
	require.NoError(t, err)
	require.Equal(t, "https://api.cursor.com", endpoint)
}

func TestAccountTestServiceValidateTransientCursorAPIKey(t *testing.T) {
	upstream := &cursorGatewayUpstreamStub{}
	gateway := newCursorGatewayForTest(upstream, nil)
	svc := NewAccountTestService(nil, nil, nil, nil, nil, nil, &config.Config{}, nil)
	svc.SetCursorGatewayService(gateway)

	result, err := svc.ValidateTransientCredentials(context.Background(), TransientCredentialValidationInput{
		Platform: PlatformCursor, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "transient-secret"},
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "cursor@example.com", result.DisplayName)
	require.Equal(t, "cursor@example.com", result.Summary)
	requests, _, accountIDs, concurrencies := upstream.snapshot()
	require.Len(t, requests, 1)
	require.Equal(t, "/v1/me", requests[0].URL.Path)
	require.Equal(t, "Bearer transient-secret", requests[0].Header.Get("Authorization"))
	require.Equal(t, int64(0), accountIDs[0])
	require.Equal(t, 1, concurrencies[0])

	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "transient-secret")
}

func TestCursorVariantPreferenceAndCloudModelParams(t *testing.T) {
	var envelope cursorRequestEnvelope
	require.NoError(t, json.Unmarshal([]byte(`{"thinking":{"type":"enabled"},"reasoning_effort":"low","output_config":{"effort":"HIGH"}}`), &envelope))
	preference := envelope.variantPreference()
	require.NotNil(t, preference.Thinking)
	require.True(t, *preference.Thinking)
	require.Equal(t, "high", preference.Effort)

	account := &Account{Credentials: map[string]any{"cursor_model_params": []map[string]any{
		{"id": "context", "value": "1m"},
		{"id": "effort", "value": "medium"},
	}}}
	ref := cursorCloudModelRef(account, "claude-fable-5", preference)
	require.NotNil(t, ref)
	require.Equal(t, "claude-fable-5", ref.ID)
	params := make(map[string]string)
	for _, item := range ref.Params {
		params[item.ID] = item.Value
	}
	require.Equal(t, map[string]string{"context": "1m", "effort": "high", "thinking": "true"}, params)

	partialRef := &cursorpkg.ModelRef{ID: "claude-fable-5", Params: []cursorpkg.ModelParam{
		{ID: "thinking", Value: "true"}, {ID: "effort", Value: "high"},
	}}
	completedRef, err := completeCursorCloudModelRef(partialRef, []cursorpkg.CloudModel{{
		ID: "claude-fable-5",
		Variants: []cursorpkg.CloudModelVariant{
			{Params: []cursorpkg.ModelParam{{ID: "thinking", Value: "true"}, {ID: "context", Value: "1m"}, {ID: "effort", Value: "medium"}}, IsDefault: true},
			{Params: []cursorpkg.ModelParam{{ID: "thinking", Value: "true"}, {ID: "context", Value: "1m"}, {ID: "effort", Value: "high"}}},
		},
	}})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"context": "1m", "effort": "high", "thinking": "true"}, cursorModelParamMap(completedRef.Params))

	logicalModel, legacyPreference := normalizeCursorCloudModel("claude-4.7-opus-high-thinking-fast", cursorVariantPreference{})
	require.Equal(t, "claude-opus-4-7", logicalModel)
	require.Equal(t, "high", legacyPreference.Effort)
	require.NotNil(t, legacyPreference.Thinking)
	require.True(t, *legacyPreference.Thinking)
	require.NotNil(t, legacyPreference.Fast)
	require.True(t, *legacyPreference.Fast)

	logicalModel, legacyPreference = normalizeCursorCloudModel("gpt-5.1-codex-max-high", cursorVariantPreference{})
	require.Equal(t, "gpt-5.1-codex-max", logicalModel)
	require.Equal(t, "high", legacyPreference.Effort)
}

func TestCursorUpstreamModelSyncAlwaysUsesLogicalCloudCatalog(t *testing.T) {
	upstream := &cursorGatewayUpstreamStub{}
	gateway := newCursorGatewayForTest(upstream, nil)
	svc := NewAccountTestService(nil, nil, nil, nil, nil, nil, &config.Config{}, nil)
	svc.SetCursorGatewayService(gateway)
	account := cursorAPIKeyAccount()
	account.Credentials["cursor_transport_mode"] = CursorTransportIDEChat
	account.Credentials["dashboard_access_token"] = "dashboard-token"

	models, err := svc.FetchUpstreamSupportedModels(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, []string{"model-a", "model-b"}, models)
	requests, _, _, _ := upstream.snapshot()
	require.Len(t, requests, 1)
	require.Equal(t, "/v1/models", requests[0].URL.Path)
}
