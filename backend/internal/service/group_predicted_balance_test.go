package service

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

type groupBalanceAccountRepoStub struct {
	AccountRepository
	accounts []Account
	err      error
}

func (r groupBalanceAccountRepoStub) ListByGroup(context.Context, int64) ([]Account, error) {
	return append([]Account(nil), r.accounts...), r.err
}

type groupBalanceGroupRepoStub struct {
	GroupRepository
	group *Group
	err   error
}

func (r groupBalanceGroupRepoStub) GetByIDLite(context.Context, int64) (*Group, error) {
	if r.group == nil {
		return nil, r.err
	}
	group := *r.group
	group.PredictedImageUnitCostUSD = cloneGroupValuePointer(r.group.PredictedImageUnitCostUSD)
	return &group, r.err
}

type groupBalancePoolReaderStub struct {
	callsMu   sync.Mutex
	snapshots map[int64]*AccountCapacitySnapshot
	errors    map[int64]error
	calls     map[int64]int
}

func (r *groupBalancePoolReaderStub) GetPoolBalance(_ context.Context, account *Account, _ bool) (*AccountCapacitySnapshot, error) {
	r.callsMu.Lock()
	if r.calls == nil {
		r.calls = make(map[int64]int)
	}
	r.calls[account.ID]++
	r.callsMu.Unlock()
	if err := r.errors[account.ID]; err != nil {
		return nil, err
	}
	return cloneAccountCapacitySnapshot(r.snapshots[account.ID]), nil
}

type groupBalanceUsageReaderStub struct {
	callsMu   sync.Mutex
	snapshots map[int64]*AccountCapacitySnapshot
	errors    map[int64]error
	calls     map[int64]int
}

func (r *groupBalanceUsageReaderStub) GetCapacityForAggregation(_ context.Context, account *Account) (*AccountCapacitySnapshot, error) {
	r.callsMu.Lock()
	if r.calls == nil {
		r.calls = make(map[int64]int)
	}
	r.calls[account.ID]++
	r.callsMu.Unlock()
	if err := r.errors[account.ID]; err != nil {
		return nil, err
	}
	return cloneAccountCapacitySnapshot(r.snapshots[account.ID]), nil
}

func TestGroupPredictedBalanceServiceMixedAccounts(t *testing.T) {
	now := time.Date(2026, time.July, 22, 2, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Minute)
	poolOne := 10.5
	poolTwo := 4.5
	normalRequests := int64(100)
	localRequests := int64(60)
	normalAverage := 0.02
	localRemaining := 3.0

	accounts := []Account{
		{ID: 1, Status: StatusActive, Schedulable: true, Type: AccountTypeAPIKey, Credentials: map[string]any{"pool_mode": true}},
		{ID: 2, Status: StatusActive, Schedulable: true, Type: AccountTypeAPIKey, Credentials: map[string]any{"pool_mode": true}},
		{ID: 3, Status: StatusActive, Schedulable: true, Type: AccountTypeOAuth},
		{ID: 4, Status: StatusActive, Schedulable: true, Type: AccountTypeOAuth},
		{ID: 5, Status: StatusActive, Schedulable: false, Type: AccountTypeOAuth},
		{ID: 6, Status: StatusActive, Schedulable: true, AutoPauseOnExpired: true, ExpiresAt: &expired, Type: AccountTypeOAuth},
		{ID: 3, Status: StatusActive, Schedulable: true, Type: AccountTypeOAuth},
	}
	poolReader := &groupBalancePoolReaderStub{snapshots: map[int64]*AccountCapacitySnapshot{
		1: {Mode: AccountCapacityModeUpstreamBalance, State: AccountCapacityStateVerified, Authoritative: true, Remaining: &poolOne, Unit: "USD"},
		2: {Mode: AccountCapacityModeUpstreamBalance, State: AccountCapacityStateVerified, Authoritative: true, Remaining: &poolTwo, Unit: "$"},
	}}
	usageReader := &groupBalanceUsageReaderStub{snapshots: map[int64]*AccountCapacitySnapshot{
		3: {
			Mode:                       AccountCapacityModeUsageWindow,
			State:                      AccountCapacityStateEstimated,
			Unit:                       "requests",
			EstimatedRemainingRequests: &normalRequests,
			AverageCostPerRequest:      &normalAverage,
			SampleRequests:             50,
		},
		4: {
			Mode:                       AccountCapacityModeLocalQuota,
			State:                      AccountCapacityStateEstimated,
			Unit:                       "USD",
			Remaining:                  &localRemaining,
			EstimatedRemainingRequests: &localRequests,
		},
	}}
	svc := NewGroupPredictedBalanceService(groupBalanceAccountRepoStub{accounts: accounts}, nil, poolReader, usageReader, nil)
	svc.now = func() time.Time { return now }

	summary, err := svc.EstimateGroupPredictedBalance(context.Background(), 17)
	require.NoError(t, err)
	require.True(t, summary.Complete)
	require.False(t, summary.Unlimited)
	require.NotNil(t, summary.RemainingBalanceUSD)
	require.InDelta(t, 20, *summary.RemainingBalanceUSD, 1e-12)
	require.NotNil(t, summary.KnownRemainingBalanceUSD)
	require.InDelta(t, 20, *summary.KnownRemainingBalanceUSD, 1e-12)
	require.InDelta(t, 15, summary.PoolAuthoritativeBalanceUSD, 1e-12)
	require.InDelta(t, 5, summary.NormalEstimatedBalanceUSD, 1e-12)
	require.Equal(t, 4, summary.KnownBalanceAccountCount)
	require.Equal(t, 2, summary.PoolAccountCount)
	require.Equal(t, 2, summary.NormalAccountCount)
	require.Equal(t, 2, summary.SkippedAccountCount)
	require.Zero(t, summary.UnknownAccountCount)
	require.False(t, summary.RequestsComplete)
	require.False(t, summary.RequestsUnlimited)
	require.NotNil(t, summary.EstimatedRemainingRequests)
	require.Equal(t, int64(160), *summary.EstimatedRemainingRequests)
	require.Equal(t, 2, summary.KnownRequestAccountCount)
	require.Equal(t, 2, summary.UnknownRequestAccountCount)
	require.Equal(t, PredictedCapacityModeHistoricalRequests, summary.PredictionMode)
	require.Equal(t, "request", summary.PredictionUnit)
	require.True(t, summary.PredictionConfigured)
	require.False(t, summary.PredictionComplete)
	require.False(t, summary.PredictionUnlimited)
	require.NotNil(t, summary.PredictedQuantity)
	require.Equal(t, "160", *summary.PredictedQuantity)
	require.Equal(t, summary.KnownRequestAccountCount, summary.KnownPredictionAccountCount)
	require.Equal(t, summary.UnknownRequestAccountCount, summary.UnknownPredictionAccountCount)
	require.Equal(t, 1, usageReader.calls[3], "duplicate account IDs must be evaluated once")
}

