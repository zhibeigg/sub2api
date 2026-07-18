package cursor

import (
	"strings"
	"testing"
)

func TestParseActionsToleratesControlCharactersAndTruncation(t *testing.T) {
	text := "before\n```json action\n{\"tool\":\"write\",\"parameters\":{\"content\":\"line 1\nline 2 with ``` inside\",\"path\":\"a.txt\"}"
	actions, clean, err := ParseActions(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Name != "write" {
		t.Fatalf("unexpected actions: %+v", actions)
	}
	content, ok := actions[0].Arguments["content"].(string)
	if actions[0].Arguments["path"] != "a.txt" || !ok || !strings.Contains(content, "line 2") {
		t.Fatalf("arguments were not preserved: %+v", actions[0].Arguments)
	}
	if clean != "before" {
		t.Fatalf("unexpected clean text %q", clean)
	}
}

func TestParseActionsNeverFabricatesParameters(t *testing.T) {
	_, _, err := ParseActions("```json action\n{\"tool\":\"read_file\"}\n```")
	if err == nil {
		t.Fatal("expected missing parameters to fail")
	}
	formatted, err := FormatAction(Action{Name: "read_file"})
	if err == nil || formatted != "" {
		t.Fatalf("expected nil arguments to fail, got %q, %v", formatted, err)
	}
}

func TestFormatAndParseMultipleActions(t *testing.T) {
	first, err := FormatAction(Action{Name: "one", Arguments: map[string]any{"x": 1}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := FormatAction(Action{Name: "two", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	actions, clean, err := ParseActions("intro\n" + first + "\ntext\n" + second + "\noutro")
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 2 || actions[0].Name != "one" || actions[1].Name != "two" {
		t.Fatalf("unexpected actions: %+v", actions)
	}
	if !strings.Contains(clean, "intro") || !strings.Contains(clean, "text") || !strings.Contains(clean, "outro") || strings.Contains(clean, "json action") {
		t.Fatalf("unexpected clean text: %q", clean)
	}
}
