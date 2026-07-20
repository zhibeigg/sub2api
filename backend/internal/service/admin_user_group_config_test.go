package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type userGroupConfigUserRepoStub struct {
	UserRepository
	user                 *User
	updatedMode          string
	updatedRestrictedIDs []int64
	updatedExclusiveIDs  []int64
	updatedRates         map[int64]*float64
	rateRepo             *userGroupConfigRateRepoStub
}

func (r *userGroupConfigUserRepoStub) GetByID(_ context.Context, id int64) (*User, error) {
	if r.user == nil || r.user.ID != id {
		return nil, ErrUserNotFound
	}
	clone := *r.user
	clone.AllowedGroups = append([]int64(nil), r.user.AllowedGroups...)
	clone.GroupAccessGroups = append([]int64(nil), r.user.GroupAccessGroups...)
	return &clone, nil
}

func (r *userGroupConfigUserRepoStub) UpdateUserGroupConfig(
	_ context.Context,
	userID int64,
	accessMode string,
	restrictedGroupIDs, exclusiveGroupIDs []int64,
	groupRates map[int64]*float64,
) error {
	r.updatedMode = accessMode
	r.updatedRestrictedIDs = append([]int64(nil), restrictedGroupIDs...)
	r.updatedExclusiveIDs = append([]int64(nil), exclusiveGroupIDs...)
	r.updatedRates = groupRates
	r.user.GroupAccessMode = accessMode
	r.user.GroupAccessGroups = append([]int64(nil), restrictedGroupIDs...)
	r.user.AllowedGroups = append([]int64(nil), exclusiveGroupIDs...)
	if r.rateRepo != nil {
		if r.rateRepo.rates == nil {
			r.rateRepo.rates = make(map[int64]float64)
		}
		for groupID, rate := range groupRates {
			if rate == nil {
				delete(r.rateRepo.rates, groupID)
			} else {
				r.rateRepo.rates[groupID] = *rate
			}
		}
	}
	return nil
}

type userGroupConfigGroupRepoStub struct {
	GroupRepository
	groups []Group
}

func (r *userGroupConfigGroupRepoStub) ListActive(context.Context) ([]Group, error) {
	return append([]Group(nil), r.groups...), nil
}

type userGroupConfigRateRepoStub struct {
	UserGroupRateRepository
	rates map[int64]float64
}

func (r *userGroupConfigRateRepoStub) GetByUserID(context.Context, int64) (map[int64]float64, error) {
	out := make(map[int64]float64, len(r.rates))
	for groupID, rate := range r.rates {
		out[groupID] = rate
	}
	return out, nil
}

type userGroupConfigInvalidatorStub struct {
	userIDs []int64
}

func (*userGroupConfigInvalidatorStub) InvalidateAuthCacheByKey(context.Context, string) {}
func (s *userGroupConfigInvalidatorStub) InvalidateAuthCacheByUserID(_ context.Context, userID int64) {
	s.userIDs = append(s.userIDs, userID)
}
func (*userGroupConfigInvalidatorStub) InvalidateAuthCacheByGroupID(context.Context, int64) {}

func TestAdminServiceGetUserGroupConfigNormalizesAndSorts(t *testing.T) {
	rateRepo := &userGroupConfigRateRepoStub{rates: map[int64]float64{2: 1.25}}
	svc := &adminServiceImpl{
		userRepo: &userGroupConfigUserRepoStub{user: &User{
			ID:                9,
			GroupAccessMode:   "unknown",
			GroupAccessGroups: []int64{3, 1, 3},
			AllowedGroups:     []int64{5, 2, 5},
		}},
		userGroupRateRepo: rateRepo,
	}

	config, err := svc.GetUserGroupConfig(context.Background(), 9)
	require.NoError(t, err)
	require.Equal(t, GroupAccessModeInherit, config.AccessMode)
	require.Equal(t, []int64{1, 3}, config.RestrictedGroupIDs)
	require.Equal(t, []int64{2, 5}, config.ExclusiveGroupIDs)
	require.Equal(t, map[int64]float64{2: 1.25}, config.GroupRates)
}

func TestAdminServiceUpdateUserGroupConfigPersistsAtomicPayloadAndInvalidatesCache(t *testing.T) {
	rate := 1.25
	rateRepo := &userGroupConfigRateRepoStub{rates: map[int64]float64{1: 0.8}}
	userRepo := &userGroupConfigUserRepoStub{
		user:     &User{ID: 9, GroupAccessMode: GroupAccessModeInherit},
		rateRepo: rateRepo,
	}
	invalidator := &userGroupConfigInvalidatorStub{}
	svc := &adminServiceImpl{
		userRepo: userRepo,
		groupRepo: &userGroupConfigGroupRepoStub{groups: []Group{
			{ID: 1, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard},
			{ID: 2, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, IsExclusive: true},
			{ID: 3, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription},
		}},
		userGroupRateRepo:    rateRepo,
		authCacheInvalidator: invalidator,
	}

	config, err := svc.UpdateUserGroupConfig(context.Background(), 9, &UpdateUserGroupConfigInput{
		AccessMode:         GroupAccessModeRestricted,
		RestrictedGroupIDs: []int64{2, 1, 2},
		ExclusiveGroupIDs:  []int64{2, 2},
		GroupRates:         map[int64]*float64{1: nil, 2: &rate},
	})
	require.NoError(t, err)
	require.Equal(t, GroupAccessModeRestricted, userRepo.updatedMode)
	require.Equal(t, []int64{1, 2}, userRepo.updatedRestrictedIDs)
	require.Equal(t, []int64{2}, userRepo.updatedExclusiveIDs)
	require.Contains(t, userRepo.updatedRates, int64(1))
	require.Nil(t, userRepo.updatedRates[1])
	require.Equal(t, []int64{9}, invalidator.userIDs)
	require.Equal(t, GroupAccessModeRestricted, config.AccessMode)
	require.Equal(t, []int64{1, 2}, config.RestrictedGroupIDs)
	require.Equal(t, []int64{2}, config.ExclusiveGroupIDs)
	require.Equal(t, map[int64]float64{2: rate}, config.GroupRates)
}

func TestAdminServiceUpdateUserGroupConfigRejectsInvalidGroupCombinations(t *testing.T) {
	newService := func() *adminServiceImpl {
		return &adminServiceImpl{
			userRepo: &userGroupConfigUserRepoStub{user: &User{ID: 9}},
			groupRepo: &userGroupConfigGroupRepoStub{groups: []Group{
				{ID: 1, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard},
				{ID: 2, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard, IsExclusive: true},
				{ID: 3, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription},
			}},
		}
	}

	_, err := newService().UpdateUserGroupConfig(context.Background(), 9, &UpdateUserGroupConfigInput{
		AccessMode:         GroupAccessModeRestricted,
		RestrictedGroupIDs: []int64{3},
	})
	require.Error(t, err)

	_, err = newService().UpdateUserGroupConfig(context.Background(), 9, &UpdateUserGroupConfigInput{
		AccessMode:         GroupAccessModeRestricted,
		RestrictedGroupIDs: []int64{2},
	})
	require.Error(t, err)
}
