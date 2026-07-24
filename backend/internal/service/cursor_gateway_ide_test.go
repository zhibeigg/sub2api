package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
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
	"google.golang.org/protobuf/encoding/protowire"
)

type cursorIDEUpstreamStub struct {
	mu             sync.Mutex
	requests       []*http.Request
	bodies         [][]byte
	chatBody       []byte
	chatBodies     [][]byte
	chatIndex      int
	modelsBody     []byte
	chatStatus     int
	modelsStatus   int
	forceHTTPMajor int
}

func (s *cursorIDEUpstreamStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	var body []byte
	if req.URL.Path == cursorpkg.AgentRunPath {
		body, _ = readCursorConnectFrame(req.Body)
		go func() { _, _ = io.Copy(io.Discard, req.Body) }()
	} else {
		body, _ = io.ReadAll(req.Body)
	}
	s.mu.Lock()
	s.requests = append(s.requests, req.Clone(req.Context()))
	s.bodies = append(s.bodies, body)
	responseBody := s.chatBody
	if req.URL.Path == cursorpkg.AgentRunPath && s.chatIndex < len(s.chatBodies) {
		responseBody = s.chatBodies[s.chatIndex]
		s.chatIndex++
	}
	s.mu.Unlock()

	status := http.StatusOK
	if req.URL.Path == cursorpkg.AgentGetUsableModelsPath {
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

type cursorIDEAutoFallbackUpstreamStub struct {
	mu             sync.Mutex
	cloud          cursorGatewayUpstreamStub
	ideChatBody    []byte
	ideRunErrors   []error
	ideRunRequests int
}

type cursorIDEResumeFallbackUpstreamStub struct {
	mu         sync.Mutex
	runFrames  [][]byte
	runRequest int
}

type cursorAgentDuplexUpstreamStub struct {
	mu                   sync.Mutex
	runRequests          int
	requestContextFrames chan []byte
	shellResultFrames    chan [][]byte
	serveErrors          chan error
}

func newCursorAgentDuplexUpstreamStub() *cursorAgentDuplexUpstreamStub {
	return &cursorAgentDuplexUpstreamStub{
		requestContextFrames: make(chan []byte, 1),
		shellResultFrames:    make(chan [][]byte, 1),
		serveErrors:          make(chan error, 1),
	}
}

func (s *cursorIDEResumeFallbackUpstreamStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	switch req.URL.Path {
	case cursorpkg.AgentGetUsableModelsPath:
		return cursorIDEHTTP2Response(req, cursorAgentModelsPayload("claude-sonnet-5")), nil
	case cursorpkg.AgentRunPath:
		frame, err := readCursorConnectFrame(req.Body)
		if err != nil {
			return nil, err
		}
		go func() { _, _ = io.Copy(io.Discard, req.Body) }()
		s.mu.Lock()
		s.runRequest++
		requestNumber := s.runRequest
		s.runFrames = append(s.runFrames, append([]byte(nil), frame...))
		s.mu.Unlock()
		switch requestNumber {
		case 1:
			return cursorIDEHTTP2Response(req, cursorIDEFrames(cursorIDEToolPayload("call_weather", "get_weather", `{"city":"Shanghai"}`, true))), nil
		case 2:
			return nil, errors.New("resume transport failed")
		default:
			return cursorIDEHTTP2Response(req, cursorIDEFrames(cursorIDETextPayload("fallback rebuilt"), cursorIDEUsagePayload(12, 3, 0, 0))), nil
		}
	default:
		return nil, fmt.Errorf("unexpected Cursor Agent path %s", req.URL.Path)
	}
}

func (s *cursorIDEResumeFallbackUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

func (s *cursorAgentDuplexUpstreamStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	switch req.URL.Path {
	case cursorpkg.AgentGetUsableModelsPath:
		return cursorIDEHTTP2Response(req, cursorAgentModelsPayload("claude-sonnet-5")), nil
	case cursorpkg.AgentRunPath:
		if _, err := readCursorConnectFrame(req.Body); err != nil {
			return nil, err
		}
		s.mu.Lock()
		s.runRequests++
		s.mu.Unlock()
		reader, writer := io.Pipe()
		go s.serveAgentStream(req.Body, writer)
		return &http.Response{
			StatusCode: http.StatusOK, Status: http.StatusText(http.StatusOK), Proto: "HTTP/2.0", ProtoMajor: 2,
			Header: make(http.Header), Body: reader, Request: req,
		}, nil
	default:
		return nil, fmt.Errorf("unexpected Cursor Agent path %s", req.URL.Path)
	}
}

func (s *cursorAgentDuplexUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

func (s *cursorAgentDuplexUpstreamStub) serveAgentStream(requestBody io.Reader, responseBody *io.PipeWriter) {
	defer func() { _ = responseBody.Close() }()
	writePayload := func(payload []byte) error {
		frame, err := cursorpkg.EncodeConnectFrame(payload, false)
		if err != nil {
			return err
		}
		_, err = responseBody.Write(frame)
		return err
	}
	fail := func(err error) {
		select {
		case s.serveErrors <- err:
		default:
		}
	}

	requestContext := protowire.AppendTag(nil, 1, protowire.VarintType)
	requestContext = protowire.AppendVarint(requestContext, 31)
	requestContext = appendProtoString(requestContext, 15, "exec-context")
	requestContext = protowire.AppendTag(requestContext, 10, protowire.BytesType)
	requestContext = protowire.AppendBytes(requestContext, nil)
	server := protowire.AppendTag(nil, 2, protowire.BytesType)
	server = protowire.AppendBytes(server, requestContext)
	if err := writePayload(server); err != nil {
		fail(err)
		return
	}
	contextFrame, err := readCursorConnectFrame(requestBody)
	if err != nil {
		fail(err)
		return
	}
	s.requestContextFrames <- contextFrame

	shellArgs := appendProtoString(nil, 1, "pwd")
	shellArgs = appendProtoString(shellArgs, 2, "D:/repo")
	shellArgs = protowire.AppendTag(shellArgs, 3, protowire.VarintType)
	shellArgs = protowire.AppendVarint(shellArgs, 5000)
	shellArgs = appendProtoString(shellArgs, 4, "call-shell")
	shellToolCall := protowire.AppendTag(nil, 1, protowire.BytesType)
	shellToolCall = protowire.AppendBytes(shellToolCall, shellArgs)
	toolCall := protowire.AppendTag(nil, 1, protowire.BytesType)
	toolCall = protowire.AppendBytes(toolCall, shellToolCall)
	completed := appendProtoString(nil, 1, "call-shell")
	completed = protowire.AppendTag(completed, 2, protowire.BytesType)
	completed = protowire.AppendBytes(completed, toolCall)
	interaction := protowire.AppendTag(nil, 3, protowire.BytesType)
	interaction = protowire.AppendBytes(interaction, completed)
	exec := protowire.AppendTag(nil, 1, protowire.VarintType)
	exec = protowire.AppendVarint(exec, 32)
	exec = appendProtoString(exec, 15, "exec-shell")
	exec = protowire.AppendTag(exec, 14, protowire.BytesType)
	exec = protowire.AppendBytes(exec, shellArgs)
	server = protowire.AppendTag(nil, 1, protowire.BytesType)
	server = protowire.AppendBytes(server, interaction)
	server = protowire.AppendTag(server, 2, protowire.BytesType)
	server = protowire.AppendBytes(server, exec)
	if err := writePayload(server); err != nil {
		fail(err)
		return
	}

	shellFrames := make([][]byte, 0, 5)
	for len(shellFrames) < 5 {
		frame, readErr := readCursorConnectFrame(requestBody)
		if readErr != nil {
			fail(readErr)
			return
		}
		shellFrames = append(shellFrames, frame)
	}
	s.shellResultFrames <- shellFrames

	if err := writePayload(cursorIDETextPayload("tool result received")); err != nil {
		fail(err)
		return
	}
	if err := writePayload(cursorIDEUsagePayload(18, 4, 0, 0)); err != nil {
		fail(err)
		return
	}
	if _, err := responseBody.Write([]byte{2, 0, 0, 0, 2, '{', '}'}); err != nil && !errors.Is(err, io.ErrClosedPipe) {
		fail(err)
	}
}

func (s *cursorAgentDuplexUpstreamStub) runRequestCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runRequests
}

func (s *cursorIDEAutoFallbackUpstreamStub) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	switch req.URL.Path {
	case cursorpkg.AgentGetUsableModelsPath:
		return cursorIDEHTTP2Response(req, cursorAgentModelsPayload("claude-sonnet-5")), nil
	case cursorpkg.AgentRunPath:
		_, _ = readCursorConnectFrame(req.Body)
		go func() { _, _ = io.Copy(io.Discard, req.Body) }()
		s.mu.Lock()
		s.ideRunRequests++
		var runErr error
		if len(s.ideRunErrors) > 0 {
			runErr = s.ideRunErrors[0]
			s.ideRunErrors = s.ideRunErrors[1:]
		}
		s.mu.Unlock()
		if runErr != nil {
			return nil, runErr
		}
		return cursorIDEHTTP2Response(req, s.ideChatBody), nil
	default:
		return s.cloud.Do(req, proxyURL, accountID, accountConcurrency)
	}
}

func (s *cursorIDEAutoFallbackUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

func (s *cursorIDEAutoFallbackUpstreamStub) ideRunRequestCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ideRunRequests
}

func readCursorConnectFrame(reader io.Reader) ([]byte, error) {
	header := make([]byte, 5)
	if _, err := io.ReadFull(reader, header); err != nil {
		return nil, err
	}
	payload := make([]byte, int(binary.BigEndian.Uint32(header[1:])))
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}
	return append(header, payload...), nil
}

func cursorIDEHTTP2Response(req *http.Request, body []byte) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     http.StatusText(http.StatusOK),
		Proto:      "HTTP/2.0",
		ProtoMajor: 2,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
		Request:    req,
	}
}

