package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	cursorpkg "github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

type cursorUsageStatsRepo struct {
	UsageLogRepository
	stats *usagestats.AccountStats
}

func (r cursorUsageStatsRepo) GetAccountTodayStats(context.Context, int64) (*usagestats.AccountStats, error) {
	return r.stats, nil
}

type cursorUsageProberFunc func(context.Context, *Account, string, string) (string, error)

func (f cursorUsageProberFunc) Probe(ctx context.Context, account *Account, model, protocol string) (string, error) {
	return f(ctx, account, model, protocol)
}

type cursorDashboardFetcherFunc func(context.Context, *Account) (*CursorDashboardUsageResult, error)

func (f cursorDashboardFetcherFunc) FetchDashboardUsage(ctx context.Context, account *Account) (*CursorDashboardUsageResult, error) {
	return f(ctx, account)
}

type cursorDashboardAccountRepo struct {
	stubOpenAIAccountRepo
	updatedCredentials map[string]any
	updatedExtra       map[string]any
}

func (r *cursorDashboardAccountRepo) UpdateCredentials(_ context.Context, _ int64, credentials map[string]any) error {
	r.updatedCredentials = shallowCopyMap(credentials)
	return nil
}

func (r *cursorDashboardAccountRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.updatedExtra = shallowCopyMap(updates)
	return nil
}

func TestAccountUsageServiceCursorLocalUsageAndForcedProbe(t *testing.T) {
	t.Parallel()

	account := Account{ID: 6101, Platform: PlatformCursor, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "cursor-key"}}
	repo := &stubOpenAIAccountRepo{accounts: []Account{account}}
	probeCalls := 0
	svc := &AccountUsageService{
		accountRepo: repo,
		usageLogRepo: cursorUsageStatsRepo{stats: &usagestats.AccountStats{
			Requests: 4, InputTokens: 10, OutputTokens: 20, CacheWriteTokens: 30, CacheReadTokens: 40, Tokens: 100, Cost: 1.25, UserCost: 1.5,
		}},
		cursorUsageProber: cursorUsageProberFunc(func(_ context.Context, got *Account, _, _ string) (string, error) {
			probeCalls++
			if got.ID != account.ID {
				t.Fatalf("probe account ID = %d, want %d", got.ID, account.ID)
			}
			return "cursor@example.com", nil
		}),
	}

	passive, err := svc.GetUsage(context.Background(), account.ID)
	if err != nil {
		t.Fatalf("GetUsage() error = %v", err)
	}
	if probeCalls != 0 {
		t.Fatalf("passive Cursor usage unexpectedly probed upstream %d time(s)", probeCalls)
	}
	if passive.Source != "local" || passive.CursorProbeState != "configured" {
		t.Fatalf("passive state = source:%q probe:%q", passive.Source, passive.CursorProbeState)
	}
	if passive.CursorLocalUsage == nil || passive.CursorLocalUsage.CacheWriteTokens != 30 || passive.CursorLocalUsage.CacheReadTokens != 40 {
		t.Fatalf("Cursor local usage = %#v", passive.CursorLocalUsage)
	}

	active, err := svc.GetUsage(context.Background(), account.ID, true)
	if err != nil {
		t.Fatalf("GetUsage(force) error = %v", err)
	}
	if probeCalls != 1 {
		t.Fatalf("forced Cursor usage probe calls = %d, want 1", probeCalls)
	}
	if active.Source != "active" || active.CursorProbeState != "verified" || active.CursorCheckedAt == "" {
		t.Fatalf("active state = source:%q probe:%q checked:%q", active.Source, active.CursorProbeState, active.CursorCheckedAt)
	}
}

