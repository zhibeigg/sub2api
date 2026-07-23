package service

import (
	"context"
	"testing"
)

// Round-trip: a key with multi-group priority bindings survives snapshot
// serialization (snapshotFromAPIKey → snapshotToAPIKey) with order + fields.
func TestAPIKeyService_SnapshotRoundTripPreservesGroupBindings(t *testing.T) {
	svc := &APIKeyService{}
	g1 := int64(11)
	g2 := int64(22)
	apiKey := &APIKey{
		ID:     1,
		UserID: 2,
		Key:    "k-multi",
		Status: StatusActive,
		User:   &User{ID: 2, Status: StatusActive, Role: RoleUser},
		GroupBindings: []APIKeyGroupBinding{
			{GroupID: g1, Priority: 0, Group: &Group{ID: g1, Name: "国模", Platform: PlatformOpenAI, EndpointProtocols: []string{string(EndpointProtocolOpenAIChatCompletions), string(EndpointProtocolOpenAIResponses)}, QuotaPlatform: PlatformOpenAI, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 0.4}},
			{GroupID: g2, Priority: 1, Group: &Group{ID: g2, Name: "GPT", Platform: PlatformOpenAI, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 0.2}},
		},
	}

	snap := svc.snapshotFromAPIKey(context.TODO(), apiKey)
	if snap == nil {
		t.Fatal("expected snapshot, got nil")
	}
	if snap.Version != apiKeyAuthSnapshotVersion {
		t.Fatalf("snapshot version = %d, want %d", snap.Version, apiKeyAuthSnapshotVersion)
	}
	if len(snap.GroupBindings) != 2 {
		t.Fatalf("snapshot bindings = %d, want 2", len(snap.GroupBindings))
	}

	restored := svc.snapshotToAPIKey("k-multi", snap)
	if restored == nil {
		t.Fatal("expected restored key, got nil")
	}
	if len(restored.GroupBindings) != 2 {
		t.Fatalf("restored bindings = %d, want 2", len(restored.GroupBindings))
	}
	if restored.GroupBindings[0].GroupID != g1 || restored.GroupBindings[0].Priority != 0 {
		t.Fatalf("binding[0] = %+v, want group %d priority 0", restored.GroupBindings[0], g1)
	}
	if restored.GroupBindings[1].GroupID != g2 || restored.GroupBindings[1].Priority != 1 {
		t.Fatalf("binding[1] = %+v, want group %d priority 1", restored.GroupBindings[1], g2)
	}
	if restored.GroupBindings[0].Group == nil || restored.GroupBindings[0].Group.Name != "国模" {
		t.Fatalf("binding[0] group not restored: %+v", restored.GroupBindings[0].Group)
	}
	if got := restored.GroupBindings[0].Group.EndpointProtocols; len(got) != 2 || got[0] != string(EndpointProtocolOpenAIChatCompletions) || got[1] != string(EndpointProtocolOpenAIResponses) {
		t.Fatalf("binding[0] endpoint protocols not restored: %v", got)
	}
	if got := restored.GroupBindings[0].Group.QuotaPlatform; got != PlatformOpenAI {
		t.Fatalf("binding[0] quota platform = %q, want %q", got, PlatformOpenAI)
	}
}

// A key with no bindings round-trips as legacy single-group (nil bindings).
func TestAPIKeyService_SnapshotSingleGroupNoBindings(t *testing.T) {
	svc := &APIKeyService{}
	gid := int64(7)
	apiKey := &APIKey{
		ID:      1,
		UserID:  2,
		Key:     "k-single",
		Status:  StatusActive,
		GroupID: &gid,
		User:    &User{ID: 2, Status: StatusActive, Role: RoleUser},
		Group:   &Group{ID: gid, Name: "solo", Platform: PlatformOpenAI, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, RateMultiplier: 1},
	}
	snap := svc.snapshotFromAPIKey(context.TODO(), apiKey)
	if len(snap.GroupBindings) != 0 {
		t.Fatalf("expected no bindings for single-group key, got %d", len(snap.GroupBindings))
	}
	restored := svc.snapshotToAPIKey("k-single", snap)
	if len(restored.GroupBindings) != 0 {
		t.Fatalf("expected no bindings restored, got %d", len(restored.GroupBindings))
	}
	if restored.GroupID == nil || *restored.GroupID != gid {
		t.Fatalf("single group id lost: %+v", restored.GroupID)
	}
}

func TestEffectiveGroupIDFromBindings(t *testing.T) {
	fb := int64(99)
	if got := effectiveGroupIDFromBindings(nil, &fb); got == nil || *got != 99 {
		t.Fatalf("no bindings should return fallback, got %v", got)
	}
	bindings := []APIKeyGroupBinding{{GroupID: 5, Priority: 0}, {GroupID: 6, Priority: 1}}
	if got := effectiveGroupIDFromBindings(bindings, &fb); got == nil || *got != 5 {
		t.Fatalf("bindings should return highest-priority group 5, got %v", got)
	}
}