func TestGroupPredictedBalanceServiceIncompleteReasons(t *testing.T) {
	now := time.Date(2026, time.July, 22, 2, 0, 0, 0, time.UTC)
	remaining := 5.0
	accounts := []Account{
		{ID: 1, Status: StatusActive, Schedulable: true, Type: AccountTypeAPIKey, Credentials: map[string]any{"pool_mode": true}},
		{ID: 2, Status: StatusActive, Schedulable: true, Type: AccountTypeAPIKey, Credentials: map[string]any{"pool_mode": true}},
		{ID: 3, Status: StatusActive, Schedulable: true, Type: AccountTypeOAuth},
	}
	poolReader := &groupBalancePoolReaderStub{snapshots: map[int64]*AccountCapacitySnapshot{
		1: {Mode: AccountCapacityModeUpstreamBalance, State: AccountCapacityStateVerified, Authoritative: true, Remaining: &remaining, Unit: "requests"},
		2: {Mode: AccountCapacityModeUpstreamBalance, State: AccountCapacityStateStale, Authoritative: true, Remaining: &remaining, Unit: "USD"},
	}}
	usageReader := &groupBalanceUsageReaderStub{snapshots: map[int64]*AccountCapacitySnapshot{
		3: {Mode: AccountCapacityModeUsageWindow, State: AccountCapacityStateUnknown, Unit: "requests"},
	}}
	svc := NewGroupPredictedBalanceService(groupBalanceAccountRepoStub{accounts: accounts}, nil, poolReader, usageReader, nil)
	svc.now = func() time.Time { return now }

	summary, err := svc.EstimateGroupPredictedBalance(context.Background(), 17)
	require.NoError(t, err)
	require.False(t, summary.Complete)
	require.Nil(t, summary.RemainingBalanceUSD)
	require.Nil(t, summary.KnownRemainingBalanceUSD)
	require.Equal(t, 3, summary.UnknownAccountCount)
	require.Equal(t, 1, summary.StaleAccountCount)
	require.Equal(t, 1, summary.IncompatibleUnitAccountCount)
	require.False(t, summary.RequestsComplete)
	require.Nil(t, summary.EstimatedRemainingRequests)
	require.Equal(t, 3, summary.UnknownRequestAccountCount)
}

