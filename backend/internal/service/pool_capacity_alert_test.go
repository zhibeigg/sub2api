package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestGroupPoolCapacityAlertPolicyFallsBackToLegacyDefaults(t *testing.T) {
	policy := (&Group{PoolCapacityAlertMetric: "invalid", PoolCapacityAlertThresholdRequests: 0}).PoolCapacityAlertPolicy()
	require.Equal(t, PoolCapacityAlertMetricPredictedRequests, policy.Metric)
	require.Equal(t, DefaultPoolCapacityAlertThresholdRequests, policy.ThresholdRequests)
}

func TestPoolCapacityAlertSubmitAfterBillingUsesFinalSelection(t *testing.T) {
	groupID := int64(17)
	accountRate := 0.5
	newBalance := 3.2
	group := &Group{
		ID:                          groupID,
		Name:                        "production",
		PoolCapacityAlertEnabled:    true,
		PoolCapacityAlertGeneration: 4,
	}
	account := &Account{
		ID:          21,
		Name:        "pool-account",
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"pool_mode": true},
	}
	apiKey := &APIKey{
		ID:      31,
		Name:    "gateway-key",
		UserID:  41,
		GroupID: &groupID,
		Group:   group,
		Quota:   100,
		Status:  StatusAPIKeyActive,
	}
	user := &User{ID: 41, Email: "owner@example.com", Balance: 10}
	usageLog := &UsageLog{
		RequestID:             "request-final-selection",
		GroupID:               &groupID,
		AccountID:             account.ID,
		APIKeyID:              apiKey.ID,
		TotalCost:             2,
		ActualCost:            0.4,
		AccountRateMultiplier: &accountRate,
		BillingType:           BillingTypeBalance,
	}
	accountState := &AccountQuotaState{TotalUsed: 51, TotalLimit: 100}
	apiKeyState := &APIKeyQuotaUsageState{QuotaUsed: 10.4, Quota: 100, Status: StatusAPIKeyActive}
	result := &UsageBillingApplyResult{
		Applied:          true,
		NewBalance:       &newBalance,
		QuotaState:       accountState,
		APIKeyQuotaState: apiKeyState,
	}
	params := &postUsageBillingParams{
		Cost:    &CostBreakdown{TotalCost: usageLog.TotalCost, ActualCost: usageLog.ActualCost},
		User:    user,
		APIKey:  apiKey,
		Account: account,
	}
	svc := &PoolCapacityAlertService{
		cfg:   &config.Config{PoolCapacityAlert: config.PoolCapacityAlertConfig{Enabled: true}},
		queue: make(chan poolCapacityAlertTask, 1),
	}

	svc.SubmitAfterBilling(usageLog, params, result)

	select {
	case task := <-svc.queue:
		require.Equal(t, usageLog.RequestID, task.RequestID)
		require.Equal(t, groupID, task.GroupID)
		require.Equal(t, int64(4), task.GroupGeneration)
		require.Equal(t, account.ID, task.AccountID)
		require.Equal(t, apiKey.ID, task.APIKeyID)
		require.Equal(t, user.ID, task.UserID)
		require.InDelta(t, 1.0, task.CurrentAccountCost, 1e-12)
		require.InDelta(t, 0.4, task.CurrentActualCost, 1e-12)
		require.NotSame(t, result.NewBalance, task.NewBalance)
		require.NotSame(t, result.QuotaState, task.AccountQuotaState)
		require.NotSame(t, result.APIKeyQuotaState, task.APIKeyQuotaState)
	default:
		t.Fatal("expected final pool-mode selection to enqueue a capacity evaluation")
	}
}

