package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type cursorGatewayUpstreamStub struct {
	mu       sync.Mutex
	requests []*http.Request
	bodies   []string
	outputs  []string
}

func (s *cursorGatewayUpstreamStub) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	return s.DoWithTLS(req, proxyURL, accountID, accountConcurrency, nil)
}

func (s *cursorGatewayUpstreamStub) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, req)
	s.bodies = append(s.bodies, string(body))
	output := "hello"
	if len(s.outputs) > 0 {
		output = s.outputs[0]
		s.outputs = s.outputs[1:]
	}
	sse := "data: {\"type\":\"text-delta\",\"delta\":\"" + output + "\"}\n\n" +
		"data: {\"type\":\"finish\",\"finishReason\":\"stop\",\"usage\":{\"inputTokens\":7,\"outputTokens\":3}}\n\n"
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(sse))}, nil
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

func TestCursorGatewayForwardAnthropic(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	upstream := &cursorGatewayUpstreamStub{outputs: []string{"hello"}}
	svc := NewCursorGatewayService(upstream, nil, nil, redisClient, &config.Config{Cursor: config.CursorConfig{BaseURL: "https://cursor.com", DefaultModel: "google/gemini-3-flash", RequestTimeoutSeconds: 10, StreamIdleTimeoutSeconds: 10}})
	account := &Account{ID: 1, Platform: PlatformCursor, Type: AccountTypeCookie, Concurrency: 1, Credentials: map[string]any{"cookie": "foo=bar; _vcrcs=secret"}}
	body := `{"model":"cursor-chat","stream":false,"messages":[{"role":"user","content":"hi"}]}`
	c, recorder := newCursorGatewayTestContext(t, "/v1/messages", body, 3)

	result, err := svc.Forward(context.Background(), c, account, []byte(body))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"text":"hello"`)
	require.Equal(t, 7, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
	require.Equal(t, "google/gemini-3-flash", result.UpstreamModel)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "foo=bar; _vcrcs=secret", upstream.requests[0].Header.Get("Cookie"))
	require.Contains(t, upstream.bodies[0], `"model":"google/gemini-3-flash"`)
}

func TestCursorResponsesPreviousResponseIsOwnerBound(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	upstream := &cursorGatewayUpstreamStub{outputs: []string{"first", "second"}}
	svc := NewCursorGatewayService(upstream, nil, nil, redisClient, &config.Config{Cursor: config.CursorConfig{BaseURL: "https://cursor.com", DefaultModel: "google/gemini-3-flash", ResponsesTTLSeconds: 60}})
	account := &Account{ID: 1, Platform: PlatformCursor, Type: AccountTypeCookie, Concurrency: 1, Credentials: map[string]any{"cookie": "_vcrcs=secret"}}

	firstBody := `{"model":"cursor-chat","input":"one","store":true}`
	firstContext, firstRecorder := newCursorGatewayTestContext(t, "/v1/responses", firstBody, 10)
	firstResult, err := svc.ForwardResponses(context.Background(), firstContext, account, []byte(firstBody))
	require.NoError(t, err)
	require.Contains(t, firstRecorder.Body.String(), `"status":"completed"`)

	secondBody := `{"model":"cursor-chat","input":"two","previous_response_id":"` + firstResult.RequestID + `"}`
	secondContext, _ := newCursorGatewayTestContext(t, "/v1/responses", secondBody, 10)
	_, err = svc.ForwardResponses(context.Background(), secondContext, account, []byte(secondBody))
	require.NoError(t, err)
	require.Len(t, upstream.bodies, 2)
	require.Contains(t, upstream.bodies[1], "first")
	require.Contains(t, upstream.bodies[1], "two")

	otherContext, _ := newCursorGatewayTestContext(t, "/v1/responses", secondBody, 11)
	_, err = svc.ForwardResponses(context.Background(), otherContext, account, []byte(secondBody))
	require.Error(t, err)
	require.Contains(t, err.Error(), "failover")
}

func TestCursorEndpointRejectsUntrustedHost(t *testing.T) {
	_, err := cursorEndpoint("https://example.com")
	require.ErrorContains(t, err, "cursor.com")
}
