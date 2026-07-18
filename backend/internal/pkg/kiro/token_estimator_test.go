package kiro

import "testing"

func TestEstimateApproxTokens(t *testing.T) {
	if got := estimateApproxTokens(""); got != 0 {
		t.Errorf("empty string should be 0 tokens, got %d", got)
	}
	// Short strings (<5 runes) use ceil(len/3).
	if got := estimateApproxTokens("hi"); got != 1 {
		t.Errorf("estimateApproxTokens(\"hi\") = %d, want 1", got)
	}
	// Non-empty text should always yield >= 1 token.
	if got := estimateApproxTokens("hello world, this is a test"); got < 1 {
		t.Errorf("non-empty text should be >= 1 token, got %d", got)
	}
	// CJK text is weighted more heavily than ASCII.
	ascii := estimateApproxTokens("aaaaaaaaaaaaaaaaaaaa") // 20 ASCII
	cjk := estimateApproxTokens("你好世界你好世界你好世界你好世界你好世界")   // 20 CJK
	if cjk <= ascii {
		t.Errorf("CJK (%d) should weigh more than ASCII (%d)", cjk, ascii)
	}
}

func TestEstimateClaudeRequestInputTokens(t *testing.T) {
	if got := EstimateClaudeRequestInputTokens(nil); got != 0 {
		t.Errorf("nil request should be 0, got %d", got)
	}
	req := &ClaudeRequest{
		System: "You are a helpful assistant.",
		Messages: []ClaudeMessage{
			{Role: "user", Content: "Write a long essay about the history of computing."},
		},
		Tools: []ClaudeTool{
			{Name: "search", Description: "search the web", InputSchema: map[string]any{"type": "object"}},
		},
	}
	if got := EstimateClaudeRequestInputTokens(req); got <= 0 {
		t.Errorf("request with content should be > 0, got %d", got)
	}
}

func TestEstimateClaudeOutputTokens(t *testing.T) {
	if got := EstimateClaudeOutputTokens("", "", nil); got != 0 {
		t.Errorf("empty output should be 0, got %d", got)
	}
	toolUses := []KiroToolUse{
		{Name: "get_weather", Input: map[string]any{"city": "Beijing"}},
	}
	if got := EstimateClaudeOutputTokens("The weather is sunny today.", "let me think", toolUses); got <= 0 {
		t.Errorf("non-empty output should be > 0, got %d", got)
	}
}

func TestEstimateOpenAIRequestInputTokens(t *testing.T) {
	req := &OpenAIRequest{
		Messages: []OpenAIMessage{
			{Role: "user", Content: "Hello, how are you doing today?"},
		},
	}
	if got := EstimateOpenAIRequestInputTokens(req); got <= 0 {
		t.Errorf("request with content should be > 0, got %d", got)
	}
}
