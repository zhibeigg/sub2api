package kiro

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"testing"
)

func TestParseModelAndThinking(t *testing.T) {
	cases := []struct {
		in           string
		suffix       string
		wantModel    string
		wantThinking bool
	}{
		{"claude-3-5-sonnet-20241022", "-thinking", "claude-sonnet-4.5", false},
		{"claude-sonnet-4-5-thinking", "-thinking", "claude-sonnet-4.5", true},
		{"claude-opus-4-8", "-thinking", "claude-opus-4.8", false},
		{"gpt-4o", "-thinking", "claude-sonnet-4.5", false},
		{"claude-sonnet-4.5", "-thinking", "claude-sonnet-4.5", false},
		{"claude-haiku-4-5", "-thinking", "claude-haiku-4.5", false},
	}
	for _, c := range cases {
		model, thinking := ParseModelAndThinking(c.in, c.suffix)
		if model != c.wantModel || thinking != c.wantThinking {
			t.Errorf("ParseModelAndThinking(%q) = (%q,%v), want (%q,%v)", c.in, model, thinking, c.wantModel, c.wantThinking)
		}
	}
}

func TestClaudeToKiroBasic(t *testing.T) {
	req := &ClaudeRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 1024,
		System:    "You are helpful.",
		Messages: []ClaudeMessage{
			{Role: "user", Content: "Hello!"},
		},
	}
	payload := ClaudeToKiro(req, false)

	if got := payload.ConversationState.CurrentMessage.UserInputMessage.Content; got != "Hello!" {
		t.Errorf("current content = %q, want %q", got, "Hello!")
	}
	if got := payload.ConversationState.CurrentMessage.UserInputMessage.ModelID; got != "claude-sonnet-4.5" {
		t.Errorf("modelID = %q, want claude-sonnet-4.5", got)
	}
	if payload.ConversationState.ChatTriggerType != "MANUAL" {
		t.Errorf("chatTriggerType = %q, want MANUAL", payload.ConversationState.ChatTriggerType)
	}
	// System prompt should be turned into a priming history pair.
	if len(payload.ConversationState.History) < 2 {
		t.Fatalf("expected priming history, got %d entries", len(payload.ConversationState.History))
	}
	if payload.ConversationState.History[0].UserInputMessage == nil ||
		payload.ConversationState.History[0].UserInputMessage.Content != "You are helpful." {
		t.Errorf("first history entry should carry the system prompt")
	}
	if payload.InferenceConfig == nil || payload.InferenceConfig.MaxTokens != 1024 {
		t.Errorf("inference config maxTokens not propagated")
	}
}

func TestClaudeToKiroThinkingInjectsPrompt(t *testing.T) {
	req := &ClaudeRequest{
		Model:    "claude-sonnet-4.5",
		Messages: []ClaudeMessage{{Role: "user", Content: "Hi"}},
	}
	payload := ClaudeToKiro(req, true)
	if len(payload.ConversationState.History) == 0 {
		t.Fatal("expected priming history for thinking mode")
	}
	first := payload.ConversationState.History[0].UserInputMessage
	if first == nil || !bytes.Contains([]byte(first.Content), []byte("<thinking_mode>enabled</thinking_mode>")) {
		t.Errorf("thinking prompt not injected into system priming")
	}
}

func TestOpenAIToKiroToolResult(t *testing.T) {
	req := &OpenAIRequest{
		Model: "gpt-4o",
		Messages: []OpenAIMessage{
			{Role: "system", Content: "sys"},
			{Role: "user", Content: "call the tool"},
		},
	}
	payload := OpenAIToKiro(req, false)
	if payload.ConversationState.CurrentMessage.UserInputMessage.ModelID != "claude-sonnet-4.5" {
		t.Errorf("model mapping failed: %s", payload.ConversationState.CurrentMessage.UserInputMessage.ModelID)
	}
	if payload.ConversationState.CurrentMessage.UserInputMessage.Content != "call the tool" {
		t.Errorf("current content wrong: %q", payload.ConversationState.CurrentMessage.UserInputMessage.Content)
	}
}

func TestKiroToClaudeResponse(t *testing.T) {
	resp := KiroToClaudeResponse("hi there", "", false, nil, 10, 5, "claude-sonnet-4.5")
	if resp.StopReason != "end_turn" {
		t.Errorf("stop reason = %q, want end_turn", resp.StopReason)
	}
	if len(resp.Content) != 1 || resp.Content[0].Type != "text" || resp.Content[0].Text != "hi there" {
		t.Errorf("unexpected content blocks: %+v", resp.Content)
	}
	if resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 5 {
		t.Errorf("usage wrong: %+v", resp.Usage)
	}

	withTool := KiroToClaudeResponse("", "", false, []KiroToolUse{{ToolUseID: "t1", Name: "search", Input: map[string]interface{}{"q": "x"}}}, 1, 1, "m")
	if withTool.StopReason != "tool_use" {
		t.Errorf("stop reason with tool = %q, want tool_use", withTool.StopReason)
	}
}

