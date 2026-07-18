package kiro

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"hash"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultPromptCacheTTL     = 5 * time.Minute
	maximumPromptCacheTTL     = time.Hour
	defaultMinCacheableTokens = 1024
	opusMinCacheableTokens    = 4096
	maximumCacheMatchPercent  = 85
)

// PromptCacheUsage describes the cache portions of an Anthropic prompt token
// count. Creation and read tokens may both be non-zero when a request reuses an
// existing prefix and extends it with a newly cached suffix.
type PromptCacheUsage struct {
	// CacheCreationInputTokens is the number of input tokens written to cache.
	CacheCreationInputTokens int
	// CacheReadInputTokens is the number of input tokens served from cache.
	CacheReadInputTokens int
	// CacheCreation5mInputTokens is the creation portion using the five-minute TTL.
	CacheCreation5mInputTokens int
	// CacheCreation1hInputTokens is the creation portion using the one-hour TTL.
	CacheCreation1hInputTokens int
}

// UncachedInputTokens returns the non-cache portion of totalInputTokens without
// mutating the usage value.
func (u PromptCacheUsage) UncachedInputTokens(totalInputTokens int) int {
	return UncachedPromptInputTokens(totalInputTokens, u)
}

// UncachedPromptInputTokens subtracts cache creation and cache reads from a
// total Anthropic input token count and clamps the result at zero.
func UncachedPromptInputTokens(totalInputTokens int, usage PromptCacheUsage) int {
	uncached := totalInputTokens - usage.CacheCreationInputTokens - usage.CacheReadInputTokens
	if uncached < 0 {
		return 0
	}
	return uncached
}

type promptCacheBreakpoint struct {
	fingerprint      [32]byte
	cumulativeTokens int
	ttl              time.Duration
}

// PromptCacheProfile is an opaque description of the cache breakpoints in one
// Anthropic request. Profiles are built by PromptCacheTracker.BuildClaudeProfile.
type PromptCacheProfile struct {
	breakpoints      []promptCacheBreakpoint
	totalInputTokens int
	model            string
}

type promptCacheEntry struct {
	expiresAt time.Time
	ttl       time.Duration
}

// PromptCacheTracker tracks Anthropic prompt cache prefixes independently for
// each upstream account. A tracker is safe for concurrent use.
type PromptCacheTracker struct {
	mu               sync.Mutex
	entriesByAccount map[string]map[[32]byte]promptCacheEntry
	now              func() time.Time
}

// NewPromptCacheTracker creates an empty tracker using Anthropic's supported
// five-minute and one-hour prompt cache TTL classes.
func NewPromptCacheTracker() *PromptCacheTracker {
	return &PromptCacheTracker{
		entriesByAccount: make(map[string]map[[32]byte]promptCacheEntry),
		now:              time.Now,
	}
}

// BuildClaudeProfile fingerprints the cacheable prefixes in req and associates
// them with totalInputTokens. It returns nil when the request has no explicit
// cache_control breakpoint.
func (t *PromptCacheTracker) BuildClaudeProfile(req *ClaudeRequest, totalInputTokens int) *PromptCacheProfile {
	if req == nil {
		return nil
	}

	blocks := flattenClaudeCacheBlocks(req)
	if len(blocks) == 0 {
		return nil
	}

	hasher := sha256.New()
	breakpoints := make([]promptCacheBreakpoint, 0)
	cumulativeTokens := 0
	var activeTTL time.Duration

	for _, block := range blocks {
		canonical := canonicalizeCacheValue(block.value)
		writeCacheHashChunk(hasher, canonical)
		cumulativeTokens += block.tokens

		breakpointTTL := time.Duration(0)
		if block.ttl > 0 {
			breakpointTTL = block.ttl
			activeTTL = block.ttl
		} else if block.isMessageEnd && activeTTL > 0 {
			breakpointTTL = activeTTL
		}
		if breakpointTTL <= 0 {
			continue
		}

		var fingerprint [32]byte
		copy(fingerprint[:], hasher.Sum(nil))
		breakpoints = append(breakpoints, promptCacheBreakpoint{
			fingerprint:      fingerprint,
			cumulativeTokens: cumulativeTokens,
			ttl:              breakpointTTL,
		})
	}

	if len(breakpoints) == 0 {
		return nil
	}
	if totalInputTokens < cumulativeTokens {
		totalInputTokens = cumulativeTokens
	}

	return &PromptCacheProfile{
		breakpoints:      breakpoints,
		totalInputTokens: totalInputTokens,
		model:            req.Model,
	}
}