func TestGroupPredictedBalanceServiceAuthoritativeUnlimitedWins(t *testing.T) {
	accounts := []Account{
		{ID: 1, Status: StatusActive, Schedulable: true, Type: AccountTypeAPIKey, Credentials: map[string]any{"pool_mode": true}},
		{ID: 2, Status: StatusActive, Schedulable: true, Type: AccountTypeOAuth},
	}
	poolReader := &groupBalancePoolReaderStub{snapshots: map[int64]*AccountCapacitySnapshot{
		1: {Mode: AccountCapacityModeUpstreamBalance, State: AccountCapacityStateUnlimited, Authoritative: true, Unit: "USD"},
	}}
	usageReader := &groupBalanceUsageReaderStub{snapshots: map[int64]*AccountCapacitySnapshot{
		2: {Mode: AccountCapacityModeUsageWindow, State: AccountCapacityStateUnknown, Unit: "requests"},
	}}
	svc := NewGroupPredictedBalanceService(groupBalanceAccountRepoStub{accounts: accounts}, nil, poolReader, usageReader, nil)

	summary, err := svc.EstimateGroupPredictedBalance(context.Background(), 17)
	require.NoError(t, err)
	require.True(t, summary.Complete)
	require.True(t, summary.Unlimited)
	require.Nil(t, summary.RemainingBalanceUSD)
	require.True(t, summary.RequestsComplete)
	require.True(t, summary.RequestsUnlimited)
	require.Nil(t, summary.EstimatedRemainingRequests)
	require.Equal(t, 1, summary.UnknownAccountCount, "diagnostics remain visible even though authoritative infinity determines health")
}

func TestGroupPredictedBalanceServiceAuthoritativeUnlimitedSurvivesOtherReadFailure(t *testing.T) {
	accounts := []Account{
		{ID: 1, Status: StatusActive, Schedulable: true, Type: AccountTypeAPIKey, Credentials: map[string]any{"pool_mode": true}},
		{ID: 2, Status: StatusActive, Schedulable: true, Type: AccountTypeOAuth},
	}
	poolReader := &groupBalancePoolReaderStub{snapshots: map[int64]*AccountCapacitySnapshot{
		1: {Mode: AccountCapacityModeUpstreamBalance, State: AccountCapacityStateUnlimited, Authoritative: true, Unit: "USD"},
	}}
	usageReader := &groupBalanceUsageReaderStub{errors: map[int64]error{2: errors.New("upstream unavailable")}}
	svc := NewGroupPredictedBalanceService(groupBalanceAccountRepoStub{accounts: accounts}, nil, poolReader, usageReader, nil)

	summary, err := svc.EstimateGroupPredictedBalance(context.Background(), 17)
	require.NoError(t, err)
	require.True(t, summary.Complete)
	require.True(t, summary.Unlimited)
	require.Nil(t, summary.RemainingBalanceUSD)
	require.True(t, summary.RequestsComplete)
	require.True(t, summary.RequestsUnlimited)
	require.Equal(t, 1, summary.UnknownAccountCount)
	require.Equal(t, 1, summary.PoolAccountCount)
	require.Equal(t, 1, summary.NormalAccountCount)
}

func TestGroupPredictedBalanceServiceReadFailureDoesNotProducePartialValue(t *testing.T) {
	accounts := []Account{{ID: 1, Status: StatusActive, Schedulable: true, Type: AccountTypeOAuth}}
	usageReader := &groupBalanceUsageReaderStub{errors: map[int64]error{1: errors.New("upstream unavailable")}}
	svc := NewGroupPredictedBalanceService(groupBalanceAccountRepoStub{accounts: accounts}, nil, &groupBalancePoolReaderStub{}, usageReader, nil)

	summary, err := svc.EstimateGroupPredictedBalance(context.Background(), 17)
	require.Error(t, err)
	require.Nil(t, summary)
}

func TestGroupPredictedBalanceServiceCompletesRequestEstimateWhenEveryAccountIsKnown(t *testing.T) {
	poolRemaining := 2.0
	poolRequests := int64(40)
	normalRequests := int64(60)
	normalAverage := 0.03
	accounts := []Account{
		{ID: 1, Status: StatusActive, Schedulable: true, Type: AccountTypeAPIKey, Credentials: map[string]any{"pool_mode": true}},
		{ID: 2, Status: StatusActive, Schedulable: true, Type: AccountTypeOAuth},
	}
	poolReader := &groupBalancePoolReaderStub{snapshots: map[int64]*AccountCapacitySnapshot{
		1: {
			Mode:                       AccountCapacityModeUpstreamBalance,
			State:                      AccountCapacityStateVerified,
			Authoritative:              true,
			Remaining:                  &poolRemaining,
			Unit:                       "USD",
			EstimatedRemainingRequests: &poolRequests,
		},
	}}
	usageReader := &groupBalanceUsageReaderStub{snapshots: map[int64]*AccountCapacitySnapshot{
		2: {
			Mode:                       AccountCapacityModeUsageWindow,
			State:                      AccountCapacityStateEstimated,
			EstimatedRemainingRequests: &normalRequests,
			AverageCostPerRequest:      &normalAverage,
			SampleRequests:             20,
		},
	}}
	svc := NewGroupPredictedBalanceService(groupBalanceAccountRepoStub{accounts: accounts}, nil, poolReader, usageReader, nil)

	summary, err := svc.EstimateGroupPredictedBalance(context.Background(), 17)
	require.NoError(t, err)
	require.True(t, summary.Complete)
	require.True(t, summary.RequestsComplete)
	require.False(t, summary.RequestsUnlimited)
	require.NotNil(t, summary.EstimatedRemainingRequests)
	require.Equal(t, int64(100), *summary.EstimatedRemainingRequests)
	require.Equal(t, 2, summary.KnownRequestAccountCount)
	require.Zero(t, summary.UnknownRequestAccountCount)
}

