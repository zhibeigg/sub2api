package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	cursorpkg "github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
)

type cursorIDEUpstreamStub struct {
	mu             sync.Mutex
	requests       []*http.Request
	bodies         [][]byte
	chatBody       []byte
	modelsBody     []byte
	chatStatus     int
	modelsStatus   int
	forceHTTPMajor int
}

func (s *cursorIDEUpstreamStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	s.mu.Lock()
	s.requests = append(s.requests, req.Clone(req.Context()))
	s.bodies = append(s.bodies, body)
	s.mu.Unlock()

	status := http.StatusOK
	responseBody := s.chatBody
	if req.URL.Path == cursorpkg.IDEModelsPath {
		responseBody = s.modelsBody
		if s.modelsStatus != 0 {
			status = s.modelsStatus
		}
	} else if s.chatStatus != 0 {
		status = s.chatStatus
	}
	major := 2
	if s.forceHTTPMajor != 0 {
		major = s.forceHTTPMajor
	}
	proto := "HTTP/2.0"
	if major == 1 {
		proto = "HTTP/1.1"
	}
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Proto:      proto,
		ProtoMajor: major,
		ProtoMinor: 0,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(responseBody)),
		Request:    req,
	}, nil
}

func (s *cursorIDEUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

func ideTestGateway(upstream HTTPUpstream) *CursorGatewayService {
	return NewCursorGatewayService(upstream, nil, nil, nil, &config.Config{Cursor: config.CursorConfig{
		BaseURL: "https://api.cursor.com", ChatBaseURL: "https://api2.cursor.sh", DashboardBaseURL: "https://api2.cursor.sh",
		DefaultTransportMode: CursorTransportAuto, ClientVersion: "3.1.0", DefaultModel: "default",
		MaxFrameBytes: 8 << 20, MaxBufferedBytes: 16 << 20, IDEStreamIdleTimeoutSeconds: 5,
	}})
}

func ideTestAccount() *Account {
	return &Account{ID: 91, Platform: PlatformCursor, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{
		"dashboard_access_token": "cursor-session-token", "cursor_transport_mode": CursorTransportIDEChat,
	}}
}

func TestCursorGatewayIDEAnthropicStreamsImmediately(t *testing.T) {
	upstream := &cursorIDEUpstreamStub{chatBody: cursorIDEFrames(
		cursorIDETextPayload("hello "), cursorIDETextPayload("world"), cursorIDEUsagePayload(9, 2, 1, 3),
	)}
	svc := ideTestGateway(upstream)
	body := `{"model":"claude-sonnet-5","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	c, recorder := newCursorGatewayTestContext(t, "/v1/messages", body, 3)

	result, err := svc.Forward(context.Background(), c, ideTestAccount(), []byte(body))
	require.NoError(t, err)
	require.True(t, result.Stream)
	require.NotNil(t, result.FirstTokenMs)
	require.Equal(t, 9, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.Contains(t, recorder.Body.String(), `"text":"hello "`)
	require.Contains(t, recorder.Body.String(), `"text":"world"`)
	require.Contains(t, recorder.Body.String(), `event: message_stop`)

	upstream.mu.Lock()
	defer upstream.mu.Unlock()
	require.Len(t, upstream.requests, 1)
	require.Equal(t, cursorpkg.IDEChatPath, upstream.requests[0].URL.Path)
	require.Equal(t, HTTPUpstreamProfileCursorH2, HTTPUpstreamProfileFromContext(upstream.requests[0].Context()))
	require.Equal(t, "Bearer cursor-session-token", upstream.requests[0].Header.Get("Authorization"))
	require.Equal(t, "application/connect+proto", upstream.requests[0].Header.Get("Content-Type"))
	require.NotEmpty(t, upstream.bodies[0])
}

func TestCursorGatewayIDENonStreamNativeToolCall(t *testing.T) {
	upstream := &cursorIDEUpstreamStub{chatBody: cursorIDEFrames(
		cursorIDEToolPayload("call_weather", "get_weather", `{"city":"Shanghai"}`, true),
		cursorIDEUsagePayload(14, 5, 0, 0),
	)}
	svc := ideTestGateway(upstream)
	body := `{"model":"claude-sonnet-5","stream":false,"messages":[{"role":"user","content":"weather"}],"tools":[{"name":"get_weather","description":"weather","input_schema":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}]}`
	c, recorder := newCursorGatewayTestContext(t, "/v1/messages", body, 3)

	result, err := svc.Forward(context.Background(), c, ideTestAccount(), []byte(body))
	require.NoError(t, err)
	require.False(t, result.Stream)
	require.Equal(t, 14, result.Usage.InputTokens)
	require.Contains(t, recorder.Body.String(), `"type":"tool_use"`)
	require.Contains(t, recorder.Body.String(), `"name":"get_weather"`)
	require.Contains(t, recorder.Body.String(), `"city":"Shanghai"`)
	require.Contains(t, recorder.Body.String(), `"stop_reason":"tool_use"`)
}

func TestCursorGatewayIDERejectsHTTP1BeforeWriting(t *testing.T) {
	upstream := &cursorIDEUpstreamStub{chatBody: cursorIDEFrames(cursorIDETextPayload("nope")), forceHTTPMajor: 1}
	svc := ideTestGateway(upstream)
	body := `{"model":"claude-sonnet-5","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	c, recorder := newCursorGatewayTestContext(t, "/v1/messages", body, 3)

	_, err := svc.Forward(context.Background(), c, ideTestAccount(), []byte(body))
	require.Error(t, err)
	require.Equal(t, 0, recorder.Body.Len())
}

func TestCursorIDEModelSyncUsesAvailableModels(t *testing.T) {
	modelsPayload := appendProtoString(nil, 1, "claude-4.6-sonnet-medium")
	modelsPayload = appendProtoString(modelsPayload, 1, "composer-2-fast")
	upstream := &cursorIDEUpstreamStub{modelsBody: cursorIDEFrames(modelsPayload)}
	gateway := ideTestGateway(upstream)
	svc := NewAccountTestService(nil, nil, nil, nil, nil, nil, &config.Config{}, nil)
	svc.SetCursorGatewayService(gateway)

	models, err := svc.FetchUpstreamSupportedModels(context.Background(), ideTestAccount())
	require.NoError(t, err)
	require.Equal(t, []string{"claude-4.6-sonnet-medium", "composer-2-fast"}, models)

	upstream.mu.Lock()
	defer upstream.mu.Unlock()
	require.Len(t, upstream.requests, 1)
	require.Equal(t, cursorpkg.IDEModelsPath, upstream.requests[0].URL.Path)
	require.Equal(t, "application/connect+proto", upstream.requests[0].Header.Get("Content-Type"))
	require.Equal(t, []byte{0, 0, 0, 0, 0}, upstream.bodies[0])
}

func TestDecodeCursorIDEModelsResponseJSON(t *testing.T) {
	models, err := decodeCursorIDEModelsResponse("application/json", []byte(`{"models":[{"name":"display","serverModelName":"claude-4.6-sonnet-medium"},{"name":"composer-2-fast"}]}`), config.CursorConfig{
		MaxFrameBytes: 8 << 20, MaxBufferedBytes: 16 << 20,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"claude-4.6-sonnet-medium", "composer-2-fast"}, models)
}

func cursorIDEFrames(payloads ...[]byte) []byte {
	var result []byte
	for _, payload := range payloads {
		frame, err := cursorpkg.EncodeConnectFrame(payload, false)
		if err != nil {
			panic(err)
		}
		result = append(result, frame...)
	}
	return append(result, 2, 0, 0, 0, 2, '{', '}')
}

func cursorIDETextPayload(text string) []byte {
	inner := protowire.AppendTag(nil, 1, protowire.BytesType)
	inner = protowire.AppendString(inner, text)
	outer := protowire.AppendTag(nil, 2, protowire.BytesType)
	return protowire.AppendBytes(outer, inner)
}

func cursorIDEToolPayload(id, name, arguments string, last bool) []byte {
	tool := appendProtoString(nil, 3, id)
	tool = appendProtoString(tool, 9, name)
	tool = appendProtoString(tool, 10, arguments)
	tool = protowire.AppendTag(tool, 11, protowire.VarintType)
	if last {
		tool = protowire.AppendVarint(tool, 1)
	} else {
		tool = protowire.AppendVarint(tool, 0)
	}
	outer := protowire.AppendTag(nil, 1, protowire.BytesType)
	return protowire.AppendBytes(outer, tool)
}

func cursorIDEUsagePayload(input, output, cacheWrite, cacheRead int) []byte {
	var usage []byte
	for field, value := range map[protowire.Number]int{1: input, 2: output, 3: cacheWrite, 4: cacheRead} {
		usage = protowire.AppendTag(usage, field, protowire.VarintType)
		usage = protowire.AppendVarint(usage, uint64(value))
	}
	outer := protowire.AppendTag(nil, 12, protowire.BytesType)
	return protowire.AppendBytes(outer, usage)
}

func appendProtoString(dst []byte, number protowire.Number, value string) []byte {
	dst = protowire.AppendTag(dst, number, protowire.BytesType)
	return protowire.AppendString(dst, value)
}