func ideTestGateway(upstream HTTPUpstream) *CursorGatewayService {
	return ideTestGatewayWithRedis(upstream, nil)
}

func ideTestGatewayWithRedis(upstream HTTPUpstream, redisClient *redis.Client) *CursorGatewayService {
	if stub, ok := upstream.(*cursorIDEUpstreamStub); ok && len(stub.modelsBody) == 0 {
		stub.modelsBody = cursorAgentModelsPayload("claude-sonnet-5")
	}
	return NewCursorGatewayService(upstream, nil, nil, redisClient, &config.Config{Cursor: config.CursorConfig{
		BaseURL: "https://api.cursor.com", ChatBaseURL: "https://api2.cursor.sh", DashboardBaseURL: "https://api2.cursor.sh",
		DefaultTransportMode: CursorTransportAuto, ClientVersion: "3.11.13", DefaultModel: "default",
		MaxFrameBytes: 8 << 20, MaxBufferedBytes: 16 << 20, IDEStreamIdleTimeoutSeconds: 5,
		AgentRPCEnabled: true, AgentCloudFallbackEnabled: true,
		AgentModelCacheTTLSeconds: 300, AgentModelStaleTTLSeconds: 1800, AgentModelProbeTimeoutSeconds: 5,
		AgentModelPrewarmEnabled: true, AgentModelPrewarmConcurrency: 3,
	}})
}

func ideTestAccount() *Account {
	return &Account{ID: 91, Platform: PlatformCursor, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{
		"dashboard_access_token": "cursor-session-token", "cursor_machine_id": "11111111-1111-4111-8111-111111111111",
		"cursor_transport_mode": CursorTransportIDEChat,
	}}
}

