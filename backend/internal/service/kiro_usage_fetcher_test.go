package service

import (
	"testing"
)

func TestDecodeKiroSnapshot(t *testing.T) {
	// typed pointer
	snap := &KiroUsageSnapshot{SubscriptionType: "PRO"}
	if got := decodeKiroSnapshot(snap); got == nil || got.SubscriptionType != "PRO" {
		t.Fatalf("pointer decode failed: %#v", got)
	}
	// value
	if got := decodeKiroSnapshot(KiroUsageSnapshot{SubscriptionType: "POWER"}); got == nil || got.SubscriptionType != "POWER" {
		t.Fatalf("value decode failed: %#v", got)
	}
	// map (as loaded from jsonb)
	m := map[string]any{"subscription_type": "PRO_PLUS", "usage_current": 12.0, "usage_limit": 100.0}
	got := decodeKiroSnapshot(m)
	if got == nil || got.SubscriptionType != "PRO_PLUS" || got.UsageCurrent != 12 || got.UsageLimit != 100 {
		t.Fatalf("map decode failed: %#v", got)
	}
	// nil
	if decodeKiroSnapshot(nil) != nil {
		t.Fatal("nil should decode to nil")
	}
}

func TestBuildKiroUsageInfo(t *testing.T) {
	// no snapshot → degraded
	acct := &Account{Extra: map[string]any{}}
	info := buildKiroUsageInfo(acct)
	if info.Error == "" {
		t.Error("expected degraded info when snapshot missing")
	}

	// with snapshot
	acct2 := &Account{Extra: map[string]any{
		kiroUsageSnapshotExtraKey: map[string]any{
			"subscription_type": "PRO_PLUS",
			"usage_current":     50.0,
			"usage_limit":       200.0,
			"usage_percent":     0.25,
			"overage_status":    "ENABLED",
			"overage_cap":       20.0,
			"next_reset_date":   "2026-08-01",
			"checked_at":        float64(1_700_000_000),
		},
	}}
	info2 := buildKiroUsageInfo(acct2)
	if info2.KiroSubscriptionType != "PRO_PLUS" {
		t.Errorf("subscription type = %q", info2.KiroSubscriptionType)
	}
	if info2.KiroUsageCurrent == nil || *info2.KiroUsageCurrent != 50 {
		t.Errorf("usage current wrong: %#v", info2.KiroUsageCurrent)
	}
	if info2.KiroOverageStatus != "ENABLED" {
		t.Errorf("overage status = %q", info2.KiroOverageStatus)
	}
	if info2.KiroOverageCap == nil || *info2.KiroOverageCap != 20 {
		t.Errorf("overage cap wrong: %#v", info2.KiroOverageCap)
	}
	if info2.KiroNextResetDate != "2026-08-01" {
		t.Errorf("next reset date = %q", info2.KiroNextResetDate)
	}
	// zero-valued optional fields should be nil, not pointer-to-zero
	if info2.KiroTrialLimit != nil {
		t.Errorf("trial limit should be nil when zero: %#v", info2.KiroTrialLimit)
	}
}