func TestPoolCapacityAlertSubmitAfterBillingRejectsNonFinalOrDisabledContexts(t *testing.T) {
	baseGroupID := int64(17)
	otherGroupID := int64(18)
	newBalance := 3.2
	newFixture := func() (*PoolCapacityAlertService, *UsageLog, *postUsageBillingParams, *UsageBillingApplyResult) {
		group := &Group{ID: baseGroupID, PoolCapacityAlertEnabled: true, PoolCapacityAlertGeneration: 2}
		account := &Account{ID: 21, Type: AccountTypeAPIKey, Credentials: map[string]any{"pool_mode": true}}
		apiKey := &APIKey{ID: 31, UserID: 41, GroupID: &baseGroupID, Group: group, Quota: 100, Status: StatusAPIKeyActive}
		usageLog := &UsageLog{RequestID: "request-1", GroupID: &baseGroupID, AccountID: account.ID, APIKeyID: apiKey.ID, TotalCost: 1, ActualCost: 1}
		params := &postUsageBillingParams{Cost: &CostBreakdown{TotalCost: 1, ActualCost: 1}, User: &User{ID: 41}, APIKey: apiKey, Account: account}
		result := &UsageBillingApplyResult{Applied: true, NewBalance: &newBalance}
		svc := &PoolCapacityAlertService{cfg: &config.Config{PoolCapacityAlert: config.PoolCapacityAlertConfig{Enabled: true}}, queue: make(chan poolCapacityAlertTask, 1)}
		return svc, usageLog, params, result
	}

	tests := []struct {
		name   string
		mutate func(*UsageLog, *postUsageBillingParams, *UsageBillingApplyResult)
	}{
		{
			name: "selected account is not pool mode",
			mutate: func(_ *UsageLog, params *postUsageBillingParams, _ *UsageBillingApplyResult) {
				params.Account.Credentials["pool_mode"] = false
			},
		},
		{
			name: "final group disabled alert",
			mutate: func(_ *UsageLog, params *postUsageBillingParams, _ *UsageBillingApplyResult) {
				params.APIKey.Group.PoolCapacityAlertEnabled = false
			},
		},
		{
			name: "usage log group differs from final group",
			mutate: func(log *UsageLog, _ *postUsageBillingParams, _ *UsageBillingApplyResult) {
				log.GroupID = &otherGroupID
			},
		},
		{
			name: "billing dedup did not apply",
			mutate: func(_ *UsageLog, _ *postUsageBillingParams, result *UsageBillingApplyResult) {
				result.Applied = false
			},
		},
		{
			name: "cyber-blocked request",
			mutate: func(log *UsageLog, _ *postUsageBillingParams, _ *UsageBillingApplyResult) {
				log.RequestType = RequestTypeCyberBlocked
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			svc, usageLog, params, result := newFixture()
			test.mutate(usageLog, params, result)
			svc.SubmitAfterBilling(usageLog, params, result)
			require.Empty(t, svc.queue)
		})
	}
}

func TestAveragePoolCapacityCostsUsesFortyNinePriorSamplesPlusCurrent(t *testing.T) {
	avgAccount, avgActual, ready := averagePoolCapacityCosts(&PoolCapacityCostSummary{
		Count:          PoolCapacityAlertSampleSize - 1,
		AccountCostSum: decimal.NewFromInt(49),
		ActualCostSum:  decimal.NewFromInt(98),
	}, 1, 2)

	require.True(t, ready)
	require.True(t, avgAccount.Equal(decimal.NewFromInt(1)))
	require.True(t, avgActual.Equal(decimal.NewFromInt(2)))

	_, _, ready = averagePoolCapacityCosts(&PoolCapacityCostSummary{
		Count:          PoolCapacityAlertSampleSize - 2,
		AccountCostSum: decimal.NewFromInt(48),
		ActualCostSum:  decimal.NewFromInt(96),
	}, 1, 2)
	require.False(t, ready, "the current request must be the 50th sample")
}

