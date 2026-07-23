package service

import (
	"context"
	"errors"
	"math"
	"strconv"
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
	PredictionMode                string
	PredictionUnit                string
	PredictionConfigured          bool
	PredictionComplete            bool
	PredictionUnlimited           bool
	PredictedQuantity             *string
	PredictionUnitCostUSD         *float64
	KnownPredictionAccountCount   int
	UnknownPredictionAccountCount int

	Complete                     bool
	Unlimited                    bool
	RemainingBalanceUSD          *float64
	KnownRemainingBalanceUSD     *float64
	PoolAuthoritativeBalanceUSD  float64
	NormalEstimatedBalanceUSD    float64
	KnownBalanceAccountCount     int
	PoolAccountCount             int
	NormalAccountCount           int
	SkippedAccountCount          int
	UnknownAccountCount          int
	StaleAccountCount            int
	IncompatibleUnitAccountCount int
	RequestsComplete             bool
	RequestsUnlimited            bool
	EstimatedRemainingRequests   *int64
	KnownRequestAccountCount     int
	UnknownRequestAccountCount   int
	EvaluatedAt                  time.Time
}

type groupAccountPrediction struct {
	balance           decimal.Decimal
	balanceUnlimited  bool
	balanceReason     string
	requests          int64
	requestsKnown     bool
	requestsUnlimited bool
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
	groupRepo      GroupRepository
	poolReader     PoolBalanceReader
	usageReader    GroupBalanceUsageReader
	maxConcurrency int
	now            func() time.Time
}

func NewGroupPredictedBalanceService(
	accountRepo AccountRepository,
	groupRepo GroupRepository,
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
		groupRepo:      groupRepo,
		poolReader:     poolReader,
		usageReader:    usageReader,
		maxConcurrency: concurrency,
		now:            time.Now,
	}
}

func ProvideGroupPredictedBalanceService(
	accountRepo AccountRepository,
	groupRepo GroupRepository,
	capacityService *AccountCapacityService,
	usageService *AccountUsageService,
	cfg *config.Config,
) *GroupPredictedBalanceService {
	return NewGroupPredictedBalanceService(accountRepo, groupRepo, capacityService, usageService, cfg)
}

