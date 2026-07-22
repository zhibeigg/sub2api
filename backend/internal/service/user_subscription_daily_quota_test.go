package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type dailyResetTrackingUserSubRepo struct {
	userSubRepoNoop

	resetDailyCalled   bool
	resetWeeklyCalled  bool
	resetMonthlyCalled bool
	resetDailyAt       time.Time
	resetWeeklyAt      time.Time
	resetMonthlyAt     time.Time
}

func (r *dailyResetTrackingUserSubRepo) ResetDailyUsage(_ context.Context, _ int64, _ *time.Time, start time.Time) error {
	r.resetDailyCalled = true
	r.resetDailyAt = start
	return nil
}

func (r *dailyResetTrackingUserSubRepo) ResetWeeklyUsage(_ context.Context, _ int64, _ *time.Time, start time.Time) error {
	r.resetWeeklyCalled = true
	r.resetWeeklyAt = start
	return nil
}

func (r *dailyResetTrackingUserSubRepo) ResetMonthlyUsage(_ context.Context, _ int64, _ *time.Time, start time.Time) error {
	r.resetMonthlyCalled = true
	r.resetMonthlyAt = start
	return nil
}

func TestAssignOrExtendSubscription_ExpiredDailyCardStartsNewOneTimeQuota(t *testing.T) {
	groupRepo := &subscriptionGroupRepoStub{
		group: &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription},
	}
	subRepo := newSubscriptionUserSubRepoStub()
	oldStart := time.Now().AddDate(0, 0, -3)
	oldWindowStart := oldStart
	subRepo.seed(&UserSubscription{
		ID:                 100,
		UserID:             200,
		GroupID:            1,
		StartsAt:           oldStart,
		ExpiresAt:          oldStart.AddDate(0, 0, 1),
		Status:             SubscriptionStatusExpired,
		DailyWindowStart:   &oldWindowStart,
		WeeklyWindowStart:  &oldWindowStart,
		MonthlyWindowStart: &oldWindowStart,
		DailyUsageUSD:      10,
		WeeklyUsageUSD:     20,
		MonthlyUsageUSD:    30,
		Notes:              "old",
	})
	svc := NewSubscriptionService(groupRepo, subRepo, nil, nil, nil)

	renewed, reused, err := svc.AssignOrExtendSubscription(context.Background(), &AssignSubscriptionInput{
		UserID:       200,
		GroupID:      1,
		ValidityDays: 1,
		Notes:        "new",
	})

	require.NoError(t, err)
	require.True(t, reused)
	require.True(t, renewed.HasOneTimeDailyQuota(), "过期后重新购买 1 日卡仍应被识别为一次性日额度")
	require.Equal(t, SubscriptionStatusActive, renewed.Status)
	require.True(t, renewed.StartsAt.After(oldStart), "重新购买过期订阅时应重置当前周期 StartsAt")
	require.False(t, renewed.ExpiresAt.After(renewed.StartsAt.AddDate(0, 0, 1)))
	require.NotNil(t, renewed.DailyWindowStart)
	require.Equal(t, renewed.StartsAt, *renewed.DailyWindowStart)
	require.Equal(t, 0.0, renewed.DailyUsageUSD)
	require.Equal(t, 0.0, renewed.WeeklyUsageUSD)
	require.Equal(t, 0.0, renewed.MonthlyUsageUSD)
	require.Equal(t, "old\nnew", renewed.Notes)
}

func TestAssignOrExtendSubscription_ExpiredSubscriptionAppendsMatchingNotes(t *testing.T) {
	groupRepo := &subscriptionGroupRepoStub{
		group: &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription},
	}
	subRepo := newSubscriptionUserSubRepoStub()
	oldStart := time.Now().AddDate(0, 0, -3)
	subRepo.seed(&UserSubscription{
		ID:        101,
		UserID:    201,
		GroupID:   1,
		StartsAt:  oldStart,
		ExpiresAt: oldStart.AddDate(0, 0, 1),
		Status:    SubscriptionStatusExpired,
		Notes:     "same",
	})
	svc := NewSubscriptionService(groupRepo, subRepo, nil, nil, nil)

	renewed, reused, err := svc.AssignOrExtendSubscription(context.Background(), &AssignSubscriptionInput{
		UserID:       201,
		GroupID:      1,
		ValidityDays: 1,
		Notes:        "same",
	})

	require.NoError(t, err)
	require.True(t, reused)
	require.Equal(t, "same\nsame", renewed.Notes)
}

