package service

import (
	"context"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/shopspring/decimal"
	"golang.org/x/sync/errgroup"
)

const (
	GroupBalanceReasonUnknown          = "unknown"
	GroupBalanceReasonStale            = "stale"
	GroupBalanceReasonIncompatibleUnit = "incompatible_unit"
	GroupBalanceReasonInvalidValue     = "invalid_value"
)

// GroupPredictedBalanceSummary is the complete group-level USD capacity view
// used by the remaining_balance_usd alert metric. Known subtotals remain
// available for diagnostics when Complete is false, but an incomplete result
// must never change the durable alert state.
type GroupPredictedBalanceSummary struct {
	Complete                     bool
	Unlimited                    bool
	RemainingBalanceUSD          *float64
	PoolAuthoritativeBalanceUSD  float64
	NormalEstimatedBalanceUSD    float64
	PoolAccountCount             int
	NormalAccountCount           int
	SkippedAccountCount          int
	UnknownAccountCount          int
	StaleAccountCount            int
	IncompatibleUnitAccountCount int
	EvaluatedAt                  time.Time
}

// GroupPredictedBalanceReader is deliberately narrow so the alert evaluator can
// be tested without constructing the full account usage stack.
type GroupPredictedBalanceReader interface {
	EstimateGroupPredictedBalance(ctx context.Context, groupID int64) (*GroupPredictedBalanceSummary, error)
}

type GroupBalanceUsageReader interface {
	GetCapacityForAggregation(ctx context.Context, account *Account) (*AccountCapacitySnapshot, error)
}

type GroupPredictedBalanceService struct {
	accountRepo    AccountRepository
	poolReader     PoolBalanceReader
	usageReader    GroupBalanceUsageReader
	maxConcurrency int
	now            func() time.Time
}

func NewGroupPredictedBalanceService(
	accountRepo AccountRepository,
	poolReader PoolBalanceReader,
	usageReader GroupBalanceUsageReader,
	cfg *config.Config,
) *GroupPredictedBalanceService {
	concurrency := 4
	if cfg != nil && cfg.PoolCapacityAlert.GroupBalanceConcurrency > 0 {
		concurrency = cfg.PoolCapacityAlert.GroupBalanceConcurrency
	}
	return &GroupPredictedBalanceService{
		accountRepo:    accountRepo,
		poolReader:     poolReader,
		usageReader:    usageReader,
		maxConcurrency: concurrency,
		now:            time.Now,
	}
}

func ProvideGroupPredictedBalanceService(
	accountRepo AccountRepository,
	capacityService *AccountCapacityService,
	usageService *AccountUsageService,
	cfg *config.Config,
) *GroupPredictedBalanceService {
	return NewGroupPredictedBalanceService(accountRepo, capacityService, usageService, cfg)
}

