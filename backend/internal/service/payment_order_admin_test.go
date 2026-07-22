package service

import (
	"strings"
	"testing"
	"time"
)

func TestAdminOrderSQLWhereUsesSharedFilters(t *testing.T) {
	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	promoID := int64(42)
	where, args := adminOrderSQLWhere(OrderListParams{
		UserID:           7,
		Status:           OrderStatusCompleted,
		OrderType:        "balance",
		PaymentType:      "alipay",
		Keyword:          "WELCOME",
		PromoCodeID:      &promoID,
		PromoAttribution: PromoAttributionNone,
		StartTime:        &start,
		EndTime:          &end,
		TimeField:        AdminOrderTimeFieldPaidAt,
	})

	for _, fragment := range []string{
		"user_id = $1",
		"status = $2",
		"order_type = $3",
		"payment_type = $4",
		"signup_promo_code ILIKE $5",
		"signup_promo_code_id = $6",
		"signup_promo_attribution = 'attributed'",
		"paid_at >= $7",
		"paid_at < $8",
	} {
		if !strings.Contains(where, fragment) {
			t.Fatalf("expected where clause to contain %q: %s", fragment, where)
		}
	}
	if len(args) != 8 {
		t.Fatalf("expected 8 arguments, got %d", len(args))
	}
}

func TestAdminOrderSQLWhereFiltersNaturalAndLegacyAttribution(t *testing.T) {
	for _, attribution := range []string{PromoAttributionNone, PromoAttributionLegacyUnknown} {
		where, args := adminOrderSQLWhere(OrderListParams{PromoAttribution: attribution})
		if !strings.Contains(where, "signup_promo_attribution = $1") {
			t.Fatalf("expected attribution filter for %q: %s", attribution, where)
		}
		if len(args) != 1 || args[0] != attribution {
			t.Fatalf("unexpected args for %q: %#v", attribution, args)
		}
	}
}

func TestAdminOrderPaidAmountsSQLUsesPaymentTimeAndGatewayAmount(t *testing.T) {
	start := time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	query, args := adminOrderPaidAmountsSQL(OrderListParams{
		PaymentType: "wxpay",
		StartTime:   &start,
		EndTime:     &end,
		TimeField:   AdminOrderTimeFieldPaidAt,
	})

	for _, fragment := range []string{
		"payment_type = $1",
		"paid_at >= $2",
		"paid_at < $3",
		"paid_at IS NOT NULL",
		"SUM(pay_amount)",
		"provider_snapshot->>'currency'",
		"ELSE 'CNY'",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("expected paid amount query to contain %q: %s", fragment, query)
		}
	}
	if strings.Contains(query, "order_type = 'balance'") {
		t.Fatalf("paid amount query must include subscription receipts: %s", query)
	}
	if len(args) != 3 {
		t.Fatalf("expected 3 arguments, got %d", len(args))
	}
}