func TestAccountUsageServiceCursorProbeFailureIsDegraded(t *testing.T) {
	t.Parallel()

	account := Account{ID: 6102, Platform: PlatformCursor, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "invalid"}}
	svc := &AccountUsageService{
		accountRepo: &stubOpenAIAccountRepo{accounts: []Account{account}},
		cursorUsageProber: cursorUsageProberFunc(func(context.Context, *Account, string, string) (string, error) {
			return "", errors.New("Cursor API request failed (HTTP 401): unauthorized")
		}),
	}

	usage, err := svc.GetUsage(context.Background(), account.ID, true)
	if err != nil {
		t.Fatalf("GetUsage(force) error = %v", err)
	}
	if usage.CursorProbeState != "error" || usage.ErrorCode != errorCodeUnauthenticated || !usage.NeedsReauth {
		t.Fatalf("degraded Cursor probe result = %#v", usage)
	}
}

func TestAccountUsageServiceCursorDashboardCachedAndActiveRefresh(t *testing.T) {
	t.Parallel()

	account := Account{
		ID:       6103,
		Platform: PlatformCursor,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":                 "cursor-key",
			"dashboard_access_token":  "old-access",
			"dashboard_refresh_token": "old-refresh",
		},
		Extra: map[string]any{
			"cursor_dashboard_enabled":                  true,
			"cursor_dashboard_total_percent_used":       1.0,
			"cursor_dashboard_first_party_percent_used": 0.0,
			"cursor_dashboard_api_percent_used":         1.0,
			"cursor_dashboard_updated_at":               "2026-08-01T00:00:00Z",
		},
	}
	repo := &cursorDashboardAccountRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}}}
	total, firstParty, api, limit, spend, remaining := 2.0, 1.0, 1.0, 2000.0, 40.0, 1960.0
	enabled := true
	svc := &AccountUsageService{
		accountRepo: repo,
		cursorUsageProber: cursorUsageProberFunc(func(context.Context, *Account, string, string) (string, error) {
			return "cursor@example.com", nil
		}),
		cursorDashboardFetcher: cursorDashboardFetcherFunc(func(context.Context, *Account) (*CursorDashboardUsageResult, error) {
			return &CursorDashboardUsageResult{
				Usage: &cursorpkg.DashboardUsage{
					Enabled:           &enabled,
					BillingCycleStart: 1785542400000,
					BillingCycleEnd:   1788220800000,
					PlanUsage: &cursorpkg.DashboardPlanUsage{
						TotalPercentUsed: &total,
						AutoPercentUsed:  &firstParty,
						APIPercentUsed:   &api,
						Limit:            &limit,
						TotalSpend:       &spend,
						Remaining:        &remaining,
					},
				},
				RefreshedAccessToken:  "new-access",
				RefreshedRefreshToken: "new-refresh",
			}, nil
		}),
	}

	passive, err := svc.GetUsage(context.Background(), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if passive.CursorDashboardState != "cached" || passive.CursorPlanUsage == nil || passive.CursorPlanUsage.TotalPercentUsed == nil || *passive.CursorPlanUsage.TotalPercentUsed != 1 {
		t.Fatalf("passive dashboard usage = %#v", passive)
	}

	active, err := svc.GetUsage(context.Background(), account.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if active.CursorDashboardState != "verified" || active.CursorPlanUsage == nil || active.CursorPlanUsage.TotalPercentUsed == nil || *active.CursorPlanUsage.TotalPercentUsed != 2 {
		t.Fatalf("active dashboard usage = %#v", active)
	}
	if repo.updatedCredentials["dashboard_access_token"] != "new-access" || repo.updatedCredentials["dashboard_refresh_token"] != "new-refresh" {
		t.Fatalf("updated credentials = %#v", repo.updatedCredentials)
	}
	if repo.updatedExtra["cursor_dashboard_total_percent_used"] != 2.0 || repo.updatedExtra["cursor_dashboard_limit_cents"] != 2000.0 {
		t.Fatalf("updated extra = %#v", repo.updatedExtra)
	}
}

func TestCursorPlanUsageSnapshotUpdatesClearsMissingFields(t *testing.T) {
	t.Parallel()
	updatedAt := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	total := 1.0
	updates := cursorPlanUsageSnapshotUpdates(&CursorPlanUsageInfo{
		Enabled:          true,
		TotalPercentUsed: &total,
		UpdatedAt:        &updatedAt,
	})
	if updates["cursor_dashboard_total_percent_used"] != 1.0 {
		t.Fatalf("total update = %#v", updates)
	}
	for _, key := range []string{
		"cursor_dashboard_first_party_percent_used",
		"cursor_dashboard_api_percent_used",
		"cursor_dashboard_limit_cents",
		"cursor_dashboard_total_spend_cents",
		"cursor_dashboard_remaining_cents",
		"cursor_dashboard_billing_cycle_start",
		"cursor_dashboard_billing_cycle_end",
	} {
		value, ok := updates[key]
		if !ok || value != nil {
			t.Fatalf("%s should be explicitly cleared, updates = %#v", key, updates)
		}
	}
}

func TestAccountUsageServiceCursorDashboardFailureKeepsStaleSnapshot(t *testing.T) {
	t.Parallel()

	account := Account{
		ID:          6104,
		Platform:    PlatformCursor,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "cursor-key", "dashboard_access_token": "expired"},
		Extra: map[string]any{
			"cursor_dashboard_enabled":            true,
			"cursor_dashboard_total_percent_used": 3.0,
			"cursor_dashboard_updated_at":         "2026-08-01T00:00:00Z",
		},
	}
	svc := &AccountUsageService{
		accountRepo: &stubOpenAIAccountRepo{accounts: []Account{account}},
		cursorUsageProber: cursorUsageProberFunc(func(context.Context, *Account, string, string) (string, error) {
			return "cursor@example.com", nil
		}),
		cursorDashboardFetcher: cursorDashboardFetcherFunc(func(context.Context, *Account) (*CursorDashboardUsageResult, error) {
			return nil, errors.New("dashboard unavailable")
		}),
	}
	usage, err := svc.GetUsage(context.Background(), account.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if usage.CursorDashboardState != "stale" || usage.CursorPlanUsage == nil || usage.CursorDashboardMessage != "dashboard unavailable" {
		t.Fatalf("stale dashboard usage = %#v", usage)
	}
}

type accountUsageCodexProbeRepo struct {
	stubOpenAIAccountRepo
	updateExtraCh chan map[string]any
	rateLimitCh   chan time.Time
}

func (r *accountUsageCodexProbeRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	if r.updateExtraCh != nil {
		copied := make(map[string]any, len(updates))
		for k, v := range updates {
			copied[k] = v
		}
		r.updateExtraCh <- copied
	}
	return nil
}

func (r *accountUsageCodexProbeRepo) SetRateLimited(_ context.Context, _ int64, resetAt time.Time) error {
	if r.rateLimitCh != nil {
		r.rateLimitCh <- resetAt
	}
	return nil
}

func TestShouldRefreshOpenAICodexSnapshot(t *testing.T) {
	t.Parallel()

	rateLimitedUntil := time.Now().Add(5 * time.Minute)
	now := time.Now()
	usage := &UsageInfo{
		FiveHour: &UsageProgress{Utilization: 0},
		SevenDay: &UsageProgress{Utilization: 0},
	}

	if !shouldRefreshOpenAICodexSnapshot(&Account{RateLimitResetAt: &rateLimitedUntil}, usage, now) {
		t.Fatal("expected rate-limited account to force codex snapshot refresh")
	}

	if shouldRefreshOpenAICodexSnapshot(&Account{}, usage, now) {
		t.Fatal("expected complete non-rate-limited usage to skip codex snapshot refresh")
	}

	if !shouldRefreshOpenAICodexSnapshot(&Account{}, &UsageInfo{FiveHour: nil, SevenDay: &UsageProgress{}}, now) {
		t.Fatal("expected missing 5h snapshot to require refresh")
	}

	staleAt := now.Add(-(openAIProbeCacheTTL + time.Minute)).Format(time.RFC3339)
	if !shouldRefreshOpenAICodexSnapshot(&Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"openai_oauth_responses_websockets_v2_enabled": true,
			"codex_usage_updated_at":                       staleAt,
		},
	}, usage, now) {
		t.Fatal("expected stale ws snapshot to trigger refresh")
	}
}