func TestCursorCompatibilityUsageDoesNotDoubleCountCachedInput(t *testing.T) {
	usage := cursorpkg.Usage{
		InputTokens: 2, OutputTokens: 7, CacheReadTokens: 3, CacheWriteTokens: 5, ReasoningTokens: 1,
	}

	openAIUsage := cursorOpenAIUsage(usage)
	require.Equal(t, 10, openAIUsage["prompt_tokens"])
	require.Equal(t, 7, openAIUsage["completion_tokens"])
	require.Equal(t, 17, openAIUsage["total_tokens"])
	require.Equal(t, gin.H{"cached_tokens": 3, "cache_write_tokens": 5}, openAIUsage["prompt_tokens_details"])

	responsesUsage := cursorResponsesUsage(usage)
	require.Equal(t, 10, responsesUsage["input_tokens"])
	require.Equal(t, 7, responsesUsage["output_tokens"])
	require.Equal(t, 17, responsesUsage["total_tokens"])
	require.Equal(t, gin.H{"cached_tokens": 3, "cache_write_tokens": 5}, responsesUsage["input_tokens_details"])
	require.Equal(t, gin.H{"reasoning_tokens": 1}, responsesUsage["output_tokens_details"])

	anthropicUsage := cursorAnthropicUsage(usage, true)
	require.Equal(t, 2, anthropicUsage["input_tokens"])
	require.Equal(t, 7, anthropicUsage["output_tokens"])
	require.Equal(t, 5, anthropicUsage["cache_creation_input_tokens"])
	require.Equal(t, 3, anthropicUsage["cache_read_input_tokens"])
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
	require.Equal(t, 5, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.Equal(t, 1, result.Usage.CacheCreationInputTokens)
	require.Equal(t, 3, result.Usage.CacheReadInputTokens)
	require.Contains(t, recorder.Body.String(), `"text":"hello "`)
	require.Contains(t, recorder.Body.String(), `"text":"world"`)
	require.Contains(t, recorder.Body.String(), `event: message_stop`)

	upstream.mu.Lock()
	defer upstream.mu.Unlock()
	require.Len(t, upstream.requests, 1)
	require.Equal(t, cursorpkg.AgentRunPath, upstream.requests[0].URL.Path)
	require.Equal(t, HTTPUpstreamProfileCursorH2, HTTPUpstreamProfileFromContext(upstream.requests[0].Context()))
	require.Equal(t, "Bearer cursor-session-token", upstream.requests[0].Header.Get("Authorization"))
	require.Equal(t, "application/connect+proto", upstream.requests[0].Header.Get("Content-Type"))
	require.NotEmpty(t, upstream.bodies[0])
}

func TestCursorGatewayIDEAnthropicInlineImageUsesSingleAgentRun(t *testing.T) {
	upstream := &cursorIDEUpstreamStub{chatBody: cursorIDEFrames(
		cursorIDETextPayload("image received"), cursorIDEUsagePayload(11, 2, 0, 0),
	)}
	svc := ideTestGateway(upstream)
	body := `{"model":"grok-4.5","stream":true,"messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBORw0KGgo="}},{"type":"text","text":"describe it"}]}]}`
	c, recorder := newCursorGatewayTestContext(t, "/v1/messages", body, 3)

	result, err := svc.Forward(context.Background(), c, ideTestAccount(), []byte(body))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.Contains(t, recorder.Body.String(), `"text":"image received"`)
	require.Contains(t, recorder.Body.String(), `event: message_stop`)

	upstream.mu.Lock()
	defer upstream.mu.Unlock()
	require.Len(t, upstream.requests, 1)
	require.Equal(t, cursorpkg.AgentRunPath, upstream.requests[0].URL.Path)
	require.NotEmpty(t, upstream.bodies[0])
}

func TestCursorGatewayIDEAnthropicGrokDirectUsageRestoresStreamAndBillingCache(t *testing.T) {
	upstream := &cursorIDEUpstreamStub{chatBody: cursorIDEFrames(
		cursorIDETextPayload("grok response"), cursorIDEGrokUsagePayload(9, 2, 1, 3),
	)}
	svc := ideTestGateway(upstream)
	body := `{"model":"grok-4.5","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	c, recorder := newCursorGatewayTestContext(t, "/v1/messages", body, 3)

	result, err := svc.Forward(context.Background(), c, ideTestAccount(), []byte(body))
	require.NoError(t, err)
	require.True(t, result.Stream)
	require.Equal(t, 5, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.Equal(t, 1, result.Usage.CacheCreationInputTokens)
	require.Equal(t, 3, result.Usage.CacheReadInputTokens)

	streamBody := recorder.Body.String()
	require.Contains(t, streamBody, `"text":"grok response"`)
	require.Contains(t, streamBody, `"usage":{"cache_creation_input_tokens":1,"cache_read_input_tokens":3,"input_tokens":5,"output_tokens":2}`)
	require.Contains(t, streamBody, `event: message_stop`)
}

func TestOpenAIGatewayCursorGrokZeroTurnEndedUsageFallsBackToEstimatedTokens(t *testing.T) {
	upstream := &cursorIDEUpstreamStub{chatBody: cursorIDEFrames(
		cursorIDETextPayload("fallback grok usage"), cursorIDEGrokUsagePayload(0, 0, 0, 0),
	)}
	cursorGateway := ideTestGateway(upstream)
	openAIGateway := &OpenAIGatewayService{}
	openAIGateway.SetCursorGatewayService(cursorGateway)
	body := `{"model":"grok-4.5","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	c, recorder := newCursorGatewayTestContext(t, "/v1/messages", body, 3)

	result, err := openAIGateway.ForwardAsAnthropic(context.Background(), c, ideTestAccount(), []byte(body), "", "")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.Positive(t, result.Usage.InputTokens)
	require.Positive(t, result.Usage.OutputTokens)
	// Anthropic streaming starts with a provisional zero-usage message_start;
	// the final message_delta must carry the estimated token counts.
	require.Contains(t, recorder.Body.String(), `"input_tokens":5,"output_tokens":5`)
}

func TestCursorGatewayIDEEstimatesUsageWhenTurnEndedOmitsUsage(t *testing.T) {
	upstream := &cursorIDEUpstreamStub{chatBody: cursorIDEFrames(
		cursorIDETextPayload("fallback usage"), cursorIDETurnEndedPayload(nil),
	)}
	svc := ideTestGateway(upstream)
	body := `{"model":"claude-sonnet-5","stream":false,"messages":[{"role":"user","content":"hi"}]}`
	c, _ := newCursorGatewayTestContext(t, "/v1/messages", body, 3)

	result, err := svc.Forward(context.Background(), c, ideTestAccount(), []byte(body))
	require.NoError(t, err)
	require.Positive(t, result.Usage.InputTokens)
	require.Positive(t, result.Usage.OutputTokens)
	require.Zero(t, result.Usage.CacheCreationInputTokens)
	require.Zero(t, result.Usage.CacheReadInputTokens)
}

func TestCursorGatewayIDENonStreamNativeToolCall(t *testing.T) {
	upstream := &cursorIDEUpstreamStub{chatBody: cursorIDEFrames(
		cursorIDEToolPayload("call_weather", "get_weather", `{"city":"Shanghai"}`, true),
		cursorIDEUsagePayload(14, 5, 0, 0),
	)}
	svc := ideTestGateway(upstream)
	defer svc.closeCursorAgentSessions()
	body := `{"model":"claude-sonnet-5","stream":false,"messages":[{"role":"user","content":"weather"}],"tools":[{"name":"get_weather","description":"weather","input_schema":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}]}`
	c, recorder := newCursorGatewayTestContext(t, "/v1/messages", body, 3)

	result, err := svc.Forward(context.Background(), c, ideTestAccount(), []byte(body))
	require.NoError(t, err)
	require.False(t, result.Stream)
	require.Zero(t, result.Usage.InputTokens)
	require.Zero(t, result.Usage.OutputTokens)
	require.Zero(t, result.Usage.CacheCreationInputTokens)
	require.Zero(t, result.Usage.CacheReadInputTokens)
	require.Contains(t, recorder.Body.String(), `"type":"tool_use"`)
	require.Contains(t, recorder.Body.String(), `"name":"get_weather"`)
	require.Contains(t, recorder.Body.String(), `"city":"Shanghai"`)
	require.Contains(t, recorder.Body.String(), `"stop_reason":"tool_use"`)
}

func TestCursorGatewayIDEMCPToolCallReturnsBeforeFinalAndResumesSameStream(t *testing.T) {
	upstream := &cursorIDEUpstreamStub{chatBody: cursorIDEFrames(
		cursorIDEToolPayload("call_weather", "get_weather", `{"city":"Shanghai"}`, true),
		cursorIDETextPayload("weather is sunny"),
		cursorIDEUsagePayload(16, 4, 7, 6),
	)}
	svc := ideTestGateway(upstream)
	defer svc.closeCursorAgentSessions()
	account := ideTestAccount()
	firstBody := `{"model":"claude-sonnet-5","stream":false,"messages":[{"role":"user","content":"weather"}],"tools":[{"type":"function","function":{"name":"get_weather","description":"weather","parameters":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}}]}`
	firstContext, firstRecorder := newCursorGatewayTestContext(t, "/v1/chat/completions", firstBody, 3)
	firstResult, err := svc.Forward(context.Background(), firstContext, account, []byte(firstBody))
	require.NoError(t, err)
	require.NotNil(t, firstResult)
	require.Zero(t, firstResult.Usage.InputTokens)
	require.Zero(t, firstResult.Usage.OutputTokens)
	require.Zero(t, firstResult.Usage.CacheCreationInputTokens)
	require.Zero(t, firstResult.Usage.CacheReadInputTokens)
	require.Contains(t, firstRecorder.Body.String(), `"prompt_tokens":0`)
	require.Contains(t, firstRecorder.Body.String(), `"finish_reason":"tool_calls"`)
	require.Contains(t, firstRecorder.Body.String(), `"id":"call_weather"`)
	require.NotContains(t, firstRecorder.Body.String(), "weather is sunny")

	secondBody := `{"model":"claude-sonnet-5","stream":false,"messages":[{"role":"user","content":"weather"},{"role":"assistant","content":null,"tool_calls":[{"id":"call_weather","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Shanghai\"}"}}]},{"role":"tool","tool_call_id":"call_weather","content":"27 C"}],"tools":[{"type":"function","function":{"name":"get_weather","description":"weather","parameters":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}}]}`
	secondContext, secondRecorder := newCursorGatewayTestContext(t, "/v1/chat/completions", secondBody, 3)
	secondResult, err := svc.Forward(context.Background(), secondContext, account, []byte(secondBody))
	require.NoError(t, err)
	require.NotNil(t, secondResult)
	require.Equal(t, 3, secondResult.Usage.InputTokens)
	require.Equal(t, 4, secondResult.Usage.OutputTokens)
	require.Equal(t, 7, secondResult.Usage.CacheCreationInputTokens)
	require.Equal(t, 6, secondResult.Usage.CacheReadInputTokens)
	require.Contains(t, secondRecorder.Body.String(), `"prompt_tokens":16`)
	require.Contains(t, secondRecorder.Body.String(), `"completion_tokens":4`)
	require.Contains(t, secondRecorder.Body.String(), `"total_tokens":20`)
	require.Contains(t, secondRecorder.Body.String(), `"cached_tokens":6`)
	require.Contains(t, secondRecorder.Body.String(), `"cache_write_tokens":7`)
	require.Contains(t, secondRecorder.Body.String(), `"content":"weather is sunny"`)
	require.Contains(t, secondRecorder.Body.String(), `"finish_reason":"stop"`)

	upstream.mu.Lock()
	defer upstream.mu.Unlock()
	require.Len(t, upstream.requests, 1, "MCP continuation must reuse the original Cursor Agent stream")
}

func TestCursorGatewayIDEMCPPersistedFallbackAfterActiveStreamLoss(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer func() { _ = redisClient.Close() }()
	upstream := &cursorIDEUpstreamStub{chatBodies: [][]byte{
		cursorIDEFrames(cursorIDEToolPayload("call_weather", "get_weather", `{"city":"Shanghai"}`, true)),
		cursorIDEFrames(cursorIDETextPayload("fallback resumed"), cursorIDEUsagePayload(12, 3, 0, 0)),
	}}
	svc := ideTestGatewayWithRedis(upstream, redisClient)
	account := ideTestAccount()
	firstBody := `{"model":"claude-sonnet-5","stream":false,"messages":[{"role":"user","content":"weather"}],"tools":[{"type":"function","function":{"name":"get_weather","description":"weather","parameters":{"type":"object"}}}]}`
	firstContext, _ := newCursorGatewayTestContext(t, "/v1/chat/completions", firstBody, 3)
	_, err := svc.Forward(context.Background(), firstContext, account, []byte(firstBody))
	require.NoError(t, err)
	svc.closeCursorAgentSessions()

	secondBody := `{"model":"claude-sonnet-5","stream":false,"messages":[{"role":"user","content":"weather"},{"role":"assistant","content":null,"tool_calls":[{"id":"call_weather","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Shanghai\"}"}}]},{"role":"tool","tool_call_id":"call_weather","content":"27 C"}],"tools":[{"type":"function","function":{"name":"get_weather","description":"weather","parameters":{"type":"object"}}}]}`
	secondContext, secondRecorder := newCursorGatewayTestContext(t, "/v1/chat/completions", secondBody, 3)
	result, err := svc.Forward(context.Background(), secondContext, account, []byte(secondBody))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, secondRecorder.Body.String(), "fallback resumed")
	upstream.mu.Lock()
	defer upstream.mu.Unlock()
	require.Len(t, upstream.requests, 2)
}

func TestCursorGatewayIDEResumeFailureRebuildsFullHistoryWithImages(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer func() { _ = redisClient.Close() }()
	upstream := &cursorIDEResumeFallbackUpstreamStub{}
	svc := ideTestGatewayWithRedis(upstream, redisClient)
	account := ideTestAccount()
	firstBody := `{"model":"claude-sonnet-5","stream":false,"messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBORw0KGgo="}},{"type":"text","text":"weather"}]}],"tools":[{"name":"get_weather","description":"weather","input_schema":{"type":"object"}}]}`
	firstContext, _ := newCursorGatewayTestContext(t, "/v1/messages", firstBody, 3)
	_, err := svc.Forward(context.Background(), firstContext, account, []byte(firstBody))
	require.NoError(t, err)
	svc.closeCursorAgentSessions()

	secondBody := `{"model":"claude-sonnet-5","stream":false,"messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBORw0KGgo="}},{"type":"text","text":"weather"}]},{"role":"assistant","content":[{"type":"tool_use","id":"call_weather","name":"get_weather","input":{"city":"Shanghai"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_weather","content":"27 C"}]}],"tools":[{"name":"get_weather","description":"weather","input_schema":{"type":"object"}}]}`
	secondContext, secondRecorder := newCursorGatewayTestContext(t, "/v1/messages", secondBody, 3)
	result, err := svc.Forward(context.Background(), secondContext, account, []byte(secondBody))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, secondRecorder.Body.String(), "fallback rebuilt")

	upstream.mu.Lock()
	runFrames := make([][]byte, len(upstream.runFrames))
	for index := range upstream.runFrames {
		runFrames[index] = append([]byte(nil), upstream.runFrames[index]...)
	}
	upstream.mu.Unlock()
	require.Len(t, runFrames, 3)

	resumeRun := firstServiceBytesField(t, runFrames[1][5:], 1)
	resumeAction := firstServiceBytesField(t, resumeRun, 2)
	require.True(t, hasServiceProtoField(resumeAction, 2), "persisted continuation must first resume the stored tool session")

	rebuiltRun := firstServiceBytesField(t, runFrames[2][5:], 1)
	rebuiltAction := firstServiceBytesField(t, rebuiltRun, 2)
	require.True(t, hasServiceProtoField(rebuiltAction, 1), "fallback must create a new user action")
	require.False(t, hasServiceProtoField(rebuiltAction, 2), "fallback must not remain in resume mode")
	rebuiltUserAction := firstServiceBytesField(t, rebuiltAction, 1)
	rebuiltUserMessage := firstServiceBytesField(t, rebuiltUserAction, 1)
	require.Equal(t, "Continue the conversation using the latest tool result.", firstServiceStringField(t, rebuiltUserMessage, 1))
	rebuiltState := firstServiceBytesField(t, rebuiltRun, 1)
	require.True(t, hasServiceProtoField(rebuiltState, 1), "fallback state must include root prompt history")
	require.True(t, hasServiceProtoField(rebuiltState, 8), "fallback state must include complete conversation turns")
	require.Equal(t, uint64(1), firstServiceVarintField(t, rebuiltRun, 19), "fallback must preserve inline image support")
}

