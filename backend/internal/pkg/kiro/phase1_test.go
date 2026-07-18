package kiro

import (
	"strings"
	"testing"
)

func TestParseSubscriptionType(t *testing.T) {
	cases := map[string]string{
		"PRO_PLUS":           "PRO_PLUS",
		"pro_plus":           "PRO_PLUS",
		"ProPlus":            "PRO_PLUS",
		"Kiro Power":         "POWER",
		"KIRO PRO":           "PRO",
		"KIRO PRO+":          "PRO", // "PRO+" does not contain PRO_PLUS/PROPLUS → PRO (matches Kiro-Go)
		"free tier":          "FREE",
		"":                   "FREE",
		"something-unmapped": "FREE",
	}
	for in, want := range cases {
		if got := parseSubscriptionType(in); got != want {
			t.Errorf("parseSubscriptionType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRegionalizeRESTURL(t *testing.T) {
	// us-east-1 profile → no-op.
	credUSE := &Credential{ProfileArn: "arn:aws:codewhisperer:us-east-1:1:profile/x"}
	in := "https://codewhisperer.us-east-1.amazonaws.com/ListAvailableModels?origin=AI_EDITOR"
	if got := regionalizeRESTURL(in, credUSE); got != in {
		t.Errorf("regionalizeRESTURL us-east-1 should be no-op, got %q", got)
	}

	// eu-west-1 profile → rewrite host to q.eu-west-1.
	credEU := &Credential{ProfileArn: "arn:aws:codewhisperer:eu-west-1:1:profile/x"}
	got := regionalizeRESTURL(in, credEU)
	if !strings.Contains(got, "q.eu-west-1.amazonaws.com") {
		t.Errorf("regionalizeRESTURL eu-west-1 should rewrite host, got %q", got)
	}
}

func TestWithProfileArnQuery(t *testing.T) {
	cred := &Credential{ProfileArn: "arn:aws:codewhisperer:us-east-1:1:profile/x y"}
	got := withProfileArnQuery("https://host/x?a=1", cred)
	if !strings.Contains(got, "profileArn=arn%3Aaws%3Acodewhisperer") {
		t.Errorf("expected escaped profileArn query, got %q", got)
	}
	if got := withProfileArnQuery("https://host/x?a=1", &Credential{}); got != "https://host/x?a=1" {
		t.Errorf("empty profileArn should be no-op, got %q", got)
	}
}

func TestClassifyAccountInfoError(t *testing.T) {
	suspended := classifyAccountInfoError(errString("HTTP 403: TEMPORARILY_SUSPENDED unusual activity"))
	if e, ok := suspended.(*AccountInfoError); !ok || !e.Suspended {
		t.Errorf("expected suspended classification, got %#v", suspended)
	}
	authErr := classifyAccountInfoError(errString("HTTP 401: token expired"))
	if e, ok := authErr.(*AccountInfoError); !ok || !e.AuthError {
		t.Errorf("expected auth-error classification, got %#v", authErr)
	}
	other := classifyAccountInfoError(errString("HTTP 500: internal"))
	if e, ok := other.(*AccountInfoError); !ok || e.Suspended || e.AuthError {
		t.Errorf("expected generic classification, got %#v", other)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestPromptFiltersDisabledByDefault(t *testing.T) {
	SetPromptFilterConfig(PromptFilterConfig{})
	in := "--- SYSTEM PROMPT ---\nYou are Claude Code\ngitStatus: dirty\nreal content"
	if got := applyPromptFilters(in); got != strings.TrimSpace(in) {
		t.Errorf("filters disabled should only trim, got %q", got)
	}
}

func TestPromptFilterStripBoundariesAndEnvNoise(t *testing.T) {
	SetPromptFilterConfig(PromptFilterConfig{FilterStripBoundaries: true, FilterEnvNoise: true})
	defer SetPromptFilterConfig(PromptFilterConfig{})
	in := "--- SYSTEM PROMPT ---\nreal content\ngitStatus: dirty\nRecent commits: abc\n--- END SYSTEM PROMPT ---"
	got := applyPromptFilters(in)
	if strings.Contains(got, "SYSTEM PROMPT") || strings.Contains(got, "gitStatus") || strings.Contains(got, "Recent commits") {
		t.Errorf("boundaries/env noise not stripped: %q", got)
	}
	if !strings.Contains(got, "real content") {
		t.Errorf("real content should be preserved: %q", got)
	}
}

func TestPromptFilterClaudeCodeReplacement(t *testing.T) {
	SetPromptFilterConfig(PromptFilterConfig{FilterClaudeCode: true})
	defer SetPromptFilterConfig(PromptFilterConfig{})
	in := "You are an interactive agent that helps users with software engineering tasks.\n# Doing tasks\n# Tone and style"
	got := applyPromptFilters(in)
	if got != claudeCodeBackendPrompt {
		t.Errorf("expected claude code replacement, got %q", got)
	}
}

func TestPromptFilterUserRule(t *testing.T) {
	SetPromptFilterConfig(PromptFilterConfig{
		Rules: []PromptFilterRule{
			{Type: "lines-containing", Match: "secret", Enabled: true},
			{Type: "regex", Match: `\d{4}`, Replace: "####", Enabled: true},
		},
	})
	defer SetPromptFilterConfig(PromptFilterConfig{})
	in := "keep this\nsecret line to drop\ncode 1234"
	got := applyPromptFilters(in)
	if strings.Contains(got, "secret") {
		t.Errorf("secret line not dropped: %q", got)
	}
	if !strings.Contains(got, "####") || strings.Contains(got, "1234") {
		t.Errorf("regex rule not applied: %q", got)
	}
}

func TestExtractThinkingFromContent(t *testing.T) {
	content, reasoning := extractThinkingFromContent("<thinking>step one</thinking>Hello <thinking>step two</thinking>World")
	if content != "HelloWorld" && content != "Hello World" {
		// spans are removed verbatim; whitespace between depends on input
		if !strings.Contains(content, "Hello") || !strings.Contains(content, "World") {
			t.Errorf("unexpected content %q", content)
		}
	}
	if !strings.Contains(reasoning, "step one") || !strings.Contains(reasoning, "step two") {
		t.Errorf("reasoning missing spans: %q", reasoning)
	}
}

func TestKiroToOpenAIResponseWithReasoning(t *testing.T) {
	// reasoning_content format
	resp := KiroToOpenAIResponseWithReasoning("answer", "why", nil, 10, 5, "claude-sonnet-4.5", "reasoning_content")
	choices, ok := resp["choices"].([]map[string]any)
	if !ok || len(choices) == 0 {
		t.Fatalf("choices has unexpected value: %#v", resp["choices"])
	}
	msg, ok := choices[0]["message"].(map[string]any)
	if !ok {
		t.Fatalf("message has unexpected value: %#v", choices[0]["message"])
	}
	if msg["reasoning_content"] != "why" || msg["content"] != "answer" {
		t.Errorf("reasoning_content format wrong: %#v", msg)
	}
	// thinking format inlines
	resp2 := KiroToOpenAIResponseWithReasoning("answer", "why", nil, 10, 5, "m", "thinking")
	choices2, ok := resp2["choices"].([]map[string]any)
	if !ok || len(choices2) == 0 {
		t.Fatalf("thinking choices has unexpected value: %#v", resp2["choices"])
	}
	msg2, ok := choices2[0]["message"].(map[string]any)
	if !ok {
		t.Fatalf("thinking message has unexpected value: %#v", choices2[0]["message"])
	}
	content, ok := msg2["content"].(string)
	if !ok || !strings.Contains(content, "<thinking>why</thinking>answer") {
		t.Errorf("thinking format wrong: %#v", msg2)
	}
}

func TestGenerateCodeChallengeDeterministic(t *testing.T) {
	verifier := "test-verifier"
	firstChallenge := generateCodeChallenge(verifier)
	secondChallenge := generateCodeChallenge(verifier)
	if firstChallenge != secondChallenge {
		t.Error("code challenge should be deterministic for the same verifier")
	}
	firstVerifier := generateCodeVerifier()
	secondVerifier := generateCodeVerifier()
	if firstVerifier == secondVerifier {
		t.Error("code verifiers should be random")
	}
}

func TestParseAuthCodeCallback(t *testing.T) {
	code, err := ParseAuthCodeCallback("http://127.0.0.1/oauth/callback?code=abc&state=xyz", "xyz")
	if err != nil || code != "abc" {
		t.Errorf("valid callback failed: code=%q err=%v", code, err)
	}
	if _, err := ParseAuthCodeCallback("http://x/cb?code=abc&state=bad", "xyz"); err == nil {
		t.Error("state mismatch should error")
	}
	if _, err := ParseAuthCodeCallback("http://x/cb?error=access_denied&state=xyz", "xyz"); err == nil {
		t.Error("error param should error")
	}
	if _, err := ParseAuthCodeCallback("http://x/cb?state=xyz", "xyz"); err == nil {
		t.Error("missing code should error")
	}
}

func TestConstantTimeEqual(t *testing.T) {
	if !constantTimeEqual("abc", "abc") {
		t.Error("equal strings should match")
	}
	if constantTimeEqual("abc", "abd") || constantTimeEqual("abc", "abcd") {
		t.Error("different strings should not match")
	}
}
