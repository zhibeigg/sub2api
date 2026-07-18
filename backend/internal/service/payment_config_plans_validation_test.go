//go:build unit

package service

import (
	"testing"

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