func TestCursorAgentSessionRequiresMatchingToolResultBeforeTake(t *testing.T) {
	svc := ideTestGateway(&cursorIDEUpstreamStub{})
	session := &cursorAgentActiveSession{
		Stream:  &cursorAgentEventAdapter{},
		Pending: &cursorAgentPendingMCP{Action: cursorpkg.Action{ID: "call-shell"}},
	}
	svc.storeCursorAgentSession("key:3", "resp-1", "call-shell", session)
	claimed, exists := svc.takeCursorAgentSession("key:3", "resp-1", nil)
	require.True(t, exists)
	require.Nil(t, claimed, "an invalid request must not consume the active session")
	claimed, exists = svc.takeCursorAgentSession("key:3", "resp-1", []string{"call-shell"})
	require.True(t, exists)
	require.Same(t, session, claimed)
	require.False(t, svc.removeCursorAgentSession(session), "a claimed session is no longer owned by the first HTTP round")
	claimed.Close()
}

func TestMergeCursorAgentDialogueMessagesAvoidsDuplicatingFullHistory(t *testing.T) {
	previous := []cursorpkg.DialogueMessage{
		{Role: "user", Text: "run pwd"},
		{Role: "assistant", ToolCalls: []cursorpkg.Action{{ID: "call-shell", Name: "shell", Arguments: map[string]any{"command": "pwd"}}}},
	}
	current := append(append([]cursorpkg.DialogueMessage(nil), previous...), cursorpkg.DialogueMessage{Role: "tool", ToolCallID: "call-shell", Text: "D:/repo"})
	merged := mergeCursorAgentDialogueMessages(previous, current)
	require.Equal(t, current, merged)

	toolOnly := []cursorpkg.DialogueMessage{{Role: "tool", ToolCallID: "call-shell", Text: "D:/repo"}}
	merged = mergeCursorAgentDialogueMessages(previous, toolOnly)
	require.Len(t, merged, 3)
	require.Equal(t, "tool", merged[2].Role)
}

func TestCursorDialogueInlineImageStatsAndEstimate(t *testing.T) {
	dialogue := &cursorpkg.Dialogue{Messages: []cursorpkg.DialogueMessage{
		{Role: "user", Text: "compare", Images: []cursorpkg.InlineImage{
			{MIMEType: "image/png", Data: []byte("first")},
			{MIMEType: "image/jpeg", Data: []byte("second-image")},
		}},
	}}

	count, totalBytes := cursorDialogueInlineImageStats(dialogue)
	require.Equal(t, 2, count)
	require.Equal(t, len("first")+len("second-image"), totalBytes)
	require.GreaterOrEqual(t, estimateCursorDialogueHistoryTokens(dialogue), 2*cursorInlineImageEstimatedTokens)
	require.NoError(t, validateCursorInlineImageFrameBudget(totalBytes, 8<<20))

	err := validateCursorInlineImageFrameBudget(800, 1024)
	var failure *UpstreamFailoverError
	require.ErrorAs(t, err, &failure)
	require.Equal(t, http.StatusRequestEntityTooLarge, failure.StatusCode)
	require.Equal(t, GatewayFailureScopeRequest, failure.Scope)
	require.Equal(t, NextAccountStop, failure.NextAccountAction)
	require.False(t, failure.ShouldReportAccountScheduleFailure())
}

func TestCursorGatewayRejectsTooManyInlineImagesBeforeUpstream(t *testing.T) {
	images := make([]string, 21)
	for index := range images {
		images[index] = `{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBORw0KGgo="}}`
	}
	body := `{"model":"grok-4.5","stream":false,"messages":[{"role":"user","content":[` + strings.Join(images, ",") + `]}]}`
	upstream := &cursorIDEUpstreamStub{}
	svc := ideTestGateway(upstream)
	c, recorder := newCursorGatewayTestContext(t, "/v1/messages", body, 3)

	result, err := svc.Forward(context.Background(), c, ideTestAccount(), []byte(body))

	require.Nil(t, result)
	require.Error(t, err)
	var failure *UpstreamFailoverError
	require.ErrorAs(t, err, &failure)
	require.Equal(t, http.StatusRequestEntityTooLarge, failure.StatusCode)
	require.Equal(t, GatewayFailureScopeRequest, failure.Scope)
	require.Equal(t, NextAccountStop, failure.NextAccountAction)
	require.False(t, failure.ShouldReportAccountScheduleFailure())
	require.Empty(t, recorder.Body.String())
	upstream.mu.Lock()
	defer upstream.mu.Unlock()
	require.Empty(t, upstream.requests)
}

func TestTrimCursorDialogueDoesNotChargeFixedAgentOverheadToHistory(t *testing.T) {
	schema, err := json.Marshal(map[string]any{
		"type":        "object",
		"description": strings.Repeat("large tool schema ", 10000),
	})
	require.NoError(t, err)
	dialogue := &cursorpkg.Dialogue{
		System: strings.Repeat("fixed system instructions ", 1000),
		Tools: []cursorpkg.ToolDefinition{{
			Name: "large_tool", Description: "Large NarraFork-style tool", InputSchema: schema,
		}},
		Messages: []cursorpkg.DialogueMessage{
			{Role: "user", Text: "Remember that the release code is ORBIT-4836."},
			{Role: "assistant", Text: "I will remember ORBIT-4836."},
			{Role: "user", Text: "What release code did I give you?"},
		},
	}
	original := append([]cursorpkg.DialogueMessage(nil), dialogue.Messages...)
	require.Greater(t, estimateCursorDialogueFixedTokens(dialogue), 12000)
	require.Less(t, estimateCursorDialogueHistoryTokens(dialogue), 12000)
	require.Greater(t, estimateCursorDialogueTokens(dialogue), 12000)

	trimCursorDialogue(dialogue, 100, 12000)

	require.Equal(t, original, dialogue.Messages)
}

func TestTrimCursorDialogueStillLimitsConversationHistory(t *testing.T) {
	messages := []cursorpkg.DialogueMessage{
		{Role: "user", Text: strings.Repeat("old user one ", 1000)},
		{Role: "assistant", Text: strings.Repeat("old answer one ", 1000)},
		{Role: "user", Text: strings.Repeat("recent user ", 1000)},
		{Role: "assistant", Text: strings.Repeat("recent answer ", 1000)},
		{Role: "user", Text: strings.Repeat("latest question ", 1000)},
	}
	dialogue := &cursorpkg.Dialogue{
		System:   strings.Repeat("fixed ", 10000),
		Messages: append([]cursorpkg.DialogueMessage(nil), messages...),
	}
	retained := &cursorpkg.Dialogue{Messages: append([]cursorpkg.DialogueMessage(nil), messages[2:]...)}
	budget := estimateCursorDialogueHistoryTokens(retained)
	require.Greater(t, estimateCursorDialogueHistoryTokens(dialogue), budget)

	trimCursorDialogue(dialogue, 100, budget)

	require.Equal(t, messages[2:], dialogue.Messages)
}

