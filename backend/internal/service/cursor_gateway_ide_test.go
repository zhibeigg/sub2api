package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"testing"
	"time"

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
	s.mu.Unlock()

	status := http.StatusOK
	responseBody := s.chatBody
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
	cloud       cursorGatewayUpstreamStub
	ideChatBody []byte
}

func (s *cursorIDEAutoFallbackUpstreamStub) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	switch req.URL.Path {
	case cursorpkg.AgentGetUsableModelsPath:
		return cursorIDEHTTP2Response(req, cursorAgentModelsPayload("claude-sonnet-5")), nil
	case cursorpkg.AgentRunPath:
		_, _ = readCursorConnectFrame(req.Body)
		go func() { _, _ = io.Copy(io.Discard, req.Body) }()
		return cursorIDEHTTP2Response(req, s.ideChatBody), nil
	default:
		return s.cloud.Do(req, proxyURL, accountID, accountConcurrency)
	}
}

func (s *cursorIDEAutoFallbackUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
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
	if stub, ok := upstream.(*cursorIDEUpstreamStub); ok && len(stub.modelsBody) == 0 {
		stub.modelsBody = cursorAgentModelsPayload("claude-sonnet-5")
	}
	return NewCursorGatewayService(upstream, nil, nil, nil, &config.Config{Cursor: config.CursorConfig{
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
	require.Equal(t, cursorpkg.AgentRunPath, upstream.requests[0].URL.Path)
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
	)}
	gateway := ideTestGateway(upstream)
	svc := NewAccountTestService(nil, nil, nil, nil, nil, nil, &config.Config{}, nil)
	svc.SetCursorGatewayService(gateway)

	models, err := svc.FetchUpstreamSupportedModels(context.Background(), ideTestAccount())
	require.NoError(t, err)
	require.Equal(t, []string{"claude-fable-5", "claude-sonnet-4-6"}, models)
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
	interaction := protowire.AppendTag(nil, 14, protowire.BytesType)
	interaction = protowire.AppendBytes(interaction, usage)
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
