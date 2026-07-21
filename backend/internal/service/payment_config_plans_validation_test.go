//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestValidatePlanRequired_AllValid(t *testing.T) {
	err := validatePlanRequired("Pro", []int64{1}, 9.99, 30, "days", nil)
	require.NoError(t, err)
}

func TestValidatePlanRequired_EmptyName(t *testing.T) {
	err := validatePlanRequired("", []int64{1}, 9.99, 30, "days", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "plan name")
}

func TestValidatePlanRequired_WhitespaceName(t *testing.T) {
	err := validatePlanRequired("   ", []int64{1}, 9.99, 30, "days", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "plan name")
}

func TestValidatePlanRequired_EmptyGroupIDs(t *testing.T) {
	err := validatePlanRequired("Pro", nil, 9.99, 30, "days", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "group")
}

func TestValidatePlanRequired_ZeroPrice(t *testing.T) {
	err := validatePlanRequired("Pro", []int64{1}, 0, 30, "days", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "price")
}

func TestValidatePlanRequired_NegativePrice(t *testing.T) {
	err := validatePlanRequired("Pro", []int64{1}, -5, 30, "days", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "price")
}

func TestValidatePlanRequired_ZeroValidityDays(t *testing.T) {
	err := validatePlanRequired("Pro", []int64{1}, 9.99, 0, "days", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "validity days")
}

func TestValidatePlanRequired_NegativeValidityDays(t *testing.T) {
	err := validatePlanRequired("Pro", []int64{1}, 9.99, -7, "days", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "validity days")
}

func TestValidatePlanRequired_EmptyValidityUnit(t *testing.T) {
	err := validatePlanRequired("Pro", []int64{1}, 9.99, 30, "", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "validity unit")
}

func TestValidatePlanRequired_WhitespaceValidityUnit(t *testing.T) {
	err := validatePlanRequired("Pro", []int64{1}, 9.99, 30, "   ", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "validity unit")
}

func TestValidatePlanRequired_NameValidatedFirst(t *testing.T) {
	err := validatePlanRequired("", nil, 0, 0, "", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "plan name")
}

func TestValidatePlanRequired_TrimmedValidName(t *testing.T) {
	err := validatePlanRequired("  Pro  ", []int64{1}, 9.99, 30, "days", nil)
	require.NoError(t, err)
}

func TestValidatePlanRequired_NegativeOriginalPrice(t *testing.T) {
	neg := -10.0
	err := validatePlanRequired("Pro", []int64{1}, 9.99, 30, "days", &neg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "original price")
}

func TestValidatePlanRequired_ZeroOriginalPrice(t *testing.T) {
	zero := 0.0
	err := validatePlanRequired("Pro", []int64{1}, 9.99, 30, "days", &zero)
	require.NoError(t, err)
}

func TestValidatePlanRequired_ValidOriginalPrice(t *testing.T) {
	op := 19.99
	err := validatePlanRequired("Pro", []int64{1}, 9.99, 30, "days", &op)
	require.NoError(t, err)
}

// --- validatePlanPatch tests ---

func TestValidatePlanPatch_NegativeOriginalPrice(t *testing.T) {
	neg := -5.0
	err := validatePlanPatch(UpdatePlanRequest{OriginalPrice: &neg})
	require.Error(t, err)
	require.Contains(t, err.Error(), "original price")
}

func TestValidatePlanPatch_ZeroOriginalPrice(t *testing.T) {
	zero := 0.0
	err := validatePlanPatch(UpdatePlanRequest{OriginalPrice: &zero})
	require.NoError(t, err)
}

func TestValidatePlanPatch_ValidOriginalPrice(t *testing.T) {
	op := 29.99
	err := validatePlanPatch(UpdatePlanRequest{OriginalPrice: &op})
	require.NoError(t, err)
}

func TestValidatePlanPatch_NilOriginalPrice(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{OriginalPrice: nil})
	require.NoError(t, err)
}

// --- validatePlanPatch: other fields ---

func ptrStr(s string) *string     { return &s }
func ptrInt(i int) *int           { return &i }
func ptrInt64(i int64) *int64     { return &i }
func ptrFloat(f float64) *float64 { return &f }

func TestValidatePlanPatch_EmptyName(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{Name: ptrStr("")})
	require.Error(t, err)
	require.Contains(t, err.Error(), "plan name")
}

func TestValidatePlanPatch_ValidName(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{Name: ptrStr("Basic")})
	require.NoError(t, err)
}