func TestGroupPredictedBalanceServiceEmptyGroupReturnsFiniteZero(t *testing.T) {
	svc := NewGroupPredictedBalanceService(groupBalanceAccountRepoStub{}, nil, &groupBalancePoolReaderStub{}, &groupBalanceUsageReaderStub{}, nil)

	summary, err := svc.EstimateGroupPredictedBalance(context.Background(), 17)
	require.NoError(t, err)
	require.True(t, summary.Complete)
	require.NotNil(t, summary.RemainingBalanceUSD)
	require.Zero(t, *summary.RemainingBalanceUSD)
	require.NotNil(t, summary.KnownRemainingBalanceUSD)
	require.Zero(t, *summary.KnownRemainingBalanceUSD)
	require.True(t, summary.RequestsComplete)
	require.NotNil(t, summary.EstimatedRemainingRequests)
	require.Zero(t, *summary.EstimatedRemainingRequests)
}

func TestGroupPredictedBalanceServiceExistingEmptyGroupReturnsFiniteZero(t *testing.T) {
	svc := NewGroupPredictedBalanceService(
		groupBalanceAccountRepoStub{},
		groupBalanceGroupRepoStub{group: &Group{ID: 17}},
		&groupBalancePoolReaderStub{},
		&groupBalanceUsageReaderStub{},
		nil,
	)

	summary, err := svc.EstimateGroupPredictedBalance(context.Background(), 17)
	require.NoError(t, err)
	require.True(t, summary.Complete)
	require.NotNil(t, summary.RemainingBalanceUSD)
	require.Zero(t, *summary.RemainingBalanceUSD)
	require.NotNil(t, summary.KnownRemainingBalanceUSD)
	require.Zero(t, *summary.KnownRemainingBalanceUSD)
}

func TestGroupPredictedBalanceServiceMissingGroupReturnsNotFound(t *testing.T) {
	svc := NewGroupPredictedBalanceService(
		groupBalanceAccountRepoStub{err: errors.New("account listing should not run")},
		groupBalanceGroupRepoStub{err: ErrGroupNotFound},
		&groupBalancePoolReaderStub{},
		&groupBalanceUsageReaderStub{},
		nil,
	)

	summary, err := svc.EstimateGroupPredictedBalance(context.Background(), 17)
	require.Nil(t, summary)
	require.ErrorIs(t, err, ErrGroupNotFound)
}

func TestGroupPredictedBalanceServiceNilGroupReturnsNotFound(t *testing.T) {
	svc := NewGroupPredictedBalanceService(
		groupBalanceAccountRepoStub{err: errors.New("account listing should not run")},
		groupBalanceGroupRepoStub{},
		&groupBalancePoolReaderStub{},
		&groupBalanceUsageReaderStub{},
		nil,
	)

	summary, err := svc.EstimateGroupPredictedBalance(context.Background(), 17)
	require.Nil(t, summary)
	require.ErrorIs(t, err, ErrGroupNotFound)
}

func TestGroupPredictedBalanceServiceTransientGroupConfigFailureKeepsLegacyAggregation(t *testing.T) {
	remaining := 5.0
	requests := int64(10)
	svc := NewGroupPredictedBalanceService(
		groupBalanceAccountRepoStub{accounts: []Account{{ID: 1, Status: StatusActive, Schedulable: true, Type: AccountTypeOAuth}}},
		groupBalanceGroupRepoStub{err: errors.New("temporary group lookup failure")},
		&groupBalancePoolReaderStub{},
		&groupBalanceUsageReaderStub{snapshots: map[int64]*AccountCapacitySnapshot{
			1: {
				Mode:                       AccountCapacityModeLocalQuota,
				State:                      AccountCapacityStateEstimated,
				Unit:                       "USD",
				Remaining:                  &remaining,
				EstimatedRemainingRequests: &requests,
			},
		}},
		nil,
	)

	summary, err := svc.EstimateGroupPredictedBalance(context.Background(), 17)
	require.NoError(t, err)
	require.True(t, summary.Complete)
	require.Equal(t, PredictedCapacityModeHistoricalRequests, summary.PredictionMode)
	require.False(t, summary.PredictionConfigured)
	require.Equal(t, "10", *summary.PredictedQuantity)
	require.Equal(t, int64(10), *summary.EstimatedRemainingRequests)
	require.InDelta(t, remaining, *summary.RemainingBalanceUSD, 1e-12)
}