func TestCursorGatewayIDEAgentToolCallReturnsAndResumesSameDuplexStream(t *testing.T) {
	upstream := newCursorAgentDuplexUpstreamStub()
	svc := ideTestGateway(upstream)
	defer svc.closeCursorAgentSessions()
	account := ideTestAccount()
	firstBody := `{"model":"claude-sonnet-5","stream":false,"messages":[{"role":"user","content":"run pwd"}],"tools":[{"type":"function","function":{"name":"shell","description":"Run a shell command","parameters":{"type":"object","properties":{"command":{"type":"string"}},"required":["command"]}}}]}`
	firstContext, firstRecorder := newCursorGatewayTestContext(t, "/v1/chat/completions", firstBody, 3)
	type forwardOutcome struct {
		result *ForwardResult
		err    error
	}
	firstDone := make(chan forwardOutcome, 1)
	go func() {
		result, err := svc.Forward(context.Background(), firstContext, account, []byte(firstBody))
		firstDone <- forwardOutcome{result: result, err: err}
	}()

	select {
	case outcome := <-firstDone:
		require.NoError(t, outcome.err)
		require.NotNil(t, outcome.result)
		require.Contains(t, firstRecorder.Body.String(), `"finish_reason":"tool_calls"`)
		require.Contains(t, firstRecorder.Body.String(), `"id":"call-shell"`)
		require.Contains(t, firstRecorder.Body.String(), `"name":"shell"`)
	case err := <-upstream.serveErrors:
		t.Fatalf("Cursor duplex upstream failed before first response: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("first Cursor gateway response did not return immediately after the tool call")
	}

	select {
	case frame := <-upstream.requestContextFrames:
		payload := frame[5:]
		exec := firstServiceBytesField(t, payload, 2)
		contextResult := firstServiceBytesField(t, exec, 10)
		success := firstServiceBytesField(t, contextResult, 1)
		requestContext := firstServiceBytesField(t, success, 1)
		tool := firstServiceBytesField(t, requestContext, 7)
		require.Equal(t, uint64(31), firstServiceVarintField(t, exec, 1))
		require.Equal(t, "exec-context", firstServiceStringField(t, exec, 15))
		require.Equal(t, "sub2api-shell", firstServiceStringField(t, tool, 1))
		require.Equal(t, "shell", firstServiceStringField(t, tool, 5))
	case err := <-upstream.serveErrors:
		t.Fatalf("Cursor request context response failed: %v", err)
	case <-time.After(time.Second):
		t.Fatal("Cursor request context result was not sent")
	}
	select {
	case <-upstream.shellResultFrames:
		t.Fatal("shell result was sent before the downstream submitted tool output")
	default:
	}
	require.Equal(t, 1, upstream.runRequestCount())

	secondBody := `{"model":"claude-sonnet-5","stream":false,"messages":[{"role":"user","content":"run pwd"},{"role":"assistant","content":null,"tool_calls":[{"id":"call-shell","type":"function","function":{"name":"shell","arguments":"{\"command\":\"pwd\"}"}}]},{"role":"tool","tool_call_id":"call-shell","content":"D:/repo\n"}],"tools":[{"type":"function","function":{"name":"shell","description":"Run a shell command","parameters":{"type":"object","properties":{"command":{"type":"string"}},"required":["command"]}}}]}`
	secondContext, secondRecorder := newCursorGatewayTestContext(t, "/v1/chat/completions", secondBody, 3)
	secondResult, err := svc.Forward(context.Background(), secondContext, account, []byte(secondBody))
	require.NoError(t, err)
	require.NotNil(t, secondResult)
	require.Contains(t, secondRecorder.Body.String(), `"content":"tool result received"`)
	require.Contains(t, secondRecorder.Body.String(), `"finish_reason":"stop"`)
	require.Equal(t, 1, upstream.runRequestCount(), "tool continuation must reuse the original Cursor Agent stream")

	select {
	case frames := <-upstream.shellResultFrames:
		require.Len(t, frames, 5)
		startExec := firstServiceBytesField(t, frames[0][5:], 2)
		startStream := firstServiceBytesField(t, startExec, 14)
		require.NotNil(t, firstServiceBytesField(t, startStream, 4))
		stdoutExec := firstServiceBytesField(t, frames[1][5:], 2)
		stdoutStream := firstServiceBytesField(t, stdoutExec, 14)
		require.Equal(t, "D:/repo\n", firstServiceStringField(t, firstServiceBytesField(t, stdoutStream, 1), 1))
		exitExec := firstServiceBytesField(t, frames[2][5:], 2)
		exitStream := firstServiceBytesField(t, exitExec, 14)
		require.Equal(t, uint64(0), firstServiceVarintField(t, firstServiceBytesField(t, exitStream, 3), 1))
		resultExec := firstServiceBytesField(t, frames[3][5:], 2)
		shellResult := firstServiceBytesField(t, resultExec, 2)
		success := firstServiceBytesField(t, shellResult, 1)
		require.Equal(t, uint64(32), firstServiceVarintField(t, resultExec, 1))
		require.Equal(t, "exec-shell", firstServiceStringField(t, resultExec, 15))
		require.Equal(t, "D:/repo\n", firstServiceStringField(t, success, 5))
		control := firstServiceBytesField(t, frames[4][5:], 5)
		require.Equal(t, uint64(32), firstServiceVarintField(t, firstServiceBytesField(t, control, 1), 1))
	case err := <-upstream.serveErrors:
		t.Fatalf("Cursor shell result continuation failed: %v", err)
	case <-time.After(time.Second):
		t.Fatal("Cursor shell result was not written back to the active stream")
	}
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

func TestCursorGatewayAutoFallsBackToCloudBeforeIDEStreamCommit(t *testing.T) {
	upstream := &cursorIDEAutoFallbackUpstreamStub{ideChatBody: cursorIDEErrorFrame("resource_exhausted", "Error")}
	svc := ideTestGateway(upstream)
	account := ideTestAccount()
	account.Credentials["api_key"] = "cloud-key"
	account.Credentials["cursor_transport_mode"] = CursorTransportAuto
	body := `{"model":"claude-sonnet-5","stream":false,"messages":[{"role":"user","content":"hi"}]}`
	c, recorder := newCursorGatewayTestContext(t, "/v1/messages", body, 3)

	result, err := svc.Forward(context.Background(), c, account, []byte(body))

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, recorder.Body.String(), "hello")
	require.Equal(t, 1, upstream.cloud.nextAgent)
}

func TestCursorGatewayAutoDoesNotFallbackRequestsRequiringAgentRPC(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		body      string
		responses bool
	}{
		{
			name: "anthropic tools",
			path: "/v1/messages",
			body: `{"model":"claude-sonnet-5","stream":false,"messages":[{"role":"user","content":"weather"}],"tools":[{"name":"get_weather","description":"weather","input_schema":{"type":"object"}}]}`,
		},
		{
			name: "openai chat tools",
			path: "/v1/chat/completions",
			body: `{"model":"claude-sonnet-5","stream":false,"messages":[{"role":"user","content":"weather"}],"tools":[{"type":"function","function":{"name":"get_weather","description":"weather","parameters":{"type":"object"}}}]}`,
		},
		{
			name:      "responses tools",
			path:      "/v1/responses",
			responses: true,
			body:      `{"model":"claude-sonnet-5","stream":false,"input":"weather","tools":[{"type":"function","name":"get_weather","description":"weather","parameters":{"type":"object"}}]}`,
		},
		{
			name: "openai tool continuation",
			path: "/v1/chat/completions",
			body: `{"model":"claude-sonnet-5","stream":false,"messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"call_weather","type":"function","function":{"name":"get_weather","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call_weather","content":"sunny"}]}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			upstream := &cursorIDEAutoFallbackUpstreamStub{ideChatBody: cursorIDEErrorFrame("resource_exhausted", "temporary capacity exhausted")}
			svc := ideTestGateway(upstream)
			account := ideTestAccount()
			account.Credentials["api_key"] = "cloud-key"
			account.Credentials["cursor_transport_mode"] = CursorTransportAuto
			c, recorder := newCursorGatewayTestContext(t, test.path, test.body, 3)

			var result *ForwardResult
			var err error
			if test.responses {
				result, err = svc.ForwardResponses(context.Background(), c, account, []byte(test.body))
			} else {
				result, err = svc.Forward(context.Background(), c, account, []byte(test.body))
			}

			require.Nil(t, result)
			require.Error(t, err)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
			require.Empty(t, recorder.Body.String())
			require.Equal(t, 0, upstream.cloud.nextAgent)
			require.Equal(t, 1, upstream.ideRunRequestCount())
		})
	}
}

func TestCursorGatewayInvalidArgumentDoesNotWriteStreamOrFallback(t *testing.T) {
	upstream := &cursorIDEAutoFallbackUpstreamStub{ideChatBody: cursorIDEErrorFrame("invalid_argument", "image payload SECRET-DATA is invalid")}
	svc := ideTestGateway(upstream)
	account := ideTestAccount()
	account.Credentials["api_key"] = "cloud-key"
	account.Credentials["cursor_transport_mode"] = CursorTransportAuto
	body := `{"model":"grok-4.5","stream":true,"messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBORw0KGgo="}}]}]}`
	c, recorder := newCursorGatewayTestContext(t, "/v1/messages", body, 3)

	result, err := svc.Forward(context.Background(), c, account, []byte(body))

	require.Nil(t, result)
	require.Error(t, err)
	var failure *UpstreamFailoverError
	require.ErrorAs(t, err, &failure)
	require.Equal(t, http.StatusBadRequest, failure.StatusCode)
	require.Equal(t, GatewayFailureScopeRequest, failure.Scope)
	require.Equal(t, NextAccountStop, failure.NextAccountAction)
	require.Equal(t, "Cursor rejected the request payload", failure.ClientMessage)
	require.NotContains(t, failure.Error(), "SECRET-DATA")
	require.Empty(t, recorder.Body.String(), "service must leave stream error serialization to the handler")
	require.Equal(t, 0, upstream.cloud.nextAgent)
	require.Equal(t, 1, upstream.ideRunRequestCount())
}

func TestCursorAgentStreamFailureMapsConnectCodes(t *testing.T) {
	tests := map[string]int{
		"resource_exhausted": http.StatusTooManyRequests,
		"unavailable":        http.StatusServiceUnavailable,
		"deadline_exceeded":  http.StatusGatewayTimeout,
		"invalid_argument":   http.StatusBadRequest,
		"unknown":            http.StatusBadGateway,
	}
	for code, expectedStatus := range tests {
		t.Run(code, func(t *testing.T) {
			statusCode, actualCode, message, details := cursorAgentStreamFailure(&cursorpkg.IDEStreamError{Code: code, Message: "upstream failure", Details: json.RawMessage(`{"retryable":true}`)})
			require.Equal(t, expectedStatus, statusCode)
			require.Equal(t, code, actualCode)
			require.Equal(t, "upstream failure", message)
			require.JSONEq(t, `{"retryable":true}`, details)
		})
	}
}

func TestCursorAgentInvalidArgumentStopsAccountFailover(t *testing.T) {
	err := cursorAgentStreamFailoverError(http.StatusBadRequest, "invalid_argument", "image payload is invalid")

	var failure *UpstreamFailoverError
	require.ErrorAs(t, err, &failure)
	require.Equal(t, GatewayFailureScopeRequest, failure.Scope)
	require.Equal(t, NextAccountStop, failure.NextAccountAction)
	require.Equal(t, http.StatusBadRequest, failure.ClientStatusCode)
	require.Equal(t, "Cursor rejected the request payload", failure.ClientMessage)
	require.False(t, failure.ShouldRetryNextAccount())
	require.False(t, failure.ShouldReportAccountScheduleFailure())
}

func TestCursorRequestRequiresAgentRPCProtectsResponsesContinuation(t *testing.T) {
	required, reason := cursorRequestRequiresAgentRPC([]byte(`{"model":"claude-sonnet-5","previous_response_id":"resp_previous","input":"continue"}`), cursorpkg.ProtocolResponses)
	require.True(t, required)
	require.Equal(t, "responses_continuation", reason)

	required, reason = cursorRequestRequiresAgentRPC([]byte(`{"model":"claude-sonnet-5","messages":[{"role":"user","content":"hi"}]}`), cursorpkg.ProtocolAnthropic)
	require.False(t, required)
	require.Empty(t, reason)

	required, reason = cursorRequestRequiresAgentRPC([]byte(`{"model":"grok-4.5","messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBORw0KGgo="}}]}]}`), cursorpkg.ProtocolAnthropic)
	require.True(t, required)
	require.Equal(t, "inline_images", reason)
}

func TestCursorGatewayRetriesAgentGOAWAYBeforeCloudFallback(t *testing.T) {
	upstream := &cursorIDEAutoFallbackUpstreamStub{
		ideRunErrors: []error{errors.New("http2: Transport: cannot retry err [GOAWAY] after Request.Body was written; define Request.GetBody to avoid this error")},
		ideChatBody: cursorIDEFrames(
			cursorIDETextPayload("retried on Agent RPC"),
			cursorIDEUsagePayload(8, 3, 0, 0),
		),
	}
	svc := ideTestGateway(upstream)
	account := ideTestAccount()
	account.Credentials["api_key"] = "cloud-key"
	account.Credentials["cursor_transport_mode"] = CursorTransportAuto
	body := `{"model":"claude-sonnet-5","stream":false,"messages":[{"role":"user","content":"hi"}]}`
	c, recorder := newCursorGatewayTestContext(t, "/v1/messages", body, 3)

	result, err := svc.Forward(context.Background(), c, account, []byte(body))

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, recorder.Body.String(), "retried on Agent RPC")
	require.Equal(t, 2, upstream.ideRunRequestCount())
	require.Equal(t, 0, upstream.cloud.nextAgent)
}

func TestCursorGatewayForcedIDEDoesNotFallbackToCloud(t *testing.T) {
	upstream := &cursorIDEAutoFallbackUpstreamStub{ideChatBody: cursorIDEErrorFrame("resource_exhausted", "Error")}
	svc := ideTestGateway(upstream)
	account := ideTestAccount()
	account.Credentials["api_key"] = "cloud-key"
	body := `{"model":"claude-sonnet-5","stream":false,"messages":[{"role":"user","content":"hi"}]}`
	c, recorder := newCursorGatewayTestContext(t, "/v1/messages", body, 3)

	_, err := svc.Forward(context.Background(), c, account, []byte(body))

	require.Error(t, err)
	require.Empty(t, recorder.Body.String())
	require.Equal(t, 0, upstream.cloud.nextAgent)
}

func TestCursorIDEModelProbeUsesAvailableModels(t *testing.T) {
	upstream := &cursorIDEUpstreamStub{modelsBody: cursorAgentModelsPayload("claude-4.6-sonnet-medium", "composer-2-fast")}
	gateway := ideTestGateway(upstream)

	models, err := gateway.FetchIDEModels(context.Background(), ideTestAccount())
	require.NoError(t, err)
	require.Equal(t, []string{"claude-4.6-sonnet-medium", "composer-2-fast"}, models)

	upstream.mu.Lock()
	defer upstream.mu.Unlock()
	require.Len(t, upstream.requests, 1)
	require.Equal(t, cursorpkg.AgentGetUsableModelsPath, upstream.requests[0].URL.Path)
	require.Equal(t, "application/proto", upstream.requests[0].Header.Get("Content-Type"))
	require.Empty(t, upstream.bodies[0])
}

func TestCursorIDEOnlyModelSyncCollapsesExecutionVariants(t *testing.T) {
	upstream := &cursorIDEUpstreamStub{modelsBody: cursorAgentDetailedModelsPayload(
		cursorIDEModel{Name: "claude-fable-5-thinking-low", ServerName: "claude-fable-5-thinking-low"},
		cursorIDEModel{Name: "claude-fable-5-thinking-high", ServerName: "claude-fable-5-thinking-high"},
		cursorIDEModel{Name: "claude-4.6-sonnet-medium-thinking", ServerName: "claude-4.6-sonnet-medium-thinking"},
		cursorIDEModel{Name: "cursor-grok-4.5-high", ServerName: "cursor-grok-4.5-high"},
		cursorIDEModel{Name: "cursor-grok-4.5-low", ServerName: "cursor-grok-4.5-low"},
		cursorIDEModel{Name: "cursor-grok-4.5-medium", ServerName: "cursor-grok-4.5-medium"},
	)}
	gateway := ideTestGateway(upstream)
	svc := NewAccountTestService(nil, nil, nil, nil, nil, nil, &config.Config{}, nil)
	svc.SetCursorGatewayService(gateway)

	models, err := svc.FetchUpstreamSupportedModels(context.Background(), ideTestAccount())
	require.NoError(t, err)
	require.Equal(t, []string{"claude-fable-5", "claude-sonnet-4-6", "grok-4.5"}, models)
}

func TestDecodeCursorIDEModelsResponseJSON(t *testing.T) {
	models, err := decodeCursorIDEModelsResponse("application/json", []byte(`{"models":[{"name":"display","serverModelName":"claude-4.6-sonnet-medium","defaultOn":true,"supportsThinking":true,"legacySlugs":["claude-sonnet-4-6"]},{"name":"composer-2-fast"}]}`), config.CursorConfig{
		MaxFrameBytes: 8 << 20, MaxBufferedBytes: 16 << 20,
	})
	require.NoError(t, err)
	require.Equal(t, []cursorIDEModel{
		{Name: "display", ServerName: "claude-4.6-sonnet-medium", DefaultOn: true, SupportsThinking: true, LegacySlugs: []string{"claude-sonnet-4-6"}},
		{Name: "composer-2-fast", ServerName: "composer-2-fast"},
	}, models)
}

func TestCursorIDEModelResolutionUsesServerModelName(t *testing.T) {
	upstream := &cursorIDEUpstreamStub{chatBody: cursorIDEFrames(cursorIDETextPayload("ok"), cursorIDEUsagePayload(1, 1, 0, 0))}
	svc := ideTestGateway(upstream)
	svc.storeIDEModelCatalog(ideTestAccount(), []cursorIDEModel{{Name: "claude-sonnet-4-6", ServerName: "claude-4.6-sonnet-medium"}})
	body := `{"model":"claude-sonnet-4-6","stream":false,"messages":[{"role":"user","content":"hi"}]}`
	c, _ := newCursorGatewayTestContext(t, "/v1/messages", body, 3)

	_, err := svc.Forward(context.Background(), c, ideTestAccount(), []byte(body))
	require.NoError(t, err)

	upstream.mu.Lock()
	defer upstream.mu.Unlock()
	require.Len(t, upstream.requests, 1)
	frames, decodeErr := cursorpkg.NewConnectDecoder(8<<20, 16<<20).Feed(upstream.bodies[0])
	require.NoError(t, decodeErr)
	require.Len(t, frames, 1)
	runRequest := firstServiceBytesField(t, frames[0].Payload, 1)
	model := firstServiceBytesField(t, runRequest, 3)
	require.Equal(t, "claude-4.6-sonnet-medium", firstServiceStringField(t, model, 1))
	require.False(t, hasServiceProtoField(runRequest, 12), "exclude_workspace_context must stay omitted")
}

func TestCursorIDEGrokLogicalModelResolvesColdCatalogAndRestoresUsage(t *testing.T) {
	upstream := &cursorIDEUpstreamStub{
		modelsBody: cursorAgentDetailedModelsPayload(
			cursorIDEModel{Name: "cursor-grok-4.5-high", ServerName: "cursor-grok-4.5-high", LegacySlugs: []string{"grok-4.5-xhigh"}},
			cursorIDEModel{Name: "cursor-grok-4.5-low", ServerName: "cursor-grok-4.5-low", LegacySlugs: []string{"grok-4.5-medium"}},
			cursorIDEModel{Name: "cursor-grok-4.5-medium", ServerName: "cursor-grok-4.5-medium", LegacySlugs: []string{"grok-4.5-high"}},
		),
		chatBody: cursorIDEFrames(cursorIDETextPayload("grok response"), cursorIDEGrokUsagePayload(9497, 12, 0, 1536)),
	}
	svc := ideTestGateway(upstream)
	body := `{"model":"grok-4.5","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	c, recorder := newCursorGatewayTestContext(t, "/v1/messages", body, 3)

	result, err := svc.Forward(context.Background(), c, ideTestAccount(), []byte(body))
	require.NoError(t, err)
	require.Equal(t, 7961, result.Usage.InputTokens)
	require.Equal(t, 12, result.Usage.OutputTokens)
	require.Equal(t, 1536, result.Usage.CacheReadInputTokens)
	require.Contains(t, recorder.Body.String(), `"cache_read_input_tokens":1536`)

	upstream.mu.Lock()
	defer upstream.mu.Unlock()
	require.Len(t, upstream.requests, 1)
	require.Equal(t, cursorpkg.AgentRunPath, upstream.requests[0].URL.Path)
	frames, decodeErr := cursorpkg.NewConnectDecoder(8<<20, 16<<20).Feed(upstream.bodies[0])
	require.NoError(t, decodeErr)
	require.Len(t, frames, 1)
	runRequest := firstServiceBytesField(t, frames[0].Payload, 1)
	model := firstServiceBytesField(t, runRequest, 3)
	require.Equal(t, "cursor-grok-4.5-high", firstServiceStringField(t, model, 1))
}

