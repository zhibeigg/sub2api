package cursor

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
)

func TestAgentRunRequestGoldenAndDescriptorFields(t *testing.T) {
	dialogue := &Dialogue{
		System: "Be concise.",
		Messages: []DialogueMessage{
			{Role: "user", Text: "old question"},
			{Role: "assistant", Text: "calling", ToolCalls: []Action{{ID: "call-1", Name: "lookup", Arguments: map[string]any{"q": "go"}}}},
			{Role: "tool", ToolCallID: "call-1", Text: "old result"},
			{Role: "user", Text: "new question"},
		},
		Tools: []ToolDefinition{{Name: "lookup", Description: "Search", InputSchema: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`)}},
	}
	payload, err := encodeAgentRunRequest(dialogue, AgentRunOptions{
		Model: "claude-4-sonnet", ConversationID: "conversation", ConversationGroupID: "group", Mode: AgentModeAgent,
		WorkspacePaths: []string{"D:/repo"}, ProjectFolder: "D:/repo", Shell: "bash", ClientSupportsSend: true,
		RequestContext: AgentRequestContext{OSVersion: "Windows 11", TimeZone: "America/New_York", MCPInfoComplete: true, EnvInfoComplete: true},
	}, sequenceUUID("message"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	if got, want := hex.EncodeToString(sum[:]), "6508afb28df5dc2006276ceaa1ddcbbedfe10a4079733005c0fee02d9abd8c26"; got != want {
		t.Fatalf("Agent request golden SHA256 = %s, want %s", got, want)
	}

	runRequest := firstBytesField(t, payload, 1)
	for _, number := range []protowire.Number{1, 2, 3, 4, 5, 9, 16, 23} {
		if !hasField(allFields(runRequest), number) {
			t.Fatalf("AgentRunRequest field %d missing", number)
		}
	}
	action := firstBytesField(t, runRequest, 2)
	userAction := firstBytesField(t, action, 1)
	userMessage := firstBytesField(t, userAction, 1)
	if firstStringField(t, userMessage, 1) != "new question" || !hasVarint(allFields(userMessage), 4, 1) {
		t.Fatalf("unexpected UserMessage: %x", userMessage)
	}
	requestContext := firstBytesField(t, userAction, 2)
	env := firstBytesField(t, requestContext, 4)
	if firstStringField(t, env, 1) != "Windows 11" || firstStringField(t, env, 11) != "D:/repo" {
		t.Fatalf("unexpected RequestContextEnv: %x", env)
	}
	history := firstBytesField(t, userAction, 7)
	historyMessages := bytesFields(history, 1)
	if len(historyMessages) != 3 {
		t.Fatalf("history message count = %d", len(historyMessages))
	}
	toolHistory := firstBytesField(t, historyMessages[2], 3)
	if firstStringField(t, toolHistory, 2) != "lookup" {
		t.Fatalf("history tool_name missing: %x", toolHistory)
	}
	mcpTools := firstBytesField(t, runRequest, 4)
	tool := firstBytesField(t, mcpTools, 1)
	if firstStringField(t, tool, 4) != "sub2api" || firstStringField(t, tool, 5) != "lookup" {
		t.Fatalf("unexpected MCP tool: %x", tool)
	}
	inputSchemaValue := firstBytesField(t, tool, 3)
	if !hasField(allFields(inputSchemaValue), 5) {
		t.Fatalf("MCP input schema is not wrapped as google.protobuf.Value.struct_value: %x", inputSchemaValue)
	}
}

func TestAgentRunRequestKeepsTrailingAssistantAndToolHistory(t *testing.T) {
	dialogue := &Dialogue{Messages: []DialogueMessage{
		{Role: "user", Text: "question"},
		{Role: "assistant", ToolCalls: []Action{{ID: "call-1", Name: "lookup", Arguments: map[string]any{"q": "go"}}}},
		{Role: "tool", ToolCallID: "call-1", Text: "result"},
	}}
	payload, err := encodeAgentRunRequest(dialogue, AgentRunOptions{Model: "model"}, sequenceUUID("message", "conversation"))
	if err != nil {
		t.Fatal(err)
	}
	runRequest := firstBytesField(t, payload, 1)
	userAction := firstBytesField(t, firstBytesField(t, runRequest, 2), 1)
	if firstStringField(t, firstBytesField(t, userAction, 1), 1) != "" {
		t.Fatal("non-user tail was incorrectly reused as an older user action")
	}
	historyMessages := bytesFields(firstBytesField(t, userAction, 7), 1)
	if len(historyMessages) != 3 {
		t.Fatalf("trailing history was truncated: %d", len(historyMessages))
	}
	toolHistory := firstBytesField(t, historyMessages[2], 3)
	if firstStringField(t, toolHistory, 2) != "lookup" {
		t.Fatalf("tool_name = %q", firstStringField(t, toolHistory, 2))
	}
}

func TestAgentGetUsableModelsUnaryCodec(t *testing.T) {
	var received []byte
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 2 {
			t.Errorf("protocol = %s", r.Proto)
		}
		if r.URL.Path != AgentGetUsableModelsPath || r.Header.Get("Content-Type") != "application/proto" {
			t.Errorf("unexpected request %s %q", r.URL.Path, r.Header.Get("Content-Type"))
		}
		received, _ = io.ReadAll(r.Body)
		model := appendString(nil, 1, "claude-4-sonnet")
		model = appendBytes(model, 2, nil)
		model = appendString(model, 3, "claude-4-sonnet-thinking")
		model = appendString(model, 4, "Claude 4 Sonnet")
		model = appendString(model, 5, "Sonnet 4")
		model = appendString(model, 6, "sonnet")
		model = appendVarint(model, 7, 1)
		var compressed bytes.Buffer
		zipWriter := gzip.NewWriter(&compressed)
		_, _ = zipWriter.Write(appendBytes(nil, 1, model))
		_ = zipWriter.Close()
		w.Header().Set("Content-Type", "application/proto")
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(compressed.Bytes())
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	client, err := NewAgentClient(server.Client(), AgentClientConfig{IDEClientConfig: IDEClientConfig{BaseURL: server.URL}})
	if err != nil {
		t.Fatal(err)
	}
	models, err := client.GetUsableModels(context.Background(), IDECredential{AccessToken: "token"}, []string{"custom-a", "custom-b"})
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{firstStringField(t, received, 1), string(bytesFields(received, 1)[1])}; !reflect.DeepEqual(got, []string{"custom-a", "custom-b"}) {
		t.Fatalf("custom models = %#v", got)
	}
	if len(models) != 1 || models[0].ID != "claude-4-sonnet" || !models[0].SupportsThinking || !models[0].SupportsMaxMode || models[0].Aliases[0] != "sonnet" {
		t.Fatalf("models = %#v", models)
	}
}

func TestAgentEventStreamMCPAggregationKVExecAndUsage(t *testing.T) {
	mcpArgs := appendString(nil, 3, "call-1")
	mcpArgs = appendString(mcpArgs, 5, "lookup")
	mcpCall := appendBytes(nil, 1, mcpArgs)
	toolCall := appendBytes(nil, 15, mcpCall)
	toolCall = appendString(toolCall, 57, "call-1")
	started := appendString(nil, 1, "call-1")
	started = appendBytes(started, 2, toolCall)
	partial := appendString(nil, 1, "call-1")
	partial = appendBytes(partial, 2, toolCall)
	partial = appendString(partial, 3, `{"q":"go"}`)
	completed := appendString(nil, 1, "call-1")
	completed = appendBytes(completed, 2, toolCall)
	turn := appendVarint(nil, 1, 10)
	turn = appendVarint(turn, 2, 4)
	turn = appendVarint(turn, 3, 2)
	turn = appendVarint(turn, 5, 1)
	interaction := appendBytes(nil, 2, started)
	interaction = appendBytes(interaction, 7, partial)
	interaction = appendBytes(interaction, 3, completed)
	interaction = appendBytes(interaction, 14, turn)

	execArgs := appendString(nil, 3, "exec-call")
	execArgs = appendString(execArgs, 5, "lookup")
	exec := appendVarint(nil, 1, 9)
	exec = appendString(exec, 15, "exec-1")
	exec = appendBytes(exec, 11, execArgs)
	kvArgs := appendBytes(nil, 1, []byte("blob"))
	kv := appendVarint(nil, 1, 7)
	kv = appendBytes(kv, 2, kvArgs)
	serverMessage := appendBytes(nil, 1, interaction)
	serverMessage = appendBytes(serverMessage, 2, exec)
	serverMessage = appendBytes(serverMessage, 4, kv)
	frame, _ := EncodeConnectFrame(serverMessage, false)
	frame = append(frame, encodeAgentConnectEndStream()...)
	stream := &AgentStream{response: &http.Response{Body: io.NopCloser(bytes.NewReader(frame))}, decoder: NewConnectDecoder(4096, 8192), tools: make(map[string]*agentToolAccumulator), maxToolBytes: 4096}
	var events []AgentEvent
	for {
		event, err := stream.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	if len(events) != 9 {
		t.Fatalf("event count = %d: %#v", len(events), events)
	}
	if events[0].Type != AgentEventToolStarted || events[1].Type != AgentEventToolPartial || events[2].Type != AgentEventToolCompleted {
		t.Fatalf("tool events = %#v", events[:3])
	}
	if events[2].Tool.Name != "lookup" || events[2].Tool.Arguments["q"] != "go" {
		t.Fatalf("completed tool = %#v", events[2].Tool)
	}
	if events[3].Type != AgentEventUsage || events[3].Usage.TotalTokens != 14 || events[3].Usage.CacheReadTokens != 2 {
		t.Fatalf("usage = %#v", events[3])
	}
	if events[5].Type != AgentEventFinish {
		t.Fatalf("turn did not produce logical finish: %#v", events[5])
	}
	if events[6].Type != AgentEventExecMCP || events[6].ExecMCP.Name != "lookup" || events[6].ExecRequestID != 9 || events[6].ExecID != "exec-1" {
		t.Fatalf("exec MCP = %#v", events[6])
	}
	if events[7].Type != AgentEventKVGet || string(events[7].KV.BlobID) != "blob" {
		t.Fatalf("KV = %#v", events[7])
	}
}

func TestAgentStreamKVAndMCPResponsesPreserveRequestIDs(t *testing.T) {
	stream := &AgentStream{ctx: context.Background(), send: make(chan []byte, 5)}
	if err := stream.SendKVGetResult(7, []byte("data")); err != nil {
		t.Fatal(err)
	}
	if err := stream.SendKVSetResult(8); err != nil {
		t.Fatal(err)
	}
	if err := stream.SendMCPResult(9, "exec-1", "ok", false); err != nil {
		t.Fatal(err)
	}
	if err := stream.SendMCPResult(10, "exec-2", "failed", true); err != nil {
		t.Fatal(err)
	}
	if err := stream.SendResume(AgentRunOptions{RequestContext: AgentRequestContext{OSVersion: "Windows", ProjectFolder: "D:/repo"}}); err != nil {
		t.Fatal(err)
	}
	getMessage := <-stream.send
	getKV := firstBytesField(t, getMessage, 3)
	if firstProtoVarint(getKV, 1) != 7 || string(firstProtoBytes(firstProtoBytes(getKV, 2), 1)) != "data" {
		t.Fatalf("KV get response = %x", getMessage)
	}
	setMessage := <-stream.send
	setKV := firstBytesField(t, setMessage, 3)
	if firstProtoVarint(setKV, 1) != 8 || firstProtoBytes(setKV, 3) == nil {
		t.Fatalf("KV set response = %x", setMessage)
	}
	mcpMessage := <-stream.send
	exec := firstBytesField(t, mcpMessage, 2)
	if firstProtoVarint(exec, 1) != 9 || firstProtoString(exec, 15) != "exec-1" || firstProtoBytes(exec, 11) == nil {
		t.Fatalf("MCP response = %x", mcpMessage)
	}
	errorMessage := <-stream.send
	errorExec := firstBytesField(t, errorMessage, 2)
	errorResult := firstBytesField(t, errorExec, 11)
	errorSuccess := firstBytesField(t, errorResult, 1)
	if firstProtoVarint(errorExec, 1) != 10 || !hasVarint(allFields(errorSuccess), 2, 1) {
		t.Fatalf("MCP error success-envelope = %x", errorMessage)
	}
	resumeMessage := <-stream.send
	conversationAction := firstBytesField(t, resumeMessage, 4)
	resume := firstBytesField(t, conversationAction, 2)
	requestContext := firstBytesField(t, resume, 2)
	if firstStringField(t, firstBytesField(t, requestContext, 4), 11) != "D:/repo" {
		t.Fatalf("resume request context = %x", resumeMessage)
	}
}

func TestAgentExecOnlyRecognizesMCP(t *testing.T) {
	shell := appendString(nil, 1, "rm -rf /")
	exec := appendVarint(nil, 1, 3)
	exec = appendString(exec, 15, "exec-local")
	exec = appendBytes(exec, 2, shell)
	events := parseAgentExecServerMessage(exec)
	if len(events) != 1 || events[0].Type != AgentEventUnsupportedExec || events[0].Unsupported.Field != 2 || events[0].Unsupported.ExecID != "exec-local" {
		t.Fatalf("unexpected local exec handling: %#v", events)
	}
}

func TestAgentClientHTTP2TrueDuplexAndIdempotentClose(t *testing.T) {
	requestDone := make(chan struct{})
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 2 {
			t.Errorf("protocol = %s", r.Proto)
		}
		first, err := readConnectFrame(r.Body)
		if err != nil || len(first.Payload) == 0 {
			t.Errorf("initial frame: %#v %v", first, err)
			return
		}
		w.Header().Set("Content-Type", "application/connect+proto")
		text := appendString(nil, 1, "duplex")
		interaction := appendBytes(nil, 1, text)
		message := appendBytes(nil, 1, interaction)
		response, _ := EncodeConnectFrame(message, false)
		_, _ = w.Write(response)
		w.(http.Flusher).Flush()
		second, err := readConnectFrame(r.Body)
		if err != nil || firstProtoBytes(second.Payload, 7) == nil {
			t.Errorf("second frame is not heartbeat: %#v %v", second, err)
		}
		end, err := readConnectFrame(r.Body)
		if err != nil || !end.EndStream() {
			t.Errorf("client end stream: %#v %v", end, err)
		}
		_, _ = w.Write(encodeAgentConnectEndStream())
		close(requestDone)
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	client, err := NewAgentClient(server.Client(), AgentClientConfig{IDEClientConfig: IDEClientConfig{BaseURL: server.URL}, HeartbeatInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	_, stream, err := client.Run(context.Background(), IDECredential{AccessToken: "token"}, &Dialogue{Messages: []DialogueMessage{{Role: "user", Text: "hi"}}}, AgentRunOptions{Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	event, err := stream.Next()
	if err != nil || event.Type != AgentEventText || event.Text != "duplex" {
		t.Fatalf("duplex event = %#v %v", event, err)
	}
	if err := stream.SendClientMessage(encodeAgentClientHeartbeat()); err != nil {
		t.Fatal(err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}
	finish, err := stream.Next()
	if err != nil || finish.Type != AgentEventFinish {
		t.Fatalf("finish = %#v %v", finish, err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-requestDone:
	case <-time.After(time.Second):
		t.Fatal("server did not observe duplex client close")
	}
}

func TestAgentStreamCancellationCleansRequest(t *testing.T) {
	var cancelled atomic.Bool
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = readConnectFrame(r.Body)
		w.Header().Set("Content-Type", "application/connect+proto")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		cancelled.Store(true)
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	client, err := NewAgentClient(server.Client(), AgentClientConfig{IDEClientConfig: IDEClientConfig{BaseURL: server.URL}, HeartbeatInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	_, stream, err := client.Run(context.Background(), IDECredential{AccessToken: "token"}, &Dialogue{Messages: []DialogueMessage{{Role: "user", Text: "hi"}}}, AgentRunOptions{Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	_ = stream.Close()
	_ = stream.Close()
	deadline := time.Now().Add(time.Second)
	for !cancelled.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !cancelled.Load() {
		t.Fatal("request context was not cancelled")
	}
}

func readConnectFrame(reader io.Reader) (ConnectFrame, error) {
	header := make([]byte, 5)
	if _, err := io.ReadFull(reader, header); err != nil {
		return ConnectFrame{}, err
	}
	length := int(uint32(header[1])<<24 | uint32(header[2])<<16 | uint32(header[3])<<8 | uint32(header[4]))
	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return ConnectFrame{}, err
	}
	return ConnectFrame{Flags: header[0], Payload: payload}, nil
}
