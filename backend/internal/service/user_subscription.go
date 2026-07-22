package service

import "time"

const (
	subscriptionDayDuration   = 24 * time.Hour
	subscriptionWeekDuration  = 7 * subscriptionDayDuration
	subscriptionMonthDuration = 30 * subscriptionDayDuration
)

type SubscriptionWindowStarts struct {
	Daily   time.Time
	Weekly  time.Time
	Monthly time.Time
}

type UserSubscription struct {
	ID           int64
	UserID       int64
	GroupID      int64
	SourcePlanID *int64
	GroupIDs     []int64

	QuotaSnapshotted bool
	DailyLimitUSD    *float64
	WeeklyLimitUSD   *float64
	MonthlyLimitUSD  *float64
	ConcurrencyLimit *int

	StartsAt  time.Time
	ExpiresAt time.Time
	Status    string

	DailyWindowStart   *time.Time
	WeeklyWindowStart  *time.Time
	MonthlyWindowStart *time.Time

	DailyUsageUSD   float64
	WeeklyUsageUSD  float64
	MonthlyUsageUSD float64

	AssignedBy *int64
	AssignedAt time.Time
	Notes      string

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time

	User           *User
	Group          *Group
	Groups         []*Group
	AssignedByUser *User
}

func (s *UserSubscription) IsActive() bool {
	return s.Status == SubscriptionStatusActive && time.Now().Before(s.ExpiresAt)
}

func (s *UserSubscription) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

func (s *UserSubscription) DaysRemaining() int {
	return s.daysRemainingAt(time.Now())
}

func (s *UserSubscription) daysRemainingAt(now time.Time) int {
	remaining := s.ExpiresAt.Sub(now)
	if remaining <= 0 {
		return 0
	}

	days := int(remaining / subscriptionDayDuration)
	if remaining%subscriptionDayDuration != 0 {
		days++
	}
	return days
}

func (s *UserSubscription) IsWindowActivated() bool {
	return s.DailyWindowStart != nil || s.WeeklyWindowStart != nil || s.MonthlyWindowStart != nil
}

func (s *UserSubscription) HasOneTimeDailyQuota() bool {
	if s == nil || s.StartsAt.IsZero() || s.ExpiresAt.IsZero() {
		return false
	}
	return !s.ExpiresAt.After(s.StartsAt.AddDate(0, 0, 1))
}

func rollingSubscriptionWindowStart(anchor, now time.Time, period time.Duration) time.Time {
	if anchor.IsZero() {
		return now
	}
	if now.Before(anchor) {
		return anchor
	}
	return anchor.Add((now.Sub(anchor) / period) * period)
}

func (s *UserSubscription) windowAnchor(fallback *time.Time, now time.Time) time.Time {
	if s != nil && !s.StartsAt.IsZero() {
		return s.StartsAt
	}
	if fallback != nil {
		return *fallback
	}
	return now
}

func (s *UserSubscription) currentWindowStartAt(now time.Time, period time.Duration, fallback *time.Time) time.Time {
	return rollingSubscriptionWindowStart(s.windowAnchor(fallback, now), now, period)
}

func (s *UserSubscription) CurrentDailyWindowStartAt(now time.Time) time.Time {
	return s.currentWindowStartAt(now, subscriptionDayDuration, s.DailyWindowStart)
}

func (s *UserSubscription) CurrentWeeklyWindowStartAt(now time.Time) time.Time {
	return s.currentWindowStartAt(now, subscriptionWeekDuration, s.WeeklyWindowStart)
}

func (s *UserSubscription) CurrentMonthlyWindowStartAt(now time.Time) time.Time {
	return s.currentWindowStartAt(now, subscriptionMonthDuration, s.MonthlyWindowStart)
}

func (s *UserSubscription) WindowStartsAt(now time.Time) SubscriptionWindowStarts {
	return SubscriptionWindowStarts{
		Daily:   s.CurrentDailyWindowStartAt(now),
		Weekly:  s.CurrentWeeklyWindowStartAt(now),
		Monthly: s.CurrentMonthlyWindowStartAt(now),
	}
}

func (s *UserSubscription) NormalizedWindowSnapshotAt(now time.Time) *UserSubscription {
	if s == nil {
		return nil
	}
	normalized := *s
	if normalized.DailyWindowStart == nil && !normalized.StartsAt.IsZero() {
		windowStart := normalized.CurrentDailyWindowStartAt(now)
		normalized.DailyWindowStart = &windowStart
	} else if normalized.NeedsDailyResetAt(now) {
		windowStart := normalized.CurrentDailyWindowStartAt(now)
		normalized.DailyWindowStart = &windowStart
		normalized.DailyUsageUSD = 0
	}
	if normalized.WeeklyWindowStart == nil && !normalized.StartsAt.IsZero() {
		windowStart := normalized.CurrentWeeklyWindowStartAt(now)
		normalized.WeeklyWindowStart = &windowStart
	} else if normalized.NeedsWeeklyResetAt(now) {
		windowStart := normalized.CurrentWeeklyWindowStartAt(now)
		normalized.WeeklyWindowStart = &windowStart
		normalized.WeeklyUsageUSD = 0
	}
	if normalized.MonthlyWindowStart == nil && !normalized.StartsAt.IsZero() {
		windowStart := normalized.CurrentMonthlyWindowStartAt(now)
		normalized.MonthlyWindowStart = &windowStart
	} else if normalized.NeedsMonthlyResetAt(now) {
		windowStart := normalized.CurrentMonthlyWindowStartAt(now)
		normalized.MonthlyWindowStart = &windowStart
		normalized.MonthlyUsageUSD = 0
	}
	return &normalized
}