// TestShouldRefreshOpenAICodexSnapshot_SparkShadowIgnoresWSv2 外审第9轮 P1:spark 影子用量走
// QueryUsage(/wham/usage,与 WSv2 无关),staleness 不得被 WSv2 门控,否则首刷后窗口永久冻结。
func TestShouldRefreshOpenAICodexSnapshot_SparkShadowIgnoresWSv2(t *testing.T) {
	t.Parallel()

	now := time.Now()
	usage := &UsageInfo{
		FiveHour: &UsageProgress{Utilization: 0},
		SevenDay: &UsageProgress{Utilization: 0},
	}
	staleAt := now.Add(-(openAIProbeCacheTTL + time.Minute)).Format(time.RFC3339)
	freshAt := now.Add(-time.Minute).Format(time.RFC3339)
	parentID := int64(7001)

	// 影子无 WSv2,但首刷后窗口已存在;过期 codex_usage_updated_at 必须触发再刷新。
	shadowStale := &Account{
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		ParentAccountID: &parentID,
		QuotaDimension:  QuotaDimensionSpark,
		Extra:           map[string]any{"codex_usage_updated_at": staleAt},
	}
	if !shouldRefreshOpenAICodexSnapshot(shadowStale, usage, now) {
		t.Fatal("expected stale spark shadow (no WSv2) to trigger refresh")
	}

	// 影子时间戳仍新鲜→不刷(TTL 生效)。
	shadowFresh := &Account{
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		ParentAccountID: &parentID,
		QuotaDimension:  QuotaDimensionSpark,
		Extra:           map[string]any{"codex_usage_updated_at": freshAt},
	}
	if shouldRefreshOpenAICodexSnapshot(shadowFresh, usage, now) {
		t.Fatal("expected fresh spark shadow to skip refresh (TTL not elapsed)")
	}

	// 反向对照:普通账号无 WSv2 + 过期时间戳→仍不刷(WSv2 门控普通账号的 probe 刷新)。
	normalNoWS := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"codex_usage_updated_at": staleAt},
	}
	if shouldRefreshOpenAICodexSnapshot(normalNoWS, usage, now) {
		t.Fatal("expected non-WSv2 normal account to skip codex probe refresh")
	}
}