func TestPoolCapacityCalculationsUsePostBillingStateAndFloorRequests(t *testing.T) {
	average := decimal.NewFromInt(1)
	accountRequests, accountRemaining, known := calculatePoolAccountCapacity(
		&Account{Type: AccountTypeAPIKey},
		&AccountQuotaState{TotalUsed: 50, TotalLimit: 100},
		average,
	)
	require.True(t, known)
	require.Equal(t, int64(50), *accountRequests)
	require.InDelta(t, 50, *accountRemaining, 1e-12)

	apiKeyRequests, apiKeyRemaining, known := calculatePoolAPIKeyCapacity(poolCapacityAlertTask{
		APIKeyQuota:      100,
		APIKeyQuotaState: &APIKeyQuotaUsageState{QuotaUsed: 50, Quota: 100},
	}, average)
	require.True(t, known)
	require.Equal(t, int64(50), *apiKeyRequests)
	require.InDelta(t, 50, *apiKeyRemaining, 1e-12)

	wallet := 49.99
	svc := &PoolCapacityAlertService{}
	walletRequests, walletRemaining, known := svc.calculatePoolWalletCapacity(poolCapacityAlertTask{NewBalance: &wallet}, average)
	require.True(t, known)
	require.Equal(t, int64(49), *walletRequests)
	require.InDelta(t, wallet, *walletRemaining, 1e-12)

	reserveSvc := &PoolCapacityAlertService{cfg: &config.Config{Billing: config.BillingConfig{MinimumBalanceReserve: 10}}}
	walletRequests, walletRemaining, known = reserveSvc.calculatePoolWalletCapacity(poolCapacityAlertTask{NewBalance: &wallet}, average)
	require.True(t, known)
	require.Equal(t, int64(49), *walletRequests, "request prediction keeps the legacy post-billing wallet balance")
	require.InDelta(t, wallet, *walletRemaining, 1e-12)
	amountRemaining, known := reserveSvc.calculatePoolWalletRemainingUSD(poolCapacityAlertTask{NewBalance: &wallet})
	require.True(t, known)
	require.InDelta(t, 39.99, *amountRemaining, 1e-12, "USD mode subtracts the configured minimum reserve")
}

func TestCalculatePoolUpstreamCapacityRequiresVerifiedSupportedUnits(t *testing.T) {
	average := decimal.NewFromInt(1)
	remaining := 49.99
	requests, amount, known := calculatePoolUpstreamCapacity(&AccountCapacitySnapshot{
		Mode:          AccountCapacityModeUpstreamBalance,
		State:         AccountCapacityStateVerified,
		Authoritative: true,
		Remaining:     &remaining,
		Unit:          "USD",
	}, average)
	require.True(t, known)
	require.Equal(t, int64(49), *requests)
	require.InDelta(t, remaining, *amount, 1e-12)

	requests, amount, known = calculatePoolUpstreamCapacity(&AccountCapacitySnapshot{
		Mode:          AccountCapacityModeUpstreamBalance,
		State:         AccountCapacityStateVerified,
		Authoritative: true,
		Remaining:     &remaining,
		Unit:          "requests",
	}, average)
	require.True(t, known)
	require.Equal(t, int64(49), *requests)
	require.Nil(t, amount)

	for _, snapshot := range []*AccountCapacitySnapshot{
		{Mode: AccountCapacityModeUpstreamBalance, State: AccountCapacityStateStale, Authoritative: false, Remaining: &remaining, Unit: "USD"},
		{Mode: AccountCapacityModeUpstreamBalance, State: AccountCapacityStateVerified, Authoritative: true, Remaining: &remaining, Unit: "EUR"},
		{Mode: AccountCapacityModeUsageWindow, State: AccountCapacityStateEstimated, Authoritative: false, Remaining: &remaining, Unit: "requests"},
	} {
		_, _, known = calculatePoolUpstreamCapacity(snapshot, average)
		require.False(t, known)
	}
}

func TestCalculatePoolUpstreamBalanceUSDRequiresAuthoritativeUSD(t *testing.T) {
	remaining := 12.5
	amount, known := calculatePoolUpstreamBalanceUSD(&AccountCapacitySnapshot{
		Mode:          AccountCapacityModeUpstreamBalance,
		State:         AccountCapacityStateVerified,
		Authoritative: true,
		Remaining:     &remaining,
		Unit:          "USD",
	})
	require.True(t, known)
	require.InDelta(t, remaining, *amount, 1e-12)

	amount, known = calculatePoolUpstreamBalanceUSD(&AccountCapacitySnapshot{
		Mode:          AccountCapacityModeUpstreamBalance,
		State:         AccountCapacityStateUnlimited,
		Authoritative: true,
	})
	require.True(t, known)
	require.Nil(t, amount)

	for _, snapshot := range []*AccountCapacitySnapshot{
		{Mode: AccountCapacityModeUpstreamBalance, State: AccountCapacityStateVerified, Authoritative: true, Remaining: &remaining, Unit: "requests"},
		{Mode: AccountCapacityModeUpstreamBalance, State: AccountCapacityStateVerified, Authoritative: true, Remaining: &remaining, Unit: "EUR"},
		{Mode: AccountCapacityModeUpstreamBalance, State: AccountCapacityStateStale, Authoritative: false, Remaining: &remaining, Unit: "USD"},
	} {
		amount, known = calculatePoolUpstreamBalanceUSD(snapshot)
		require.False(t, known)
		require.Nil(t, amount)
	}
}

