package cursor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
)

func TestBuildIDEHeadersGolden(t *testing.T) {
	fixed := time.Unix(1_700_000_000, 0).UTC()
	uuids := sequenceUUID("11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222")
	headers, err := BuildIDEHeaders(IDECredential{AccessToken: "token", MachineID: "machine"}, IDEClientConfig{
		ClientVersion: "9.9.9", ClientOS: "win32", ClientArch: "x64", ClientOSVersion: "11",
		Timezone: "America/New_York", GhostMode: true, Now: func() time.Time { return fixed }, UUID: uuids,
	})
	if err != nil {
		t.Fatal(err)
	}
	golden := map[string]string{
		"Authorization":              "Bearer token",
		"Connect-Protocol-Version":   "1",
		"User-Agent":                 "connect-es/1.6.1",
		"X-Amzn-Trace-Id":            "Root=11111111-1111-4111-8111-111111111111",
		"X-Client-Key":               "3c469e9d6c5875d37a43f353d4f88e61fcf812c66eee3457465a40b0da4153e0",
		"X-Cursor-Checksum":          "paaotEjtmachine",
		"X-Cursor-Client-Version":    "9.9.9",
		"X-Cursor-Client-Type":       "ide",
		"X-Cursor-Client-Os":         "win32",
		"X-Cursor-Client-Arch":       "x64",
		"X-Cursor-Client-Os-Version": "11",
		"X-Cursor-Config-Version":    "22222222-2222-4222-8222-222222222222",
		"X-Cursor-Timezone":          "America/New_York",
		"X-Ghost-Mode":               "true",
		"X-Session-Id":               "6acb5226-51a3-5bbd-a6f2-483361f60efe",
		"X-Request-Id":               "11111111-1111-4111-8111-111111111111",
	}
	for name, want := range golden {
		if got := headers.Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestIDERequestAskAndAgentGolden(t *testing.T) {
	dialogue := &Dialogue{
		System: "Stay concise.",
		Messages: []DialogueMessage{
			{Role: "user", Text: "hello"},
			{Role: "assistant", Text: "checking", ToolCalls: []Action{{ID: "call-1", Name: "lookup", Arguments: map[string]any{"q": "go"}}}},
			{Role: "tool", ToolCallID: "call-1", Text: "result"},
		},
		Tools: []ToolDefinition{{Name: "lookup", Description: "Search", InputSchema: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`)}},
	}
	metadata := []byte("fixed-metadata")
	askPayload, err := encodeIDEChatRequest(dialogue, IDEChatOptions{Model: "model", ConversationID: "conversation", Mode: IDEModeAsk}, sequenceUUID("m1", "m2", "m3"), metadata)
	if err != nil {
		t.Fatal(err)
	}
	agentPayload, err := encodeIDEChatRequest(dialogue, IDEChatOptions{Model: "model", ConversationID: "conversation", Mode: IDEModeAgent, Thinking: true}, sequenceUUID("m1", "m2", "m3"), metadata)
	if err != nil {
		t.Fatal(err)
	}
	assertGoldenSHA256(t, "ask", askPayload, "e849f0b84e41696dbb75069e323476a7da51e687a1b97f6b27266efd597dd71f")
	assertGoldenSHA256(t, "agent", agentPayload, "447e20ac1dd1684b38a8ec4888dbfbc797fe9a4409b4b8bc11ccd5d27e230587")

	askRequest := firstBytesField(t, askPayload, 1)
	if fields := allFields(askRequest); hasField(fields, 29) || hasField(fields, 34) {
		t.Fatalf("Ask request unexpectedly contains tools: %#v", fields)
	}
	if got := firstStringField(t, firstBytesField(t, askRequest, 3), 1); got != dialogue.System {
		t.Fatalf("instruction = %q", got)
	}
	agentRequest := firstBytesField(t, agentPayload, 1)
	fields := allFields(agentRequest)
	if !hasVarint(fields, 27, 1) || !hasVarint(fields, 29, 19) || !hasField(fields, 34) || !hasVarint(fields, 46, 2) || !hasVarint(fields, 49, 1) {
		t.Fatalf("Agent fields missing: %#v", fields)
	}
	mcp := firstBytesField(t, agentRequest, 34)
	if firstStringField(t, mcp, 1) != "lookup" || firstStringField(t, mcp, 4) != "custom" {
		t.Fatalf("unexpected MCP custom tool: %x", mcp)
	}
	messages := bytesFields(agentRequest, 1)
	if len(messages) != 3 || !strings.Contains(firstStringField(t, messages[1], 1), "[tool_call id=call-1 name=lookup]") || !strings.Contains(firstStringField(t, messages[2], 1), "[tool_result id=call-1]") {
		t.Fatalf("history semantics were lost")
	}
}

func TestConnectDecoderIncrementalGzipErrorsAndLimits(t *testing.T) {
	frame, err := EncodeConnectFrame([]byte("hello"), true)
	if err != nil {
		t.Fatal(err)
	}
	decoder := NewConnectDecoder(64, 128)
	var frames []ConnectFrame
	for _, value := range frame {
		parsed, parseErr := decoder.Feed([]byte{value})
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		frames = append(frames, parsed...)
	}
	if len(frames) != 1 || string(frames[0].Payload) != "hello" || !frames[0].Compressed() {
		t.Fatalf("unexpected frames: %#v", frames)
	}
	if err := decoder.Finish(); err != nil {
		t.Fatal(err)
	}

	oversized := []byte{0, 0, 0, 1, 0}
	if _, err := NewConnectDecoder(16, 32).Feed(oversized); err == nil || !IsKind(err, ErrorProtocol) {
		t.Fatalf("expected frame limit error, got %v", err)
	}
	truncated := NewConnectDecoder(64, 128)
	_, _ = truncated.Feed(frame[:4])
	if err := truncated.Finish(); err == nil || !IsKind(err, ErrorProtocol) {
		t.Fatalf("expected truncated frame error, got %v", err)
	}
	badGzip := []byte{1, 0, 0, 0, 3, 1, 2, 3}
	if _, err := NewConnectDecoder(64, 128).Feed(badGzip); err == nil || !IsKind(err, ErrorProtocol) {
		t.Fatalf("expected gzip error, got %v", err)
	}
}

func TestIDEEventStreamCanonicalEvents(t *testing.T) {
	var content []byte
	content = appendString(content, 1, "answer")
	var reasoning []byte
	reasoning = appendString(reasoning, 1, "thought")
	content = appendBytes(content, 25, reasoning)
	var tool []byte
	tool = appendString(tool, 3, "call-1")
	tool = appendString(tool, 9, "lookup")
	tool = appendString(tool, 10, `{"q":"go"}`)
	var usage []byte
	usage = appendVarint(usage, 1, 10)
	usage = appendVarint(usage, 2, 4)
	usage = appendVarint(usage, 5, 2)
	var responsePayload []byte
	responsePayload = appendBytes(responsePayload, 2, content)
	responsePayload = appendBytes(responsePayload, 1, tool)
	responsePayload = appendBytes(responsePayload, 12, usage)
	dataFrame, _ := EncodeConnectFrame(responsePayload, true)
	errorFramePayload := []byte(`{"error":{"code":"resource_exhausted","message":"quota"}}`)
	errorFrame, _ := EncodeConnectFrame(errorFramePayload, false)
	errorFrame[0] = 2
	body := append(dataFrame, errorFrame...)
	stream := &IDEEventStream{response: &http.Response{Body: io.NopCloser(bytes.NewReader(body))}, decoder: NewConnectDecoder(1024, 2048)}
	var events []IDEEvent
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
	if len(events) != 5 || events[0].Type != IDEEventThinking || events[0].Thinking != "thought" || events[1].Type != IDEEventText || events[1].Text != "answer" {
		t.Fatalf("unexpected content events: %#v", events)
	}
	if events[2].Type != IDEEventToolCall || events[2].ToolCall.Name != "lookup" || events[2].ToolCall.Arguments["q"] != "go" {
		t.Fatalf("unexpected tool event: %#v", events[2])
	}
	if events[3].Type != IDEEventUsage || events[3].Usage.TotalTokens != 14 || events[3].Usage.ReasoningTokens != 2 {
		t.Fatalf("unexpected usage: %#v", events[3])
	}
	if events[4].Type != IDEEventError || events[4].Error.Code != "resource_exhausted" {
		t.Fatalf("unexpected error event: %#v", events[4])
	}
}

func TestIDEEventStreamAggregatesNativeMCPToolFragments(t *testing.T) {
	var firstTool []byte
	firstTool = appendString(firstTool, 3, "call-mcp")
	firstTool = appendString(firstTool, 9, "lookup")
	firstTool = appendString(firstTool, 10, `{"q":"`)
	firstTool = appendVarint(firstTool, 11, 0)
	var secondMCP []byte
	secondMCP = appendString(secondMCP, 1, "lookup")
	secondMCP = appendString(secondMCP, 2, `go"}`)
	secondMCP = appendString(secondMCP, 3, "call-mcp")
	var secondTool []byte
	secondTool = appendBytes(secondTool, 27, secondMCP)
	secondTool = appendVarint(secondTool, 11, 1)
	var firstOuter, secondOuter []byte
	firstOuter = appendBytes(firstOuter, 1, firstTool)
	secondOuter = appendBytes(secondOuter, 1, secondTool)
	firstFrame, _ := EncodeConnectFrame(firstOuter, false)
	secondFrame, _ := EncodeConnectFrame(secondOuter, false)
	body := append(firstFrame, secondFrame...)
	body = append(body, 2, 0, 0, 0, 2, '{', '}')
	stream := &IDEEventStream{response: &http.Response{Body: io.NopCloser(bytes.NewReader(body))}, decoder: NewConnectDecoder(1024, 2048)}
	event, err := stream.Next()
	if err != nil || event.Type != IDEEventToolCall || event.ToolCall.ID != "call-mcp" || event.ToolCall.Name != "lookup" || event.ToolCall.Arguments["q"] != "go" {
		t.Fatalf("unexpected aggregated tool call: %#v %v", event, err)
	}
	finish, err := stream.Next()
	if err != nil || finish.Type != IDEEventFinish || finish.FinishReason != "tool_calls" {
		t.Fatalf("unexpected tool finish: %#v %v", finish, err)
	}
}

func TestIDEClientHTTP2RoutesHeadersAndStreaming(t *testing.T) {
	var calls int
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.ProtoMajor != 2 {
			t.Errorf("protocol = %s", r.Proto)
		}
		if r.Header.Get("Authorization") != "Bearer token" || r.Header.Get("Connect-Protocol-Version") != "1" {
			t.Errorf("missing IDE headers")
		}
		switch r.URL.Path {
		case IDEChatPath:
			if r.Header.Get("Content-Type") != "application/connect+proto" {
				t.Errorf("chat content type = %q", r.Header.Get("Content-Type"))
			}
			body, _ := io.ReadAll(r.Body)
			frames, err := NewConnectDecoder(1<<20, 2<<20).Feed(body)
			if err != nil || len(frames) != 1 {
				t.Errorf("invalid request frame: %v %#v", err, frames)
			}
			var inner []byte
			inner = appendString(inner, 1, "ok")
			var outer []byte
			outer = appendBytes(outer, 2, inner)
			frame, _ := EncodeConnectFrame(outer, false)
			frame = append(frame, 2, 0, 0, 0, 2, '{', '}')
			_, _ = w.Write(frame)
		case IDEModelsPath:
			if r.Header.Get("Content-Type") != "application/connect+proto" {
				t.Errorf("models content type = %q", r.Header.Get("Content-Type"))
			}
			body, _ := io.ReadAll(r.Body)
			decoder := NewConnectDecoder(1024, 2048)
			frames, decodeErr := decoder.Feed(body)
			if decodeErr != nil || decoder.Finish() != nil || len(frames) != 1 || len(frames[0].Payload) != 0 {
				t.Errorf("invalid models request frame: %v %#v", decodeErr, frames)
			}
			var model []byte
			model = appendString(model, 1, "claude-4-sonnet")
			var payload []byte
			payload = appendString(payload, 1, "composer-1")
			payload = appendBytes(payload, 2, model)
			responseFrame, _ := EncodeConnectFrame(payload, false)
			responseFrame = append(responseFrame, 2, 0, 0, 0, 2, '{', '}')
			_, _ = w.Write(responseFrame)
		default:
			http.NotFound(w, r)
		}
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	client, err := NewIDEClient(server.Client(), IDEClientConfig{BaseURL: server.URL, Now: func() time.Time { return time.Unix(1_700_000_000, 0) }, UUID: sequenceUUID("r1", "c1", "m1")})
	if err != nil {
		t.Fatal(err)
	}
	resp, stream, err := client.StreamUnifiedChatWithTools(context.Background(), IDECredential{AccessToken: "token"}, &Dialogue{Messages: []DialogueMessage{{Role: "user", Text: "hi"}}}, IDEChatOptions{Model: "model"})
	if err != nil || resp.StatusCode != http.StatusOK || stream.Response() != resp {
		t.Fatalf("chat failed: %#v %v", resp, err)
	}
	event, err := stream.Next()
	if err != nil || event.Type != IDEEventText || event.Text != "ok" {
		t.Fatalf("unexpected stream event: %#v %v", event, err)
	}
	finish, err := stream.Next()
	if err != nil || finish.Type != IDEEventFinish {
		t.Fatalf("unexpected finish: %#v %v", finish, err)
	}
	modelsResp, err := client.AvailableModels(context.Background(), IDECredential{AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	modelsBody, _ := io.ReadAll(modelsResp.Body)
	_ = modelsResp.Body.Close()
	models, decodeErr := DecodeIDEAvailableModels(modelsBody, 1024, 2048)
	if decodeErr != nil || !reflect.DeepEqual(models, []string{"composer-1", "claude-4-sonnet"}) || calls != 2 {
		t.Fatalf("unexpected models response/calls: %#v %v %d", models, decodeErr, calls)
	}
}

func TestIDEClientAvailableModelsFallsBackToJSONOn415(t *testing.T) {
	calls := 0
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, _ := io.ReadAll(r.Body)
		if calls == 1 {
			if r.Header.Get("Content-Type") != "application/connect+proto" || !bytes.Equal(body, []byte{0, 0, 0, 0, 0}) {
				t.Fatalf("unexpected protobuf attempt: %q %v", r.Header.Get("Content-Type"), body)
			}
			w.WriteHeader(http.StatusUnsupportedMediaType)
			return
		}
		if r.Header.Get("Content-Type") != "application/json" || string(body) != "{}" {
			t.Fatalf("unexpected JSON fallback: %q %q", r.Header.Get("Content-Type"), body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"composer-1"}]}`))
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()

	client, err := NewIDEClient(server.Client(), IDEClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.AvailableModels(context.Background(), IDECredential{AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if calls != 2 || resp.Header.Get("Content-Type") != "application/json" || !bytes.Contains(body, []byte("composer-1")) {
		t.Fatalf("unexpected fallback response: calls=%d content-type=%q body=%q", calls, resp.Header.Get("Content-Type"), body)
	}
}

func TestIDEClientHTTPErrorLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("0123456789"))
	}))
	defer server.Close()
	client, err := NewIDEClient(server.Client(), IDEClientConfig{BaseURL: server.URL, MaxErrorBody: 4})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.AvailableModels(context.Background(), IDECredential{AccessToken: "token"})
	var cursorErr *Error
	if !errors.As(err, &cursorErr) || cursorErr.Body != "0123..." || !cursorErr.FailoverSafe {
		t.Fatalf("unexpected HTTP error: %#v", err)
	}
}

func sequenceUUID(values ...string) func() string {
	index := 0
	return func() string {
		if index >= len(values) {
			return "00000000-0000-4000-8000-000000000000"
		}
		value := values[index]
		index++
		return value
	}
}

func assertGoldenSHA256(t *testing.T, name string, data []byte, want string) {
	t.Helper()
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if got != want {
		t.Fatalf("%s protobuf golden SHA256 = %s, want %s", name, got, want)
	}
}

type wireField struct {
	number protowire.Number
	typeID protowire.Type
	bytes  []byte
	value  uint64
}

func allFields(payload []byte) []wireField {
	var fields []wireField
	for len(payload) > 0 {
		number, wireType, n := protowire.ConsumeTag(payload)
		if n < 0 {
			break
		}
		payload = payload[n:]
		field := wireField{number: number, typeID: wireType}
		switch wireType {
		case protowire.BytesType:
			value, size := protowire.ConsumeBytes(payload)
			if size < 0 {
				return fields
			}
			field.bytes = append([]byte(nil), value...)
			payload = payload[size:]
		case protowire.VarintType:
			value, size := protowire.ConsumeVarint(payload)
			if size < 0 {
				return fields
			}
			field.value = value
			payload = payload[size:]
		default:
			size := protowire.ConsumeFieldValue(number, wireType, payload)
			if size < 0 {
				return fields
			}
			payload = payload[size:]
		}
		fields = append(fields, field)
	}
	return fields
}

func firstBytesField(t *testing.T, payload []byte, number protowire.Number) []byte {
	t.Helper()
	for _, field := range allFields(payload) {
		if field.number == number && field.typeID == protowire.BytesType {
			return field.bytes
		}
	}
	t.Fatalf("field %d not found in %x", number, payload)
	return nil
}

func bytesFields(payload []byte, number protowire.Number) [][]byte {
	var values [][]byte
	for _, field := range allFields(payload) {
		if field.number == number && field.typeID == protowire.BytesType {
			values = append(values, field.bytes)
		}
	}
	return values
}

func firstStringField(t *testing.T, payload []byte, number protowire.Number) string {
	t.Helper()
	return string(firstBytesField(t, payload, number))
}

func hasField(fields []wireField, number protowire.Number) bool {
	for _, field := range fields {
		if field.number == number {
			return true
		}
	}
	return false
}

func hasVarint(fields []wireField, number protowire.Number, value uint64) bool {
	for _, field := range fields {
		if field.number == number && field.typeID == protowire.VarintType && field.value == value {
			return true
		}
	}
	return false
}