func TestCursorIDEGrokLogicalModelPrefersDefaultPrewarmedVariant(t *testing.T) {
	upstream := &cursorIDEUpstreamStub{modelsBody: cursorAgentDetailedModelsPayload(
		cursorIDEModel{Name: "cursor-grok-4.5-high", ServerName: "cursor-grok-4.5-high"},
		cursorIDEModel{Name: "cursor-grok-4.5-low", ServerName: "cursor-grok-4.5-low"},
		cursorIDEModel{Name: "cursor-grok-4.5-medium", ServerName: "cursor-grok-4.5-medium"},
	)}
	svc := ideTestGateway(upstream)
	account := ideTestAccount()
	_, err := svc.fetchIDEModelCatalog(context.Background(), account)
	require.NoError(t, err)

	selection, err := svc.resolveCursorIDEModel(context.Background(), account, "grok-4.5", cursorVariantPreference{})
	require.NoError(t, err)
	require.Equal(t, "cursor-grok-4.5-high", selection.ServerName)
}

func TestCursorIDEModelResolutionInfersUnaliasedAgentVariants(t *testing.T) {
	svc := ideTestGateway(&cursorIDEUpstreamStub{})
	account := ideTestAccount()
	svc.storeIDEModelCatalog(account, []cursorIDEModel{
		{Name: "claude-fable-5-thinking-high", ServerName: "claude-fable-5-thinking-high"},
		{Name: "claude-fable-5-low", ServerName: "claude-fable-5-low"},
		{Name: "claude-fable-5-medium", ServerName: "claude-fable-5-medium"},
		{Name: "claude-fable-5-thinking-medium", ServerName: "claude-fable-5-thinking-medium"},
		{Name: "claude-fable-5-high", ServerName: "claude-fable-5-high"},
	})

	selection, err := svc.resolveCursorIDEModel(context.Background(), account, "claude-fable-5", cursorVariantPreference{})
	require.NoError(t, err)
	require.Equal(t, "claude-fable-5-medium", selection.ServerName)
	require.False(t, selection.Thinking)
	require.Equal(t, "medium", selection.Effort)

	thinking := true
	selection, err = svc.resolveCursorIDEModel(context.Background(), account, "claude-fable-5", cursorVariantPreference{Thinking: &thinking, Effort: "high"})
	require.NoError(t, err)
	require.Equal(t, "claude-fable-5-thinking-high", selection.ServerName)
	require.True(t, selection.Thinking)
	require.Equal(t, "high", selection.Effort)
}

