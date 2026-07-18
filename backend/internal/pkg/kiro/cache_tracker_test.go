package kiro

import (
	"strings"
	"testing"
	"time"
)

func TestPromptCacheTrackerFirstCreationAndRepeatedRead(t *testing.T) {
	tracker := NewPromptCacheTracker()
	profile := tracker.BuildClaudeProfile(cacheTestRequest("claude-sonnet-4.5", defaultPromptCacheControl()), 4096)
	if profile == nil {
		t.Fatal("expected cache profile")
	}

	first := tracker.Compute("account-1", profile)
	if first.CacheCreationInputTokens <= 0 || first.CacheReadInputTokens != 0 {
		t.Fatalf("expected first request to create cache only, got %+v", first)
	}

	tracker.Update("account-1", profile)
	second := tracker.Compute("account-1", profile)
	if second.CacheReadInputTokens <= 0 || second.CacheCreationInputTokens != 0 {
		t.Fatalf("expected repeated request to read cache only, got %+v", second)
	}
}

func TestPromptCacheTrackerTTLClassificationAndMaximum(t *testing.T) {
	tracker := NewPromptCacheTracker()
	req := &ClaudeRequest{
		Model: "claude-sonnet-4.5",
		System: []any{
			map[string]any{
				"type": "text",
				"text": cacheTestText(1200),
				"cache_control": map[string]any{
					"type": "ephemeral",
					"ttl":  "90m",
				},
			},
		},
		Messages: []ClaudeMessage{{
			Role: "user",
			Content: []any{
				map[string]any{
					"type":          "text",
					"text":          cacheTestText(1200),
					"cache_control": defaultPromptCacheControl(),
				},
			},
		}},
	}

	profile := tracker.BuildClaudeProfile(req, 8192)
	usage := tracker.Compute("account-1", profile)
	if usage.CacheCreation5mInputTokens <= 0 || usage.CacheCreation1hInputTokens <= 0 {
		t.Fatalf("expected both TTL classes, got %+v", usage)
	}
	if usage.CacheCreation5mInputTokens+usage.CacheCreation1hInputTokens != usage.CacheCreationInputTokens {
		t.Fatalf("TTL creation breakdown does not match creation total: %+v", usage)
	}
	if got := profile.breakpoints[0].ttl; got != time.Hour {
		t.Fatalf("expected TTL above five minutes to normalize to one hour, got %s", got)
	}
}

func TestPromptCacheTrackerOpusMinimum(t *testing.T) {
	tracker := NewPromptCacheTracker()
	sonnetReq := cacheTestRequest("claude-sonnet-4.5", defaultPromptCacheControl())
	sonnetProfile := tracker.BuildClaudeProfile(sonnetReq, 3000)
	if usage := tracker.Compute("sonnet", sonnetProfile); usage.CacheCreationInputTokens == 0 {
		t.Fatalf("expected ordinary model breakpoint to meet 1024-token threshold: %+v", usage)
	}

	opusReq := cacheTestRequest("claude-opus-4.6", defaultPromptCacheControl())
	opusProfile := tracker.BuildClaudeProfile(opusReq, 3000)
	if usage := tracker.Compute("opus", opusProfile); usage.CacheCreationInputTokens != 0 {
		t.Fatalf("expected Opus breakpoint below 4096 tokens to be ignored: %+v", usage)
	}
}

func TestPromptCacheTrackerCapsRepeatedMatchAt85Percent(t *testing.T) {
	tracker := NewPromptCacheTracker()
	profile := tracker.BuildClaudeProfile(cacheTestRequest("claude-sonnet-4.5", defaultPromptCacheControl()), 0)
	tracker.Update("account-1", profile)

	usage := tracker.Compute("account-1", profile)
	want := profile.totalInputTokens * maximumCacheMatchPercent / 100
	if usage.CacheReadInputTokens != want {
		t.Fatalf("expected cache read cap %d, got %+v", want, usage)
	}
	if usage.CacheReadInputTokens > profile.totalInputTokens*85/100 {
		t.Fatalf("cache read exceeded 85%% cap: %+v", usage)
	}
}

func TestPromptCacheTrackerIsolatesAccounts(t *testing.T) {
	tracker := NewPromptCacheTracker()
	profile := tracker.BuildClaudeProfile(cacheTestRequest("claude-sonnet-4.5", defaultPromptCacheControl()), 4096)
	tracker.Update("account-1", profile)

	if usage := tracker.Compute("account-1", profile); usage.CacheReadInputTokens == 0 {
		t.Fatalf("expected stored account to read cache: %+v", usage)
	}
	if usage := tracker.Compute("account-2", profile); usage.CacheReadInputTokens != 0 || usage.CacheCreationInputTokens == 0 {
		t.Fatalf("expected separate account to create its own cache: %+v", usage)
	}
}

func TestPromptCacheTrackerIgnoresBillingHeaderDrift(t *testing.T) {
	tracker := NewPromptCacheTracker()
	build := func(header string) *ClaudeRequest {
		return &ClaudeRequest{
			Model: "claude-sonnet-4.5",
			System: []any{
				map[string]any{"type": "text", "text": header},
				map[string]any{
					"type":          "text",
					"text":          cacheTestText(1200),
					"cache_control": defaultPromptCacheControl(),
				},
			},
			Messages: []ClaudeMessage{{Role: "user", Content: "hello"}},
		}
	}

	first := tracker.BuildClaudeProfile(build("x-anthropic-billing-header: cc_version=1; cch=aaaa"), 4096)
	tracker.Update("account-1", first)
	second := tracker.BuildClaudeProfile(build("  X-Anthropic-Billing-Header: cc_version=2; cch=bbbb; padding=changed"), 4096)
	if usage := tracker.Compute("account-1", second); usage.CacheReadInputTokens == 0 {
		t.Fatalf("expected billing header drift to preserve cache hit: %+v", usage)
	}
}

