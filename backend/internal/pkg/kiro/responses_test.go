package kiro

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseResponsesInputString(t *testing.T) {
	msgs, err := ParseResponsesInput(json.RawMessage(`"hello"`))
	if err != nil || len(msgs) != 1 || msgs[0].Role != "user" {
		t.Fatalf("string input: %v %#v", err, msgs)
	}
	if txt := extractOpenAIMessageText(msgs[0].Content); txt != "hello" {
		t.Errorf("expected hello, got %q", txt)
	}
}

func TestParseResponsesInputItems(t *testing.T) {
	raw := json.RawMessage(`[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]},
		{"type":"function_call","call_id":"c1","name":"getWeather","arguments":"{\"city\":\"SF\"}"},
		{"type":"function_call_output","call_id":"c1","output":"sunny"}
	]`)
	msgs, err := ParseResponsesInput(raw)
	if err != nil {
		t.Fatalf("parse err: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d: %#v", len(msgs), msgs)
	}
	if msgs[0].Role != "user" {
		t.Errorf("msg0 role = %q", msgs[0].Role)
	}
	if msgs[1].Role != "assistant" || len(msgs[1].ToolCalls) != 1 || msgs[1].ToolCalls[0].Function.Name != "getWeather" {
		t.Errorf("msg1 tool call wrong: %#v", msgs[1])
	}
	if msgs[2].Role != "tool" || msgs[2].ToolCallID != "c1" {
		t.Errorf("msg2 tool result wrong: %#v", msgs[2])
	}
}

func TestParseResponsesInputParallelToolCalls(t *testing.T) {
	raw := json.RawMessage(`[
		{"type":"function_call","call_id":"a","name":"f1","arguments":"{}"},
		{"type":"function_call","call_id":"b","name":"f2","arguments":"{}"}
	]`)
	msgs, err := ParseResponsesInput(raw)
	if err != nil {
		t.Fatalf("parse err: %v", err)
	}
	// parallel calls with no text between them merge into one assistant message
	if len(msgs) != 1 || len(msgs[0].ToolCalls) != 2 {
		t.Fatalf("expected merged parallel tool calls, got %#v", msgs)
	}
}

func TestResponsesStoreAndHistory(t *testing.T) {
	store := NewResponsesStore(time.Minute)

	// Save a root response with an input + assistant output.
	root := &ResponsesObject{
		ID:          "resp_root",
		Object:      "response",
		Status:      "completed",
		Model:       "claude-sonnet-4.5",
		StoredInput: json.RawMessage(`"first question"`),
		Output: []ResponseOutputItem{
			{Type: "message", Role: "assistant", Content: []ResponseContentPart{{Type: "output_text", Text: "first answer"}}},
		},
	}
	store.Save(root)

	child := &ResponsesObject{
		ID:                 "resp_child",
		PreviousResponseID: "resp_root",
		StoredInput:        json.RawMessage(`"second question"`),
		Output: []ResponseOutputItem{
			{Type: "message", Role: "assistant", Content: []ResponseContentPart{{Type: "output_text", Text: "second answer"}}},
		},
	}
	store.Save(child)

	msgs := store.ExpandPreviousResponseHistory(child)
	// oldest-first: root input, root output, child input, child output
	if len(msgs) != 4 {
		t.Fatalf("expected 4 history messages, got %d: %#v", len(msgs), msgs)
	}
	if extractOpenAIMessageText(msgs[0].Content) != "first question" {
		t.Errorf("history[0] = %q", extractOpenAIMessageText(msgs[0].Content))
	}
	if extractOpenAIMessageText(msgs[3].Content) != "second answer" {
		t.Errorf("history[3] = %q", extractOpenAIMessageText(msgs[3].Content))
	}
}

func TestBuildResponsesMessagesWithInstructions(t *testing.T) {
	store := NewResponsesStore(time.Minute)
	req := &ResponsesRequest{
		Model:        "claude-sonnet-4.5",
		Instructions: "be terse",
		Input:        json.RawMessage(`"hello"`),
	}
	msgs, err := store.BuildResponsesMessages(req)
	if err != nil {
		t.Fatalf("build err: %v", err)
	}
	if len(msgs) != 2 || msgs[0].Role != "system" || msgs[1].Role != "user" {
		t.Fatalf("expected [system,user], got %#v", msgs)
	}
}

func TestBuildResponsesObject(t *testing.T) {
	obj := BuildResponsesObject("resp_x", "claude-sonnet-4.5", "answer", nil, 10, 5, &ResponsesRequest{PreviousResponseID: "prev"})
	if obj.Status != "completed" || obj.Object != "response" {
		t.Errorf("unexpected object header: %#v", obj)
	}
	if obj.Usage.TotalTokens != 15 {
		t.Errorf("total tokens = %d", obj.Usage.TotalTokens)
	}
	if len(obj.Output) != 1 || obj.Output[0].Type != "message" {
		t.Errorf("unexpected output: %#v", obj.Output)
	}
	if obj.PreviousResponseID != "prev" {
		t.Errorf("previous id = %q", obj.PreviousResponseID)
	}
}

func TestBuildResponsesObjectWithToolCalls(t *testing.T) {
	toolUses := []KiroToolUse{{ToolUseID: "t1", Name: "search", Input: map[string]any{"q": "x"}}}
	obj := BuildResponsesObject("resp_y", "m", "", toolUses, 1, 1, nil)
	// one function_call output item (no empty message when content is empty but tools present)
	if len(obj.Output) != 1 || obj.Output[0].Type != "function_call" || obj.Output[0].Name != "search" {
		t.Fatalf("unexpected tool output: %#v", obj.Output)
	}
}

// captureSSE is a minimal SSEWriter for testing the Responses stream state.
type captureSSE struct {
	events []string
	datas  []string
}

func (w *captureSSE) WriteSSE(event string, data []byte) error {
	w.events = append(w.events, event)
	w.datas = append(w.datas, string(data))
	return nil
}
func (w *captureSSE) Flush() {}

func TestResponsesStreamStateBasicFlow(t *testing.T) {
	w := &captureSSE{}
	state := NewResponsesStreamState(w, "claude-sonnet-4.5", "resp_s", &ResponsesRequest{}, time.Now().Unix())
	cb := state.Callback()
	cb.OnText("Hello", false)
	cb.OnText(" world", false)
	cb.OnComplete(7, 3)
	obj, in, out, _ := state.Finish(false, 0)

	if obj == nil || obj.Status != "completed" {
		t.Fatalf("unexpected finish object: %#v", obj)
	}
	if in != 7 || out != 3 {
		t.Errorf("tokens = %d/%d", in, out)
	}
	// must have emitted response.created and terminated with [DONE]
	foundCreated, foundDone := false, false
	for i, ev := range w.events {
		if ev == "response.created" {
			foundCreated = true
		}
		if ev == "" && w.datas[i] == "[DONE]" {
			foundDone = true
		}
	}
	if !foundCreated || !foundDone {
		t.Errorf("missing created/done events: %#v", w.events)
	}
}