func (s *GroupPredictedBalanceService) EstimateGroupPredictedBalance(ctx context.Context, groupID int64) (*GroupPredictedBalanceSummary, error) {
	now := time.Now().UTC()
	if s != nil && s.now != nil {
		now = s.now().UTC()
	}
	predictionMode := DefaultPredictedCapacityMode
	var predictionUnitCostUSD *float64
	predictionConfigAvailable := true
	if s != nil && s.groupRepo != nil && groupID > 0 {
		group, err := s.groupRepo.GetByIDLite(ctx, groupID)
		if err != nil {
			if errors.Is(err, ErrGroupNotFound) {
				return nil, err
			}
			// The balance/request aggregation is also used by durable alerts. A display
			// configuration read failure must not change those established semantics.
			predictionConfigAvailable = false
		} else if group == nil {
			return nil, ErrGroupNotFound
		} else {
			predictionMode = NormalizePredictedCapacityMode(group.PredictedCapacityMode)
			predictionUnitCostUSD = cloneGroupValuePointer(group.PredictedImageUnitCostUSD)
		}
	}
	summary := &GroupPredictedBalanceSummary{EvaluatedAt: now}
	if s == nil || s.accountRepo == nil || groupID <= 0 {
		finalizeGroupPredictionSummary(summary, predictionMode, predictionUnitCostUSD, predictionConfigAvailable, decimal.Zero)
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
	var requestTotal int64
	g, gctx := errgroup.WithContext(ctx)
	limit := s.maxConcurrency
	if limit < 1 {
		limit = 1
	}
	g.SetLimit(limit)

	for i := range eligible {
		account := eligible[i]
		g.Go(func() error {
			isPool := account.IsPoolMode()
			var prediction groupAccountPrediction
			var readErr error
			if isPool {
				prediction, readErr = s.poolAccountPrediction(gctx, &account)
			} else {
				prediction, readErr = s.normalAccountPrediction(gctx, &account)
			}

			mu.Lock()
			defer mu.Unlock()
			if isPool {
				summary.PoolAccountCount++
			} else {
				summary.NormalAccountCount++
			}
			if readErr != nil {
				if firstReadErr == nil {
					firstReadErr = readErr
				}
				applyGroupBalanceReason(summary, GroupBalanceReasonUnknown)
				summary.UnknownRequestAccountCount++
				return nil
			}

			if prediction.balanceUnlimited {
				summary.Unlimited = true
			} else if prediction.balanceReason != "" {
				applyGroupBalanceReason(summary, prediction.balanceReason)
			} else {
				summary.KnownBalanceAccountCount++
				if isPool {
					poolTotal = poolTotal.Add(prediction.balance)
				} else {
					normalTotal = normalTotal.Add(prediction.balance)
				}
			}

			switch {
			case prediction.requestsUnlimited:
				summary.RequestsUnlimited = true
			case !prediction.requestsKnown:
				summary.UnknownRequestAccountCount++
			case prediction.requests > 0 && requestTotal > math.MaxInt64-prediction.requests:
				requestTotal = math.MaxInt64
				summary.UnknownRequestAccountCount++
			default:
				requestTotal += prediction.requests
				summary.KnownRequestAccountCount++
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	knownBalance := poolTotal.Add(normalTotal)
	if poolBalance, ok := finiteGroupBalanceFloat64(poolTotal); ok {
		summary.PoolAuthoritativeBalanceUSD = poolBalance
	}
	if normalBalance, ok := finiteGroupBalanceFloat64(normalTotal); ok {
		summary.NormalEstimatedBalanceUSD = normalBalance
	}
	if totalBalance, ok := finiteGroupBalanceFloat64(knownBalance); ok && (summary.KnownBalanceAccountCount > 0 || summary.UnknownAccountCount == 0) {
		summary.KnownRemainingBalanceUSD = &totalBalance
	}
	finalizeGroupRequestSummary(summary, requestTotal)
	finalizeGroupPredictionSummary(summary, predictionMode, predictionUnitCostUSD, predictionConfigAvailable, knownBalance)
	if summary.Unlimited {
		summary.Complete = true
		return summary, nil
	}
	if firstReadErr != nil {
		return nil, firstReadErr
	}
	if summary.UnknownAccountCount > 0 || summary.KnownRemainingBalanceUSD == nil {
		return summary, nil
	}

	summary.RemainingBalanceUSD = cloneGroupValuePointer(summary.KnownRemainingBalanceUSD)
	summary.Complete = true
	return summary, nil
}

func finalizeGroupRequestSummary(summary *GroupPredictedBalanceSummary, requestTotal int64) {
	if summary == nil {
		return
	}
	if summary.RequestsUnlimited {
		summary.RequestsComplete = true
		summary.EstimatedRemainingRequests = nil
		return
	}
	summary.RequestsComplete = summary.UnknownRequestAccountCount == 0
	if summary.KnownRequestAccountCount > 0 || summary.RequestsComplete {
		total := requestTotal
		summary.EstimatedRemainingRequests = &total
	}
}

func finalizeGroupPredictionSummary(
	summary *GroupPredictedBalanceSummary,
	mode string,
	unitCostUSD *float64,
	configAvailable bool,
	knownBalance decimal.Decimal,
) {
	if summary == nil {
		return
	}
	mode = NormalizePredictedCapacityMode(mode)
	summary.PredictionMode = mode
	summary.PredictionUnitCostUSD = cloneGroupValuePointer(unitCostUSD)

	switch mode {
	case PredictedCapacityModeHistoricalRequests:
		summary.PredictionUnit = "request"
		summary.PredictionConfigured = configAvailable
		summary.PredictionComplete = summary.RequestsComplete
		summary.PredictionUnlimited = summary.RequestsUnlimited
		summary.KnownPredictionAccountCount = summary.KnownRequestAccountCount
		summary.UnknownPredictionAccountCount = summary.UnknownRequestAccountCount
		if summary.EstimatedRemainingRequests != nil {
			quantity := strconv.FormatInt(*summary.EstimatedRemainingRequests, 10)
			summary.PredictedQuantity = &quantity
		}
	case PredictedCapacityModeFixedImageCost:
		summary.PredictionUnit = "image"
		summary.KnownPredictionAccountCount = summary.KnownBalanceAccountCount
		summary.UnknownPredictionAccountCount = summary.UnknownAccountCount
		if !configAvailable || ValidatePredictedCapacityConfig(mode, unitCostUSD) != nil {
			return
		}
		summary.PredictionConfigured = true
		if summary.Unlimited {
			summary.PredictionComplete = true
			summary.PredictionUnlimited = true
			return
		}
		summary.PredictionComplete = summary.UnknownAccountCount == 0
		if summary.KnownBalanceAccountCount == 0 && !summary.PredictionComplete {
			return
		}
		cost := decimal.NewFromFloat(*unitCostUSD)
		quantity := knownBalance.Div(cost).Floor().StringFixed(0)
		summary.PredictedQuantity = &quantity
	default:
		// Invalid persisted configuration remains visibly unconfigured and never
		// turns unknown account capacity into a synthetic zero.
	}
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

func (s *GroupPredictedBalanceService) poolAccountPrediction(ctx context.Context, account *Account) (groupAccountPrediction, error) {
	prediction := groupAccountPrediction{balanceReason: GroupBalanceReasonUnknown}
	if s.poolReader == nil {
		return prediction, nil
	}
	snapshot, err := s.poolReader.GetPoolBalance(ctx, account, false)
	if err != nil {
		return groupAccountPrediction{}, err
	}
	authoritativeBalance := snapshot != nil && snapshot.Mode == AccountCapacityModeUpstreamBalance && snapshot.Authoritative
	prediction.requests, prediction.requestsKnown, prediction.requestsUnlimited = estimatedGroupRequests(snapshot, authoritativeBalance)
	if !authoritativeBalance {
		return prediction, nil
	}
	if snapshot.State == AccountCapacityStateUnlimited {
		prediction.balanceUnlimited = true
		prediction.balanceReason = ""
		return prediction, nil
	}
	if snapshot.State == AccountCapacityStateStale {
		prediction.balanceReason = GroupBalanceReasonStale
		return prediction, nil
	}
	if snapshot.State != AccountCapacityStateVerified {
		return prediction, nil
	}
	if !isUSDUnit(snapshot.Unit) {
		prediction.balanceReason = GroupBalanceReasonIncompatibleUnit
		return prediction, nil
	}
	amount, ok := validGroupBalanceAmount(snapshot.Remaining)
	if !ok {
		prediction.balanceReason = GroupBalanceReasonInvalidValue
		return prediction, nil
	}
	prediction.balance = decimal.NewFromFloat(amount)
	prediction.balanceReason = ""
	return prediction, nil
}

func (s *GroupPredictedBalanceService) normalAccountPrediction(ctx context.Context, account *Account) (groupAccountPrediction, error) {
	prediction := groupAccountPrediction{balanceReason: GroupBalanceReasonUnknown}
	if s.usageReader == nil {
		return prediction, nil
	}
	snapshot, err := s.usageReader.GetCapacityForAggregation(ctx, account)
	if err != nil {
		return groupAccountPrediction{}, err
	}
	prediction.requests, prediction.requestsKnown, prediction.requestsUnlimited = estimatedGroupRequests(snapshot, false)
	prediction.balance, prediction.balanceReason, err = estimatedNormalBalanceUSD(snapshot)
	return prediction, err
}

func estimatedGroupRequests(snapshot *AccountCapacitySnapshot, authoritativeUnlimited bool) (int64, bool, bool) {
	if snapshot == nil || snapshot.State == AccountCapacityStateStale {
		return 0, false, false
	}
	if authoritativeUnlimited && snapshot.State == AccountCapacityStateUnlimited {
		return 0, false, true
	}
	if snapshot.State != AccountCapacityStateEstimated && snapshot.State != AccountCapacityStateVerified {
		return 0, false, false
	}
	if snapshot.EstimatedRemainingRequests == nil || *snapshot.EstimatedRemainingRequests < 0 {
		return 0, false, false
	}
	return *snapshot.EstimatedRemainingRequests, true, false
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
			(snapshot.State == AccountCapacityStateUnknown &&
				snapshot.MessageCode != "insufficient_cost_sample" &&
				snapshot.MessageCode != "request_estimate_overflow") {
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

func finiteGroupBalanceFloat64(value decimal.Decimal) (float64, bool) {
	if value.IsNegative() {
		return 0, false
	}
	converted := value.InexactFloat64()
	if math.IsNaN(converted) || math.IsInf(converted, 0) {
		return 0, false
	}
	return converted, true
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