func TestPoolCapacityAlertEvaluateUsesVerifiedUpstreamWithLocalSafetyLimit(t *testing.T) {
	group := &Group{ID: 17, Name: "production", PoolCapacityAlertEnabled: true, PoolCapacityAlertGeneration: 4}
	account := &Account{
		ID:          21,
		Name:        "pool-account",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"pool_mode": true, "base_url": "https://relay.example.com", "api_key": "key"},
		Extra:       map[string]any{"quota_limit": 100.0, "quota_used": 60.0},
	}
	upstreamRemaining := 49.0
	repo := &poolCapacityDispatchRepo{}
	reader := &poolCapacityBalanceReaderStub{snapshot: &AccountCapacitySnapshot{
		Mode:          AccountCapacityModeUpstreamBalance,
		State:         AccountCapacityStateVerified,
		Authoritative: true,
		Remaining:     &upstreamRemaining,
		Unit:          "USD",
	}}
	svc := &PoolCapacityAlertService{
		repo:           repo,
		usageReader:    poolCapacityUsageReaderStub{summary: &PoolCapacityCostSummary{Count: 49, AccountCostSum: decimal.NewFromInt(49), ActualCostSum: decimal.NewFromInt(49)}},
		groupRepo:      poolCapacityGroupRepoStub{group: group},
		accountRepo:    &stubOpenAIAccountRepo{accounts: []Account{*account}},
		capacityReader: reader,
	}
	err := svc.evaluate(context.Background(), poolCapacityAlertTask{
		RequestID:          "request-50",
		GroupID:            group.ID,
		GroupGeneration:    group.PoolCapacityAlertGeneration,
		AccountID:          account.ID,
		APIKeyID:           31,
		UserID:             41,
		IsSubscriptionBill: true,
		CurrentAccountCost: 1,
		CurrentActualCost:  1,
		AccountQuotaState:  &AccountQuotaState{TotalLimit: 100, TotalUsed: 60},
	})
	require.NoError(t, err)
	require.Equal(t, 1, reader.calls)
	require.Len(t, repo.evaluations, 1)
	evaluation := repo.evaluations[0]
	require.Equal(t, int64(40), *evaluation.AccountRequests, "local quota remains a safety upper bound")
	require.Equal(t, int64(40), *evaluation.PredictedRequests)
	require.InDelta(t, 40, *evaluation.AccountRemaining, 1e-12)
	require.Equal(t, "account", evaluation.Bottleneck)
}

