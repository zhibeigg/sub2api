package service

import (
	"encoding/json"
	"time"
)

// decodeKiroSnapshot normalizes an Account.Extra value (which may be a typed
// struct, a pointer, or a JSON-decoded map) into a *KiroUsageSnapshot.
func decodeKiroSnapshot(raw any) *KiroUsageSnapshot {
	if raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case *KiroUsageSnapshot:
		return v
	case KiroUsageSnapshot:
		return &v
	default:
		data, err := json.Marshal(raw)
		if err != nil {
			return nil
		}
		var out KiroUsageSnapshot
		if err := json.Unmarshal(data, &out); err != nil {
			return nil
		}
		return &out
	}
}

// buildKiroUsageInfo maps a persisted snapshot into a UsageInfo for the account
// usage endpoint. Returns a minimal UsageInfo when no snapshot exists yet.
func buildKiroUsageInfo(account *Account) *UsageInfo {
	now := time.Now()
	usage := &UsageInfo{Source: "passive", UpdatedAt: &now}
	if account == nil || account.Extra == nil {
		usage.Error = "Kiro usage is unknown until the first usage probe"
		return usage
	}
	snapshot := decodeKiroSnapshot(account.Extra[kiroUsageSnapshotExtraKey])
	if snapshot == nil {
		usage.Error = "Kiro usage is unknown until the first usage probe"
		return usage
	}

	if snapshot.CheckedAt > 0 {
		t := time.Unix(snapshot.CheckedAt, 0)
		usage.UpdatedAt = &t
	}
	usage.SubscriptionTier = snapshot.SubscriptionType
	usage.SubscriptionTierRaw = snapshot.SubscriptionRaw
	usage.KiroSubscriptionType = snapshot.SubscriptionType
	usage.KiroSubscriptionRaw = snapshot.SubscriptionRaw
	usage.KiroUsageCurrent = floatPtr(snapshot.UsageCurrent)
	usage.KiroUsageLimit = floatPtr(snapshot.UsageLimit)
	usage.KiroUsagePercent = floatPtr(snapshot.UsagePercent)
	usage.KiroTrialCurrent = floatPtrNonZero(snapshot.TrialCurrent)
	usage.KiroTrialLimit = floatPtrNonZero(snapshot.TrialLimit)
	usage.KiroTrialStatus = snapshot.TrialStatus
	usage.KiroNextResetDate = snapshot.NextResetDate
	usage.KiroOverageStatus = snapshot.OverageStatus
	usage.KiroOverageCap = floatPtrNonZero(snapshot.OverageCap)
	usage.KiroOverageRate = floatPtrNonZero(snapshot.OverageRate)
	usage.KiroCurrentOverages = floatPtrNonZero(snapshot.CurrentOverages)
	return usage
}

func floatPtr(v float64) *float64 { return &v }

func floatPtrNonZero(v float64) *float64 {
	if v == 0 {
		return nil
	}
	return &v
}