func (s *UserSubscription) needsWindowResetAt(now time.Time, period time.Duration, windowStart *time.Time) bool {
	if windowStart == nil {
		return false
	}
	return s.currentWindowStartAt(now, period, windowStart).After(*windowStart)
}

func (s *UserSubscription) NeedsDailyReset() bool {
	return s.NeedsDailyResetAt(time.Now())
}

func (s *UserSubscription) NeedsDailyResetAt(now time.Time) bool {
	if s == nil || s.DailyWindowStart == nil || s.HasOneTimeDailyQuota() {
		return false
	}
	return s.needsWindowResetAt(now, subscriptionDayDuration, s.DailyWindowStart)
}

func (s *UserSubscription) NeedsWeeklyReset() bool {
	return s.NeedsWeeklyResetAt(time.Now())
}

func (s *UserSubscription) NeedsWeeklyResetAt(now time.Time) bool {
	if s == nil || s.WeeklyWindowStart == nil {
		return false
	}
	return s.needsWindowResetAt(now, subscriptionWeekDuration, s.WeeklyWindowStart)
}

func (s *UserSubscription) NeedsMonthlyReset() bool {
	return s.NeedsMonthlyResetAt(time.Now())
}

func (s *UserSubscription) NeedsMonthlyResetAt(now time.Time) bool {
	if s == nil || s.MonthlyWindowStart == nil {
		return false
	}
	return s.needsWindowResetAt(now, subscriptionMonthDuration, s.MonthlyWindowStart)
}

func (s *UserSubscription) DailyResetTime() *time.Time {
	return s.DailyResetTimeAt(time.Now())
}

func (s *UserSubscription) DailyResetTimeAt(now time.Time) *time.Time {
	if s == nil || (s.DailyWindowStart == nil && s.StartsAt.IsZero()) {
		return nil
	}
	if s.HasOneTimeDailyQuota() {
		t := s.ExpiresAt
		return &t
	}
	t := s.CurrentDailyWindowStartAt(now).Add(subscriptionDayDuration)
	return &t
}

func (s *UserSubscription) WeeklyResetTime() *time.Time {
	return s.WeeklyResetTimeAt(time.Now())
}

func (s *UserSubscription) WeeklyResetTimeAt(now time.Time) *time.Time {
	if s == nil || (s.WeeklyWindowStart == nil && s.StartsAt.IsZero()) {
		return nil
	}
	t := s.CurrentWeeklyWindowStartAt(now).Add(subscriptionWeekDuration)
	return &t
}

func (s *UserSubscription) MonthlyResetTime() *time.Time {
	return s.MonthlyResetTimeAt(time.Now())
}

func (s *UserSubscription) MonthlyResetTimeAt(now time.Time) *time.Time {
	if s == nil || (s.MonthlyWindowStart == nil && s.StartsAt.IsZero()) {
		return nil
	}
	t := s.CurrentMonthlyWindowStartAt(now).Add(subscriptionMonthDuration)
	return &t
}

func (s *UserSubscription) EffectiveDailyLimit(group *Group) *float64 {
	if s != nil && s.QuotaSnapshotted {
		return s.DailyLimitUSD
	}
	if group == nil {
		return nil
	}
	return group.DailyLimitUSD
}

func (s *UserSubscription) EffectiveWeeklyLimit(group *Group) *float64 {
	if s != nil && s.QuotaSnapshotted {
		return s.WeeklyLimitUSD
	}
	if group == nil {
		return nil
	}
	return group.WeeklyLimitUSD
}

func (s *UserSubscription) EffectiveMonthlyLimit(group *Group) *float64 {
	if s != nil && s.QuotaSnapshotted {
		return s.MonthlyLimitUSD
	}
	if group == nil {
		return nil
	}
	return group.MonthlyLimitUSD
}

func (s *UserSubscription) CheckDailyLimit(group *Group, additionalCost float64) bool {
	limit := s.EffectiveDailyLimit(group)
	return limit == nil || s.DailyUsageUSD+additionalCost <= *limit
}

func (s *UserSubscription) CheckWeeklyLimit(group *Group, additionalCost float64) bool {
	limit := s.EffectiveWeeklyLimit(group)
	return limit == nil || s.WeeklyUsageUSD+additionalCost <= *limit
}

func (s *UserSubscription) CheckMonthlyLimit(group *Group, additionalCost float64) bool {
	limit := s.EffectiveMonthlyLimit(group)
	return limit == nil || s.MonthlyUsageUSD+additionalCost <= *limit
}

func (s *UserSubscription) CheckAllLimits(group *Group, additionalCost float64) (daily, weekly, monthly bool) {
	daily = s.CheckDailyLimit(group, additionalCost)
	weekly = s.CheckWeeklyLimit(group, additionalCost)
	monthly = s.CheckMonthlyLimit(group, additionalCost)
	return
}
