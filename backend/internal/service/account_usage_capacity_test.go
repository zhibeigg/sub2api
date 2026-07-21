package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type poolBalanceReaderStub struct {
	calls int
	force bool
	value *AccountCapacitySnapshot
	err   error
}

func (s *poolBalanceReaderStub) GetPoolBalance(_ context.Context, _ *Account, force bool) (*AccountCapacitySnapshot, error) {
	s.calls++
	s.force = force
	return cloneAccountCapacitySnapshot(s.value), s.err
}

func TestEstimateAccountUsageCapacityUsesTightestRequestWindow(t *testing.T) {
	t.Parallel()
	usage := &UsageInfo{
		FiveHour: &UsageProgress{LimitRequests: 100, UsedRequests: 60, WindowStats: &WindowStats{Requests: 60, Cost: 30}},
		SevenDay: &UsageProgress{LimitRequests: 1000, UsedRequests: 990},
	}

	capacity := estimateAccountUsageCapacity(&Account{}, usage)
	require.Equal(t, AccountCapacityModeUsageWindow, capacity.Mode)
	require.Equal(t, AccountCapacityStateEstimated, capacity.State)
	require.Equal(t, "seven_day", capacity.Scope)
	require.Equal(t, int64(10), *capacity.EstimatedRemainingRequests)
	require.InDelta(t, 10, *capacity.Remaining, 1e-12)
	require.InDelta(t, 1000, *capacity.Total, 1e-12)
	require.InDelta(t, 990, *capacity.Used, 1e-12)
}

func TestEstimateAccountUsageCapacityUsesUtilizationFormula(t *testing.T) {
	t.Parallel()
	usage := &UsageInfo{
		FiveHour: &UsageProgress{
			Utilization: 20,
			WindowStats: &WindowStats{Requests: 25, Cost: 5},
		},
	}

	capacity := estimateAccountUsageCapacity(&Account{}, usage)
	require.Equal(t, AccountCapacityStateEstimated, capacity.State)
	require.Equal(t, int64(100), *capacity.EstimatedRemainingRequests)
	require.Equal(t, int64(25), capacity.SampleRequests)
	require.InDelta(t, 0.2, *capacity.AverageCostPerRequest, 1e-12)

	usage.FiveHour.Utilization = 100
	capacity = estimateAccountUsageCapacity(&Account{}, usage)
	require.Equal(t, int64(0), *capacity.EstimatedRemainingRequests)
}

func TestEstimateAccountUsageCapacityFallsBackToLocalQuota(t *testing.T) {
	t.Parallel()
	account := &Account{Extra: map[string]any{
		"quota_limit": 10.0,
		"quota_used":  4.0,
	}}
	usage := &UsageInfo{CursorLocalUsage: &WindowStats{Requests: 3, Cost: 3}}

	capacity := estimateAccountUsageCapacity(account, usage)
	require.Equal(t, AccountCapacityModeLocalQuota, capacity.Mode)
	require.Equal(t, AccountCapacityStateEstimated, capacity.State)
	require.Equal(t, "local_quota_total", capacity.Scope)
	require.Equal(t, int64(6), *capacity.EstimatedRemainingRequests)
	require.InDelta(t, 6, *capacity.Remaining, 1e-12)
	require.InDelta(t, 1, *capacity.AverageCostPerRequest, 1e-12)
}

func TestEstimateAccountUsageCapacityDoesNotTurnUnknownIntoZero(t *testing.T) {
	t.Parallel()
	capacity := estimateAccountUsageCapacity(&Account{}, &UsageInfo{
		FiveHour: &UsageProgress{Utilization: 50},
	})
	require.Equal(t, AccountCapacityStateUnknown, capacity.State)
	require.Nil(t, capacity.Remaining)
	require.Nil(t, capacity.EstimatedRemainingRequests)

	capacity = estimateAccountUsageCapacity(&Account{}, &UsageInfo{})
	require.Equal(t, AccountCapacityStateUnlimited, capacity.State)
	require.Equal(t, AccountCapacityModeLocalQuota, capacity.Mode)
}

func TestAccountUsageServiceEstimatesNonPoolAPIKeyFromLocalQuota(t *testing.T) {
	t.Parallel()
	account := Account{
		ID:       9000,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			"quota_limit": 12.0,
			"quota_used":  4.0,
		},
	}
	service := &AccountUsageService{
		accountRepo: &stubOpenAIAccountRepo{accounts: []Account{account}},
		usageLogRepo: cursorUsageStatsRepo{stats: &usagestats.AccountStats{
			Requests: 4,
			Cost:     4,
		}},
	}

	usage, err := service.GetUsage(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, "local", usage.Source)
	require.NotNil(t, usage.LocalUsage)
	require.NotNil(t, usage.Capacity)
	require.Equal(t, AccountCapacityModeLocalQuota, usage.Capacity.Mode)
	require.Equal(t, AccountCapacityStateEstimated, usage.Capacity.State)
	require.Equal(t, int64(8), *usage.Capacity.EstimatedRemainingRequests)
}

func TestAccountUsageServiceRoutesPoolAccountsToUpstreamCapacity(t *testing.T) {
	t.Parallel()
	remaining := 17.0
	reader := &poolBalanceReaderStub{value: &AccountCapacitySnapshot{
		Mode:          AccountCapacityModeUpstreamBalance,
		State:         AccountCapacityStateVerified,
		Provider:      AccountCapacityProviderSub2API,
		Authoritative: true,
		Remaining:     &remaining,
		Unit:          "USD",
	}}
	account := Account{
		ID:       9001,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"pool_mode": true,
			"base_url":  "https://relay.example.com",
			"api_key":   "key",
		},
	}
	service := &AccountUsageService{
		accountRepo:     &stubOpenAIAccountRepo{accounts: []Account{account}},
		capacityService: reader,
	}

	usage, err := service.GetUsage(context.Background(), account.ID, true)
	require.NoError(t, err)
	require.NotNil(t, usage.Capacity)
	require.Equal(t, AccountCapacityStateVerified, usage.Capacity.State)
	require.InDelta(t, 17, *usage.Capacity.Remaining, 1e-12)
	require.Equal(t, 1, reader.calls)
	require.True(t, reader.force)
}