func TestCursorIDEModelResolutionRoutesLogicalModelByThinkingAndEffort(t *testing.T) {
	upstream := &cursorIDEUpstreamStub{}
	svc := ideTestGateway(upstream)
	account := ideTestAccount()
	svc.storeIDEModelCatalog(account, []cursorIDEModel{
		{Name: "claude-4.6-sonnet-low", ServerName: "claude-4.6-sonnet-low"},
		{Name: "claude-4.6-sonnet-medium-thinking", ServerName: "claude-4.6-sonnet-medium-thinking", DefaultOn: true, SupportsThinking: true, LegacySlugs: []string{"claude-sonnet-4-6"}},
		{Name: "claude-4.6-sonnet-high-thinking", ServerName: "claude-4.6-sonnet-high-thinking", SupportsThinking: true, LegacySlugs: []string{"claude-sonnet-4-6"}},
	})

	selection, err := svc.resolveCursorIDEModel(context.Background(), account, "claude-sonnet-4-6", cursorVariantPreference{})
	require.NoError(t, err)
	require.Equal(t, "claude-4.6-sonnet-medium-thinking", selection.ServerName)
	require.True(t, selection.Thinking)
	require.Equal(t, "medium", selection.Effort)

	thinkingDisabled := false
	selection, err = svc.resolveCursorIDEModel(context.Background(), account, "claude-sonnet-4-6", cursorVariantPreference{Thinking: &thinkingDisabled, Effort: "low"})
	require.NoError(t, err)
	require.Equal(t, "claude-4.6-sonnet-low", selection.ServerName)
	require.False(t, selection.Thinking)
	require.Equal(t, "low", selection.Effort)

	thinkingEnabled := true
	selection, err = svc.resolveCursorIDEModel(context.Background(), account, "claude-sonnet-4-6", cursorVariantPreference{Thinking: &thinkingEnabled, Effort: "high"})
	require.NoError(t, err)
	require.Equal(t, "claude-4.6-sonnet-high-thinking", selection.ServerName)
	require.True(t, selection.Thinking)
	require.Equal(t, "high", selection.Effort)
}

func TestCursorAgentModelCatalogRefreshUsesSingleflight(t *testing.T) {
	svc := ideTestGateway(&cursorIDEUpstreamStub{})
	account := ideTestAccount()
	begin := make(chan struct{})
	release := make(chan struct{})
	var ready sync.WaitGroup
	var callsMu sync.Mutex
	calls := 0
	const workers = 8
	ready.Add(workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			ready.Done()
			<-begin
			_, err := svc.refreshIDEModelCatalog(context.Background(), account, func(context.Context) ([]cursorIDEModel, error) {
				callsMu.Lock()
				calls++
				callsMu.Unlock()
				<-release
				return []cursorIDEModel{{Name: "model", ServerName: "model"}}, nil
			})
			require.NoError(t, err)
		}()
	}
	ready.Wait()
	close(begin)
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()
	callsMu.Lock()
	defer callsMu.Unlock()
	require.Equal(t, 1, calls)
}

func TestCursorIDEVariantParsingPreservesCodexMaxLogicalModel(t *testing.T) {
	require.Equal(t, "gpt-5.1-codex-max", cursorIDEVariantFamily("gpt-5.1-codex-max-high"))
	require.Equal(t, "high", cursorIDEVariantEffort("gpt-5.1-codex-max-high"))
	require.Equal(t, "gpt-5.1-codex-max", cursorIDEVariantFamily("gpt-5.1-codex-max"))
	require.Empty(t, cursorIDEVariantEffort("gpt-5.1-codex-max"))
	require.Equal(t, "claude-fable-5", cursorIDEVariantFamily("claude-fable-5-thinking-max"))
	require.Equal(t, "max", cursorIDEVariantEffort("claude-fable-5-thinking-max"))
}