func TestPoolCapacityAlertEvaluateRemainingBalanceUSDDoesNotRequireHistory(t *testing.T) {
	threshold := 8.0
	group := &Group{
		ID:                                 17,
		Name:                               "production",
		PoolCapacityAlertEnabled:           true,
		PoolCapacityAlertGeneration:        4,
		PoolCapacityAlertMetric:            PoolCapacityAlertMetricRemainingBalanceUSD,
		PoolCapacityAlertThresholdRequests: DefaultPoolCapacityAlertThresholdRequests,
		PoolCapacityAlertThresholdUSD:      &threshold,
	}
	account := &Account{
		ID:          21,
		Name:        "pool-account",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"pool_mode": true},
	}
	upstreamRemaining := 12.0
	wallet := 9.0
	repo := &poolCapacityDispatchRepo{}
	svc := &PoolCapacityAlertService{
		repo:        repo,
		groupRepo:   poolCapacityGroupRepoStub{group: group},
		accountRepo: &stubOpenAIAccountRepo{accounts: []Account{*account}},
		capacityReader: &poolCapacityBalanceReaderStub{snapshot: &AccountCapacitySnapshot{
			Mode:          AccountCapacityModeUpstreamBalance,
			State:         AccountCapacityStateVerified,
			Authoritative: true,
			Remaining:     &upstreamRemaining,
			Unit:          "USD",
		}},
		cfg: &config.Config{Billing: config.BillingConfig{MinimumBalanceReserve: 1}},
	}
	err := svc.evaluate(context.Background(), poolCapacityAlertTask{
		RequestID:          "request-first",
		GroupID:            group.ID,
		GroupGeneration:    group.PoolCapacityAlertGeneration,
		AccountID:          account.ID,
		APIKeyID:           31,
		UserID:             41,
		CurrentAccountCost: 1,
		CurrentActualCost:  1,
		APIKeyQuota:        20,
		APIKeyQuotaState:   &APIKeyQuotaUsageState{Quota: 20, QuotaUsed: 13},
		NewBalance:         &wallet,
		AccountQuotaState:  &AccountQuotaState{TotalLimit: 100, TotalUsed: 90},
	})
	require.NoError(t, err)
	require.Len(t, repo.evaluations, 1)
	evaluation := repo.evaluations[0]
	require.Equal(t, PoolCapacityAlertMetricRemainingBalanceUSD, evaluation.AlertMetric)
	require.Nil(t, evaluation.PredictedRequests)
	require.Zero(t, evaluation.SampleCount)
	require.InDelta(t, 7, *evaluation.RemainingBalanceUSD, 1e-12)
	require.InDelta(t, 10, *evaluation.AccountRemaining, 1e-12)
	require.InDelta(t, 7, *evaluation.APIKeyRemaining, 1e-12)
	require.InDelta(t, 8, *evaluation.WalletRemaining, 1e-12)
	require.Equal(t, DefaultPoolCapacityAlertThresholdRequests, *evaluation.ThresholdRequests, "state snapshots retain the complete group policy")
	require.InDelta(t, threshold, *evaluation.ThresholdUSD, 1e-12)
	require.Equal(t, "api_key", evaluation.Bottleneck)
}

func TestPoolCapacityAlertEvaluateRemainingBalanceUSDSkipsIncompatibleUpstreamUnit(t *testing.T) {
	threshold := 10.0
	group := &Group{
		ID:                            17,
		PoolCapacityAlertEnabled:      true,
		PoolCapacityAlertGeneration:   4,
		PoolCapacityAlertMetric:       PoolCapacityAlertMetricRemainingBalanceUSD,
		PoolCapacityAlertThresholdUSD: &threshold,
	}
	account := Account{ID: 21, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"pool_mode": true}}
	remaining := 2.0
	repo := &poolCapacityDispatchRepo{}
	svc := &PoolCapacityAlertService{
		repo:        repo,
		groupRepo:   poolCapacityGroupRepoStub{group: group},
		accountRepo: &stubOpenAIAccountRepo{accounts: []Account{account}},
		capacityReader: &poolCapacityBalanceReaderStub{snapshot: &AccountCapacitySnapshot{
			Mode:          AccountCapacityModeUpstreamBalance,
			State:         AccountCapacityStateVerified,
			Authoritative: true,
			Remaining:     &remaining,
			Unit:          "requests",
		}},
	}
	err := svc.evaluate(context.Background(), poolCapacityAlertTask{
		GroupID:            group.ID,
		GroupGeneration:    group.PoolCapacityAlertGeneration,
		AccountID:          account.ID,
		APIKeyID:           31,
		UserID:             41,
		IsSubscriptionBill: true,
		CurrentAccountCost: 1,
		CurrentActualCost:  1,
	})
	require.NoError(t, err)
	require.Empty(t, repo.evaluations)
}

