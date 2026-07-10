package service

import (
	"context"
	"encoding/json"

	dbent "github.com/Wei-Shaw/sub2api/ent"
)

type paymentSubscriptionSnapshot struct {
	SchemaVersion   int      `json:"schema_version"`
	PlanID          int64    `json:"plan_id"`
	GroupIDs        []int64  `json:"group_ids"`
	ValidityDays    int      `json:"validity_days"`
	DailyLimitUSD   *float64 `json:"daily_limit_usd"`
	WeeklyLimitUSD  *float64 `json:"weekly_limit_usd"`
	MonthlyLimitUSD *float64 `json:"monthly_limit_usd"`
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
	return &paymentSubscriptionSnapshot{
		SchemaVersion: 1, PlanID: plan.ID, GroupIDs: groupIDs,
		ValidityDays:  psComputeValidityDays(plan.ValidityDays, plan.ValidityUnit),
		DailyLimitUSD: plan.DailyLimitUsd, WeeklyLimitUSD: plan.WeeklyLimitUsd, MonthlyLimitUSD: plan.MonthlyLimitUsd,
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
				return &snapshot
			}
		}
	}
	if order.SubscriptionGroupID == nil || order.SubscriptionDays == nil {
		return nil
	}
	return &paymentSubscriptionSnapshot{SchemaVersion: 0, PlanID: valueInt64(order.PlanID), GroupIDs: []int64{*order.SubscriptionGroupID}, ValidityDays: *order.SubscriptionDays}
}

func valueInt64(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
