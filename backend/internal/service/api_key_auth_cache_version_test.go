package service

import "testing"

func TestAPIKeyService_RejectsV10AuthSnapshotWithoutModelsListConfig(t *testing.T) {
	groupID := int64(9)
	svc := &APIKeyService{}

	apiKey, ok, err := svc.applyAuthCacheEntry("k-legacy-models-list", &APIKeyAuthCacheEntry{
		Snapshot: &APIKeyAuthSnapshot{
			Version:  10,
			APIKeyID: 1,
			UserID:   2,
			GroupID:  &groupID,
			Status:   StatusActive,
			User: APIKeyAuthUserSnapshot{
				ID:          2,
				Status:      StatusActive,
				Role:        RoleUser,
				Balance:     10,
				Concurrency: 3,
			},
			Group: &APIKeyAuthGroupSnapshot{
				ID:               groupID,
				Name:             "openai",
				Platform:         PlatformOpenAI,
				Status:           StatusActive,
				SubscriptionType: SubscriptionTypeStandard,
				RateMultiplier:   1,
			},
		},
	})

	if err != nil {
		t.Fatalf("expected stale snapshot to be ignored without error, got %v", err)
	}
	if ok {
		t.Fatalf("expected v10 auth snapshot to be rejected after models_list_config was added")
	}
	if apiKey != nil {
		t.Fatalf("expected no API key from stale snapshot, got %#v", apiKey)
	}
}

func TestAPIKeyService_RejectsV20AuthSnapshotWithoutPoolCapacityAlertPolicy(t *testing.T) {
	svc := &APIKeyService{}

	apiKey, ok, err := svc.applyAuthCacheEntry("k-legacy-pool-capacity-alert", &APIKeyAuthCacheEntry{
		Snapshot: &APIKeyAuthSnapshot{
			Version:  20,
			APIKeyID: 1,
			UserID:   2,
			Status:   StatusActive,
			User: APIKeyAuthUserSnapshot{
				ID:     2,
				Status: StatusActive,
				Role:   RoleUser,
			},
		},
	})

	if err != nil {
		t.Fatalf("expected stale snapshot to be ignored without error, got %v", err)
	}
	if ok {
		t.Fatal("expected v20 auth snapshot to be rejected after newer group fields were added")
	}
	if apiKey != nil {
		t.Fatalf("expected no API key from stale snapshot, got %#v", apiKey)
	}
}

func TestAPIKeyService_RejectsV15AuthSnapshotWithoutReasoningEffortPolicy(t *testing.T) {
	svc := &APIKeyService{}

	apiKey, ok, err := svc.applyAuthCacheEntry("k-legacy-reasoning-mappings", &APIKeyAuthCacheEntry{
		Snapshot: &APIKeyAuthSnapshot{Version: 15},
	})

	if err != nil {
		t.Fatalf("expected stale snapshot to be ignored without error, got %v", err)
	}
	if ok {
		t.Fatal("expected v15 auth snapshot to be rejected after reasoning effort policy was added")
	}
	if apiKey != nil {
		t.Fatalf("expected no API key from stale snapshot, got %#v", apiKey)
	}
}

func TestGroupPolicyAuthSnapshotRoundTrip(t *testing.T) {
	thresholdUSD := 12.5
	group := &Group{
		ID:                                 26,
		Name:                               "policy-group",
		Platform:                           PlatformOpenAI,
		Status:                             StatusActive,
		SubscriptionType:                   SubscriptionTypeStandard,
		RateMultiplier:                     1,
		PoolCapacityAlertEnabled:           true,
		PoolCapacityAlertMetric:            PoolCapacityAlertMetricRemainingBalanceUSD,
		PoolCapacityAlertThresholdRequests: 123,
		PoolCapacityAlertThresholdUSD:      &thresholdUSD,
		PoolCapacityAlertGeneration:        42,
		MaxReasoningEffort:                 "medium",
		ReasoningEffortMappings:            []ReasoningEffortMapping{{From: "max", To: "xhigh"}},
	}

	snapshot := groupToAuthSnapshot(group)
	if snapshot == nil {
		t.Fatal("expected group snapshot")
	}
	if !snapshot.PoolCapacityAlertEnabled || snapshot.PoolCapacityAlertMetric != PoolCapacityAlertMetricRemainingBalanceUSD || snapshot.PoolCapacityAlertThresholdRequests != 123 || snapshot.PoolCapacityAlertThresholdUSD == nil || *snapshot.PoolCapacityAlertThresholdUSD != thresholdUSD || snapshot.PoolCapacityAlertGeneration != 42 {
		t.Fatalf("pool capacity alert fields not copied to snapshot: %#v", snapshot)
	}
	if snapshot.MaxReasoningEffort != "medium" || len(snapshot.ReasoningEffortMappings) != 1 || snapshot.ReasoningEffortMappings[0].To != "xhigh" {
		t.Fatalf("reasoning effort fields not copied to snapshot: %#v", snapshot)
	}

	restored := groupFromAuthSnapshot(snapshot)
	if restored == nil {
		t.Fatal("expected restored group")
	}
	if !restored.PoolCapacityAlertEnabled || restored.PoolCapacityAlertMetric != PoolCapacityAlertMetricRemainingBalanceUSD || restored.PoolCapacityAlertThresholdRequests != 123 || restored.PoolCapacityAlertThresholdUSD == nil || *restored.PoolCapacityAlertThresholdUSD != thresholdUSD || restored.PoolCapacityAlertGeneration != 42 {
		t.Fatalf("pool capacity alert fields not restored from snapshot: %#v", restored)
	}
	if restored.MaxReasoningEffort != "medium" || len(restored.ReasoningEffortMappings) != 1 || restored.ReasoningEffortMappings[0].To != "xhigh" {
		t.Fatalf("reasoning effort fields not restored from snapshot: %#v", restored)
	}
}
