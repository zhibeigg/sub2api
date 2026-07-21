package service

import (
	"math"
	"time"

	"github.com/shopspring/decimal"
)

type usageCapacityCandidate struct {
	scope             string
	estimatedRequests int64
	usedRequests      float64
	totalRequests     float64
	sampleRequests    int64
	averageCost       *float64
	resetAt           *time.Time
}

func estimateAccountUsageCapacity(account *Account, usage *UsageInfo) *AccountCapacitySnapshot {
	if account == nil || usage == nil {
		return localCapacityState(AccountCapacityModeUsageWindow, AccountCapacityStateUnknown, "usage_unavailable")
	}

	windows := []struct {
		scope    string
		progress *UsageProgress
	}{
		{scope: "five_hour", progress: usage.FiveHour},
		{scope: "seven_day", progress: usage.SevenDay},
		{scope: "seven_day_sonnet", progress: usage.SevenDaySonnet},
		{scope: "seven_day_fable", progress: usage.SevenDayFable},
		{scope: "gemini_shared_daily", progress: usage.GeminiSharedDaily},
		{scope: "gemini_pro_daily", progress: usage.GeminiProDaily},
		{scope: "gemini_flash_daily", progress: usage.GeminiFlashDaily},
		{scope: "gemini_shared_minute", progress: usage.GeminiSharedMinute},
		{scope: "gemini_pro_minute", progress: usage.GeminiProMinute},
		{scope: "gemini_flash_minute", progress: usage.GeminiFlashMinute},
	}

	hasUsageWindow := false
	candidates := make([]usageCapacityCandidate, 0, len(windows)+3)
	for _, window := range windows {
		if window.progress == nil {
			continue
		}
		hasUsageWindow = true
		if candidate, ok := usageProgressCapacityCandidate(window.scope, window.progress); ok {
			candidates = append(candidates, candidate)
		}
	}

	if usage.GrokRequestQuota != nil {
		hasUsageWindow = true
		if usage.GrokRequestQuota.Remaining != nil && *usage.GrokRequestQuota.Remaining >= 0 {
			remaining := *usage.GrokRequestQuota.Remaining
			total := float64(remaining)
			used := 0.0
			if usage.GrokRequestQuota.Limit != nil && *usage.GrokRequestQuota.Limit >= remaining {
				total = float64(*usage.GrokRequestQuota.Limit)
				used = total - float64(remaining)
			}
			candidates = append(candidates, usageCapacityCandidate{
				scope:             "grok_requests",
				estimatedRequests: remaining,
				usedRequests:      used,
				totalRequests:     total,
				resetAt:           parseCapacityResetAt(usage.GrokRequestQuota.ResetAt),
			})
		}
	}

	if usage.KiroUsageLimit != nil {
		hasUsageWindow = true
		limit := *usage.KiroUsageLimit
		used := 0.0
		if usage.KiroUsageCurrent != nil {
			used = *usage.KiroUsageCurrent
		}
		if limit >= 0 && used >= 0 {
			remaining := math.Max(limit-used, 0)
			candidates = append(candidates, usageCapacityCandidate{
				scope:             "kiro_subscription",
				estimatedRequests: decimal.NewFromFloat(remaining).Floor().IntPart(),
				usedRequests:      used,
				totalRequests:     limit,
				resetAt:           parseCapacityResetAt(usage.KiroNextResetDate),
			})
		}
	}

	if usage.CursorPlanUsage != nil && usage.CursorPlanUsage.TotalPercentUsed != nil && usage.CursorLocalUsage != nil {
		hasUsageWindow = true
		progress := &UsageProgress{
			Utilization: *usage.CursorPlanUsage.TotalPercentUsed,
			WindowStats: usage.CursorLocalUsage,
			ResetsAt:    usage.CursorPlanUsage.BillingCycleEnd,
		}
		if candidate, ok := usageProgressCapacityCandidate("cursor_billing_cycle", progress); ok {
			candidates = append(candidates, candidate)
		}
	}

	if len(candidates) > 0 {
		selected := candidates[0]
		for _, candidate := range candidates[1:] {
			if candidate.estimatedRequests < selected.estimatedRequests {
				selected = candidate
			}
		}
		fetchedAt := usageCapacityFetchedAt(usage)
		remaining := float64(selected.estimatedRequests)
		return &AccountCapacitySnapshot{
			Mode:                       AccountCapacityModeUsageWindow,
			State:                      AccountCapacityStateEstimated,
			Provider:                   AccountCapacityProviderLocal,
			Scope:                      selected.scope,
			Authoritative:              false,
			Remaining:                  &remaining,
			Total:                      capacityFloat64Ptr(selected.totalRequests),
			Used:                       capacityFloat64Ptr(selected.usedRequests),
			Unit:                       "requests",
			EstimatedRemainingRequests: cloneCapacityInt64Ptr(&selected.estimatedRequests),
			AverageCostPerRequest:      cloneCapacityFloat64Ptr(selected.averageCost),
			SampleRequests:             selected.sampleRequests,
			FetchedAt:                  &fetchedAt,
			ResetAt:                    cloneCapacityTimePtr(selected.resetAt),
		}
	}

	if hasUsageWindow {
		return localCapacityState(AccountCapacityModeUsageWindow, AccountCapacityStateUnknown, "insufficient_window_sample")
	}
	return estimateLocalQuotaCapacity(account, usage)
}

