package cursor

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
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
	state, blobs, err := PrepareAgentConversationState(dialogue, nil, nil, sequenceUUID("history-message"))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := encodeAgentRunRequest(dialogue, AgentRunOptions{
		Model: "claude-4-sonnet", ConversationID: "conversation", ConversationGroupID: "group", Mode: AgentModeAgent,
		WorkspacePaths: []string{"D:/repo"}, ProjectFolder: "D:/repo", Shell: "bash", ClientSupportsSend: true,
		ConversationState: state,
		RequestContext:    AgentRequestContext{OSVersion: "Windows 11", TimeZone: "America/New_York", MCPInfoComplete: true, EnvInfoComplete: true},
	}, sequenceUUID("message"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	if got, want := hex.EncodeToString(sum[:]), "61caa916d2b8d9629949f8ba4b8dc8986cbf85ec084dda634481c799893318e5"; got != want {
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
	if hasField(allFields(userAction), 7) {
		t.Fatalf("UserMessageAction contains obsolete history field 7: %x", userAction)
	}
	requestContext := firstBytesField(t, userAction, 2)
	env := firstBytesField(t, requestContext, 4)
	if firstStringField(t, env, 1) != "Windows 11" || firstStringField(t, env, 11) != "D:/repo" {
		t.Fatalf("unexpected RequestContextEnv: %x", env)
	}

	encodedState := firstBytesField(t, runRequest, 1)
	rootPromptIDs := bytesFields(encodedState, 1)
	turnIDs := bytesFields(encodedState, 8)
	if len(rootPromptIDs) != 4 || len(turnIDs) != 1 {
		t.Fatalf("conversation state root=%d turns=%d", len(rootPromptIDs), len(turnIDs))
	}
	rootPrompt := agentTestBlob(t, blobs, rootPromptIDs[0])
	if !bytes.Contains(rootPrompt, []byte(`"content":"Be concise."`)) {
		t.Fatalf("root prompt blob = %s", rootPrompt)
	}
	historyUserRoot := agentTestBlob(t, blobs, rootPromptIDs[1])
	if !bytes.Contains(historyUserRoot, []byte(`"role":"user"`)) || !bytes.Contains(historyUserRoot, []byte("old question")) {
		t.Fatalf("history user root prompt blob = %s", historyUserRoot)
	}
	historyAssistantRoot := agentTestBlob(t, blobs, rootPromptIDs[2])
	if !bytes.Contains(historyAssistantRoot, []byte(`"role":"assistant"`)) || !bytes.Contains(historyAssistantRoot, []byte("calling")) || !bytes.Contains(historyAssistantRoot, []byte("[Tool: lookup]")) {
		t.Fatalf("history assistant root prompt blob = %s", historyAssistantRoot)
	}
	historyToolRoot := agentTestBlob(t, blobs, rootPromptIDs[3])
	if !bytes.Contains(historyToolRoot, []byte(`"role":"user"`)) || !bytes.Contains(historyToolRoot, []byte("old result")) {
		t.Fatalf("history tool root prompt blob = %s", historyToolRoot)
	}
	turn := agentTestBlob(t, blobs, turnIDs[0])
	agentTurn := firstBytesField(t, turn, 1)
	if requestID := firstStringField(t, agentTurn, 3); requestID == "" {
		t.Fatalf("history turn request_id missing: %x", agentTurn)
	}
	historyUser := agentTestBlob(t, blobs, firstBytesField(t, agentTurn, 1))
	if firstStringField(t, historyUser, 1) != "old question" {
		t.Fatalf("history user = %x", historyUser)
	}
	stepIDs := bytesFields(agentTurn, 2)
	if len(stepIDs) != 2 {
		t.Fatalf("history step count = %d", len(stepIDs))
	}
	firstStep := firstBytesField(t, agentTestBlob(t, blobs, stepIDs[0]), 1)
	if text := firstStringField(t, firstStep, 1); !strings.Contains(text, "calling") || !strings.Contains(text, "[Tool: lookup]") {
		t.Fatalf("assistant history text = %q", text)
	}
	secondStep := firstBytesField(t, agentTestBlob(t, blobs, stepIDs[1]), 1)
	if text := firstStringField(t, secondStep, 1); !strings.Contains(text, "old result") {
		t.Fatalf("tool history text = %q", text)
	}

	mcpTools := firstBytesField(t, runRequest, 4)
	tool := firstBytesField(t, mcpTools, 1)
	if firstStringField(t, tool, 1) != "sub2api-lookup" || firstStringField(t, tool, 4) != "sub2api" || firstStringField(t, tool, 5) != "lookup" {
		t.Fatalf("unexpected MCP tool: %x", tool)
	}
	inputSchemaValue := firstBytesField(t, tool, 3)
	if !hasField(allFields(inputSchemaValue), 5) {
		t.Fatalf("MCP input schema is not wrapped as google.protobuf.Value.struct_value: %x", inputSchemaValue)
	}
}

func TestAgentMCPToolNamesUseProviderScopedInternalName(t *testing.T) {
	tools := []ToolDefinition{
		{Name: "Glob", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "Grep", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "Read", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "Write", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
	encoded, err := encodeAgentMCPTools(tools, "sub2api")
	if err != nil {
		t.Fatal(err)
	}
	definitions := bytesFields(encoded, 1)
	if len(definitions) != len(tools) {
		t.Fatalf("MCP tool count = %d, want %d", len(definitions), len(tools))
	}
	for index, definition := range definitions {
		if got, want := firstProtoString(definition, 1), "sub2api-"+tools[index].Name; got != want {
			t.Fatalf("MCP internal name = %q, want %q", got, want)
		}
		if got := firstProtoString(definition, 5); got != tools[index].Name {
			t.Fatalf("MCP client-visible name = %q, want %q", got, tools[index].Name)
		}
	}
}

func TestAgentRunRequestKeepsTrailingAssistantAndToolHistory(t *testing.T) {
	dialogue := &Dialogue{Messages: []DialogueMessage{
		{Role: "user", Text: "question"},
		{Role: "assistant", ToolCalls: []Action{{ID: "call-1", Name: "lookup", Arguments: map[string]any{"q": "go"}}}},
		{Role: "tool", ToolCallID: "call-1", Text: "result"},
	}}
	state, blobs, err := PrepareAgentConversationState(dialogue, nil, nil, sequenceUUID("history-message"))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := encodeAgentRunRequest(dialogue, AgentRunOptions{Model: "model", ConversationState: state}, sequenceUUID("message", "conversation"))
	if err != nil {
		t.Fatal(err)
	}
	runRequest := firstBytesField(t, payload, 1)
	userAction := firstBytesField(t, firstBytesField(t, runRequest, 2), 1)
	if got := firstStringField(t, firstBytesField(t, userAction, 1), 1); got != "Continue the conversation using the latest tool result." {
		t.Fatalf("tool-result continuation action = %q", got)
	}
	turnIDs := bytesFields(firstBytesField(t, runRequest, 1), 8)
	if len(turnIDs) != 1 {
		t.Fatalf("trailing history turn count = %d", len(turnIDs))
	}
	agentTurn := firstBytesField(t, agentTestBlob(t, blobs, turnIDs[0]), 1)
	if got := firstStringField(t, agentTestBlob(t, blobs, firstBytesField(t, agentTurn, 1)), 1); got != "question" {
		t.Fatalf("history user = %q", got)
	}
	stepIDs := bytesFields(agentTurn, 2)
	if len(stepIDs) != 2 {
		t.Fatalf("trailing history step count = %d", len(stepIDs))
	}
	toolStep := firstBytesField(t, agentTestBlob(t, blobs, stepIDs[1]), 1)
	if text := firstStringField(t, toolStep, 1); !strings.Contains(text, "result") {
		t.Fatalf("tool history text = %q", text)
	}
}

func agentTestBlob(t *testing.T, blobs map[string][]byte, blobID []byte) []byte {
	t.Helper()
	key := base64.RawURLEncoding.EncodeToString(blobID)
	blob, ok := blobs[key]
	if !ok {
		t.Fatalf("blob %s is missing", key)
	}
	return blob
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
	mcpArgs := appendString(nil, 1, "sub2api-lookup")
	mcpArgs = appendString(mcpArgs, 3, "call-1")
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
	usage := appendVarint(nil, 1, 10)
	usage = appendVarint(usage, 2, 4)
	usage = appendVarint(usage, 3, 2)
	usage = appendVarint(usage, 4, 3)
	usage = appendVarint(usage, 5, 1)
	turnEnded := appendBytes(nil, 1, usage)
	turnEnded = appendVarint(turnEnded, 2, 1)
	interaction := appendBytes(nil, 2, started)
	interaction = appendBytes(interaction, 7, partial)
	interaction = appendBytes(interaction, 3, completed)
	interaction = appendBytes(interaction, 14, turnEnded)

	execArgs := appendString(nil, 1, "sub2api-lookup")
	execArgs = appendString(execArgs, 3, "exec-call")
	execArgs = appendString(execArgs, 5, "lookup")
	exec := appendVarint(nil, 1, 9)
	exec = appendString(exec, 15, "exec-1")
	exec = appendBytes(exec, 11, execArgs)
	kvArgs := appendBytes(nil, 1, []byte("blob"))
	kv := appendVarint(nil, 1, 7)
	kv = appendBytes(kv, 2, kvArgs)
	kv = appendBytes(kv, 4, []byte("trace-metadata"))
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
	if events[3].Type != AgentEventUsage || events[3].Usage.InputTokens != 5 || events[3].Usage.TotalTokens != 14 || events[3].Usage.CacheReadTokens != 2 || events[3].Usage.CacheWriteTokens != 3 {
		t.Fatalf("usage = %#v", events[3])
	}
	if events[5].Type != AgentEventFinish {
		t.Fatalf("turn did not produce logical finish: %#v", events[5])
	}
	if events[6].Type != AgentEventExecMCP || events[6].ExecMCP.Name != "lookup" || events[6].ExecRequestID != 9 || events[6].ExecID != "exec-1" {
		t.Fatalf("exec MCP = %#v", events[6])
	}
	if events[7].Type != AgentEventKVGet || string(events[7].KV.BlobID) != "blob" || string(events[7].KV.Metadata) != "trace-metadata" {
		t.Fatalf("KV = %#v", events[7])
	}
}

func TestAgentGrokTurnEndedDirectUsageIsParsed(t *testing.T) {
	turnEnded := appendVarint(nil, 1, 10)
	turnEnded = appendVarint(turnEnded, 2, 4)
	turnEnded = appendVarint(turnEnded, 3, 2)
	turnEnded = appendVarint(turnEnded, 4, 3)
	turnEnded = appendVarint(turnEnded, 5, 1)
	interaction := appendBytes(nil, 14, turnEnded)

	events := (&AgentStream{}).parseInteractionUpdate(interaction)
	if len(events) != 3 {
		t.Fatalf("event count = %d: %#v", len(events), events)
	}
	usage := events[0].Usage
	if events[0].Type != AgentEventUsage || usage == nil || usage.InputTokens != 5 || usage.OutputTokens != 4 || usage.CacheReadTokens != 2 || usage.CacheWriteTokens != 3 || usage.TotalTokens != 14 {
		t.Fatalf("usage event = %#v", events[0])
	}
	if events[1].Type != AgentEventTurnEnded || events[1].Usage == nil || events[2].Type != AgentEventFinish || events[2].Usage == nil {
		t.Fatalf("terminal events = %#v", events[1:])
	}
}

func TestAgentTurnEndedWithoutUsageDoesNotEmitZeroUsage(t *testing.T) {
	turnEnded := appendVarint(nil, 2, 1)
	interaction := appendBytes(nil, 14, turnEnded)

	events := (&AgentStream{}).parseInteractionUpdate(interaction)
	if len(events) != 2 {
		t.Fatalf("event count = %d: %#v", len(events), events)
	}
	if events[0].Type != AgentEventTurnEnded || events[0].Usage != nil {
		t.Fatalf("turn ended event = %#v", events[0])
	}
	if events[1].Type != AgentEventFinish || events[1].Usage != nil {
		t.Fatalf("finish event = %#v", events[1])
	}
}

func TestParseAgentTurnUsageNormalizesCachedInput(t *testing.T) {
	tests := []struct {
		name       string
		rawInput   uint64
		output     uint64
		cacheRead  uint64
		cacheWrite uint64
		wantInput  int
		wantTotal  int
	}{
		{name: "cache write", rawInput: 24054, output: 17, cacheWrite: 24052, wantInput: 2, wantTotal: 24071},
		{name: "cache read", rawInput: 207612, output: 1371, cacheRead: 163456, wantInput: 44156, wantTotal: 208983},
		{name: "cache fields exceed raw input", rawInput: 2, output: 5, cacheRead: 3, cacheWrite: 4, wantInput: 0, wantTotal: 12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := appendVarint(nil, 1, tt.rawInput)
			payload = appendVarint(payload, 2, tt.output)
			payload = appendVarint(payload, 3, tt.cacheRead)
			payload = appendVarint(payload, 4, tt.cacheWrite)

			usage := parseAgentTurnUsage(payload)
			if usage.InputTokens != tt.wantInput || usage.OutputTokens != int(tt.output) || usage.CacheReadTokens != int(tt.cacheRead) || usage.CacheWriteTokens != int(tt.cacheWrite) || usage.TotalTokens != tt.wantTotal {
				t.Fatalf("usage = %#v", usage)
			}
		})
	}
}

func TestAgentStreamKVAndMCPResponsesPreserveRequestIDs(t *testing.T) {
	stream := &AgentStream{ctx: context.Background(), send: make(chan []byte, 5)}
	if err := stream.SendKVGetResult(7, []byte("data"), []byte("get-metadata")); err != nil {
		t.Fatal(err)
	}
	if err := stream.SendKVSetResult(8, []byte("set-metadata")); err != nil {
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
	if firstProtoVarint(getKV, 1) != 7 || string(firstProtoBytes(firstProtoBytes(getKV, 2), 1)) != "data" || string(firstProtoBytes(getKV, 4)) != "get-metadata" {
		t.Fatalf("KV get response = %x", getMessage)
	}
	setMessage := <-stream.send
	setKV := firstBytesField(t, setMessage, 3)
	if firstProtoVarint(setKV, 1) != 8 || firstProtoBytes(setKV, 3) == nil || string(firstProtoBytes(setKV, 4)) != "set-metadata" {
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

func TestAgentToolCallFieldOneDecodesShell(t *testing.T) {
	shellArgs := appendString(nil, 1, "go test ./...")
	shellArgs = appendString(shellArgs, 2, "D:/repo")
	shellArgs = appendString(shellArgs, 4, "call-shell")
	shellToolCall := appendBytes(nil, 1, shellArgs)
	toolCall := appendBytes(nil, 1, shellToolCall)
	completed := appendString(nil, 1, "call-shell")
	completed = appendBytes(completed, 2, toolCall)
	interaction := appendBytes(nil, 3, completed)
	serverMessage := appendBytes(nil, 1, interaction)
	frame, _ := EncodeConnectFrame(serverMessage, false)
	frame = append(frame, encodeAgentConnectEndStream()...)
	stream := &AgentStream{response: &http.Response{Body: io.NopCloser(bytes.NewReader(frame))}, decoder: NewConnectDecoder(4096, 8192), tools: make(map[string]*agentToolAccumulator), maxToolBytes: 4096}

	event, err := stream.Next()
	if err != nil {
		t.Fatal(err)
	}
	if event.Type != AgentEventToolCompleted || event.Tool == nil || event.Tool.ID != "call-shell" || event.Tool.Name != "shell" {
		t.Fatalf("unexpected shell ToolCall field 1 event: %#v", event)
	}
	if event.Tool.Arguments["command"] != "go test ./..." || event.Tool.Arguments["working_directory"] != "D:/repo" {
		t.Fatalf("unexpected shell ToolCall field 1 args: %#v", event.Tool.Arguments)
	}
}

func TestAgentStreamShellAndRequestContextResponsesPreserveProtocolFields(t *testing.T) {
	stream := &AgentStream{ctx: context.Background(), send: make(chan []byte, 8)}
	action := &Action{ID: "call-shell", Name: "shell", Arguments: map[string]any{"command": "pwd", "working_directory": "D:/repo"}}
	if err := stream.SendShellResult(21, "exec-shell", action, "D:/repo\n", false, false); err != nil {
		t.Fatal(err)
	}
	if err := stream.SendShellResult(22, "exec-stream", action, "failed", true, true); err != nil {
		t.Fatal(err)
	}
	tools := []ToolDefinition{{Name: "lookup", Description: "Search", InputSchema: json.RawMessage(`{"type":"object"}`)}}
	if err := stream.SendRequestContextResult(23, "exec-context", tools, "sub2api"); err != nil {
		t.Fatal(err)
	}

	shellMessage := <-stream.send
	shellExec := firstBytesField(t, shellMessage, 2)
	shellResult := firstBytesField(t, shellExec, 2)
	shellSuccess := firstBytesField(t, shellResult, 1)
	if firstProtoVarint(shellExec, 1) != 21 || firstProtoString(shellExec, 15) != "exec-shell" || firstProtoString(shellSuccess, 1) != "pwd" || firstProtoString(shellSuccess, 2) != "D:/repo" || firstProtoString(shellSuccess, 5) != "D:/repo\n" {
		t.Fatalf("shell result = %x", shellMessage)
	}

	streamStart := firstBytesField(t, firstBytesField(t, <-stream.send, 2), 14)
	if firstProtoBytes(streamStart, 4) == nil {
		t.Fatalf("shell stream start missing: %x", streamStart)
	}
	streamStderr := firstBytesField(t, firstBytesField(t, <-stream.send, 2), 14)
	if firstProtoString(firstBytesField(t, streamStderr, 2), 1) != "failed" {
		t.Fatalf("shell stream stderr = %x", streamStderr)
	}
	streamExit := firstBytesField(t, firstBytesField(t, <-stream.send, 2), 14)
	if firstProtoVarint(firstBytesField(t, streamExit, 3), 1) != 1 {
		t.Fatalf("shell stream exit = %x", streamExit)
	}
	streamResultMessage := <-stream.send
	streamExec := firstBytesField(t, streamResultMessage, 2)
	streamFailure := firstBytesField(t, firstBytesField(t, streamExec, 2), 2)
	if firstProtoVarint(streamExec, 1) != 22 || firstProtoString(streamExec, 15) != "exec-stream" || firstProtoVarint(streamFailure, 3) != 1 || firstProtoString(streamFailure, 6) != "failed" {
		t.Fatalf("shell stream final result = %x", streamResultMessage)
	}
	streamClose := <-stream.send
	control := firstBytesField(t, streamClose, 5)
	if firstProtoVarint(firstBytesField(t, control, 1), 1) != 22 {
		t.Fatalf("shell stream close = %x", streamClose)
	}

	contextMessage := <-stream.send
	contextExec := firstBytesField(t, contextMessage, 2)
	contextResult := firstBytesField(t, contextExec, 10)
	contextSuccess := firstBytesField(t, contextResult, 1)
	requestContext := firstBytesField(t, contextSuccess, 1)
	contextTool := firstBytesField(t, requestContext, 7)
	if firstProtoVarint(contextExec, 1) != 23 || firstProtoString(contextExec, 15) != "exec-context" || firstProtoString(contextTool, 1) != "sub2api-lookup" || firstProtoString(contextTool, 4) != "sub2api" || firstProtoString(contextTool, 5) != "lookup" {
		t.Fatalf("request context result = %x", contextMessage)
	}
}

func TestAgentExecRecognizesShellAndRequestContext(t *testing.T) {
	shell := appendString(nil, 1, "pwd")
	shell = appendString(shell, 2, "D:/repo")
	shell = appendVarint(shell, 3, 5000)
	shell = appendString(shell, 4, "call-shell")
	for _, field := range []protowire.Number{2, 14} {
		exec := appendVarint(nil, 1, 3)
		exec = appendString(exec, 15, "exec-local")
		exec = appendBytes(exec, field, shell)
		events := parseAgentExecServerMessage(exec)
		if len(events) != 1 || events[0].Type != AgentEventExecShell || events[0].ExecField != int(field) || events[0].ExecRequestID != 3 || events[0].ExecID != "exec-local" {
			t.Fatalf("unexpected shell exec handling for field %d: %#v", field, events)
		}
		if events[0].ExecShell == nil || events[0].ExecShell.ID != "call-shell" || events[0].ExecShell.Arguments["command"] != "pwd" || events[0].ExecShell.Arguments["working_directory"] != "D:/repo" {
			t.Fatalf("unexpected shell args for field %d: %#v", field, events[0].ExecShell)
		}
	}

	requestContext := appendVarint(nil, 1, 4)
	requestContext = appendString(requestContext, 15, "exec-context")
	requestContext = appendBytes(requestContext, 10, nil)
	events := parseAgentExecServerMessage(requestContext)
	if len(events) != 1 || events[0].Type != AgentEventExecRequestContext || events[0].ExecRequestID != 4 || events[0].ExecID != "exec-context" {
		t.Fatalf("unexpected request context handling: %#v", events)
	}

	unsupported := appendVarint(nil, 1, 5)
	unsupported = appendBytes(unsupported, 3, nil)
	events = parseAgentExecServerMessage(unsupported)
	if len(events) != 1 || events[0].Type != AgentEventUnsupportedExec || events[0].Unsupported.Field != 3 {
		t.Fatalf("unexpected unsupported exec handling: %#v", events)
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
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("response writer does not support flushing")
			return
		}
		flusher.Flush()
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

func TestAgentClientHTTP2KVMetadataRoundTrip(t *testing.T) {
	requestDone := make(chan struct{})
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := readConnectFrame(r.Body); err != nil {
			t.Errorf("initial frame: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/connect+proto")
		getArgs := appendBytes(nil, 1, []byte("blob-id"))
		kvRequest := appendVarint(nil, 1, 9)
		kvRequest = appendBytes(kvRequest, 2, getArgs)
		kvRequest = appendBytes(kvRequest, 4, []byte("trace-metadata"))
		message := appendBytes(nil, 4, kvRequest)
		response, _ := EncodeConnectFrame(message, false)
		_, _ = w.Write(response)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("response writer does not support flushing")
			return
		}
		flusher.Flush()

		result, err := readConnectFrame(r.Body)
		if err != nil {
			t.Errorf("KV result frame: %v", err)
			return
		}
		kvResult := firstProtoBytes(result.Payload, 3)
		if firstProtoVarint(kvResult, 1) != 9 || string(firstProtoBytes(firstProtoBytes(kvResult, 2), 1)) != "blob-data" || string(firstProtoBytes(kvResult, 4)) != "trace-metadata" {
			t.Errorf("KV result = %x", result.Payload)
		}
		if _, err := readConnectFrame(r.Body); err != nil {
			t.Errorf("client end stream: %v", err)
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
	if err != nil || event.Type != AgentEventKVGet || event.KV == nil || string(event.KV.Metadata) != "trace-metadata" {
		t.Fatalf("KV event = %#v %v", event, err)
	}
	if err := stream.SendKVGetResult(event.KV.ID, []byte("blob-data"), event.KV.Metadata); err != nil {
		t.Fatal(err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}
	if finish, err := stream.Next(); err != nil || finish.Type != AgentEventFinish {
		t.Fatalf("finish = %#v %v", finish, err)
	}
	select {
	case <-requestDone:
	case <-time.After(time.Second):
		t.Fatal("server did not observe KV metadata response")
	}
}

func TestAgentStreamCancellationCleansRequest(t *testing.T) {
	var cancelled atomic.Bool
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = readConnectFrame(r.Body)
		w.Header().Set("Content-Type", "application/connect+proto")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("response writer does not support flushing")
			return
		}
		flusher.Flush()
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
