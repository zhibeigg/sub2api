//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type availableGroupsUserRepoStub struct {
	UserRepository
	user *User
}

func (s *availableGroupsUserRepoStub) GetByID(context.Context, int64) (*User, error) {
	return s.user, nil
}

type availableGroupsGroupRepoStub struct {
	GroupRepository
	groups []Group
}

func (s *availableGroupsGroupRepoStub) ListActive(context.Context) ([]Group, error) {
	return append([]Group(nil), s.groups...), nil
}

type availableGroupsSubscriptionRepoStub struct {
	UserSubscriptionRepository
	subscriptions []UserSubscription
}

func (s *availableGroupsSubscriptionRepoStub) ListActiveByUserID(context.Context, int64) ([]UserSubscription, error) {
	return append([]UserSubscription(nil), s.subscriptions...), nil
}

func TestAPIKeyServiceGetAvailableGroups_IncludesSharedSubscriptionGroups(t *testing.T) {
	user := &User{ID: 9, Status: StatusActive, AllowedGroups: nil}
	groups := []Group{
		{ID: 1, Name: "public-standard", Status: StatusActive, SubscriptionType: SubscriptionTypeStandard},
		{ID: 2, Name: "exclusive-standard-from-plan", Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, IsExclusive: true},
		{ID: 3, Name: "subscription-from-plan", Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription, IsExclusive: true},
		{ID: 4, Name: "exclusive-standard-no-subscription", Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, IsExclusive: true},
		{ID: 5, Name: "subscription-no-subscription", Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription},
	}
	subscriptions := []UserSubscription{
		{ID: 100, UserID: user.ID, GroupID: 2, GroupIDs: []int64{2, 3}, Status: SubscriptionStatusActive},
	}

	svc := NewAPIKeyService(
		nil,
		&availableGroupsUserRepoStub{user: user},
		&availableGroupsGroupRepoStub{groups: groups},
		&availableGroupsSubscriptionRepoStub{subscriptions: subscriptions},
		nil,
		nil,
		&config.Config{},
	)

	available, err := svc.GetAvailableGroups(context.Background(), user.ID)
	require.NoError(t, err)

	ids := make([]int64, 0, len(available))
	for _, group := range available {
		ids = append(ids, group.ID)
	}
	require.Equal(t, []int64{1, 2, 3}, ids)
}
