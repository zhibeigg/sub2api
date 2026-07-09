package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
)

func TestBuildKiroClaudeUsageSplitsPromptTokens(t *testing.T) {
	usage := buildKiroClaudeUsage(1000, 25, kiro.PromptCacheUsage{
		CacheCreationInputTokens:   200,
		CacheReadInputTokens:       300,
		CacheCreation5mInputTokens: 150,
		CacheCreation1hInputTokens: 50,
	})

	if usage.InputTokens != 500 || usage.OutputTokens != 25 {
		t.Fatalf("unexpected regular tokens: %+v", usage)
	}
	if usage.CacheCreationInputTokens != 200 || usage.CacheReadInputTokens != 300 {
		t.Fatalf("unexpected cache tokens: %+v", usage)
	}
	if usage.CacheCreation5mTokens != 150 || usage.CacheCreation1hTokens != 50 {
		t.Fatalf("unexpected cache TTL breakdown: %+v", usage)
	}
	if got := usage.InputTokens + usage.CacheCreationInputTokens + usage.CacheReadInputTokens; got != 1000 {
		t.Fatalf("prompt token split = %d, want 1000", got)
	}
}

func TestBuildKiroClaudeUsageClampsCacheToFinalTotal(t *testing.T) {
	usage := buildKiroClaudeUsage(100, 7, kiro.PromptCacheUsage{
		CacheCreationInputTokens:   50,
		CacheReadInputTokens:       80,
		CacheCreation5mInputTokens: 30,
		CacheCreation1hInputTokens: 20,
	})

	if usage.InputTokens != 0 || usage.CacheReadInputTokens != 80 || usage.CacheCreationInputTokens != 20 {
		t.Fatalf("cache usage was not clamped to total input: %+v", usage)
	}
	if usage.CacheCreation5mTokens+usage.CacheCreation1hTokens != usage.CacheCreationInputTokens {
		t.Fatalf("cache creation breakdown is not conserved: %+v", usage)
	}
	if got := usage.InputTokens + usage.CacheCreationInputTokens + usage.CacheReadInputTokens; got != 100 {
		t.Fatalf("prompt token split = %d, want 100", got)
	}
}

func TestToKiroClaudeUsageIncludesCacheBreakdown(t *testing.T) {
	converted := toKiroClaudeUsage(ClaudeUsage{
		InputTokens:              10,
		OutputTokens:             5,
		CacheCreationInputTokens: 30,
		CacheReadInputTokens:     40,
		CacheCreation5mTokens:    20,
		CacheCreation1hTokens:    10,
	})

	if converted.InputTokens != 10 || converted.OutputTokens != 5 || converted.CacheCreationInputTokens != 30 || converted.CacheReadInputTokens != 40 {
		t.Fatalf("unexpected converted usage: %+v", converted)
	}
	if converted.CacheCreation == nil || converted.CacheCreation.Ephemeral5mInputTokens != 20 || converted.CacheCreation.Ephemeral1hInputTokens != 10 {
		t.Fatalf("missing cache creation breakdown: %+v", converted.CacheCreation)
	}
}
