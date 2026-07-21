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

func TestAPIKeyService_RejectsV19AuthSnapshotWithoutPoolCapacityAlertGeneration(t *testing.T) {
	svc := &APIKeyService{}

	apiKey, ok, err := svc.applyAuthCacheEntry("k-legacy-pool-capacity-alert", &APIKeyAuthCacheEntry{
		Snapshot: &APIKeyAuthSnapshot{
			Version:  19,
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
		t.Fatal("expected v19 auth snapshot to be rejected after pool capacity alert fields were added")
	}
	if apiKey != nil {
		t.Fatalf("expected no API key from stale snapshot, got %#v", apiKey)
	}
}

func TestGroupPoolCapacityAlertAuthSnapshotRoundTrip(t *testing.T) {
	group := &Group{
		ID:                          26,
		Name:                        "pool-alert",
		Platform:                    PlatformAnthropic,
		Status:                      StatusActive,
		SubscriptionType:            SubscriptionTypeStandard,
		RateMultiplier:              1,
		PoolCapacityAlertEnabled:    true,
		PoolCapacityAlertGeneration: 42,
	}

	snapshot := groupToAuthSnapshot(group)
	if snapshot == nil {
		t.Fatal("expected group snapshot")
	}
	if !snapshot.PoolCapacityAlertEnabled || snapshot.PoolCapacityAlertGeneration != 42 {
		t.Fatalf("pool capacity alert fields not copied to snapshot: %#v", snapshot)
	}

	restored := groupFromAuthSnapshot(snapshot)
	if restored == nil {
		t.Fatal("expected restored group")
	}
	if !restored.PoolCapacityAlertEnabled || restored.PoolCapacityAlertGeneration != 42 {
		t.Fatalf("pool capacity alert fields not restored from snapshot: %#v", restored)
	}
}
