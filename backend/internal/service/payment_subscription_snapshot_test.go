//go:build unit

package service

import (
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestParsePaymentSubscriptionSnapshot_V2PlanTypeQuotaSemantics(t *testing.T) {
	daily := 10.0
	weekly := 20.0
	monthly := 30.0

	tests := []struct {
		name            string
		planType        string
		daily           any
		weekly          any
		monthly         any
		wantSnapshotted bool
		wantDaily       *float64
		wantWeekly      *float64
		wantMonthly     *float64
	}{
		{
			name:            "subscription has no quota snapshot",
			planType:        domain.SubscriptionPlanTypeSubscription,
			wantSnapshotted: false,
		},
		{
			name:            "standard quota retains plan limits",
			planType:        domain.SubscriptionPlanTypeStandardQuota,
			daily:           daily,
			weekly:          weekly,
			monthly:         monthly,
			wantSnapshotted: true,
			wantDaily:       &daily,
			wantWeekly:      &weekly,
			wantMonthly:     &monthly,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			order := &dbent.PaymentOrder{SubscriptionSnapshot: map[string]any{
				"schema_version":    2,
				"plan_id":           101,
				"plan_type":         tt.planType,
				"group_ids":         []any{int64(11), int64(12)},
				"validity_days":     30,
				"daily_limit_usd":   tt.daily,
				"weekly_limit_usd":  tt.weekly,
				"monthly_limit_usd": tt.monthly,
			}}

			snapshot := parsePaymentSubscriptionSnapshot(order)
			require.NotNil(t, snapshot)
			require.Equal(t, tt.planType, snapshot.PlanType)
			require.Equal(t, tt.wantSnapshotted, snapshot.quotaSnapshotted())
			require.Equal(t, tt.wantDaily, snapshot.DailyLimitUSD)
			require.Equal(t, tt.wantWeekly, snapshot.WeeklyLimitUSD)
			require.Equal(t, tt.wantMonthly, snapshot.MonthlyLimitUSD)
		})
	}
}

func TestParsePaymentSubscriptionSnapshot_V3ConcurrencyLimit(t *testing.T) {
	limit := 6
	order := &dbent.PaymentOrder{SubscriptionSnapshot: map[string]any{
		"schema_version":    3,
		"plan_id":           101,
		"plan_type":         domain.SubscriptionPlanTypeStandardQuota,
		"group_ids":         []any{int64(11), int64(12)},
		"validity_days":     30,
		"daily_limit_usd":   10.0,
		"concurrency_limit": limit,
	}}

	snapshot := parsePaymentSubscriptionSnapshot(order)
	require.NotNil(t, snapshot)
	require.Equal(t, 3, snapshot.SchemaVersion)
	require.Equal(t, &limit, snapshot.ConcurrencyLimit)

	mapped := subscriptionSnapshotMap(snapshot)
	require.EqualValues(t, 3, mapped["schema_version"])
	require.EqualValues(t, limit, mapped["concurrency_limit"])
}

func TestParsePaymentSubscriptionSnapshot_OldAndNonStandardSnapshotsIgnoreConcurrencyLimit(t *testing.T) {
	for _, tt := range []struct {
		name          string
		schemaVersion int
		planType      string
	}{
		{name: "v2 standard quota", schemaVersion: 2, planType: domain.SubscriptionPlanTypeStandardQuota},
		{name: "v3 subscription", schemaVersion: 3, planType: domain.SubscriptionPlanTypeSubscription},
		{name: "v3 legacy", schemaVersion: 3, planType: domain.SubscriptionPlanTypeLegacySharedSubscription},
	} {
		t.Run(tt.name, func(t *testing.T) {
			order := &dbent.PaymentOrder{SubscriptionSnapshot: map[string]any{
				"schema_version":    tt.schemaVersion,
				"plan_id":           101,
				"plan_type":         tt.planType,
				"group_ids":         []any{int64(11)},
				"validity_days":     30,
				"daily_limit_usd":   10.0,
				"concurrency_limit": 9,
			}}

			snapshot := parsePaymentSubscriptionSnapshot(order)
			require.NotNil(t, snapshot)
			require.Nil(t, snapshot.ConcurrencyLimit)
		})
	}
}

func TestParsePaymentSubscriptionSnapshot_LegacyVersionsRemainCompatible(t *testing.T) {
	t.Run("schema v0 snapshot infers subscription", func(t *testing.T) {
		order := &dbent.PaymentOrder{SubscriptionSnapshot: map[string]any{
			"schema_version": 0,
			"plan_id":        1,
			"group_ids":      []any{int64(7)},
			"validity_days":  30,
		}}
		snapshot := parsePaymentSubscriptionSnapshot(order)
		require.NotNil(t, snapshot)
		require.Equal(t, domain.SubscriptionPlanTypeSubscription, snapshot.PlanType)
		require.False(t, snapshot.quotaSnapshotted())
	})

	t.Run("schema v1 empty plan type infers legacy", func(t *testing.T) {
		order := &dbent.PaymentOrder{SubscriptionSnapshot: map[string]any{
			"schema_version": 1,
			"plan_id":        2,
			"plan_type":      "",
			"group_ids":      []any{int64(8), int64(9)},
			"validity_days":  60,
		}}
		snapshot := parsePaymentSubscriptionSnapshot(order)
		require.NotNil(t, snapshot)
		require.Equal(t, domain.SubscriptionPlanTypeLegacySharedSubscription, snapshot.PlanType)
		require.True(t, snapshot.quotaSnapshotted())
	})

	t.Run("legacy scalar order fields still parse as v0", func(t *testing.T) {
		groupID := int64(10)
		days := 15
		planID := int64(3)
		order := &dbent.PaymentOrder{
			PlanID:              &planID,
			SubscriptionGroupID: &groupID,
			SubscriptionDays:    &days,
		}
		snapshot := parsePaymentSubscriptionSnapshot(order)
		require.NotNil(t, snapshot)
		require.Equal(t, 0, snapshot.SchemaVersion)
		require.Equal(t, planID, snapshot.PlanID)
		require.Equal(t, []int64{groupID}, snapshot.GroupIDs)
		require.Equal(t, domain.SubscriptionPlanTypeSubscription, snapshot.PlanType)
		require.False(t, snapshot.quotaSnapshotted())
	})
}