func TestGroupPredictedBalanceServiceFixedImageCompleteAndLegacyRequestsCoexist(t *testing.T) {
	unitCost := 2.5
	poolBalance := 12.5
	normalBalance := 5.0
	poolRequests := int64(3)
	normalRequests := int64(4)
	accounts := []Account{
		{ID: 1, Status: StatusActive, Schedulable: true, Type: AccountTypeAPIKey, Credentials: map[string]any{"pool_mode": true}},
		{ID: 2, Status: StatusActive, Schedulable: true, Type: AccountTypeOAuth},
	}
	groupRepo := groupBalanceGroupRepoStub{group: &Group{
		PredictedCapacityMode:     PredictedCapacityModeFixedImageCost,
		PredictedImageUnitCostUSD: &unitCost,
	}}
	poolReader := &groupBalancePoolReaderStub{snapshots: map[int64]*AccountCapacitySnapshot{
		1: {
			Mode:                       AccountCapacityModeUpstreamBalance,
			State:                      AccountCapacityStateVerified,
			Authoritative:              true,
			Remaining:                  &poolBalance,
			Unit:                       "USD",
			EstimatedRemainingRequests: &poolRequests,
		},
	}}
	usageReader := &groupBalanceUsageReaderStub{snapshots: map[int64]*AccountCapacitySnapshot{
		2: {
			Mode:                       AccountCapacityModeLocalQuota,
			State:                      AccountCapacityStateEstimated,
			Remaining:                  &normalBalance,
			Unit:                       "USD",
			EstimatedRemainingRequests: &normalRequests,
		},
	}}
	svc := NewGroupPredictedBalanceService(groupBalanceAccountRepoStub{accounts: accounts}, groupRepo, poolReader, usageReader, nil)

	summary, err := svc.EstimateGroupPredictedBalance(context.Background(), 17)
	require.NoError(t, err)
	require.Equal(t, PredictedCapacityModeFixedImageCost, summary.PredictionMode)
	require.Equal(t, "image", summary.PredictionUnit)
	require.True(t, summary.PredictionConfigured)
	require.True(t, summary.PredictionComplete)
	require.False(t, summary.PredictionUnlimited)
	require.Equal(t, "7", *summary.PredictedQuantity)
	require.Equal(t, 2, summary.KnownPredictionAccountCount)
	require.Zero(t, summary.UnknownPredictionAccountCount)
	require.Equal(t, unitCost, *summary.PredictionUnitCostUSD)

	// 旧 requests_* 字段保留独立语义，不随展示模式切换。
	require.True(t, summary.RequestsComplete)
	require.False(t, summary.RequestsUnlimited)
	require.Equal(t, int64(7), *summary.EstimatedRemainingRequests)
	require.Equal(t, 2, summary.KnownRequestAccountCount)
	require.Zero(t, summary.UnknownRequestAccountCount)
}

func TestGroupPredictedBalanceServiceFixedImagePartialUsesKnownBalanceFloor(t *testing.T) {
	unitCost := 3.0
	knownBalance := 10.0
	knownRequests := int64(9)
	accounts := []Account{
		{ID: 1, Status: StatusActive, Schedulable: true, Type: AccountTypeOAuth},
		{ID: 2, Status: StatusActive, Schedulable: true, Type: AccountTypeOAuth},
	}
	groupRepo := groupBalanceGroupRepoStub{group: &Group{
		PredictedCapacityMode:     PredictedCapacityModeFixedImageCost,
		PredictedImageUnitCostUSD: &unitCost,
	}}
	usageReader := &groupBalanceUsageReaderStub{snapshots: map[int64]*AccountCapacitySnapshot{
		1: {
			Mode:                       AccountCapacityModeLocalQuota,
			State:                      AccountCapacityStateEstimated,
			Remaining:                  &knownBalance,
			Unit:                       "USD",
			EstimatedRemainingRequests: &knownRequests,
		},
		2: {Mode: AccountCapacityModeUsageWindow, State: AccountCapacityStateUnknown, Unit: "requests"},
	}}
	svc := NewGroupPredictedBalanceService(groupBalanceAccountRepoStub{accounts: accounts}, groupRepo, &groupBalancePoolReaderStub{}, usageReader, nil)

	summary, err := svc.EstimateGroupPredictedBalance(context.Background(), 17)
	require.NoError(t, err)
	require.False(t, summary.Complete)
	require.True(t, summary.PredictionConfigured)
	require.False(t, summary.PredictionComplete)
	require.False(t, summary.PredictionUnlimited)
	require.Equal(t, "3", *summary.PredictedQuantity)
	require.Equal(t, 1, summary.KnownPredictionAccountCount)
	require.Equal(t, 1, summary.UnknownPredictionAccountCount)
	require.False(t, summary.RequestsComplete)
	require.Equal(t, int64(9), *summary.EstimatedRemainingRequests)
}

