package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUserStandardGroupRestrictionSemantics(t *testing.T) {
	user := &User{AllowedGroups: []int64{2}}

	require.True(t, user.CanBindGroup(1, false), "inherit mode keeps public standard groups available")
	require.True(t, user.CanBindGroup(2, true), "inherit mode keeps legacy exclusive grants")
	require.False(t, user.CanBindGroup(3, true), "inherit mode does not grant new exclusive groups")

	user.GroupAccessMode = GroupAccessModeRestricted
	user.GroupAccessGroups = []int64{1, 2}
	require.True(t, user.CanBindGroup(1, false))
	require.True(t, user.CanBindGroup(2, true))
	require.False(t, user.CanBindGroup(3, false))

	user.GroupAccessGroups = nil
	require.False(t, user.CanBindGroup(1, false), "an empty restricted allowlist denies every standard group")
}

func TestAPIKeyRestrictionPolicyComposesWithGroupTypeAndExclusiveGrant(t *testing.T) {
	user := &User{
		GroupAccessMode:   GroupAccessModeRestricted,
		GroupAccessGroups: []int64{1, 2},
		AllowedGroups:     []int64{2},
	}
	key := &APIKey{User: user}

	require.True(t, key.AllowsGroupByUserRestriction(&Group{ID: 1, SubscriptionType: SubscriptionTypeStandard}))
	require.True(t, key.AllowsGroupByUserRestriction(&Group{ID: 2, IsExclusive: true, SubscriptionType: SubscriptionTypeStandard}))
	require.False(t, key.AllowsGroupByUserRestriction(&Group{ID: 3, SubscriptionType: SubscriptionTypeStandard}))
	require.False(t, key.AllowsGroupByUserRestriction(&Group{ID: 1, IsExclusive: true, SubscriptionType: SubscriptionTypeStandard}), "restricted exclusive groups still require the legacy grant")
	require.True(t, key.AllowsGroupByUserRestriction(&Group{ID: 99, SubscriptionType: SubscriptionTypeSubscription}), "subscription groups are outside the standard-group restriction")
}

func TestAPIKeyRestrictionPolicySurvivesAuthSnapshotRoundTrip(t *testing.T) {
	svc := &APIKeyService{}
	apiKey := &APIKey{
		ID:     1,
		UserID: 2,
		Key:    "restricted-key",
		Status: StatusActive,
		User: &User{
			ID:                2,
			Status:            StatusActive,
			Role:              RoleUser,
			AllowedGroups:     []int64{7},
			GroupAccessMode:   GroupAccessModeRestricted,
			GroupAccessGroups: []int64{5, 7},
		},
	}

	snapshot := svc.snapshotFromAPIKey(context.Background(), apiKey)
	require.NotNil(t, snapshot)
	require.Equal(t, GroupAccessModeRestricted, snapshot.User.GroupAccessMode)
	require.Equal(t, []int64{5, 7}, snapshot.User.GroupAccessGroups)

	restored := svc.snapshotToAPIKey(apiKey.Key, snapshot)
	require.NotNil(t, restored)
	require.Equal(t, GroupAccessModeRestricted, restored.User.GroupAccessMode)
	require.Equal(t, []int64{5, 7}, restored.User.GroupAccessGroups)
	require.Equal(t, []int64{7}, restored.User.AllowedGroups)
}

func TestMultiGroupResolversSkipRestrictedStandardGroups(t *testing.T) {
	blocked := &Group{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard}
	allowed := &Group{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard}
	user := &User{GroupAccessMode: GroupAccessModeRestricted, GroupAccessGroups: []int64{2}}
	key := &APIKey{User: user, GroupBindings: []APIKeyGroupBinding{
		{GroupID: blocked.ID, Priority: 0, Group: blocked},
		{GroupID: allowed.ID, Priority: 1, Group: allowed},
	}}
	repo := schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: []Account{
		{ID: 10, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, GroupIDs: []int64{blocked.ID}},
		{ID: 20, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, GroupIDs: []int64{allowed.ID}},
	}}}
	svc := &OpenAIGatewayService{accountRepo: repo}

	require.Same(t, allowed, svc.ResolveEffectiveGroupBinding(context.Background(), key, "gpt-5.4"))

	key.GroupBindings = key.GroupBindings[:1]
	require.False(t, key.HasAllowedGroupBindingByUserRestriction())
	require.Nil(t, svc.ResolveEffectiveGroupBinding(context.Background(), key, "gpt-5.4"))
}

func TestImageMultiGroupResolverSkipsRestrictedStandardGroups(t *testing.T) {
	blocked := &Group{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, AllowImageGeneration: true}
	allowed := &Group{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, AllowImageGeneration: true}
	repo := schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: []Account{
		{ID: 10, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, GroupIDs: []int64{blocked.ID}},
		{ID: 20, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, GroupIDs: []int64{allowed.ID}},
	}}}
	svc := &OpenAIGatewayService{accountRepo: repo}
	key := &APIKey{
		User: &User{GroupAccessMode: GroupAccessModeRestricted, GroupAccessGroups: []int64{allowed.ID}},
		GroupBindings: []APIKeyGroupBinding{
			{GroupID: blocked.ID, Priority: 0, Group: blocked},
			{GroupID: allowed.ID, Priority: 1, Group: allowed},
		},
	}

	selected := svc.ResolveEffectiveImageGroupBinding(context.Background(), key, "dall-e-3", openAIImagesGenerationsEndpoint, OpenAIImagesCapabilityNative)
	require.Same(t, allowed, selected)
}