func usageProgressCapacityCandidate(scope string, progress *UsageProgress) (usageCapacityCandidate, bool) {
	if progress == nil {
		return usageCapacityCandidate{}, false
	}
	if progress.LimitRequests > 0 && progress.UsedRequests >= 0 {
		remaining := progress.LimitRequests - progress.UsedRequests
		if remaining < 0 {
			remaining = 0
		}
		candidate := usageCapacityCandidate{
			scope:             scope,
			estimatedRequests: remaining,
			usedRequests:      float64(progress.UsedRequests),
			totalRequests:     float64(progress.LimitRequests),
			sampleRequests:    progress.UsedRequests,
			resetAt:           cloneCapacityTimePtr(progress.ResetsAt),
		}
		if progress.WindowStats != nil && progress.WindowStats.Requests > 0 {
			candidate.sampleRequests = progress.WindowStats.Requests
			candidate.averageCost = averageAccountCost(progress.WindowStats)
		}
		return candidate, true
	}
	if progress.WindowStats == nil || progress.WindowStats.Requests <= 0 || progress.Utilization <= 0 {
		return usageCapacityCandidate{}, false
	}

	requests := progress.WindowStats.Requests
	remaining := int64(0)
	if progress.Utilization < 100 {
		remaining = decimal.NewFromInt(requests).
			Mul(decimal.NewFromFloat(100 - progress.Utilization)).
			Div(decimal.NewFromFloat(progress.Utilization)).
			Floor().
			IntPart()
		if remaining < 0 {
			remaining = 0
		}
	}
	return usageCapacityCandidate{
		scope:             scope,
		estimatedRequests: remaining,
		usedRequests:      float64(requests),
		totalRequests:     float64(requests + remaining),
		sampleRequests:    requests,
		averageCost:       averageAccountCost(progress.WindowStats),
		resetAt:           cloneCapacityTimePtr(progress.ResetsAt),
	}, true
}

