package admin

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func TestSanitizeAdminPaymentOrderForResponseAddsCurrency(t *testing.T) {
	now := time.Now()
	order := &dbent.PaymentOrder{
		ID:                     1,
		UserID:                 2,
		Amount:                 100,
		PayAmount:              108,
		FeeRate:                8,
		OutTradeNo:             "sub2_202606250001",
		PaymentType:            "stripe",
		OrderType:              "subscription",
		Status:                 "COMPLETED",
		SignupPromoAttribution: service.PromoAttributionNone,
		CompletedAt:            &now,
		ExpiresAt:              now,
		CreatedAt:              now,
		UpdatedAt:              now,
		ProviderSnapshot: map[string]any{
			"schema_version": 2,
			"currency":       "USD",
		},
	}

	got := sanitizeAdminPaymentOrderForResponse(order)
	if got == nil {
		t.Fatal("expected sanitized order")
	}
	if got.Currency != "USD" {
		t.Fatalf("expected currency USD, got %q", got.Currency)
	}

	body, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal sanitized order: %v", err)
	}
	if strings.Contains(string(body), "provider_snapshot") {
		t.Fatalf("expected provider_snapshot to be omitted, got %s", string(body))
	}
	if got.SignupPromoAttribution != service.PromoAttributionNone {
		t.Fatalf("expected none attribution, got %q", got.SignupPromoAttribution)
	}
	if got.NetRechargeAmount != 0 {
		t.Fatalf("subscription order should not count as recharge, got %v", got.NetRechargeAmount)
	}
}

func TestParseAdminOrderFiltersDateAndPromo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("GET", "/?start_date=2026-03-01&end_date=2026-03-08&timezone=Asia%2FShanghai&time_field=paid_at&promo_code_id=42", nil)

	params, err := parseAdminOrderFilters(ctx, 2, 20)
	if err != nil {
		t.Fatalf("parse filters: %v", err)
	}
	if params.TimeField != service.AdminOrderTimeFieldPaidAt {
		t.Fatalf("expected paid_at, got %q", params.TimeField)
	}
	if params.PromoCodeID == nil || *params.PromoCodeID != 42 {
		t.Fatalf("expected promo code 42, got %#v", params.PromoCodeID)
	}
	if params.StartTime == nil || params.EndTime == nil {
		t.Fatal("expected parsed date boundaries")
	}
	if got := params.StartTime.Format(time.RFC3339); got != "2026-02-28T16:00:00Z" {
		t.Fatalf("unexpected start boundary %s", got)
	}
	if got := params.EndTime.Format(time.RFC3339); got != "2026-03-08T16:00:00Z" {
		t.Fatalf("unexpected half-open end boundary %s", got)
	}
}

func TestParseAdminOrderFiltersRejectsInvalidValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, rawURL := range []string{
		"/?user_id=oops",
		"/?promo_attribution=deleted",
		"/?time_field=completed_at",
		"/?timezone=Invalid%2FZone",
		"/?start_date=2026-03-10&end_date=2026-03-01",
	} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest("GET", rawURL, nil)
		if _, err := parseAdminOrderFilters(ctx, 1, 20); err == nil {
			t.Fatalf("expected %s to fail", rawURL)
		}
	}
}

func TestSafeCSVTextPreventsFormulaInjection(t *testing.T) {
	for _, input := range []string{"=1+1", "+cmd", "-2+3", "@SUM(A1:A2)", "  =HYPERLINK(\"x\")", "\tformula", "\rformula"} {
		if got := safeCSVText(input); !strings.HasPrefix(got, "'") {
			t.Fatalf("expected formula protection for %q, got %q", input, got)
		}
	}
	if got := safeCSVText("safe,value"); got != "safe,value" {
		t.Fatalf("expected safe text unchanged, got %q", got)
	}
	if got := safeCSVText("a\x00b"); got != "ab" {
		t.Fatalf("expected NUL removal, got %q", got)
	}
}

func TestAdminSubscriptionPlansForResponseIncludesCompositeGroupInfo(t *testing.T) {
	weekly := 25.0
	now := time.Now()
	plans := []*dbent.SubscriptionPlan{
		{
			ID:           11,
			GroupID:      7,
			Name:         "All models",
			Description:  "Composite access",
			Price:        19.99,
			Currency:     "CNY",
			ValidityDays: 30,
			ValidityUnit: "days",
			Features:     "OpenAI\nClaude\nGemini\nGrok",
			ProductName:  "Sub2API",
			ForSale:      true,
			SortOrder:    1,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
	}
	groupInfo := map[int64]service.PlanGroupInfo{
		7: {
			Platform:       service.PlatformComposite,
			Name:           "Bucket 2 composite",
			RateMultiplier: 1.5,
			WeeklyLimitUSD: &weekly,
			ModelScopes:    []string{"openai", "claude", "gemini", "grok"},
		},
	}

	got := adminSubscriptionPlansForResponse(plans, groupInfo)

	if len(got) != 1 {
		t.Fatalf("expected one plan, got %d", len(got))
	}
	if got[0].GroupPlatform != service.PlatformComposite {
		t.Fatalf("expected composite group platform, got %q", got[0].GroupPlatform)
	}
	if got[0].GroupName != "Bucket 2 composite" {
		t.Fatalf("expected group name to be included, got %q", got[0].GroupName)
	}
	if got[0].WeeklyLimitUSD == nil || *got[0].WeeklyLimitUSD != weekly {
		t.Fatalf("expected weekly limit to be included, got %#v", got[0].WeeklyLimitUSD)
	}
	if strings.Join(got[0].ModelScopes, ",") != "openai,claude,gemini,grok" {
		t.Fatalf("expected model scopes to be preserved, got %#v", got[0].ModelScopes)
	}
	// 投影必须保留 ent 原始响应的全部套餐字段：currency 丢失曾导致编辑保存时
	// 静默清空套餐货币（PlanEditDialog 回传空串 → SetCurrency("")）。
	if got[0].Currency != "CNY" {
		t.Fatalf("expected currency to be preserved, got %q", got[0].Currency)
	}
	if !got[0].CreatedAt.Equal(now) || !got[0].UpdatedAt.Equal(now) {
		t.Fatalf("expected created_at/updated_at to be preserved, got %v / %v", got[0].CreatedAt, got[0].UpdatedAt)
	}
}