func TestValidatePlanPatch_ZeroGroupID(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{GroupID: ptrInt64(0)})
	require.Error(t, err)
	require.Contains(t, err.Error(), "group")
}

func TestValidatePlanPatch_NegativePrice(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{Price: ptrFloat(-1)})
	require.Error(t, err)
	require.Contains(t, err.Error(), "price")
}

func TestValidatePlanPatch_ZeroPrice(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{Price: ptrFloat(0)})
	require.Error(t, err)
	require.Contains(t, err.Error(), "price")
}

func TestValidatePlanPatch_ValidPrice(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{Price: ptrFloat(9.99)})
	require.NoError(t, err)
}

func TestValidatePlanPatch_ZeroValidityDays(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{ValidityDays: ptrInt(0)})
	require.Error(t, err)
	require.Contains(t, err.Error(), "validity days")
}

func TestValidatePlanPatch_EmptyValidityUnit(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{ValidityUnit: ptrStr("")})
	require.Error(t, err)
	require.Contains(t, err.Error(), "validity unit")
}

func TestValidatePlanPatch_ValidValidityUnit(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{ValidityUnit: ptrStr("days")})
	require.NoError(t, err)
}

func TestValidatePlanPatch_AllNil(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{})
	require.NoError(t, err)
}

func TestValidatePlanSemantics_DualPlanTypes(t *testing.T) {
	quota := 10.0
	tests := []struct {
		name     string
		planType string
		groupIDs []int64
		daily    *float64
		weekly   *float64
		monthly  *float64
		wantCode string
	}{
		{name: "subscription single group allows empty quota", planType: domain.SubscriptionPlanTypeSubscription, groupIDs: []int64{1}},
		{name: "subscription rejects multiple groups", planType: domain.SubscriptionPlanTypeSubscription, groupIDs: []int64{1, 2}, wantCode: "PLAN_GROUP_COUNT_INVALID"},
		{name: "standard quota allows multiple groups with daily quota", planType: domain.SubscriptionPlanTypeStandardQuota, groupIDs: []int64{1, 2}, daily: &quota},
		{name: "standard quota rejects empty quota", planType: domain.SubscriptionPlanTypeStandardQuota, groupIDs: []int64{1}, wantCode: "PLAN_QUOTA_REQUIRED"},
		{name: "standard quota rejects empty groups", planType: domain.SubscriptionPlanTypeStandardQuota, daily: &quota, wantCode: "PLAN_GROUP_REQUIRED"},
		{name: "invalid plan type rejected", planType: "metered", groupIDs: []int64{1}, daily: &quota, wantCode: "PLAN_TYPE_INVALID"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePlanSemantics(tt.planType, tt.groupIDs, tt.daily, tt.weekly, tt.monthly)
			if tt.wantCode == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Equal(t, tt.wantCode, infraerrors.Reason(err))
		})
	}
}

// Empty must stay empty (not coerced to the default payment currency),
// so existing plans keep rendering without any currency label.
func TestNormalizePlanCurrency_EmptyKeepsEmpty(t *testing.T) {
	currency, err := normalizePlanCurrency("")
	require.NoError(t, err)
	require.Equal(t, "", currency)
}

func TestNormalizePlanCurrency_WhitespaceKeepsEmpty(t *testing.T) {
	currency, err := normalizePlanCurrency("   ")
	require.NoError(t, err)
	require.Equal(t, "", currency)
}

func TestNormalizePlanCurrency_LowercaseNormalized(t *testing.T) {
	currency, err := normalizePlanCurrency("nzd")
	require.NoError(t, err)
	require.Equal(t, "NZD", currency)
}

func TestNormalizePlanCurrency_ValidUppercase(t *testing.T) {
	currency, err := normalizePlanCurrency("USD")
	require.NoError(t, err)
	require.Equal(t, "USD", currency)
}

func TestNormalizePlanCurrency_TooShort(t *testing.T) {
	_, err := normalizePlanCurrency("NZ")
	require.Error(t, err)
	require.Contains(t, err.Error(), "currency")
}

func TestNormalizePlanCurrency_TooLong(t *testing.T) {
	_, err := normalizePlanCurrency("NZDD")
	require.Error(t, err)
	require.Contains(t, err.Error(), "currency")
}

func TestNormalizePlanCurrency_NonLetter(t *testing.T) {
	_, err := normalizePlanCurrency("N2D")
	require.Error(t, err)
	require.Contains(t, err.Error(), "currency")
}