func estimateLocalQuotaCapacity(account *Account, usage *UsageInfo) *AccountCapacitySnapshot {
	type quotaCandidate struct {
		scope     string
		remaining float64
		total     float64
		used      float64
	}
	candidates := make([]quotaCandidate, 0, 3)
	if limit := account.GetQuotaLimit(); limit > 0 {
		used := math.Max(account.GetQuotaUsed(), 0)
		candidates = append(candidates, quotaCandidate{scope: "local_quota_total", remaining: math.Max(limit-used, 0), total: limit, used: used})
	}
	if limit := account.GetQuotaDailyLimit(); limit > 0 {
		used := 0.0
		if !account.IsDailyQuotaPeriodExpired() {
			used = math.Max(account.GetQuotaDailyUsed(), 0)
		}
		candidates = append(candidates, quotaCandidate{scope: "local_quota_daily", remaining: math.Max(limit-used, 0), total: limit, used: used})
	}
	if limit := account.GetQuotaWeeklyLimit(); limit > 0 {
		used := 0.0
		if !account.IsWeeklyQuotaPeriodExpired() {
			used = math.Max(account.GetQuotaWeeklyUsed(), 0)
		}
		candidates = append(candidates, quotaCandidate{scope: "local_quota_weekly", remaining: math.Max(limit-used, 0), total: limit, used: used})
	}
	if len(candidates) == 0 {
		snapshot := localCapacityState(AccountCapacityModeLocalQuota, AccountCapacityStateUnlimited, "local_quota_unlimited")
		snapshot.Scope = "local_quota"
		snapshot.Unit = "USD"
		return snapshot
	}

	selected := candidates[0]
	for _, candidate := range candidates[1:] {
		if candidate.remaining < selected.remaining {
			selected = candidate
		}
	}
	fetchedAt := usageCapacityFetchedAt(usage)
	snapshot := &AccountCapacitySnapshot{
		Mode:          AccountCapacityModeLocalQuota,
		State:         AccountCapacityStateUnknown,
		Provider:      AccountCapacityProviderLocal,
		Scope:         selected.scope,
		Authoritative: false,
		Remaining:     capacityFloat64Ptr(selected.remaining),
		Total:         capacityFloat64Ptr(selected.total),
		Used:          capacityFloat64Ptr(selected.used),
		Unit:          "USD",
		FetchedAt:     &fetchedAt,
		MessageCode:   "insufficient_cost_sample",
	}

	if selected.remaining == 0 {
		zero := int64(0)
		snapshot.State = AccountCapacityStateEstimated
		snapshot.EstimatedRemainingRequests = &zero
		snapshot.MessageCode = ""
		return snapshot
	}

	stats := capacitySampleStats(usage)
	average := averageAccountCost(stats)
	if average == nil || *average <= 0 {
		return snapshot
	}
	estimated := decimal.NewFromFloat(selected.remaining).
		Div(decimal.NewFromFloat(*average)).
		Floor().
		IntPart()
	if estimated < 0 {
		estimated = 0
	}
	snapshot.State = AccountCapacityStateEstimated
	snapshot.EstimatedRemainingRequests = &estimated
	snapshot.AverageCostPerRequest = cloneCapacityFloat64Ptr(average)
	snapshot.SampleRequests = stats.Requests
	snapshot.MessageCode = ""
	return snapshot
}

func capacitySampleStats(usage *UsageInfo) *WindowStats {
	if usage == nil {
		return nil
	}
	windows := []*UsageProgress{
		usage.FiveHour,
		usage.SevenDay,
		usage.SevenDaySonnet,
		usage.SevenDayFable,
		usage.GeminiSharedDaily,
		usage.GeminiProDaily,
		usage.GeminiFlashDaily,
		usage.GeminiSharedMinute,
		usage.GeminiProMinute,
		usage.GeminiFlashMinute,
	}
	for _, window := range windows {
		if window != nil && averageAccountCost(window.WindowStats) != nil {
			return window.WindowStats
		}
	}
	for _, stats := range []*WindowStats{
		usage.LocalUsage,
		usage.CursorLocalUsage,
		usage.GrokLocalUsage24h,
		usage.GrokLocalUsage7d,
		usage.GrokLocalUsageMonthly,
		usage.GrokLocalUsage,
	} {
		if averageAccountCost(stats) != nil {
			return stats
		}
	}
	return nil
}

func averageAccountCost(stats *WindowStats) *float64 {
	if stats == nil || stats.Requests <= 0 || stats.Cost <= 0 || math.IsNaN(stats.Cost) || math.IsInf(stats.Cost, 0) {
		return nil
	}
	value := stats.Cost / float64(stats.Requests)
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return nil
	}
	return &value
}

func usageCapacityFetchedAt(usage *UsageInfo) time.Time {
	if usage != nil && usage.UpdatedAt != nil {
		return usage.UpdatedAt.UTC()
	}
	return time.Now().UTC()
}

func parseCapacityResetAt(raw string) *time.Time {
	if raw == "" {
		return nil
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		parsed = parsed.UTC()
		return &parsed
	}
	if parsed, err := time.Parse("2006-01-02", raw); err == nil {
		parsed = parsed.UTC()
		return &parsed
	}
	return nil
}

func localCapacityState(mode, state, messageCode string) *AccountCapacitySnapshot {
	return &AccountCapacitySnapshot{
		Mode:          mode,
		State:         state,
		Provider:      AccountCapacityProviderLocal,
		Authoritative: false,
		MessageCode:   messageCode,
	}
}