func (s *GroupPredictedBalanceService) EstimateGroupPredictedBalance(ctx context.Context, groupID int64) (*GroupPredictedBalanceSummary, error) {
	now := time.Now().UTC()
	if s != nil && s.now != nil {
		now = s.now().UTC()
	}
	summary := &GroupPredictedBalanceSummary{EvaluatedAt: now}
	if s == nil || s.accountRepo == nil || groupID <= 0 {
		return summary, nil
	}

	accounts, err := s.accountRepo.ListByGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}

	eligible := make([]Account, 0, len(accounts))
	seen := make(map[int64]struct{}, len(accounts))
	for i := range accounts {
		account := accounts[i]
		if account.ID <= 0 {
			summary.SkippedAccountCount++
			continue
		}
		if _, exists := seen[account.ID]; exists {
			continue
		}
		seen[account.ID] = struct{}{}
		if !groupBalanceAccountEligible(&account, now) {
			summary.SkippedAccountCount++
			continue
		}
		eligible = append(eligible, account)
	}

	var mu sync.Mutex
	var firstReadErr error
	poolTotal := decimal.Zero
	normalTotal := decimal.Zero
	g, gctx := errgroup.WithContext(ctx)
	limit := s.maxConcurrency
	if limit < 1 {
		limit = 1
	}
	g.SetLimit(limit)

	for i := range eligible {
		account := eligible[i]
		g.Go(func() error {
			if account.IsPoolMode() {
				amount, unlimited, reason, readErr := s.poolAccountBalance(gctx, &account)
				mu.Lock()
				summary.PoolAccountCount++
				if readErr != nil {
					if firstReadErr == nil {
						firstReadErr = readErr
					}
					applyGroupBalanceReason(summary, GroupBalanceReasonUnknown)
					mu.Unlock()
					return nil
				}
				if unlimited {
					summary.Unlimited = true
				} else if reason != "" {
					applyGroupBalanceReason(summary, reason)
				} else {
					poolTotal = poolTotal.Add(amount)
				}
				mu.Unlock()
				return nil
			}

			amount, reason, readErr := s.normalAccountBalance(gctx, &account)
			mu.Lock()
			summary.NormalAccountCount++
			if readErr != nil {
				if firstReadErr == nil {
					firstReadErr = readErr
				}
				applyGroupBalanceReason(summary, GroupBalanceReasonUnknown)
				mu.Unlock()
				return nil
			}
			if reason != "" {
				applyGroupBalanceReason(summary, reason)
			} else {
				normalTotal = normalTotal.Add(amount)
			}
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	summary.PoolAuthoritativeBalanceUSD = poolTotal.InexactFloat64()
	summary.NormalEstimatedBalanceUSD = normalTotal.InexactFloat64()
	if summary.Unlimited {
		summary.Complete = true
		return summary, nil
	}
	if firstReadErr != nil {
		return nil, firstReadErr
	}
	if summary.UnknownAccountCount > 0 {
		return summary, nil
	}

	total := poolTotal.Add(normalTotal).InexactFloat64()
	summary.RemainingBalanceUSD = &total
	summary.Complete = true
	return summary, nil
}

func groupBalanceAccountEligible(account *Account, now time.Time) bool {
	if account == nil || account.Status != StatusActive || !account.Schedulable {
		return false
	}
	if account.AutoPauseOnExpired && account.ExpiresAt != nil && !now.Before(account.ExpiresAt.UTC()) {
		return false
	}
	return true
}

func (s *GroupPredictedBalanceService) poolAccountBalance(ctx context.Context, account *Account) (decimal.Decimal, bool, string, error) {
	if s.poolReader == nil {
		return decimal.Zero, false, GroupBalanceReasonUnknown, nil
	}
	snapshot, err := s.poolReader.GetPoolBalance(ctx, account, false)
	if err != nil {
		return decimal.Zero, false, "", err
	}
	if snapshot == nil || snapshot.Mode != AccountCapacityModeUpstreamBalance || !snapshot.Authoritative {
		return decimal.Zero, false, GroupBalanceReasonUnknown, nil
	}
	if snapshot.State == AccountCapacityStateUnlimited {
		return decimal.Zero, true, "", nil
	}
	if snapshot.State == AccountCapacityStateStale {
		return decimal.Zero, false, GroupBalanceReasonStale, nil
	}
	if snapshot.State != AccountCapacityStateVerified {
		return decimal.Zero, false, GroupBalanceReasonUnknown, nil
	}
	if !isUSDUnit(snapshot.Unit) {
		return decimal.Zero, false, GroupBalanceReasonIncompatibleUnit, nil
	}
	amount, ok := validGroupBalanceAmount(snapshot.Remaining)
	if !ok {
		return decimal.Zero, false, GroupBalanceReasonInvalidValue, nil
	}
	return decimal.NewFromFloat(amount), false, "", nil
}

func (s *GroupPredictedBalanceService) normalAccountBalance(ctx context.Context, account *Account) (decimal.Decimal, string, error) {
	if s.usageReader == nil {
		return decimal.Zero, GroupBalanceReasonUnknown, nil
	}
	snapshot, err := s.usageReader.GetCapacityForAggregation(ctx, account)
	if err != nil {
		return decimal.Zero, "", err
	}
	return estimatedNormalBalanceUSD(snapshot)
}

func estimatedNormalBalanceUSD(snapshot *AccountCapacitySnapshot) (decimal.Decimal, string, error) {
	if snapshot == nil {
		return decimal.Zero, GroupBalanceReasonUnknown, nil
	}
	if snapshot.State == AccountCapacityStateStale {
		return decimal.Zero, GroupBalanceReasonStale, nil
	}

	switch snapshot.Mode {
	case AccountCapacityModeLocalQuota:
		if snapshot.State == AccountCapacityStateUnlimited {
			// No configured local limit is not proof that the upstream account is unlimited.
			return decimal.Zero, GroupBalanceReasonUnknown, nil
		}
		if snapshot.State == AccountCapacityStateUnsupported ||
			(snapshot.State == AccountCapacityStateUnknown && snapshot.MessageCode != "insufficient_cost_sample") {
			return decimal.Zero, GroupBalanceReasonUnknown, nil
		}
		if !isUSDUnit(snapshot.Unit) {
			return decimal.Zero, GroupBalanceReasonIncompatibleUnit, nil
		}
		amount, ok := validGroupBalanceAmount(snapshot.Remaining)
		if !ok {
			return decimal.Zero, GroupBalanceReasonInvalidValue, nil
		}
		return decimal.NewFromFloat(amount), "", nil

	case AccountCapacityModeUsageWindow:
		if snapshot.State != AccountCapacityStateEstimated || snapshot.EstimatedRemainingRequests == nil || snapshot.AverageCostPerRequest == nil || snapshot.SampleRequests <= 0 {
			return decimal.Zero, GroupBalanceReasonUnknown, nil
		}
		requests := *snapshot.EstimatedRemainingRequests
		average := *snapshot.AverageCostPerRequest
		if requests < 0 || average <= 0 || math.IsNaN(average) || math.IsInf(average, 0) {
			return decimal.Zero, GroupBalanceReasonInvalidValue, nil
		}
		return decimal.NewFromInt(requests).Mul(decimal.NewFromFloat(average)), "", nil
	default:
		return decimal.Zero, GroupBalanceReasonUnknown, nil
	}
}

func validGroupBalanceAmount(value *float64) (float64, bool) {
	if value == nil || *value < 0 || math.IsNaN(*value) || math.IsInf(*value, 0) {
		return 0, false
	}
	return *value, true
}

func isUSDUnit(unit string) bool {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "usd", "$":
		return true
	default:
		return false
	}
}

func applyGroupBalanceReason(summary *GroupPredictedBalanceSummary, reason string) {
	if summary == nil {
		return
	}
	summary.UnknownAccountCount++
	switch reason {
	case GroupBalanceReasonStale:
		summary.StaleAccountCount++
	case GroupBalanceReasonIncompatibleUnit:
		summary.IncompatibleUnitAccountCount++
	}
}