func TestGroupPredictedBalanceServiceFixedImageUnlimited(t *testing.T) {
	unitCost := 0.25
	accounts := []Account{
		{ID: 1, Status: StatusActive, Schedulable: true, Type: AccountTypeAPIKey, Credentials: map[string]any{"pool_mode": true}},
		{ID: 2, Status: StatusActive, Schedulable: true, Type: AccountTypeOAuth},
	}
	groupRepo := groupBalanceGroupRepoStub{group: &Group{
		PredictedCapacityMode:     PredictedCapacityModeFixedImageCost,
		PredictedImageUnitCostUSD: &unitCost,
	}}
	poolReader := &groupBalancePoolReaderStub{snapshots: map[int64]*AccountCapacitySnapshot{
		1: {Mode: AccountCapacityModeUpstreamBalance, State: AccountCapacityStateUnlimited, Authoritative: true, Unit: "USD"},
	}}
	usageReader := &groupBalanceUsageReaderStub{snapshots: map[int64]*AccountCapacitySnapshot{
		2: {Mode: AccountCapacityModeUsageWindow, State: AccountCapacityStateUnknown, Unit: "requests"},
	}}
	svc := NewGroupPredictedBalanceService(groupBalanceAccountRepoStub{accounts: accounts}, groupRepo, poolReader, usageReader, nil)

	summary, err := svc.EstimateGroupPredictedBalance(context.Background(), 17)
	require.NoError(t, err)
	require.True(t, summary.Unlimited)
	require.True(t, summary.PredictionConfigured)
	require.True(t, summary.PredictionComplete)
	require.True(t, summary.PredictionUnlimited)
	require.Nil(t, summary.PredictedQuantity)
}

func TestGroupPredictedBalanceServiceFixedImageInvalidConfigIsUnconfigured(t *testing.T) {
	remaining := 8.0
	requests := int64(16)
	accounts := []Account{{ID: 1, Status: StatusActive, Schedulable: true, Type: AccountTypeOAuth}}
	groupRepo := groupBalanceGroupRepoStub{group: &Group{PredictedCapacityMode: PredictedCapacityModeFixedImageCost}}
	usageReader := &groupBalanceUsageReaderStub{snapshots: map[int64]*AccountCapacitySnapshot{
		1: {
			Mode:                       AccountCapacityModeLocalQuota,
			State:                      AccountCapacityStateEstimated,
			Remaining:                  &remaining,
			Unit:                       "USD",
			EstimatedRemainingRequests: &requests,
		},
	}}
	svc := NewGroupPredictedBalanceService(groupBalanceAccountRepoStub{accounts: accounts}, groupRepo, &groupBalancePoolReaderStub{}, usageReader, nil)

	summary, err := svc.EstimateGroupPredictedBalance(context.Background(), 17)
	require.NoError(t, err)
	require.True(t, summary.Complete)
	require.Equal(t, "image", summary.PredictionUnit)
	require.False(t, summary.PredictionConfigured)
	require.False(t, summary.PredictionComplete)
	require.False(t, summary.PredictionUnlimited)
	require.Nil(t, summary.PredictedQuantity)
	require.True(t, summary.RequestsComplete)
	require.Equal(t, int64(16), *summary.EstimatedRemainingRequests)
}

func TestGroupPredictedBalanceServiceFixedImageEmptyGroupReturnsZero(t *testing.T) {
	unitCost := 0.25
	groupRepo := groupBalanceGroupRepoStub{group: &Group{
		PredictedCapacityMode:     PredictedCapacityModeFixedImageCost,
		PredictedImageUnitCostUSD: &unitCost,
	}}
	svc := NewGroupPredictedBalanceService(groupBalanceAccountRepoStub{}, groupRepo, &groupBalancePoolReaderStub{}, &groupBalanceUsageReaderStub{}, nil)

	summary, err := svc.EstimateGroupPredictedBalance(context.Background(), 17)
	require.NoError(t, err)
	require.True(t, summary.Complete)
	require.True(t, summary.PredictionConfigured)
	require.True(t, summary.PredictionComplete)
	require.False(t, summary.PredictionUnlimited)
	require.Equal(t, "0", *summary.PredictedQuantity)
	require.Zero(t, summary.KnownPredictionAccountCount)
	require.Zero(t, summary.UnknownPredictionAccountCount)
}