func TestCursorIDETransportAndMetadataCompatibility(t *testing.T) {
	svc := ideTestGateway(&cursorIDEUpstreamStub{})
	autoLegacy := &Account{ID: 1, Platform: PlatformCursor, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key": "cloud-key", "dashboard_access_token": "dashboard-token",
	}}
	require.Equal(t, CursorTransportIDEChat, svc.cursorTransportMode(autoLegacy))
	require.Equal(t, CursorTransportCloudAgent, svc.cursorForwardTransportMode(autoLegacy))
	autoLegacy.Credentials["cursor_machine_id"] = "machine"
	require.Equal(t, CursorTransportIDEChat, svc.cursorForwardTransportMode(autoLegacy))
	require.Equal(t, "x64", cursorIDEClientArch("amd64"))
	require.Equal(t, cursorpkg.AgentModeAsk, prepareCursorAgentMode(&cursorpkg.Dialogue{}))
}

func hasServiceProtoField(payload []byte, wanted protowire.Number) bool {
	for len(payload) > 0 {
		number, wireType, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return false
		}
		payload = payload[n:]
		if number == wanted {
			return true
		}
		size := protowire.ConsumeFieldValue(number, wireType, payload)
		if size < 0 {
			return false
		}
		payload = payload[size:]
	}
	return false
}

func firstServiceBytesField(t *testing.T, payload []byte, wanted protowire.Number) []byte {
	t.Helper()
	for len(payload) > 0 {
		number, wireType, n := protowire.ConsumeTag(payload)
		require.GreaterOrEqual(t, n, 0)
		payload = payload[n:]
		if wireType == protowire.BytesType {
			value, size := protowire.ConsumeBytes(payload)
			require.GreaterOrEqual(t, size, 0)
			payload = payload[size:]
			if number == wanted {
				return value
			}
			continue
		}
		size := protowire.ConsumeFieldValue(number, wireType, payload)
		require.GreaterOrEqual(t, size, 0)
		payload = payload[size:]
	}
	t.Fatalf("field %d not found", wanted)
	return nil
}

func firstServiceStringField(t *testing.T, payload []byte, wanted protowire.Number) string {
	t.Helper()
	return string(firstServiceBytesField(t, payload, wanted))
}

func firstServiceVarintField(t *testing.T, payload []byte, wanted protowire.Number) uint64 {
	t.Helper()
	for len(payload) > 0 {
		number, wireType, n := protowire.ConsumeTag(payload)
		require.GreaterOrEqual(t, n, 0)
		payload = payload[n:]
		if wireType == protowire.VarintType {
			value, size := protowire.ConsumeVarint(payload)
			require.GreaterOrEqual(t, size, 0)
			payload = payload[size:]
			if number == wanted {
				return value
			}
			continue
		}
		size := protowire.ConsumeFieldValue(number, wireType, payload)
		require.GreaterOrEqual(t, size, 0)
		payload = payload[size:]
	}
	t.Fatalf("field %d not found", wanted)
	return 0
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

func cursorIDEErrorFrame(code, message string) []byte {
	payload, err := json.Marshal(map[string]any{"error": map[string]any{"code": code, "message": message}})
	if err != nil {
		panic(err)
	}
	frame, err := cursorpkg.EncodeConnectFrame(payload, false)
	if err != nil {
		panic(err)
	}
	frame[0] = 2
	return frame
}

func cursorIDETextPayload(text string) []byte {
	assistant := appendProtoString(nil, 1, text)
	interaction := protowire.AppendTag(nil, 1, protowire.BytesType)
	interaction = protowire.AppendBytes(interaction, assistant)
	server := protowire.AppendTag(nil, 1, protowire.BytesType)
	return protowire.AppendBytes(server, interaction)
}

func cursorIDEToolPayload(id, name, arguments string, _ bool) []byte {
	var values map[string]any
	if err := json.Unmarshal([]byte(arguments), &values); err != nil {
		panic(err)
	}
	mcp := appendProtoString(nil, 1, name)
	for key, value := range values {
		entry := appendProtoString(nil, 1, key)
		entry = protowire.AppendTag(entry, 2, protowire.BytesType)
		entry = protowire.AppendBytes(entry, cursorAgentProtoValue(value))
		mcp = protowire.AppendTag(mcp, 2, protowire.BytesType)
		mcp = protowire.AppendBytes(mcp, entry)
	}
	mcp = appendProtoString(mcp, 3, id)
	mcp = appendProtoString(mcp, 4, "sub2api")
	mcp = appendProtoString(mcp, 5, name)
	exec := protowire.AppendTag(nil, 1, protowire.VarintType)
	exec = protowire.AppendVarint(exec, 42)
	exec = appendProtoString(exec, 15, "exec-"+id)
	exec = protowire.AppendTag(exec, 11, protowire.BytesType)
	exec = protowire.AppendBytes(exec, mcp)
	server := protowire.AppendTag(nil, 2, protowire.BytesType)
	return protowire.AppendBytes(server, exec)
}

func cursorIDEUsagePayload(input, output, cacheWrite, cacheRead int) []byte {
	var usage []byte
	for field, value := range map[protowire.Number]int{1: input, 2: output, 3: cacheRead, 4: cacheWrite} {
		usage = protowire.AppendTag(usage, field, protowire.VarintType)
		usage = protowire.AppendVarint(usage, uint64(value))
	}
	return cursorIDETurnEndedPayload(usage)
}

func cursorIDEGrokUsagePayload(input, output, cacheWrite, cacheRead int) []byte {
	var turnEnded []byte
	for _, field := range []struct {
		number protowire.Number
		value  int
	}{
		{number: 1, value: input},
		{number: 2, value: output},
		{number: 3, value: cacheRead},
		{number: 4, value: cacheWrite},
	} {
		turnEnded = protowire.AppendTag(turnEnded, field.number, protowire.VarintType)
		turnEnded = protowire.AppendVarint(turnEnded, uint64(field.value))
	}
	interaction := protowire.AppendTag(nil, 14, protowire.BytesType)
	interaction = protowire.AppendBytes(interaction, turnEnded)
	server := protowire.AppendTag(nil, 1, protowire.BytesType)
	return protowire.AppendBytes(server, interaction)
}

func cursorIDETurnEndedPayload(usage []byte) []byte {
	var turnEnded []byte
	if usage != nil {
		turnEnded = protowire.AppendTag(turnEnded, 1, protowire.BytesType)
		turnEnded = protowire.AppendBytes(turnEnded, usage)
	}
	turnEnded = protowire.AppendTag(turnEnded, 2, protowire.VarintType)
	turnEnded = protowire.AppendVarint(turnEnded, 1)
	interaction := protowire.AppendTag(nil, 14, protowire.BytesType)
	interaction = protowire.AppendBytes(interaction, turnEnded)
	server := protowire.AppendTag(nil, 1, protowire.BytesType)
	return protowire.AppendBytes(server, interaction)
}

func cursorAgentModelsPayload(models ...string) []byte {
	var payload []byte
	for _, name := range models {
		model := appendProtoString(nil, 1, name)
		model = appendProtoString(model, 3, name)
		model = appendProtoString(model, 4, name)
		model = appendProtoString(model, 5, name)
		payload = protowire.AppendTag(payload, 1, protowire.BytesType)
		payload = protowire.AppendBytes(payload, model)
	}
	return payload
}

func cursorAgentDetailedModelsPayload(models ...cursorIDEModel) []byte {
	var payload []byte
	for _, item := range models {
		model := appendProtoString(nil, 1, item.ServerName)
		if item.SupportsThinking {
			model = protowire.AppendTag(model, 2, protowire.BytesType)
			model = protowire.AppendBytes(model, nil)
		}
		model = appendProtoString(model, 3, item.Name)
		for _, alias := range item.LegacySlugs {
			model = appendProtoString(model, 6, alias)
		}
		payload = protowire.AppendTag(payload, 1, protowire.BytesType)
		payload = protowire.AppendBytes(payload, model)
	}
	return payload
}

func cursorAgentProtoValue(value any) []byte {
	switch typed := value.(type) {
	case string:
		return appendProtoString(nil, 3, typed)
	case bool:
		result := protowire.AppendTag(nil, 4, protowire.VarintType)
		if typed {
			return protowire.AppendVarint(result, 1)
		}
		return protowire.AppendVarint(result, 0)
	case float64:
		result := protowire.AppendTag(nil, 2, protowire.Fixed64Type)
		return protowire.AppendFixed64(result, math.Float64bits(typed))
	default:
		return appendProtoString(nil, 3, fmt.Sprint(typed))
	}
}

func appendProtoString(dst []byte, number protowire.Number, value string) []byte {
	dst = protowire.AppendTag(dst, number, protowire.BytesType)
	return protowire.AppendString(dst, value)
}