func TestPoolCapacityAlertEvaluateSkipsStaleBalanceWithoutStateTransition(t *testing.T) {
	group := &Group{ID: 17, PoolCapacityAlertEnabled: true, PoolCapacityAlertGeneration: 4}
	account := Account{ID: 21, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"pool_mode": true}}
	remaining := 10.0
	repo := &poolCapacityDispatchRepo{}
	svc := &PoolCapacityAlertService{
		repo:        repo,
		usageReader: poolCapacityUsageReaderStub{summary: &PoolCapacityCostSummary{Count: 49, AccountCostSum: decimal.NewFromInt(49), ActualCostSum: decimal.NewFromInt(49)}},
		groupRepo:   poolCapacityGroupRepoStub{group: group},
		accountRepo: &stubOpenAIAccountRepo{accounts: []Account{account}},
		capacityReader: &poolCapacityBalanceReaderStub{snapshot: &AccountCapacitySnapshot{
			Mode:          AccountCapacityModeUpstreamBalance,
			State:         AccountCapacityStateStale,
			Authoritative: false,
			Remaining:     &remaining,
			Unit:          "USD",
		}},
	}
	err := svc.evaluate(context.Background(), poolCapacityAlertTask{
		RequestID:          "request-50",
		GroupID:            group.ID,
		GroupGeneration:    group.PoolCapacityAlertGeneration,
		AccountID:          account.ID,
		APIKeyID:           31,
		UserID:             41,
		IsSubscriptionBill: true,
		CurrentAccountCost: 1,
		CurrentActualCost:  1,
	})
	require.NoError(t, err)
	require.Empty(t, repo.evaluations, "stale balance must not recover or create an alert episode")
}

func TestPoolCapacityAlertAmountNotificationsKeepLegacyRequestFieldsAsNA(t *testing.T) {
	remaining := 9.25
	threshold := 10.0
	event := PoolCapacityAlertEvent{
		AlertMetric:         PoolCapacityAlertMetricRemainingBalanceUSD,
		RemainingBalanceUSD: &remaining,
		ThresholdUSD:        &threshold,
		GroupName:           "production",
		AccountName:         "pool-account",
		APIKeyName:          "gateway-key",
		CreatedAt:           time.Date(2026, time.July, 22, 1, 0, 0, 0, time.UTC),
	}

	variables := poolCapacityAlertEmailVariables(event)
	require.Equal(t, "9.250000", variables["alert_metric_value"])
	require.Equal(t, "10.000000", variables["alert_metric_threshold"])
	require.Equal(t, "USD", variables["alert_metric_unit"])
	require.Equal(t, "N/A", variables["predicted_requests"])
	require.Equal(t, "N/A", variables["threshold_requests"])
	require.Equal(t, "N/A", variables["avg_account_cost"])
	require.Equal(t, "N/A", variables["account_requests"])

	message := renderPoolCapacityQQMessage(event)
	require.Contains(t, message, "剩余可用金额：$9.250000（阈值 $10.000000）")
	require.NotContains(t, message, "最近 50 次均值")
}

func TestPoolCapacityAlertDispatchClaimsOnlyImmediatelySendableWave(t *testing.T) {
	repo := &poolCapacityDispatchRepo{}
	for id := int64(1); id <= 5; id++ {
		repo.pending = append(repo.pending, PoolCapacityAlertDelivery{
			ID:           id,
			Channel:      PoolCapacityAlertChannelQQBot,
			AttemptCount: 1,
			MaxAttempts:  3,
		})
	}
	notifier := &poolCapacityQQNotifierStub{}
	svc := &PoolCapacityAlertService{
		repo:       repo,
		qqNotifier: notifier,
		owner:      "worker-1",
		cfg: &config.Config{PoolCapacityAlert: config.PoolCapacityAlertConfig{
			DeliveryWorkerCount: 2,
			DeliveryBatchSize:   5,
			LeaseSeconds:        90,
			SendTimeoutSeconds:  20,
		}},
	}

	require.NoError(t, svc.dispatchDue(context.Background()))
	require.Equal(t, []int{2, 2, 1}, repo.claimLimits)
	require.ElementsMatch(t, []int64{1, 2, 3, 4, 5}, repo.sent)
	require.Len(t, notifier.sent, 5)
}

func TestPoolCapacityAlertSendCancelsDeliveryThatIsNoLongerCurrent(t *testing.T) {
	repo := &poolCapacityDispatchRepo{currentOverride: map[int64]bool{7: false}}
	notifier := &poolCapacityQQNotifierStub{}
	svc := &PoolCapacityAlertService{repo: repo, qqNotifier: notifier, owner: "worker-1"}

	svc.sendDelivery(context.Background(), PoolCapacityAlertDelivery{
		ID:           7,
		Channel:      PoolCapacityAlertChannelQQBot,
		AttemptCount: 1,
		MaxAttempts:  3,
	})

	require.Empty(t, notifier.sent)
	require.Equal(t, []int64{7}, repo.cancelled)
}