func TestSanitizeToolName(t *testing.T) {
	cases := map[string]string{
		"get_weather":       "getWeather",
		"mcp__server__tool": "mcpServerTool",
		"already":           "already",
		"a-b-c":             "aBC",
	}
	for in, want := range cases {
		if got := sanitizeToolName(in); got != want {
			t.Errorf("sanitizeToolName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeChunk(t *testing.T) {
	var prev string
	if got := normalizeChunk("Hello", &prev); got != "Hello" {
		t.Errorf("first chunk = %q, want Hello", got)
	}
	if got := normalizeChunk("Hello world", &prev); got != " world" {
		t.Errorf("cumulative delta = %q, want ' world'", got)
	}
	if got := normalizeChunk("Hello world", &prev); got != "" {
		t.Errorf("duplicate should yield empty, got %q", got)
	}
}

// buildEventStreamFrame builds one AWS event-stream frame with a :event-type
// header and a JSON payload, matching the wire format parseEventStream expects.
func buildEventStreamFrame(eventType string, payload map[string]interface{}) []byte {
	payloadBytes, _ := json.Marshal(payload)

	// Header: nameLen(1) + name + valueType(1=7 String) + valueLen(2) + value
	name := ":event-type"
	var headers bytes.Buffer
	headers.WriteByte(byte(len(name)))
	headers.WriteString(name)
	headers.WriteByte(7) // string
	valLen := make([]byte, 2)
	binary.BigEndian.PutUint16(valLen, uint16(len(eventType)))
	headers.Write(valLen)
	headers.WriteString(eventType)

	headerBytes := headers.Bytes()
	totalLength := 12 + len(headerBytes) + len(payloadBytes) + 4

	var frame bytes.Buffer
	prelude := make([]byte, 12)
	binary.BigEndian.PutUint32(prelude[0:4], uint32(totalLength))
	binary.BigEndian.PutUint32(prelude[4:8], uint32(len(headerBytes)))
	// preludeCRC (bytes 8-12) left zero; parser ignores it.
	frame.Write(prelude)
	frame.Write(headerBytes)
	frame.Write(payloadBytes)
	frame.Write([]byte{0, 0, 0, 0}) // messageCRC, ignored
	return frame.Bytes()
}

func TestParseEventStream(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(buildEventStreamFrame("assistantResponseEvent", map[string]interface{}{"content": "Hello"}))
	buf.Write(buildEventStreamFrame("assistantResponseEvent", map[string]interface{}{"content": "Hello, world"}))
	buf.Write(buildEventStreamFrame("meteringEvent", map[string]interface{}{"usage": 1.5}))

	var text string
	var credits float64
	var completed bool
	cb := &StreamCallback{
		OnText:     func(t string, isThinking bool) { text += t },
		OnCredits:  func(c float64) { credits = c },
		OnComplete: func(in, out int) { completed = true },
	}
	if err := parseEventStream(&buf, cb); err != nil {
		t.Fatalf("parseEventStream error: %v", err)
	}
	if text != "Hello, world" {
		t.Errorf("assembled text = %q, want 'Hello, world'", text)
	}
	if credits != 1.5 {
		t.Errorf("credits = %v, want 1.5", credits)
	}
	if !completed {
		t.Error("OnComplete not called")
	}
}

func TestParseEventStreamToolUse(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(buildEventStreamFrame("toolUseEvent", map[string]interface{}{
		"toolUseId": "tool_1", "name": "search", "input": `{"q":"golang"}`, "stop": true,
	}))

	var got KiroToolUse
	cb := &StreamCallback{
		OnToolUse:  func(tu KiroToolUse) { got = tu },
		OnComplete: func(in, out int) {},
	}
	if err := parseEventStream(&buf, cb); err != nil {
		t.Fatalf("parseEventStream error: %v", err)
	}
	if got.ToolUseID != "tool_1" || got.Name != "search" {
		t.Errorf("tool use = %+v", got)
	}
	if got.Input["q"] != "golang" {
		t.Errorf("tool input not parsed: %+v", got.Input)
	}
}

func TestRegionFromProfileArn(t *testing.T) {
	arn := "arn:aws:codewhisperer:eu-west-1:123456789012:profile/ABCDEF"
	if got := regionFromProfileArn(arn); got != "eu-west-1" {
		t.Errorf("region = %q, want eu-west-1", got)
	}
	if got := regionalizeURLForProfile("https://q.us-east-1.amazonaws.com/x", arn); got != "https://q.eu-west-1.amazonaws.com/x" {
		t.Errorf("regionalized url = %q", got)
	}
	// us-east-1 stays unchanged.
	usArn := "arn:aws:codewhisperer:us-east-1:1:profile/x"
	if got := regionalizeURLForProfile("https://q.us-east-1.amazonaws.com/x", usArn); got != "https://q.us-east-1.amazonaws.com/x" {
		t.Errorf("us-east-1 url should be unchanged, got %q", got)
	}
}