// Compute reports how a profile would be split between cache creation and
// cache reads for accountID. Compute refreshes the matched entry's TTL but does
// not store newly created prefixes; call Update after a successful upstream
// request to store them.
func (t *PromptCacheTracker) Compute(accountID string, profile *PromptCacheProfile) PromptCacheUsage {
	if t == nil || profile == nil || len(profile.breakpoints) == 0 || accountID == "" {
		return PromptCacheUsage{}
	}

	minTokens := minCacheableTokensForModel(profile.model)
	last := profile.breakpoints[len(profile.breakpoints)-1]
	lastTokens := minCacheInt(last.cumulativeTokens, profile.totalInputTokens)
	now := t.currentTime()

	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneExpiredLocked(now)

	entries := t.entriesByAccount[accountID]
	if len(entries) == 0 {
		if lastTokens < minTokens {
			return PromptCacheUsage{}
		}
		cache5m, cache1h := computePromptCacheTTLBreakdown(profile, 0, lastTokens)
		return PromptCacheUsage{
			CacheCreationInputTokens:   lastTokens,
			CacheCreation5mInputTokens: cache5m,
			CacheCreation1hInputTokens: cache1h,
		}
	}

	maxCacheable := profile.totalInputTokens * maximumCacheMatchPercent / 100
	cacheableEnd := minCacheInt(lastTokens, maxCacheable)
	matchedTokens := 0
	for i := len(profile.breakpoints) - 1; i >= 0; i-- {
		breakpoint := profile.breakpoints[i]
		if breakpoint.cumulativeTokens < minTokens {
			continue
		}
		entry, ok := entries[breakpoint.fingerprint]
		if !ok || !entry.expiresAt.After(now) {
			continue
		}
		entry.expiresAt = now.Add(entry.ttl)
		entries[breakpoint.fingerprint] = entry
		matchedTokens = minCacheInt(breakpoint.cumulativeTokens, cacheableEnd)
		break
	}

	creation := cacheableEnd - matchedTokens
	if creation < 0 {
		creation = 0
	}
	cache5m, cache1h := computePromptCacheTTLBreakdown(profile, matchedTokens, cacheableEnd)
	return PromptCacheUsage{
		CacheCreationInputTokens:   creation,
		CacheReadInputTokens:       matchedTokens,
		CacheCreation5mInputTokens: cache5m,
		CacheCreation1hInputTokens: cache1h,
	}
}

// Update stores all eligible cache breakpoints in profile for accountID. It is
// intended to be called only after the corresponding upstream request succeeds.
func (t *PromptCacheTracker) Update(accountID string, profile *PromptCacheProfile) {
	if t == nil || profile == nil || len(profile.breakpoints) == 0 || accountID == "" {
		return
	}

	minTokens := minCacheableTokensForModel(profile.model)
	now := t.currentTime()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneExpiredLocked(now)

	entries := t.entriesByAccount[accountID]
	if entries == nil {
		entries = make(map[[32]byte]promptCacheEntry)
		t.entriesByAccount[accountID] = entries
	}
	for _, breakpoint := range profile.breakpoints {
		if breakpoint.cumulativeTokens < minTokens {
			continue
		}
		entries[breakpoint.fingerprint] = promptCacheEntry{
			expiresAt: now.Add(breakpoint.ttl),
			ttl:       breakpoint.ttl,
		}
	}
	if len(entries) == 0 {
		delete(t.entriesByAccount, accountID)
	}
}

func (t *PromptCacheTracker) currentTime() time.Time {
	if t.now == nil {
		return time.Now()
	}
	return t.now()
}

func (t *PromptCacheTracker) pruneExpiredLocked(now time.Time) {
	for accountID, entries := range t.entriesByAccount {
		for fingerprint, entry := range entries {
			if !entry.expiresAt.After(now) {
				delete(entries, fingerprint)
			}
		}
		if len(entries) == 0 {
			delete(t.entriesByAccount, accountID)
		}
	}
}

func minCacheableTokensForModel(model string) int {
	if strings.Contains(strings.ToLower(model), "opus") {
		return opusMinCacheableTokens
	}
	return defaultMinCacheableTokens
}

type cacheablePromptBlock struct {
	value        any
	tokens       int
	ttl          time.Duration
	isMessageEnd bool
}

func flattenClaudeCacheBlocks(req *ClaudeRequest) []cacheablePromptBlock {
	blocks := make([]cacheablePromptBlock, 0)
	blocks = append(blocks, buildCachePreludeBlock(req))

	for toolIndex, tool := range req.Tools {
		toolValue := map[string]any{
			"kind":          "tool",
			"tool_index":    toolIndex,
			"name":          tool.Name,
			"description":   tool.Description,
			"input_schema":  tool.InputSchema,
			"cache_control": tool.CacheControl,
		}
		fingerprintValue := stripCachePositionKeys(toolValue)
		blocks = append(blocks, cacheablePromptBlock{
			value:  fingerprintValue,
			tokens: estimateApproxTokens(canonicalizeCacheValue(fingerprintValue)),
			ttl:    normalizePromptCacheTTL(extractPromptCacheTTL(toolValue)),
		})
	}

	appendSystemCacheBlocks(&blocks, req.System)
	for messageIndex, message := range req.Messages {
		appendMessageCacheBlocks(&blocks, messageIndex, message)
	}
	return blocks
}