func TestFinalizeGroupPredictionSummaryUsesArbitraryPrecisionDecimalFloor(t *testing.T) {
	unitCost := 0.000000000001
	summary := &GroupPredictedBalanceSummary{}
	knownBalance := decimal.RequireFromString("123456789012345678901234567890.999999999999")

	finalizeGroupPredictionSummary(summary, PredictedCapacityModeFixedImageCost, &unitCost, true, knownBalance)

	require.True(t, summary.PredictionConfigured)
	require.True(t, summary.PredictionComplete)
	require.Equal(t, "123456789012345678901234567890999999999999", *summary.PredictedQuantity)
}

func TestGroupPredictedBalanceServiceAggregateBalanceOverflowStaysIncomplete(t *testing.T) {
	unitCost := 1.0
	remaining := math.MaxFloat64
	requests := int64(1)
	accounts := []Account{
		{ID: 1, Status: StatusActive, Schedulable: true, Type: AccountTypeOAuth},
		{ID: 2, Status: StatusActive, Schedulable: true, Type: AccountTypeOAuth},
	}
	groupRepo := groupBalanceGroupRepoStub{group: &Group{
		PredictedCapacityMode:     PredictedCapacityModeFixedImageCost,
		PredictedImageUnitCostUSD: &unitCost,
	}}
	usageReader := &groupBalanceUsageReaderStub{snapshots: map[int64]*AccountCapacitySnapshot{
		1: {Mode: AccountCapacityModeLocalQuota, State: AccountCapacityStateEstimated, Unit: "USD", Remaining: &remaining, EstimatedRemainingRequests: &requests},
		2: {Mode: AccountCapacityModeLocalQuota, State: AccountCapacityStateEstimated, Unit: "USD", Remaining: &remaining, EstimatedRemainingRequests: &requests},
	}}
	svc := NewGroupPredictedBalanceService(groupBalanceAccountRepoStub{accounts: accounts}, groupRepo, &groupBalancePoolReaderStub{}, usageReader, nil)

	summary, err := svc.EstimateGroupPredictedBalance(context.Background(), 17)
	require.NoError(t, err)
	require.False(t, summary.Complete)
	require.Nil(t, summary.RemainingBalanceUSD)
	require.Nil(t, summary.KnownRemainingBalanceUSD)
	require.False(t, math.IsInf(summary.NormalEstimatedBalanceUSD, 0))
	require.Equal(t, 2, summary.KnownBalanceAccountCount)
	require.Zero(t, summary.UnknownAccountCount)
	require.True(t, summary.PredictionConfigured)
	require.True(t, summary.PredictionComplete)
	require.NotNil(t, summary.PredictedQuantity)
	require.Greater(t, len(*summary.PredictedQuantity), 300)
}

func TestGroupPredictedBalanceServiceUsageWindowBalanceOverflowStaysIncomplete(t *testing.T) {
	unitCost := 1.0
	requests := int64(math.MaxInt64)
	average := math.MaxFloat64
	accounts := []Account{{ID: 1, Status: StatusActive, Schedulable: true, Type: AccountTypeOAuth}}
	groupRepo := groupBalanceGroupRepoStub{group: &Group{
		PredictedCapacityMode:     PredictedCapacityModeFixedImageCost,
		PredictedImageUnitCostUSD: &unitCost,
	}}
	usageReader := &groupBalanceUsageReaderStub{snapshots: map[int64]*AccountCapacitySnapshot{
		1: {
			Mode:                       AccountCapacityModeUsageWindow,
			State:                      AccountCapacityStateEstimated,
			EstimatedRemainingRequests: &requests,
			AverageCostPerRequest:      &average,
			SampleRequests:             1,
		},
	}}
	svc := NewGroupPredictedBalanceService(groupBalanceAccountRepoStub{accounts: accounts}, groupRepo, &groupBalancePoolReaderStub{}, usageReader, nil)

	summary, err := svc.EstimateGroupPredictedBalance(context.Background(), 17)
	require.NoError(t, err)
	require.False(t, summary.Complete)
	require.Nil(t, summary.RemainingBalanceUSD)
	require.Nil(t, summary.KnownRemainingBalanceUSD)
	require.False(t, math.IsInf(summary.NormalEstimatedBalanceUSD, 0))
	require.True(t, summary.PredictionComplete)
	require.NotNil(t, summary.PredictedQuantity)
	require.Greater(t, len(*summary.PredictedQuantity), 320)
	require.True(t, summary.RequestsComplete)
	require.Equal(t, int64(math.MaxInt64), *summary.EstimatedRemainingRequests)
}