func TestPromptCacheTrackerAddsImplicitMessageEndBreakpoints(t *testing.T) {
	tracker := NewPromptCacheTracker()
	system := []any{
		map[string]any{
			"type":          "text",
			"text":          cacheTestText(1200),
			"cache_control": defaultPromptCacheControl(),
		},
	}
	first := tracker.BuildClaudeProfile(&ClaudeRequest{
		Model:    "claude-sonnet-4.5",
		System:   system,
		Messages: []ClaudeMessage{{Role: "user", Content: "question one"}},
	}, 4096)
	if len(first.breakpoints) < 2 {
		t.Fatalf("expected explicit and implicit breakpoints, got %d", len(first.breakpoints))
	}
	tracker.Update("account-1", first)

	continued := tracker.BuildClaudeProfile(&ClaudeRequest{
		Model:  "claude-sonnet-4.5",
		System: system,
		Messages: []ClaudeMessage{
			{Role: "user", Content: "question one"},
			{Role: "assistant", Content: "answer one"},
			{Role: "user", Content: "follow-up"},
		},
	}, 8192)
	if usage := tracker.Compute("account-1", continued); usage.CacheReadInputTokens == 0 {
		t.Fatalf("expected cache hit at implicit message boundary: %+v", usage)
	}
}

func TestPromptCacheTrackerExpiresEntries(t *testing.T) {
	tracker := NewPromptCacheTracker()
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	tracker.now = func() time.Time { return now }
	profile := tracker.BuildClaudeProfile(cacheTestRequest("claude-sonnet-4.5", defaultPromptCacheControl()), 4096)
	tracker.Update("account-1", profile)

	now = now.Add(defaultPromptCacheTTL + time.Second)
	usage := tracker.Compute("account-1", profile)
	if usage.CacheReadInputTokens != 0 || usage.CacheCreationInputTokens == 0 {
		t.Fatalf("expected expired entry to require cache creation: %+v", usage)
	}
	if _, ok := tracker.entriesByAccount["account-1"]; ok {
		t.Fatal("expected expired account entries to be pruned")
	}
}

func TestPromptCacheUsagePreservesTotalInput(t *testing.T) {
	tracker := NewPromptCacheTracker()
	profile := tracker.BuildClaudeProfile(cacheTestRequest("claude-sonnet-4.5", defaultPromptCacheControl()), 4096)
	tracker.Update("account-1", profile)
	usage := tracker.Compute("account-1", profile)

	uncached := usage.UncachedInputTokens(profile.totalInputTokens)
	if got := uncached + usage.CacheCreationInputTokens + usage.CacheReadInputTokens; got != profile.totalInputTokens {
		t.Fatalf("input token split = %d, want %d; usage=%+v", got, profile.totalInputTokens, usage)
	}
	if got := UncachedPromptInputTokens(profile.totalInputTokens, usage); got != uncached {
		t.Fatalf("function and method disagree: %d vs %d", got, uncached)
	}
}

func TestPromptCacheTrackerIgnoresCacheControlAndWrapperPositions(t *testing.T) {
	first := canonicalizeCacheValue(stripCachePositionKeys(map[string]any{
		"kind":         "system",
		"system_index": 0,
		"block": map[string]any{
			"type":          "text",
			"text":          "stable",
			"cache_control": map[string]any{"type": "ephemeral", "ttl": "5m"},
		},
	}))
	second := canonicalizeCacheValue(stripCachePositionKeys(map[string]any{
		"kind":         "system",
		"system_index": 9,
		"block": map[string]any{
			"type":          "text",
			"text":          "stable",
			"cache_control": map[string]any{"type": "ephemeral", "ttl": "1h"},
		},
	}))
	if first != second {
		t.Fatalf("expected cache metadata and wrapper positions to be ignored: %q != %q", first, second)
	}
}

func TestPromptCacheTrackerSupportsToolCacheControl(t *testing.T) {
	tracker := NewPromptCacheTracker()
	req := &ClaudeRequest{
		Model: "claude-sonnet-4.5",
		Tools: []ClaudeTool{{
			Name:         "large_tool",
			Description:  cacheTestText(1200),
			InputSchema:  map[string]any{"type": "object"},
			CacheControl: defaultPromptCacheControl(),
		}},
		Messages: []ClaudeMessage{{Role: "user", Content: "hello"}},
	}
	profile := tracker.BuildClaudeProfile(req, 4096)
	if profile == nil || len(profile.breakpoints) == 0 {
		t.Fatal("expected tool cache_control to create a breakpoint")
	}

	payload := ClaudeToKiro(req, false)
	context := payload.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext
	if context == nil || len(context.Tools) != 1 || context.Tools[0].ToolSpecification.Name != "largeTool" {
		t.Fatalf("tool conversion changed unexpectedly: %#v", context)
	}
}

func cacheTestRequest(model string, cacheControl any) *ClaudeRequest {
	return &ClaudeRequest{
		Model: model,
		System: []any{
			map[string]any{
				"type":          "text",
				"text":          cacheTestText(1200),
				"cache_control": cacheControl,
			},
		},
		Messages: []ClaudeMessage{{Role: "user", Content: "hello"}},
	}
}

func cacheTestText(repetitions int) string {
	return strings.Repeat("cache ", repetitions)
}

func defaultPromptCacheControl() map[string]any {
	return map[string]any{"type": "ephemeral"}
}