func buildCachePreludeBlock(req *ClaudeRequest) cacheablePromptBlock {
	prelude := map[string]any{
		"kind":        "request_prelude",
		"model":       req.Model,
		"tool_choice": req.ToolChoice,
	}
	return cacheablePromptBlock{
		value:  prelude,
		tokens: estimateApproxTokens(canonicalizeCacheValue(prelude)),
	}
}

func appendSystemCacheBlocks(blocks *[]cacheablePromptBlock, system any) {
	switch value := system.(type) {
	case string:
		appendPromptCacheBlock(blocks, map[string]any{
			"kind":         "system",
			"system_index": 0,
			"block": map[string]any{
				"type": "text",
				"text": value,
			},
		}, false)
	case []any:
		for index, block := range value {
			appendPromptCacheBlock(blocks, map[string]any{
				"kind":         "system",
				"system_index": index,
				"block":        block,
			}, false)
		}
	case []string:
		for index, block := range value {
			appendPromptCacheBlock(blocks, map[string]any{
				"kind":         "system",
				"system_index": index,
				"block": map[string]any{
					"type": "text",
					"text": block,
				},
			}, false)
		}
	}
}

func appendMessageCacheBlocks(blocks *[]cacheablePromptBlock, messageIndex int, message ClaudeMessage) {
	switch content := message.Content.(type) {
	case string:
		appendPromptCacheBlock(blocks, map[string]any{
			"kind":          "message",
			"message_index": messageIndex,
			"role":          message.Role,
			"block_index":   0,
			"block": map[string]any{
				"type": "text",
				"text": content,
			},
		}, true)
	case []any:
		lastIndex := len(content) - 1
		for blockIndex, block := range content {
			appendPromptCacheBlock(blocks, map[string]any{
				"kind":          "message",
				"message_index": messageIndex,
				"role":          message.Role,
				"block_index":   blockIndex,
				"block":         block,
			}, blockIndex == lastIndex)
		}
	default:
		if content != nil {
			appendPromptCacheBlock(blocks, map[string]any{
				"kind":          "message",
				"message_index": messageIndex,
				"role":          message.Role,
				"block_index":   0,
				"block":         content,
			}, true)
		}
	}
}

func appendPromptCacheBlock(blocks *[]cacheablePromptBlock, wrapper map[string]any, isMessageEnd bool) {
	blockValue := wrapper["block"]
	if isAnthropicBillingHeaderBlock(blockValue) {
		return
	}

	fingerprintValue := stripCachePositionKeys(wrapper)
	canonical := canonicalizeCacheValue(fingerprintValue)
	*blocks = append(*blocks, cacheablePromptBlock{
		value:        fingerprintValue,
		tokens:       estimateApproxTokens(canonical),
		ttl:          normalizePromptCacheTTL(extractPromptCacheTTL(blockValue)),
		isMessageEnd: isMessageEnd,
	})
}

func stripCachePositionKeys(value map[string]any) map[string]any {
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		if isCachePositionKey(key) {
			continue
		}
		cloned[key] = item
	}
	return cloned
}

func isCachePositionKey(key string) bool {
	switch key {
	case "tool_index", "system_index", "message_index", "block_index":
		return true
	default:
		return false
	}
}

func isAnthropicBillingHeaderBlock(value any) bool {
	block, ok := cacheValueMap(value)
	if !ok {
		return false
	}
	if blockType, ok := block["type"].(string); ok && blockType != "" && blockType != "text" {
		return false
	}
	text, ok := block["text"].(string)
	if !ok {
		return false
	}
	trimmed := strings.TrimLeft(text, " \t\r\n")
	return strings.HasPrefix(strings.ToLower(trimmed), "x-anthropic-billing-header:")
}

func extractPromptCacheTTL(value any) time.Duration {
	block, ok := cacheValueMap(value)
	if !ok {
		return 0
	}

	cacheControlValue, hasWrapper := block["cache_control"]
	if hasWrapper {
		block, ok = cacheValueMap(cacheControlValue)
		if !ok {
			return 0
		}
	}
	cacheType, _ := block["type"].(string)
	if !strings.EqualFold(cacheType, "ephemeral") {
		return 0
	}
	if ttl, ok := parsePromptCacheTTLValue(block["ttl"]); ok {
		return ttl
	}
	return defaultPromptCacheTTL
}

