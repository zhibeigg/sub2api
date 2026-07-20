package opencode

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const WorkspaceServerFunctionID = "def39973159c7f0483d8793a822b8dbb10d067e12c65455fcb4608459ba0234f"

const QuotaUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36"

var quotaWhitespaceRE = regexp.MustCompile(`\s+`)

type QuotaWindow struct {
	Status         string     `json:"status"`
	UsagePercent   int        `json:"usage_percent"`
	ResetInSeconds int        `json:"reset_in_seconds"`
	ResetAt        *time.Time `json:"reset_at,omitempty"`
}

type QuotaData struct {
	Rolling     QuotaWindow  `json:"rolling"`
	Weekly      QuotaWindow  `json:"weekly"`
	Monthly     *QuotaWindow `json:"monthly,omitempty"`
	WorkspaceID string       `json:"workspace_id,omitempty"`
	FetchedAt   time.Time    `json:"fetched_at"`
}

type quotaPage struct {
	Rolling *quotaWindowData `json:"rollingUsage"`
	Weekly  *quotaWindowData `json:"weeklyUsage"`
	Monthly *quotaWindowData `json:"monthlyUsage"`
}

type quotaWindowData struct {
	Status       *string  `json:"status"`
	UsagePercent *float64 `json:"usagePercent"`
	UsedPercent  *float64 `json:"usedPercent"`
	ResetInSec   *float64 `json:"resetInSec"`
}

func SanitizeQuotaCookie(raw string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return ""
	}
	text = regexp.MustCompile(`(?i)^cookie\s*:\s*`).ReplaceAllString(text, "")
	parts := strings.Split(text, ";")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			cleaned = append(cleaned, part)
		}
	}
	if len(cleaned) == 0 {
		return ""
	}
	cookie := strings.Join(cleaned, "; ")
	if !strings.Contains(cookie, "=") {
		return "auth=" + cookie
	}
	return cookie
}

func ParseWorkspaceIDs(text string) []string {
	seen := make(map[string]struct{})
	ids := make([]string, 0)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if !strings.HasPrefix(value, "wrk_") {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		ids = append(ids, value)
	}

	re := regexp.MustCompile(`id\s*[:=]\s*"(wrk_[^"]+)"`)
	for _, match := range re.FindAllStringSubmatch(text, -1) {
		add(match[1])
	}
	if len(ids) > 0 {
		return ids
	}

	var document any
	if err := json.Unmarshal([]byte(text), &document); err != nil {
		return nil
	}
	var walk func(any)
	walk = func(value any) {
		switch typed := value.(type) {
		case string:
			add(typed)
		case []any:
			for _, item := range typed {
				walk(item)
			}
		case map[string]any:
			for _, item := range typed {
				walk(item)
			}
		}
	}
	walk(document)
	return ids
}

