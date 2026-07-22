package service

import (
	"context"
	"errors"
	"testing"
	"time"

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

type groupBalancePoolReaderStub struct {
	snapshots map[int64]*AccountCapacitySnapshot
	errors    map[int64]error
	calls     map[int64]int
}

func (r *groupBalancePoolReaderStub) GetPoolBalance(_ context.Context, account *Account, _ bool) (*AccountCapacitySnapshot, error) {
	if r.calls == nil {
		r.calls = make(map[int64]int)
	}
	r.calls[account.ID]++
	if err := r.errors[account.ID]; err != nil {
		return nil, err
	}
	return cloneAccountCapacitySnapshot(r.snapshots[account.ID]), nil
}

type groupBalanceUsageReaderStub struct {
	snapshots map[int64]*AccountCapacitySnapshot
	errors    map[int64]error
	calls     map[int64]int
}

func (r *groupBalanceUsageReaderStub) GetCapacityForAggregation(_ context.Context, account *Account) (*AccountCapacitySnapshot, error) {
	if r.calls == nil {
		r.calls = make(map[int64]int)
	}
	r.calls[account.ID]++
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
		4: {Mode: AccountCapacityModeLocalQuota, State: AccountCapacityStateEstimated, Unit: "USD", Remaining: &localRemaining},
	}}
	svc := NewGroupPredictedBalanceService(groupBalanceAccountRepoStub{accounts: accounts}, poolReader, usageReader, nil)
	svc.now = func() time.Time { return now }

	summary, err := svc.EstimateGroupPredictedBalance(context.Background(), 17)
	require.NoError(t, err)
	require.True(t, summary.Complete)
	require.False(t, summary.Unlimited)
	require.NotNil(t, summary.RemainingBalanceUSD)
	require.InDelta(t, 20, *summary.RemainingBalanceUSD, 1e-12)
	require.InDelta(t, 15, summary.PoolAuthoritativeBalanceUSD, 1e-12)
	require.InDelta(t, 5, summary.NormalEstimatedBalanceUSD, 1e-12)
	require.Equal(t, 2, summary.PoolAccountCount)
	require.Equal(t, 2, summary.NormalAccountCount)
	require.Equal(t, 2, summary.SkippedAccountCount)
	require.Zero(t, summary.UnknownAccountCount)
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
	svc := NewGroupPredictedBalanceService(groupBalanceAccountRepoStub{accounts: accounts}, poolReader, usageReader, nil)
	svc.now = func() time.Time { return now }

	summary, err := svc.EstimateGroupPredictedBalance(context.Background(), 17)
	require.NoError(t, err)
	require.False(t, summary.Complete)
	require.Nil(t, summary.RemainingBalanceUSD)
	require.Equal(t, 3, summary.UnknownAccountCount)
	require.Equal(t, 1, summary.StaleAccountCount)
	require.Equal(t, 1, summary.IncompatibleUnitAccountCount)
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
	svc := NewGroupPredictedBalanceService(groupBalanceAccountRepoStub{accounts: accounts}, poolReader, usageReader, nil)

	summary, err := svc.EstimateGroupPredictedBalance(context.Background(), 17)
	require.NoError(t, err)
	require.True(t, summary.Complete)
	require.True(t, summary.Unlimited)
	require.Nil(t, summary.RemainingBalanceUSD)
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
	svc := NewGroupPredictedBalanceService(groupBalanceAccountRepoStub{accounts: accounts}, poolReader, usageReader, nil)

	summary, err := svc.EstimateGroupPredictedBalance(context.Background(), 17)
	require.NoError(t, err)
	require.True(t, summary.Complete)
	require.True(t, summary.Unlimited)
	require.Nil(t, summary.RemainingBalanceUSD)
	require.Equal(t, 1, summary.UnknownAccountCount)
	require.Equal(t, 1, summary.PoolAccountCount)
	require.Equal(t, 1, summary.NormalAccountCount)
}

func TestGroupPredictedBalanceServiceReadFailureDoesNotProducePartialValue(t *testing.T) {
	accounts := []Account{{ID: 1, Status: StatusActive, Schedulable: true, Type: AccountTypeOAuth}}
	usageReader := &groupBalanceUsageReaderStub{errors: map[int64]error{1: errors.New("upstream unavailable")}}
	svc := NewGroupPredictedBalanceService(groupBalanceAccountRepoStub{accounts: accounts}, &groupBalancePoolReaderStub{}, usageReader, nil)

	summary, err := svc.EstimateGroupPredictedBalance(context.Background(), 17)
	require.Error(t, err)
	require.Nil(t, summary)
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
