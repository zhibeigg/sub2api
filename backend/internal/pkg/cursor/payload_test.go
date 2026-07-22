package cursor

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseAnthropicAndBuildPayload(t *testing.T) {
	raw := []byte(`{
		"system":[{"type":"text","text":"Be concise."}],
		"messages":[
			{"role":"user","content":"inspect the project"},
			{"role":"assistant","content":[{"type":"tool_use","id":"call_1","name":"read_file","input":{"path":"main.go"}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"package main"}]}
		],
		"tools":[{"name":"read_file","description":"Read a file","input_schema":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}}]
	}`)
	dialogue, err := ParseAnthropic(raw)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := BuildPayload(dialogue, BuildOptions{Model: "claude-test"})
	if err != nil {
		t.Fatal(err)
	}
	if payload.Model != "claude-test" || payload.Trigger != "submit-message" || payload.ID == "" {
		t.Fatalf("unexpected payload metadata: %+v", payload)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	body := string(encoded)
	for _, expected := range []string{"System instructions", "read_file", "main.go", "Tool output for call_1"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("payload missing %q: %s", expected, body)
		}
	}
	if strings.Contains(body, `"path":"example`) || strings.Contains(body, `"parameters":{"input":"value"}`) {
		t.Fatalf("payload contains fabricated fallback parameters: %s", body)
	}
}

func TestBuildPayloadPreservesImageOnlyMessagesWithoutEmbeddingData(t *testing.T) {
	dialogue := &Dialogue{Messages: []DialogueMessage{{
		Role:   "user",
		Images: []InlineImage{{MIMEType: "image/png", Data: []byte("SECRET-IMAGE-BYTES")}},
	}}}

	payload, err := BuildPayload(dialogue, BuildOptions{Model: "claude-test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.Messages) != 1 || payload.Messages[0].Parts[0].Text != "[Attached image: image/png]" {
		t.Fatalf("image-only message was not preserved: %+v", payload.Messages)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "SECRET-IMAGE-BYTES") {
		t.Fatalf("payload embedded raw image data: %s", encoded)
	}
}

func TestOpenAIParsersRejectMultimodalContent(t *testing.T) {
	tests := []struct {
		name     string
		protocol Protocol
		body     string
	}{
		{"openai audio", ProtocolOpenAIChat, `{"messages":[{"role":"user","content":[{"type":"input_audio","input_audio":{"data":"x"}}]}]}`},
		{"responses file", ProtocolResponses, `{"input":[{"type":"input_file","file_id":"file_1"}]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseRequest(test.protocol, []byte(test.body))
			if err == nil || !IsKind(err, ErrorBadRequest) {
				t.Fatalf("expected bad request, got %v", err)
			}
		})
	}
}

func TestParseOpenAIChatAndResponses(t *testing.T) {
	chat, err := ParseOpenAIChat([]byte(`{
		"messages":[
			{"role":"developer","content":"Follow policy"},
			{"role":"user","content":[{"type":"text","text":"hello"}]},
			{"role":"assistant","content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"lookup","arguments":"{\"q\":\"go\"}"}}]},
			{"role":"tool","tool_call_id":"c1","content":"result"}
		],
		"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if chat.System != "Follow policy" || len(chat.Messages) != 3 || chat.Messages[1].ToolCalls[0].Arguments["q"] != "go" {
		t.Fatalf("unexpected chat conversion: %+v", chat)
	}

	responses, err := ParseResponses([]byte(`{
		"instructions":"Be useful",
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"find it"}]},
			{"type":"function_call","call_id":"c2","name":"lookup","arguments":"{\"q\":\"cursor\"}"},
			{"type":"function_call_output","call_id":"c2","output":"done"}
		],
		"tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if responses.System != "Be useful" || len(responses.Messages) != 3 || responses.Messages[1].ToolCalls[0].Name != "lookup" {
		t.Fatalf("unexpected responses conversion: %+v", responses)
	}
}

func TestEstimateAndTrimHistory(t *testing.T) {
	if EstimateTokens("hello world") <= 0 || EstimateTokens("你好世界") <= 0 {
		t.Fatal("token estimates must be positive")
	}
	messages := []Message{
		newMessage("user", "protected instructions", 0),
		newMessage("user", strings.Repeat("old ", 100), 1),
		newMessage("assistant", "old answer", 2),
		newMessage("user", "latest question", 3),
	}
	trimmed := TrimHistory(messages, 1, 2, 100, 0)
	if len(trimmed) < 2 || trimmed[0].Parts[0].Text != "protected instructions" || trimmed[len(trimmed)-1].Parts[0].Text != "latest question" {
		t.Fatalf("unexpected trimmed history: %+v", trimmed)
	}
}

func TestTrimHistoryDoesNotChargeProtectedPreambleToHistoryBudget(t *testing.T) {
	messages := []Message{
		newMessage("user", strings.Repeat("fixed system and tool schema ", 10000), 0),
		newMessage("user", "remember ORBIT-4836", 1),
		newMessage("assistant", "remembered", 2),
		newMessage("user", "what was the code?", 3),
	}
	historyBudget := 0
	for _, message := range messages[1:] {
		historyBudget += EstimateMessageTokens(message)
	}
	if EstimateMessageTokens(messages[0]) <= historyBudget {
		t.Fatal("test preamble must exceed the history budget")
	}

	trimmed := TrimHistory(messages, 1, 100, historyBudget, 0)

	if len(trimmed) != len(messages) {
		t.Fatalf("protected preamble evicted conversation history: got %d messages, want %d", len(trimmed), len(messages))
	}
}