func TestGroupPredictedBalanceServiceRequestOverflowBecomesPartial(t *testing.T) {
	remaining := 1.0
	maxRequests := int64(math.MaxInt64)
	oneRequest := int64(1)
	accounts := []Account{
		{ID: 1, Status: StatusActive, Schedulable: true, Type: AccountTypeOAuth},
		{ID: 2, Status: StatusActive, Schedulable: true, Type: AccountTypeOAuth},
	}
	usageReader := &groupBalanceUsageReaderStub{snapshots: map[int64]*AccountCapacitySnapshot{
		1: {Mode: AccountCapacityModeLocalQuota, State: AccountCapacityStateEstimated, Unit: "USD", Remaining: &remaining, EstimatedRemainingRequests: &maxRequests},
		2: {Mode: AccountCapacityModeLocalQuota, State: AccountCapacityStateEstimated, Unit: "USD", Remaining: &remaining, EstimatedRemainingRequests: &oneRequest},
	}}
	svc := NewGroupPredictedBalanceService(groupBalanceAccountRepoStub{accounts: accounts}, nil, &groupBalancePoolReaderStub{}, usageReader, nil)

	summary, err := svc.EstimateGroupPredictedBalance(context.Background(), 17)
	require.NoError(t, err)
	require.True(t, summary.Complete)
	require.False(t, summary.RequestsComplete)
	require.NotNil(t, summary.EstimatedRemainingRequests)
	require.Equal(t, int64(math.MaxInt64), *summary.EstimatedRemainingRequests)
	require.Equal(t, 1, summary.KnownRequestAccountCount)
	require.Equal(t, 1, summary.UnknownRequestAccountCount)
}

func TestEstimatedGroupRequestsRejectsInvalidAndLocalUnlimitedValues(t *testing.T) {
	negative := int64(-1)
	requests, known, unlimited := estimatedGroupRequests(&AccountCapacitySnapshot{
		State:                      AccountCapacityStateEstimated,
		EstimatedRemainingRequests: &negative,
	}, false)
	require.Zero(t, requests)
	require.False(t, known)
	require.False(t, unlimited)

	requests, known, unlimited = estimatedGroupRequests(&AccountCapacitySnapshot{State: AccountCapacityStateUnlimited}, false)
	require.Zero(t, requests)
	require.False(t, known)
	require.False(t, unlimited, "local unlimited only means no configured local limit")
}

func TestEstimatedNormalBalanceUSD(t *testing.T) {
	requests := int64(25)
	average := 0.04
	amount, reason, err := estimatedNormalBalanceUSD(&AccountCapacitySnapshot{
		Mode:                       AccountCapacityModeUsageWindow,
		State:                      AccountCapacityStateEstimated,
		EstimatedRemainingRequests: &requests,
		AverageCostPerRequest:      &average,
		SampleRequests:             10,
	})
	require.NoError(t, err)
	require.Empty(t, reason)
	require.InDelta(t, 1, amount.InexactFloat64(), 1e-12)

	local := 2.75
	amount, reason, err = estimatedNormalBalanceUSD(&AccountCapacitySnapshot{
		Mode:      AccountCapacityModeLocalQuota,
		State:     AccountCapacityStateEstimated,
		Unit:      "USD",
		Remaining: &local,
	})
	require.NoError(t, err)
	require.Empty(t, reason)
	require.InDelta(t, local, amount.InexactFloat64(), 1e-12)

	amount, reason, err = estimatedNormalBalanceUSD(&AccountCapacitySnapshot{
		Mode:        AccountCapacityModeLocalQuota,
		State:       AccountCapacityStateUnknown,
		MessageCode: "insufficient_cost_sample",
		Unit:        "USD",
		Remaining:   &local,
	})
	require.NoError(t, err)
	require.Empty(t, reason, "a known local USD remainder stays usable even when request prediction lacks cost samples")
	require.InDelta(t, local, amount.InexactFloat64(), 1e-12)

	amount, reason, err = estimatedNormalBalanceUSD(&AccountCapacitySnapshot{
		Mode:        AccountCapacityModeLocalQuota,
		State:       AccountCapacityStateUnknown,
		MessageCode: "request_estimate_overflow",
		Unit:        "USD",
		Remaining:   &local,
	})
	require.NoError(t, err)
	require.Empty(t, reason, "a request-count overflow must not discard a known local USD remainder")
	require.InDelta(t, local, amount.InexactFloat64(), 1e-12)

	_, reason, err = estimatedNormalBalanceUSD(&AccountCapacitySnapshot{
		Mode:        AccountCapacityModeLocalQuota,
		State:       AccountCapacityStateUnknown,
		MessageCode: "capacity_unavailable",
		Unit:        "USD",
		Remaining:   &local,
	})
	require.NoError(t, err)
	require.Equal(t, GroupBalanceReasonUnknown, reason)

	_, reason, err = estimatedNormalBalanceUSD(&AccountCapacitySnapshot{
		Mode:  AccountCapacityModeLocalQuota,
		State: AccountCapacityStateUnlimited,
		Unit:  "USD",
	})
	require.NoError(t, err)
	require.Equal(t, GroupBalanceReasonUnknown, reason)
}