func TestExtractOpenAICodexProbeUpdatesAccepts429WithCodexHeaders(t *testing.T) {
	t.Parallel()

	headers := make(http.Header)
	headers.Set("x-codex-primary-used-percent", "100")
	headers.Set("x-codex-primary-reset-after-seconds", "604800")
	headers.Set("x-codex-primary-window-minutes", "10080")
	headers.Set("x-codex-secondary-used-percent", "100")
	headers.Set("x-codex-secondary-reset-after-seconds", "18000")
	headers.Set("x-codex-secondary-window-minutes", "300")

	updates, err := extractOpenAICodexProbeUpdates(&http.Response{StatusCode: http.StatusTooManyRequests, Header: headers})
	if err != nil {
		t.Fatalf("extractOpenAICodexProbeUpdates() error = %v", err)
	}
	if len(updates) == 0 {
		t.Fatal("expected codex probe updates from 429 headers")
	}
	if got := updates["codex_5h_used_percent"]; got != 100.0 {
		t.Fatalf("codex_5h_used_percent = %v, want 100", got)
	}
	if got := updates["codex_7d_used_percent"]; got != 100.0 {
		t.Fatalf("codex_7d_used_percent = %v, want 100", got)
	}
}

func TestAccountUsageService_PersistOpenAICodexProbeSnapshotOnlyUpdatesExtra(t *testing.T) {
	t.Parallel()

	repo := &accountUsageCodexProbeRepo{
		updateExtraCh: make(chan map[string]any, 1),
		rateLimitCh:   make(chan time.Time, 1),
	}
	svc := &AccountUsageService{accountRepo: repo}
	svc.persistOpenAICodexProbeSnapshot(321, map[string]any{
		"codex_7d_used_percent": 100.0,
		"codex_7d_reset_at":     time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second).Format(time.RFC3339),
	})

	select {
	case updates := <-repo.updateExtraCh:
		if got := updates["codex_7d_used_percent"]; got != 100.0 {
			t.Fatalf("codex_7d_used_percent = %v, want 100", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("等待 codex 探测快照写入 extra 超时")
	}

	select {
	case got := <-repo.rateLimitCh:
		t.Fatalf("不应将探测快照写入运行时限流状态: %v", got)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestAccountUsageService_GetOpenAIUsage_DoesNotPromoteCodexExtraToRateLimit(t *testing.T) {
	t.Parallel()

	resetAt := time.Now().Add(6 * 24 * time.Hour).UTC().Truncate(time.Second)
	repo := &accountUsageCodexProbeRepo{
		rateLimitCh: make(chan time.Time, 1),
	}
	svc := &AccountUsageService{accountRepo: repo}
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"codex_5h_used_percent": 1.0,
			"codex_5h_reset_at":     time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second).Format(time.RFC3339),
			"codex_7d_used_percent": 100.0,
			"codex_7d_reset_at":     resetAt.Format(time.RFC3339),
		},
	}

	usage, err := svc.getOpenAIUsage(context.Background(), account, false)
	if err != nil {
		t.Fatalf("getOpenAIUsage() error = %v", err)
	}
	if usage.SevenDay == nil || usage.SevenDay.Utilization != 100.0 {
		t.Fatalf("预期 7 天用量仍然可见，实际为 %#v", usage.SevenDay)
	}
	if account.RateLimitResetAt != nil {
		t.Fatalf("不应让已耗尽的 codex extra 改写运行时限流状态: %v", account.RateLimitResetAt)
	}
	select {
	case got := <-repo.rateLimitCh:
		t.Fatalf("不应将已耗尽的 codex extra 持久化为运行时限流状态: %v", got)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestBuildCodexUsageProgressFromExtra_ZerosExpiredWindow(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 16, 12, 0, 0, 0, time.UTC)

	t.Run("expired 5h window zeroes utilization", func(t *testing.T) {
		extra := map[string]any{
			"codex_5h_used_percent": 42.0,
			"codex_5h_reset_at":     "2026-03-16T10:00:00Z", // 2h ago
		}
		progress := buildCodexUsageProgressFromExtra(extra, "5h", now)
		if progress == nil {
			t.Fatal("expected non-nil progress")
		}
		if progress.Utilization != 0 {
			t.Fatalf("expected Utilization=0 for expired window, got %v", progress.Utilization)
		}
		if progress.RemainingSeconds != 0 {
			t.Fatalf("expected RemainingSeconds=0, got %v", progress.RemainingSeconds)
		}
	})

	t.Run("active 5h window keeps utilization", func(t *testing.T) {
		resetAt := now.Add(2 * time.Hour).Format(time.RFC3339)
		extra := map[string]any{
			"codex_5h_used_percent": 42.0,
			"codex_5h_reset_at":     resetAt,
		}
		progress := buildCodexUsageProgressFromExtra(extra, "5h", now)
		if progress == nil {
			t.Fatal("expected non-nil progress")
		}
		if progress.Utilization != 42.0 {
			t.Fatalf("expected Utilization=42, got %v", progress.Utilization)
		}
	})

	t.Run("expired 7d window zeroes utilization", func(t *testing.T) {
		extra := map[string]any{
			"codex_7d_used_percent": 88.0,
			"codex_7d_reset_at":     "2026-03-15T00:00:00Z", // yesterday
		}
		progress := buildCodexUsageProgressFromExtra(extra, "7d", now)
		if progress == nil {
			t.Fatal("expected non-nil progress")
		}
		if progress.Utilization != 0 {
			t.Fatalf("expected Utilization=0 for expired 7d window, got %v", progress.Utilization)
		}
	})
}