func cacheValueMap(value any) (map[string]any, bool) {
	if value == nil {
		return nil, false
	}
	if block, ok := value.(map[string]any); ok {
		return block, true
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, false
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, false
	}
	return decoded, true
}

func parsePromptCacheTTLValue(value any) (time.Duration, bool) {
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(strings.ToLower(typed))
		if trimmed == "" {
			return 0, false
		}
		if duration, err := time.ParseDuration(trimmed); err == nil {
			return duration, true
		}
		if seconds, err := strconv.Atoi(trimmed); err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second, true
		}
	case float64:
		if typed > 0 {
			return time.Duration(typed * float64(time.Second)), true
		}
	case float32:
		if typed > 0 {
			return time.Duration(float64(typed) * float64(time.Second)), true
		}
	case int:
		if typed > 0 {
			return time.Duration(typed) * time.Second, true
		}
	case int64:
		if typed > 0 {
			return time.Duration(typed) * time.Second, true
		}
	case json.Number:
		if seconds, err := strconv.ParseFloat(string(typed), 64); err == nil && seconds > 0 {
			return time.Duration(seconds * float64(time.Second)), true
		}
	}
	return 0, false
}

func normalizePromptCacheTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return 0
	}
	if ttl > defaultPromptCacheTTL {
		return maximumPromptCacheTTL
	}
	return defaultPromptCacheTTL
}

func computePromptCacheTTLBreakdown(profile *PromptCacheProfile, startTokens, endTokens int) (int, int) {
	if profile == nil || len(profile.breakpoints) == 0 || endTokens <= startTokens {
		return 0, 0
	}

	cache5m := 0
	cache1h := 0
	previousBreakpoint := 0
	for _, breakpoint := range profile.breakpoints {
		currentBreakpoint := minCacheInt(breakpoint.cumulativeTokens, profile.totalInputTokens)
		segmentStart := maxCacheInt(previousBreakpoint, startTokens)
		segmentEnd := minCacheInt(currentBreakpoint, endTokens)
		if segmentEnd > segmentStart {
			delta := segmentEnd - segmentStart
			if breakpoint.ttl >= maximumPromptCacheTTL {
				cache1h += delta
			} else {
				cache5m += delta
			}
		}
		if currentBreakpoint > previousBreakpoint {
			previousBreakpoint = currentBreakpoint
		}
		if currentBreakpoint >= endTokens {
			break
		}
	}
	return cache5m, cache1h
}

func canonicalizeCacheValue(value any) string {
	var buffer bytes.Buffer
	writeCanonicalCacheJSON(&buffer, normalizeCacheJSONValue(value))
	return buffer.String()
}

func normalizeCacheJSONValue(value any) any {
	switch value.(type) {
	case nil, string, bool, float64, float32, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64, json.Number, []any, map[string]any:
		return value
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return value
	}
	return decoded
}

func writeCanonicalCacheJSON(buffer *bytes.Buffer, value any) {
	switch typed := value.(type) {
	case nil:
		_, _ = buffer.WriteString("null")
	case string:
		encoded, _ := json.Marshal(typed)
		_, _ = buffer.Write(encoded)
	case bool:
		if typed {
			_, _ = buffer.WriteString("true")
		} else {
			_, _ = buffer.WriteString("false")
		}
	case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
		encoded, _ := json.Marshal(typed)
		_, _ = buffer.Write(encoded)
	case []any:
		_ = buffer.WriteByte('[')
		for index, item := range typed {
			if index > 0 {
				_ = buffer.WriteByte(',')
			}
			writeCanonicalCacheJSON(buffer, normalizeCacheJSONValue(item))
		}
		_ = buffer.WriteByte(']')
	case map[string]any:
		_ = buffer.WriteByte('{')
		keys := make([]string, 0, len(typed))
		for key := range typed {
			if key == "cache_control" {
				continue
			}
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for index, key := range keys {
			if index > 0 {
				_ = buffer.WriteByte(',')
			}
			encoded, _ := json.Marshal(key)
			_, _ = buffer.Write(encoded)
			_ = buffer.WriteByte(':')
			writeCanonicalCacheJSON(buffer, normalizeCacheJSONValue(typed[key]))
		}
		_ = buffer.WriteByte('}')
	default:
		normalized := normalizeCacheJSONValue(typed)
		switch normalized.(type) {
		case []any, map[string]any:
			writeCanonicalCacheJSON(buffer, normalized)
		default:
			encoded, _ := json.Marshal(normalized)
			_, _ = buffer.Write(encoded)
		}
	}
}

func writeCacheHashChunk(hasher hash.Hash, chunk string) {
	_, _ = hasher.Write([]byte(strconv.Itoa(len(chunk))))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(chunk))
	_, _ = hasher.Write([]byte{0})
}

func minCacheInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxCacheInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