func TestValidatePlanConcurrencyLimit(t *testing.T) {
	valid := 5
	maxValid := maxPlanConcurrencyLimit
	zero := 0
	negative := -1
	tooLarge := maxPlanConcurrencyLimit + 1

	for _, limit := range []*int{nil, &valid, &maxValid} {
		require.NoError(t, validatePlanConcurrencyLimit(limit))
	}
	for _, limit := range []*int{&zero, &negative, &tooLarge} {
		err := validatePlanConcurrencyLimit(limit)
		require.Error(t, err)
		require.Equal(t, "PLAN_CONCURRENCY_INVALID", infraerrors.Reason(err))
	}
}

func TestNormalizePlanConcurrencyLimit_OnlyStandardQuotaKeepsValue(t *testing.T) {
	limit := 7

	standardLimit, err := normalizePlanConcurrencyLimit(domain.SubscriptionPlanTypeStandardQuota, &limit)
	require.NoError(t, err)
	require.Equal(t, &limit, standardLimit)

	for _, planType := range []string{
		domain.SubscriptionPlanTypeSubscription,
		domain.SubscriptionPlanTypeLegacySharedSubscription,
	} {
		normalized, err := normalizePlanConcurrencyLimit(planType, &limit)
		require.NoError(t, err)
		require.Nil(t, normalized)
	}
}

func TestUpdatePlanRequest_ExplicitNullConcurrencyLimit(t *testing.T) {
	var req UpdatePlanRequest
	require.NoError(t, json.Unmarshal([]byte(`{"concurrency_limit":null,"concurrency_limit_set":true}`), &req))
	require.True(t, req.ConcurrencyLimitSet)
	require.Nil(t, req.ConcurrencyLimit)
}

func TestPaymentConfigService_PlanConcurrencyCRUDAndTypeSwitch(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	standardGroup, err := client.Group.Create().
		SetName("standard-concurrency").
		SetSubscriptionType(domain.SubscriptionTypeStandard).
		Save(ctx)
	require.NoError(t, err)
	subscriptionGroup, err := client.Group.Create().
		SetName("native-subscription-concurrency").
		SetSubscriptionType(domain.SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)

	daily := 10.0
	initialLimit := 4
	svc := NewPaymentConfigService(client, nil, nil)
	plan, err := svc.CreatePlan(ctx, CreatePlanRequest{
		PlanType:         domain.SubscriptionPlanTypeStandardQuota,
		GroupIDs:         []int64{standardGroup.ID},
		Name:             "Standard Quota",
		Price:            9.99,
		DailyLimitUSD:    &daily,
		ConcurrencyLimit: &initialLimit,
		ValidityDays:     30,
		ValidityUnit:     "day",
		ForSale:          true,
	})
	require.NoError(t, err)
	require.Equal(t, &initialLimit, plan.ConcurrencyLimit)
	responses := svc.PlanResponses(ctx, []*dbent.SubscriptionPlan{plan})
	require.Len(t, responses, 1)
	require.Equal(t, &initialLimit, responses[0].ConcurrencyLimit)

	assignment, err := NewSubscriptionService(nil, nil, nil, client, nil).
		BuildPlanAssignmentInput(ctx, 100, plan.ID, 0, 200, "admin assignment")
	require.NoError(t, err)
	require.Equal(t, &initialLimit, assignment.ConcurrencyLimit)

	snapshot, err := (&PaymentService{configService: svc}).buildSubscriptionSnapshot(ctx, plan)
	require.NoError(t, err)
	require.Equal(t, 3, snapshot.SchemaVersion)
	require.Equal(t, &initialLimit, snapshot.ConcurrencyLimit)

	plan, err = svc.UpdatePlan(ctx, plan.ID, UpdatePlanRequest{ConcurrencyLimitSet: true})
	require.NoError(t, err)
	require.Nil(t, plan.ConcurrencyLimit)

	updatedLimit := 6
	plan, err = svc.UpdatePlan(ctx, plan.ID, UpdatePlanRequest{ConcurrencyLimit: &updatedLimit})
	require.NoError(t, err)
	require.Equal(t, &updatedLimit, plan.ConcurrencyLimit)

	nativeType := domain.SubscriptionPlanTypeSubscription
	plan, err = svc.UpdatePlan(ctx, plan.ID, UpdatePlanRequest{
		PlanType: &nativeType,
		GroupIDs: []int64{subscriptionGroup.ID},
	})
	require.NoError(t, err)
	require.Equal(t, domain.SubscriptionPlanTypeSubscription, plan.PlanType)
	require.Nil(t, plan.ConcurrencyLimit)
	require.Nil(t, plan.DailyLimitUsd)
}