func LooksSignedOut(text string) bool {
	lower := strings.ToLower(text)
	for _, keyword := range []string{"login", "sign in", "auth/authorize", "not associated with an account", `actor of type "public"`} {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}

// LooksQuotaUnavailable identifies an authenticated workspace page that
// explicitly has no OpenCode Go entitlement/quota object. Keep this separate
// from parser failures so an upstream markup change still fails closed.
func LooksQuotaUnavailable(text string) bool {
	lower := strings.ToLower(text)
	if strings.Contains(lower, "rollingusage") || strings.Contains(lower, "weeklyusage") {
		return false
	}
	compact := quotaWhitespaceRE.ReplaceAllString(lower, "")
	compact = strings.NewReplacer(`"`, "", `'`, "").Replace(compact)
	for _, marker := range []string{
		"monthlylimit:null",
		"monthlyusage:null",
		"subscription:null",
		"lite:null",
	} {
		if !strings.Contains(compact, marker) {
			return false
		}
	}
	return true
}

func ParseQuotaPage(text, workspaceID string, now time.Time) (*QuotaData, error) {
	if now.IsZero() {
		now = time.Now()
	}
	if parsed, err := parseQuotaJSON(text, workspaceID, now); err == nil {
		return parsed, nil
	}
	return parseQuotaRegex(text, workspaceID, now)
}

func parseQuotaJSON(text, workspaceID string, now time.Time) (*QuotaData, error) {
	var page quotaPage
	if err := json.Unmarshal([]byte(text), &page); err != nil {
		return nil, err
	}
	if page.Rolling == nil || page.Weekly == nil {
		return nil, fmt.Errorf("quota response is missing rolling or weekly usage")
	}
	result := &QuotaData{
		Rolling:     buildQuotaWindow(quotaStatus(page.Rolling), quotaPercent(page.Rolling), quotaResetSeconds(page.Rolling), now),
		Weekly:      buildQuotaWindow(quotaStatus(page.Weekly), quotaPercent(page.Weekly), quotaResetSeconds(page.Weekly), now),
		WorkspaceID: workspaceID,
		FetchedAt:   now.UTC(),
	}
	if page.Monthly != nil {
		monthly := buildQuotaWindow(quotaStatus(page.Monthly), quotaPercent(page.Monthly), quotaResetSeconds(page.Monthly), now)
		if monthly.Status != "unlimited" {
			result.Monthly = &monthly
		}
	}
	return result, nil
}

func parseQuotaRegex(text, workspaceID string, now time.Time) (*QuotaData, error) {
	extract := func(key string) (int, int, bool) {
		percentRE := regexp.MustCompile(key + `[^}]*?(?:usagePercent|usedPercent)"?\s*[:=]\s*([0-9]+(?:\.[0-9]+)?)`)
		percentMatch := percentRE.FindStringSubmatch(text)
		if len(percentMatch) < 2 {
			return 0, 0, false
		}
		percentValue, _ := strconv.ParseFloat(percentMatch[1], 64)
		resetRE := regexp.MustCompile(key + `[^}]*?resetInSec"?\s*[:=]\s*([0-9]+(?:\.[0-9]+)?)`)
		resetMatch := resetRE.FindStringSubmatch(text)
		resetSeconds := 0
		if len(resetMatch) >= 2 {
			resetValue, _ := strconv.ParseFloat(resetMatch[1], 64)
			resetSeconds = int(resetValue)
		}
		return clampQuotaPercent(int(percentValue)), resetSeconds, true
	}

	rollingPercent, rollingReset, rollingOK := extract("rollingUsage")
	weeklyPercent, weeklyReset, weeklyOK := extract("weeklyUsage")
	if !rollingOK || !weeklyOK {
		return nil, fmt.Errorf("failed to parse OpenCode Go quota response")
	}
	result := &QuotaData{
		Rolling:     buildQuotaWindow("active", rollingPercent, rollingReset, now),
		Weekly:      buildQuotaWindow("active", weeklyPercent, weeklyReset, now),
		WorkspaceID: workspaceID,
		FetchedAt:   now.UTC(),
	}
	if monthlyPercent, monthlyReset, ok := extract("monthlyUsage"); ok {
		monthly := buildQuotaWindow("active", monthlyPercent, monthlyReset, now)
		result.Monthly = &monthly
	}
	return result, nil
}

func quotaStatus(window *quotaWindowData) string {
	if window != nil && window.Status != nil {
		return strings.ToLower(strings.TrimSpace(*window.Status))
	}
	return "active"
}

func quotaPercent(window *quotaWindowData) int {
	if window == nil {
		return 0
	}
	if window.UsagePercent != nil {
		return clampQuotaPercent(int(*window.UsagePercent))
	}
	if window.UsedPercent != nil {
		return clampQuotaPercent(int(*window.UsedPercent))
	}
	return 0
}

func quotaResetSeconds(window *quotaWindowData) int {
	if window == nil || window.ResetInSec == nil || *window.ResetInSec <= 0 {
		return 0
	}
	return int(*window.ResetInSec)
}

func buildQuotaWindow(status string, percent, resetSeconds int, now time.Time) QuotaWindow {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		status = "active"
	}
	window := QuotaWindow{
		Status:         status,
		UsagePercent:   clampQuotaPercent(percent),
		ResetInSeconds: max(resetSeconds, 0),
	}
	if resetSeconds > 0 {
		resetAt := now.Add(time.Duration(resetSeconds) * time.Second).UTC()
		window.ResetAt = &resetAt
	}
	return window
}

func clampQuotaPercent(value int) int {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}
