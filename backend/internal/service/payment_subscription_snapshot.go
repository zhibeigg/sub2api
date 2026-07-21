package service

import (
	"context"
	"encoding/json"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type paymentSubscriptionSnapshot struct {
	SchemaVersion    int      `json:"schema_version"`
	PlanID           int64    `json:"plan_id"`
	PlanType         string   `json:"plan_type"`
	GroupIDs         []int64  `json:"group_ids"`
	ValidityDays     int      `json:"validity_days"`
	DailyLimitUSD    *float64 `json:"daily_limit_usd"`
	WeeklyLimitUSD   *float64 `json:"weekly_limit_usd"`
	MonthlyLimitUSD  *float64 `json:"monthly_limit_usd"`
	ConcurrencyLimit *int     `json:"concurrency_limit"`
}

func (s *PaymentService) buildSubscriptionSnapshot(ctx context.Context, plan *dbent.SubscriptionPlan) (*paymentSubscriptionSnapshot, error) {
	groups := s.configService.GetPlanGroupsMap(ctx, []*dbent.SubscriptionPlan{plan})[plan.ID]
	groupIDs := make([]int64, 0, len(groups))
	for _, item := range groups {
		groupIDs = append(groupIDs, item.ID)
	}
	if len(groupIDs) == 0 {
		groupIDs = []int64{plan.GroupID}
	}
	planType := normalizeSubscriptionPlanType(plan.PlanType)
	dailyLimit := plan.DailyLimitUsd
	weeklyLimit := plan.WeeklyLimitUsd
	monthlyLimit := plan.MonthlyLimitUsd
	concurrencyLimit := plan.ConcurrencyLimit
	if planType == domain.SubscriptionPlanTypeSubscription {
		dailyLimit = nil
		weeklyLimit = nil
		monthlyLimit = nil
	}
	if planType != domain.SubscriptionPlanTypeStandardQuota {
		concurrencyLimit = nil
	}
	return &paymentSubscriptionSnapshot{
		SchemaVersion: 3, PlanID: plan.ID, PlanType: planType, GroupIDs: groupIDs,
		ValidityDays:  psComputeValidityDays(plan.ValidityDays, plan.ValidityUnit),
		DailyLimitUSD: dailyLimit, WeeklyLimitUSD: weeklyLimit, MonthlyLimitUSD: monthlyLimit,
		ConcurrencyLimit: concurrencyLimit,
	}, nil
}

func subscriptionSnapshotMap(snapshot *paymentSubscriptionSnapshot) map[string]any {
	if snapshot == nil {
		return nil
	}
	raw, _ := json.Marshal(snapshot)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return out
}

func parsePaymentSubscriptionSnapshot(order *dbent.PaymentOrder) *paymentSubscriptionSnapshot {
	if order == nil {
		return nil
	}
	if len(order.SubscriptionSnapshot) > 0 {
		raw, err := json.Marshal(order.SubscriptionSnapshot)
		if err == nil {
			var snapshot paymentSubscriptionSnapshot
			if json.Unmarshal(raw, &snapshot) == nil && len(snapshot.GroupIDs) > 0 && snapshot.ValidityDays > 0 {
				if strings.TrimSpace(snapshot.PlanType) == "" {
					if snapshot.SchemaVersion > 0 {
						snapshot.PlanType = domain.SubscriptionPlanTypeLegacySharedSubscription
					} else {
						snapshot.PlanType = domain.SubscriptionPlanTypeSubscription
					}
				}
				if snapshot.SchemaVersion < 3 || normalizeSubscriptionPlanType(snapshot.PlanType) != domain.SubscriptionPlanTypeStandardQuota {
					snapshot.ConcurrencyLimit = nil
				}
				return &snapshot
			}
		}
	}
	if order.SubscriptionGroupID == nil || order.SubscriptionDays == nil {
		return nil
	}
	return &paymentSubscriptionSnapshot{
		SchemaVersion: 0,
		PlanID:        valueInt64(order.PlanID),
		PlanType:      domain.SubscriptionPlanTypeSubscription,
		GroupIDs:      []int64{*order.SubscriptionGroupID},
		ValidityDays:  *order.SubscriptionDays,
	}
}

func (snapshot *paymentSubscriptionSnapshot) quotaSnapshotted() bool {
	if snapshot == nil {
		return false
	}
	if snapshot.SchemaVersion >= 2 {
		return normalizeSubscriptionPlanType(snapshot.PlanType) != domain.SubscriptionPlanTypeSubscription
	}
	return snapshot.SchemaVersion > 0
}

func valueInt64(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
