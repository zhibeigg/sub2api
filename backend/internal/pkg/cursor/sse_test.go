package cursor

import (
	"context"
	"strings"
	"testing"
)

func TestParseSSEMultilineUsageAndDone(t *testing.T) {
	stream := strings.Join([]string{
		": keepalive",
		"data: {\"type\":\"text-delta\",",
		"data: \"delta\":\"hello\"}",
		"",
		"data:{\"type\":\"finish\",\"finishReason\":\"stop\",\"messageMetadata\":{\"usage\":{\"inputTokens\":3,\"outputTokens\":2}}}",
		"",
	}, "\n")
	var events []SSEEvent
	err := ParseSSE(context.Background(), strings.NewReader(stream), func(event SSEEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Delta != "hello" {
		t.Fatalf("unexpected events: %+v", events)
	}
	usage := events[1].EventUsage()
	if usage == nil || usage.InputTokens != 3 || usage.OutputTokens != 2 || usage.TotalTokens != 5 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
}

func TestParseSSEInvalidDataIsProtocolError(t *testing.T) {
	err := ParseSSE(context.Background(), strings.NewReader("data: not-json\n\n"), nil)
	if err == nil || !IsKind(err, ErrorProtocol) {
		t.Fatalf("expected protocol error, got %v", err)
	}
}

func TestAggregatorEmitsTextToolAndFinish(t *testing.T) {
	var events []Event
	aggregator := NewAggregator(func(event Event) error {
		events = append(events, event)
		return nil
	})
	if err := aggregator.HandleSSE(SSEEvent{Type: "text-delta", Delta: "working\n```json action\n{\"tool\":\"read\",\"parameters\":{\"path\":\"a.go\"}}\n```"}); err != nil {
		t.Fatal(err)
	}
	if err := aggregator.HandleSSE(SSEEvent{Type: "finish", MessageMetadata: &MessageMetadata{Usage: &Usage{InputTokens: 4, OutputTokens: 5}}}); err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("expected text, tool, finish events, got %#v", events)
	}
	call, ok := events[1].(ToolCall)
	if !ok || call.Name != "read" || call.Arguments["path"] != "a.go" {
		t.Fatalf("unexpected tool event: %#v", events[1])
	}
	finish, ok := events[2].(Finish)
	if !ok || finish.Reason != "tool_calls" || finish.Usage.TotalTokens != 9 {
		t.Fatalf("unexpected finish event: %#v", events[2])
	}
	if aggregator.CleanText() != "working" {
		t.Fatalf("unexpected clean text %q", aggregator.CleanText())
	}
}