func TestUserSubscriptionNeedsDailyReset_DailyCardKeepsOneTimeQuota(t *testing.T) {
	start := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	dailyWindowStart := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	sub := &UserSubscription{
		StartsAt:         start,
		ExpiresAt:        start.Add(24 * time.Hour),
		DailyWindowStart: &dailyWindowStart,
		DailyUsageUSD:    10,
	}

	require.True(t, sub.HasOneTimeDailyQuota())
	require.False(t, sub.NeedsDailyResetAt(dailyWindowStart.Add(25*time.Hour)), "日卡应作为一次性配额，跨 0 点后不再刷新日额度")
}

func TestUserSubscriptionNeedsDailyReset_MultiDaySubscriptionStillRefreshes(t *testing.T) {
	start := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	dailyWindowStart := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	sub := &UserSubscription{
		StartsAt:         start,
		ExpiresAt:        start.AddDate(0, 0, 2),
		DailyWindowStart: &dailyWindowStart,
	}

	require.False(t, sub.HasOneTimeDailyQuota())
	require.True(t, sub.NeedsDailyResetAt(dailyWindowStart.Add(24*time.Hour)), "多日订阅仍应按 24 小时日窗口刷新")
}

func TestUserSubscriptionDailyResetTime_DailyCardReturnsExpiry(t *testing.T) {
	start := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	dailyWindowStart := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	expiresAt := start.Add(24 * time.Hour)
	sub := &UserSubscription{
		StartsAt:         start,
		ExpiresAt:        expiresAt,
		DailyWindowStart: &dailyWindowStart,
	}

	resetAt := sub.DailyResetTime()
	require.NotNil(t, resetAt)
	require.Equal(t, expiresAt, *resetAt, "日卡展示的日额度结束时间应为订阅过期时间")
}

func TestCheckAndResetWindows_DailyCardDoesNotResetDailyUsage(t *testing.T) {
	now := time.Now()
	startsAt := now.Add(-23 * time.Hour)
	dailyWindowStart := now.Add(-25 * time.Hour)
	repo := &dailyResetTrackingUserSubRepo{}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	sub := &UserSubscription{
		ID:               1,
		UserID:           10,
		GroupID:          20,
		StartsAt:         startsAt,
		ExpiresAt:        startsAt.Add(24 * time.Hour),
		DailyUsageUSD:    10,
		DailyWindowStart: &dailyWindowStart,
	}

	err := svc.CheckAndResetWindows(context.Background(), sub)

	require.NoError(t, err)
	require.False(t, repo.resetDailyCalled, "日卡作为一次性配额，过了 24 小时日窗口也不应重置 daily usage")
	require.Equal(t, 10.0, sub.DailyUsageUSD)
}

func TestCheckAndResetWindows_MultiDaySubscriptionStillResetsDailyUsage(t *testing.T) {
	now := time.Now()
	startsAt := now.Add(-48 * time.Hour)
	dailyWindowStart := now.Add(-25 * time.Hour)
	repo := &dailyResetTrackingUserSubRepo{}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	sub := &UserSubscription{
		ID:               1,
		UserID:           10,
		GroupID:          20,
		StartsAt:         startsAt,
		ExpiresAt:        startsAt.AddDate(0, 0, 2),
		DailyUsageUSD:    10,
		DailyWindowStart: &dailyWindowStart,
	}

	err := svc.CheckAndResetWindows(context.Background(), sub)

	require.NoError(t, err)
	require.True(t, repo.resetDailyCalled, "多日订阅仍应重置过期 daily window")
	require.Equal(t, 0.0, sub.DailyUsageUSD)
}

func TestValidateAndCheckLimits_DailyCardDoesNotAllowSecondQuotaAfterMidnight(t *testing.T) {
	start := time.Now().Add(-23 * time.Hour)
	dailyWindowStart := time.Now().Add(-25 * time.Hour)
	dailyLimit := 10.0
	sub := &UserSubscription{
		Status:           SubscriptionStatusActive,
		StartsAt:         start,
		ExpiresAt:        start.Add(24 * time.Hour),
		DailyWindowStart: &dailyWindowStart,
		DailyUsageUSD:    dailyLimit + 0.01,
	}
	group := &Group{
		SubscriptionType: SubscriptionTypeSubscription,
		DailyLimitUSD:    &dailyLimit,
	}
	svc := NewSubscriptionService(groupRepoNoop{}, userSubRepoNoop{}, nil, nil, nil)

	needsMaintenance, err := svc.ValidateAndCheckLimits(sub, group)

	require.False(t, needsMaintenance, "日卡跨过日窗口后不应触发 daily reset 维护")
	require.True(t, errors.Is(err, ErrDailyLimitExceeded))
	require.Equal(t, dailyLimit+0.01, sub.DailyUsageUSD, "热路径不应清零日卡已用额度")
}