func TestPoolCapacityAlertSendCancelsUnavailableQQBotRecipientWithoutRetry(t *testing.T) {
	repo := &poolCapacityDispatchRepo{}
	notifier := &poolCapacityQQNotifierStub{err: ErrQQBotRecipientUnavailable}
	svc := &PoolCapacityAlertService{repo: repo, qqNotifier: notifier, owner: "worker-1"}

	svc.sendDelivery(context.Background(), PoolCapacityAlertDelivery{
		ID:                8,
		Channel:           PoolCapacityAlertChannelQQBot,
		IdentityChannelID: 21,
		AttemptCount:      1,
		MaxAttempts:       3,
	})

	require.Equal(t, []int64{8}, repo.cancelled)
	require.Empty(t, repo.failed)
}

type poolCapacityUsageReaderStub struct {
	summary *PoolCapacityCostSummary
	err     error
}

func (s poolCapacityUsageReaderStub) GetRecentPoolCapacityCostSummary(context.Context, int64, string, int64, int) (*PoolCapacityCostSummary, error) {
	return s.summary, s.err
}

type poolCapacityGroupRepoStub struct {
	GroupRepository
	group *Group
	err   error
}

func (s poolCapacityGroupRepoStub) GetByIDLite(context.Context, int64) (*Group, error) {
	return s.group, s.err
}

type poolCapacityBalanceReaderStub struct {
	snapshot *AccountCapacitySnapshot
	err      error
	calls    int
}

func (s *poolCapacityBalanceReaderStub) GetPoolBalance(context.Context, *Account, bool) (*AccountCapacitySnapshot, error) {
	s.calls++
	return cloneAccountCapacitySnapshot(s.snapshot), s.err
}

type poolCapacityDispatchRepo struct {
	mu              sync.Mutex
	pending         []PoolCapacityAlertDelivery
	claimLimits     []int
	currentOverride map[int64]bool
	sent            []int64
	cancelled       []int64
	failed          []int64
	evaluations     []PoolCapacityEvaluation
}

func (r *poolCapacityDispatchRepo) EvaluateAndMaybeCreateEvent(_ context.Context, evaluation PoolCapacityEvaluation, _ time.Time) (*PoolCapacityAlertEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.evaluations = append(r.evaluations, evaluation)
	return nil, nil
}

func (r *poolCapacityDispatchRepo) ClaimDeliveries(_ context.Context, _ string, _ time.Time, _ time.Duration, limit int) ([]PoolCapacityAlertDelivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.claimLimits = append(r.claimLimits, limit)
	if limit > len(r.pending) {
		limit = len(r.pending)
	}
	claimed := append([]PoolCapacityAlertDelivery(nil), r.pending[:limit]...)
	r.pending = r.pending[limit:]
	return claimed, nil
}

func (r *poolCapacityDispatchRepo) IsDeliveryCurrent(_ context.Context, deliveryID int64, _ string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if current, ok := r.currentOverride[deliveryID]; ok {
		return current, nil
	}
	return true, nil
}

func (r *poolCapacityDispatchRepo) MarkDeliverySent(_ context.Context, deliveryID int64, _ string, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, deliveryID)
	return nil
}

func (r *poolCapacityDispatchRepo) MarkDeliveryFailed(_ context.Context, deliveryID int64, _, _, _ string, _ *time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failed = append(r.failed, deliveryID)
	return nil
}

func (r *poolCapacityDispatchRepo) MarkDeliveryCancelled(_ context.Context, deliveryID int64, _, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cancelled = append(r.cancelled, deliveryID)
	return nil
}

type poolCapacityQQNotifierStub struct {
	mu   sync.Mutex
	err  error
	sent []int64
}

func (n *poolCapacityQQNotifierStub) ActiveQQBotAppID() (string, bool) {
	return "app-1", true
}

func (n *poolCapacityQQNotifierStub) SendAdminProactiveAlert(_ context.Context, identityChannelID int64, _ string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sent = append(n.sent, identityChannelID)
	return n.err
}