func TestUserSubscriptionRollingWindowsUsePurchaseTimeAnchor(t *testing.T) {
	startsAt := time.Date(2026, 7, 1, 10, 30, 0, 0, time.UTC)
	dailyStart := startsAt.Add(20 * subscriptionDayDuration)
	weeklyStart := startsAt.Add(2 * subscriptionWeekDuration)
	monthlyStart := startsAt
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	sub := &UserSubscription{
		StartsAt:           startsAt,
		ExpiresAt:          startsAt.Add(60 * subscriptionDayDuration),
		DailyWindowStart:   &dailyStart,
		WeeklyWindowStart:  &weeklyStart,
		MonthlyWindowStart: &monthlyStart,
	}

	require.True(t, sub.NeedsDailyResetAt(now))
	require.True(t, sub.NeedsWeeklyResetAt(now))
	require.False(t, sub.NeedsMonthlyResetAt(now))
	require.Equal(t, time.Date(2026, 7, 22, 10, 30, 0, 0, time.UTC), sub.CurrentDailyWindowStartAt(now))
	require.Equal(t, time.Date(2026, 7, 22, 10, 30, 0, 0, time.UTC), sub.CurrentWeeklyWindowStartAt(now))
	require.Equal(t, startsAt, sub.CurrentMonthlyWindowStartAt(now))
	require.Equal(t, time.Date(2026, 7, 23, 10, 30, 0, 0, time.UTC), *sub.DailyResetTimeAt(now))
	require.Equal(t, time.Date(2026, 7, 29, 10, 30, 0, 0, time.UTC), *sub.WeeklyResetTimeAt(now))
	require.Equal(t, time.Date(2026, 7, 31, 10, 30, 0, 0, time.UTC), *sub.MonthlyResetTimeAt(now))
}

func TestUserSubscriptionLegacyMidnightWindowWaitsForPurchaseTimeBoundary(t *testing.T) {
	startsAt := time.Date(2026, 7, 1, 10, 30, 0, 0, time.UTC)
	legacyMidnightStart := time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC)
	sub := &UserSubscription{
		StartsAt:         startsAt,
		ExpiresAt:        startsAt.Add(60 * subscriptionDayDuration),
		DailyWindowStart: &legacyMidnightStart,
	}

	require.False(t, sub.NeedsDailyResetAt(time.Date(2026, 7, 22, 9, 0, 0, 0, time.UTC)))
	require.True(t, sub.NeedsDailyResetAt(time.Date(2026, 7, 22, 10, 30, 0, 0, time.UTC)))
}

func TestCheckAndResetWindowsAdvancesEachPeriodToAnchoredBoundary(t *testing.T) {
	startsAt := time.Date(2026, 5, 20, 10, 30, 0, 0, time.UTC)
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	dailyStart := startsAt
	weeklyStart := startsAt
	monthlyStart := startsAt
	repo := &dailyResetTrackingUserSubRepo{}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	sub := &UserSubscription{
		ID:                 1,
		UserID:             10,
		GroupID:            20,
		StartsAt:           startsAt,
		ExpiresAt:          now.Add(30 * subscriptionDayDuration),
		DailyWindowStart:   &dailyStart,
		WeeklyWindowStart:  &weeklyStart,
		MonthlyWindowStart: &monthlyStart,
		DailyUsageUSD:      1,
		WeeklyUsageUSD:     2,
		MonthlyUsageUSD:    3,
	}

	err := svc.checkAndResetWindowsAt(context.Background(), sub, now)

	require.NoError(t, err)
	require.True(t, repo.resetDailyCalled)
	require.True(t, repo.resetWeeklyCalled)
	require.True(t, repo.resetMonthlyCalled)
	require.Equal(t, time.Date(2026, 7, 22, 10, 30, 0, 0, time.UTC), repo.resetDailyAt)
	require.Equal(t, time.Date(2026, 7, 22, 10, 30, 0, 0, time.UTC), repo.resetWeeklyAt)
	require.Equal(t, time.Date(2026, 7, 19, 10, 30, 0, 0, time.UTC), repo.resetMonthlyAt)
	require.Zero(t, sub.DailyUsageUSD)
	require.Zero(t, sub.WeeklyUsageUSD)
	require.Zero(t, sub.MonthlyUsageUSD)
}
