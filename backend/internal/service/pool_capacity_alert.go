package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/shopspring/decimal"
)

const (
	PoolCapacityAlertSampleSize        = 50
	PoolCapacityAlertThresholdRequests = DefaultPoolCapacityAlertThresholdRequests

	PoolCapacityAlertStatusHealthy = "healthy"
	PoolCapacityAlertStatusLow     = "low"

	PoolCapacityAlertChannelEmail = "email"
	PoolCapacityAlertChannelQQBot = "qqbot"

	PoolCapacityAlertDeliveryPending   = "pending"
	PoolCapacityAlertDeliverySending   = "sending"
	PoolCapacityAlertDeliverySent      = "sent"
	PoolCapacityAlertDeliveryRetry     = "retry"
	PoolCapacityAlertDeliveryDead      = "dead"
	PoolCapacityAlertDeliveryCancelled = "cancelled"
)

// PoolCapacityCostSummary is the aggregate of the most recent successful,
// actually-applied usage rows before the current request.
type PoolCapacityCostSummary struct {
	Count          int
	AccountCostSum decimal.Decimal
	ActualCostSum  decimal.Decimal
}

// PoolCapacityUsageReader is intentionally narrower than UsageLogRepository so
// existing tests and read-only usage implementations do not need alert methods.
type PoolCapacityUsageReader interface {
	GetRecentPoolCapacityCostSummary(ctx context.Context, groupID int64, excludeRequestID string, excludeAPIKeyID int64, limit int) (*PoolCapacityCostSummary, error)
}

// PoolCapacityEvaluation is the normalized result persisted by the alert state
// machine. Nil capacities are unbounded.
type PoolCapacityEvaluation struct {
	GroupID             int64
	GroupGeneration     int64
	AccountID           int64
	APIKeyID            int64
	UserID              int64
	BillingType         int8
	GroupName           string
	AccountName         string
	APIKeyName          string
	UserEmail           string
	AlertMetric         string
	PredictedRequests   *int64
	RemainingBalanceUSD *float64
	ThresholdRequests   *int64
	ThresholdUSD        *float64
	AccountRequests     *int64
	APIKeyRequests      *int64
	WalletRequests      *int64
	AverageAccountCost  float64
	AverageActualCost   float64
	AccountRemaining    *float64
	APIKeyRemaining     *float64
	WalletRemaining     *float64
	SampleCount         int
	Bottleneck          string
	QQBotAppID          string
	ReminderCooldown    time.Duration
	DeliveryMaxAttempts int
}

type PoolCapacityAlertEvent struct {
	ID                  int64
	StateID             int64
	Episode             int64
	GroupID             int64
	GroupGeneration     int64
	AccountID           int64
	APIKeyID            int64
	UserID              int64
	BillingType         int8
	GroupName           string
	AccountName         string
	APIKeyName          string
	UserEmail           string
	AlertMetric         string
	PredictedRequests   *int64
	RemainingBalanceUSD *float64
	ThresholdRequests   *int64
	ThresholdUSD        *float64
	AccountRequests     *int64
	APIKeyRequests      *int64
	WalletRequests      *int64
	AverageAccountCost  float64
	AverageActualCost   float64
	AccountRemaining    *float64
	APIKeyRemaining     *float64
	WalletRemaining     *float64
	SampleCount         int
	Bottleneck          string
	QQBotAppID          string
	CreatedAt           time.Time
}

type PoolCapacityAlertDelivery struct {
	ID                int64
	Event             PoolCapacityAlertEvent
	Channel           string
	RecipientUserID   int64
	IdentityChannelID int64
	RecipientEmail    string
	RecipientName     string
	Locale            string
	AttemptCount      int
	MaxAttempts       int
}

// PoolCapacityAlertRepository owns the cross-instance state transition and the
// durable per-recipient delivery queue.
type PoolCapacityAlertRepository interface {
	EvaluateAndMaybeCreateEvent(ctx context.Context, evaluation PoolCapacityEvaluation, now time.Time) (*PoolCapacityAlertEvent, error)
	ClaimDeliveries(ctx context.Context, owner string, now time.Time, lease time.Duration, limit int) ([]PoolCapacityAlertDelivery, error)
	IsDeliveryCurrent(ctx context.Context, deliveryID int64, owner string) (bool, error)
	MarkDeliverySent(ctx context.Context, deliveryID int64, owner string, sentAt time.Time) error
	MarkDeliveryFailed(ctx context.Context, deliveryID int64, owner, class, message string, nextAttemptAt *time.Time) error
	MarkDeliveryCancelled(ctx context.Context, deliveryID int64, owner, reason string) error
}

// PoolCapacityQQNotifier is implemented by QQBotService without introducing a
// service -> qqbot package cycle.
type PoolCapacityQQNotifier interface {
	ActiveQQBotAppID() (string, bool)
	SendAdminProactiveAlert(ctx context.Context, identityChannelID int64, content string) error
}

type poolCapacityAlertTask struct {
	RequestID          string
	GroupID            int64
	GroupGeneration    int64
	AccountID          int64
	APIKeyID           int64
	UserID             int64
	BillingType        int8
	IsSubscriptionBill bool
	GroupName          string
	AccountName        string
	APIKeyName         string
	UserEmail          string
	CurrentAccountCost float64
	CurrentActualCost  float64
	APIKeyQuota        float64
	APIKeyStatus       string
	NewBalance         *float64
	AccountQuotaState  *AccountQuotaState
	APIKeyQuotaState   *APIKeyQuotaUsageState
}

// PoolCapacityAlertService evaluates predictions asynchronously and dispatches
// durable email / QQ deliveries outside request and usage-record workers.
type PoolCapacityAlertService struct {
	repo           PoolCapacityAlertRepository
	usageReader    PoolCapacityUsageReader
	groupRepo      GroupRepository
	accountRepo    AccountRepository
	capacityReader PoolBalanceReader
	notifications  *NotificationEmailService
	qqNotifier     PoolCapacityQQNotifier
	cfg            *config.Config

	owner string
	queue chan poolCapacityAlertTask

	mu     sync.Mutex
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewPoolCapacityAlertService(
	repo PoolCapacityAlertRepository,
	usageLogRepo UsageLogRepository,
	groupRepo GroupRepository,
	accountRepo AccountRepository,
	capacityReader PoolBalanceReader,
	notifications *NotificationEmailService,
	qqNotifier PoolCapacityQQNotifier,
	cfg *config.Config,
) *PoolCapacityAlertService {
	var usageReader PoolCapacityUsageReader
	if typed, ok := usageLogRepo.(PoolCapacityUsageReader); ok {
		usageReader = typed
	}
	queueSize := 256
	if cfg != nil && cfg.PoolCapacityAlert.QueueSize > 0 {
		queueSize = cfg.PoolCapacityAlert.QueueSize
	}
	host, _ := os.Hostname()
	return &PoolCapacityAlertService{
		repo:           repo,
		usageReader:    usageReader,
		groupRepo:      groupRepo,
		accountRepo:    accountRepo,
		capacityReader: capacityReader,
		notifications:  notifications,
		qqNotifier:     qqNotifier,
		cfg:            cfg,
		owner:          fmt.Sprintf("%s:%d:%d", host, os.Getpid(), time.Now().UnixNano()),
		queue:          make(chan poolCapacityAlertTask, queueSize),
	}
}

func ProvidePoolCapacityAlertService(
	repo PoolCapacityAlertRepository,
	usageLogRepo UsageLogRepository,
	groupRepo GroupRepository,
	accountRepo AccountRepository,
	capacityService *AccountCapacityService,
	notifications *NotificationEmailService,
	qqNotifier PoolCapacityQQNotifier,
	cfg *config.Config,
) *PoolCapacityAlertService {
	svc := NewPoolCapacityAlertService(repo, usageLogRepo, groupRepo, accountRepo, capacityService, notifications, qqNotifier, cfg)
	svc.Start()
	return svc
}

// PoolCapacityAlertGatewayBinding exists only to make the post-construction
// gateway injection explicit to Wire without changing the two heavily used
// gateway constructors.
type PoolCapacityAlertGatewayBinding struct{}

func ProvidePoolCapacityAlertGatewayBinding(gateway *GatewayService, openAI *OpenAIGatewayService, alert *PoolCapacityAlertService) *PoolCapacityAlertGatewayBinding {
	if gateway != nil {
		gateway.poolCapacityAlertService = alert
	}
	if openAI != nil {
		openAI.poolCapacityAlertService = alert
	}
	return &PoolCapacityAlertGatewayBinding{}
}

func (s *PoolCapacityAlertService) Start() {
	if s == nil || s.repo == nil || s.usageReader == nil || s.groupRepo == nil || s.accountRepo == nil || s.capacityReader == nil {
		return
	}
	if s.cfg != nil && !s.cfg.PoolCapacityAlert.Enabled {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	workers := 2
	if s.cfg != nil && s.cfg.PoolCapacityAlert.EvaluationWorkerCount > 0 {
		workers = s.cfg.PoolCapacityAlert.EvaluationWorkerCount
	}
	for i := 0; i < workers; i++ {
		s.wg.Add(1)
		go s.evaluationWorker(ctx)
	}
	s.wg.Add(1)
	go s.deliveryLoop(ctx)
}

func (s *PoolCapacityAlertService) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	cancel := s.cancel
	s.cancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.wg.Wait()
}

func (s *PoolCapacityAlertService) SubmitAfterBilling(usageLog *UsageLog, p *postUsageBillingParams, result *UsageBillingApplyResult) {
	if s == nil || result == nil || !result.Applied || usageLog == nil || p == nil || p.Cost == nil || p.Account == nil || p.APIKey == nil || p.User == nil {
		return
	}
	if s.cfg != nil && !s.cfg.PoolCapacityAlert.Enabled {
		return
	}
	if !p.Account.IsPoolMode() || p.APIKey.Group == nil || p.APIKey.GroupID == nil || !p.APIKey.Group.PoolCapacityAlertEnabled {
		return
	}
	if strings.TrimSpace(usageLog.RequestID) == "" || usageLog.GroupID == nil || *usageLog.GroupID != *p.APIKey.GroupID {
		return
	}
	if usageLog.RequestType == RequestTypeCyberBlocked || p.Cost.ActualCost <= 0 {
		return
	}
	accountCost := effectiveUsageLogAccountCost(usageLog)
	if accountCost <= 0 {
		return
	}
	if !p.IsSubscriptionBill && result.NewBalance == nil {
		return
	}

	task := poolCapacityAlertTask{
		RequestID:          strings.TrimSpace(usageLog.RequestID),
		GroupID:            *p.APIKey.GroupID,
		GroupGeneration:    p.APIKey.Group.PoolCapacityAlertGeneration,
		AccountID:          p.Account.ID,
		APIKeyID:           p.APIKey.ID,
		UserID:             p.User.ID,
		BillingType:        usageLog.BillingType,
		IsSubscriptionBill: p.IsSubscriptionBill,
		GroupName:          p.APIKey.Group.Name,
		AccountName:        p.Account.Name,
		APIKeyName:         p.APIKey.Name,
		UserEmail:          p.User.Email,
		CurrentAccountCost: accountCost,
		CurrentActualCost:  p.Cost.ActualCost,
		APIKeyQuota:        p.APIKey.Quota,
		APIKeyStatus:       p.APIKey.Status,
		NewBalance:         cloneFloat64Ptr(result.NewBalance),
		AccountQuotaState:  cloneAccountQuotaState(result.QuotaState),
		APIKeyQuotaState:   cloneAPIKeyQuotaUsageState(result.APIKeyQuotaState),
	}
	select {
	case s.queue <- task:
	default:
		slog.Warn("pool capacity alert evaluation queue full", "group_id", task.GroupID, "account_id", task.AccountID, "api_key_id", task.APIKeyID)
	}
}

func cloneFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneAccountQuotaState(value *AccountQuotaState) *AccountQuotaState {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneAPIKeyQuotaUsageState(value *APIKeyQuotaUsageState) *APIKeyQuotaUsageState {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func effectiveUsageLogAccountCost(log *UsageLog) float64 {
	if log == nil {
		return 0
	}
	base := log.TotalCost
	if log.AccountStatsCost != nil {
		base = *log.AccountStatsCost
	}
	multiplier := 1.0
	if log.AccountRateMultiplier != nil {
		multiplier = *log.AccountRateMultiplier
	}
	return base * multiplier
}

func (s *PoolCapacityAlertService) evaluationWorker(ctx context.Context) {
	defer s.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case task := <-s.queue:
			taskCtx, cancel := context.WithTimeout(ctx, s.evaluationTimeout())
			err := s.evaluate(taskCtx, task)
			cancel()
			if err != nil && !errors.Is(err, context.Canceled) {
				slog.Error("pool capacity alert evaluation failed", "group_id", task.GroupID, "account_id", task.AccountID, "api_key_id", task.APIKeyID, "error", err)
			}
		}
	}
}

func (s *PoolCapacityAlertService) evaluationTimeout() time.Duration {
	seconds := 15
	if s.cfg != nil && s.cfg.PoolCapacityAlert.EvaluationTimeoutSeconds > 0 {
		seconds = s.cfg.PoolCapacityAlert.EvaluationTimeoutSeconds
	}
	return time.Duration(seconds) * time.Second
}

func (s *PoolCapacityAlertService) evaluate(ctx context.Context, task poolCapacityAlertTask) error {
	group, err := s.groupRepo.GetByIDLite(ctx, task.GroupID)
	if err != nil {
		return err
	}
	if !group.PoolCapacityAlertEnabled || group.PoolCapacityAlertGeneration != task.GroupGeneration {
		return nil
	}
	account, err := s.accountRepo.GetByID(ctx, task.AccountID)
	if err != nil {
		return err
	}
	if !account.IsPoolMode() || s.capacityReader == nil {
		return nil
	}

	upstreamCapacity, capacityErr := s.capacityReader.GetPoolBalance(ctx, account, false)
	if capacityErr != nil {
		if errors.Is(capacityErr, context.Canceled) || errors.Is(capacityErr, context.DeadlineExceeded) {
			return capacityErr
		}
		return nil
	}

	policy := group.PoolCapacityAlertPolicy()
	metric := policy.Metric
	thresholdRequests := policy.ThresholdRequests
	evaluation := PoolCapacityEvaluation{
		GroupID:             task.GroupID,
		GroupGeneration:     task.GroupGeneration,
		AccountID:           task.AccountID,
		APIKeyID:            task.APIKeyID,
		UserID:              task.UserID,
		BillingType:         task.BillingType,
		GroupName:           firstNonEmpty(group.Name, task.GroupName),
		AccountName:         firstNonEmpty(account.Name, task.AccountName),
		APIKeyName:          task.APIKeyName,
		UserEmail:           task.UserEmail,
		AlertMetric:         metric,
		ThresholdRequests:   &thresholdRequests,
		ThresholdUSD:        cloneFloat64Ptr(policy.ThresholdUSD),
		QQBotAppID:          s.activeQQBotAppID(),
		ReminderCooldown:    s.reminderCooldown(),
		DeliveryMaxAttempts: s.deliveryMaxAttempts(),
	}

	switch metric {
	case PoolCapacityAlertMetricRemainingBalanceUSD:
		if policy.ThresholdUSD == nil {
			return nil
		}
		upstreamRemaining, upstreamKnown := calculatePoolUpstreamBalanceUSD(upstreamCapacity)
		if !upstreamKnown {
			return nil
		}
		localRemaining, localKnown := calculatePoolAccountRemainingUSD(account, task.AccountQuotaState)
		if account.HasAnyQuotaLimit() && !localKnown {
			return nil
		}
		accountRemaining := minimumFiniteAmountValue(upstreamRemaining, localRemaining)
		apiKeyRemaining, apiKeyKnown := calculatePoolAPIKeyRemainingUSD(task)
		if task.APIKeyQuota > 0 && !apiKeyKnown {
			return nil
		}
		walletRemaining, walletKnown := s.calculatePoolWalletRemainingUSD(task)
		if !task.IsSubscriptionBill && !walletKnown {
			return nil
		}
		remaining, bottleneck := minimumFiniteAmount(accountRemaining, apiKeyRemaining, walletRemaining)
		evaluation.RemainingBalanceUSD = remaining
		evaluation.AccountRemaining = accountRemaining
		evaluation.APIKeyRemaining = apiKeyRemaining
		evaluation.WalletRemaining = walletRemaining
		evaluation.Bottleneck = bottleneck

	case PoolCapacityAlertMetricPredictedRequests:
		if s.usageReader == nil {
			return nil
		}
		history, historyErr := s.usageReader.GetRecentPoolCapacityCostSummary(ctx, task.GroupID, task.RequestID, task.APIKeyID, PoolCapacityAlertSampleSize-1)
		if historyErr != nil {
			return historyErr
		}
		avgAccount, avgActual, ready := averagePoolCapacityCosts(history, task.CurrentAccountCost, task.CurrentActualCost)
		if !ready {
			return nil
		}
		upstreamRequests, upstreamRemaining, upstreamKnown := calculatePoolUpstreamCapacity(upstreamCapacity, avgAccount)
		if !upstreamKnown {
			return nil
		}
		localRequests, localRemaining, localKnown := calculatePoolAccountCapacity(account, task.AccountQuotaState, avgAccount)
		if account.HasAnyQuotaLimit() && !localKnown {
			return nil
		}
		accountRequests, accountRemaining := minimumAccountCapacity(upstreamRequests, upstreamRemaining, localRequests, localRemaining)
		apiKeyRequests, apiKeyRemaining, apiKeyKnown := calculatePoolAPIKeyCapacity(task, avgActual)
		if task.APIKeyQuota > 0 && !apiKeyKnown {
			return nil
		}
		walletRequests, walletRemaining, walletKnown := s.calculatePoolWalletCapacity(task, avgActual)
		if !task.IsSubscriptionBill && !walletKnown {
			return nil
		}
		predicted, bottleneck := minimumFiniteCapacity(accountRequests, apiKeyRequests, walletRequests)
		evaluation.PredictedRequests = predicted
		evaluation.AccountRequests = accountRequests
		evaluation.APIKeyRequests = apiKeyRequests
		evaluation.WalletRequests = walletRequests
		evaluation.AverageAccountCost = avgAccount.InexactFloat64()
		evaluation.AverageActualCost = avgActual.InexactFloat64()
		evaluation.AccountRemaining = accountRemaining
		evaluation.APIKeyRemaining = apiKeyRemaining
		evaluation.WalletRemaining = walletRemaining
		evaluation.SampleCount = PoolCapacityAlertSampleSize
		evaluation.Bottleneck = bottleneck

	default:
		return nil
	}

	_, err = s.repo.EvaluateAndMaybeCreateEvent(ctx, evaluation, time.Now().UTC())
	return err
}

func (s *PoolCapacityAlertService) activeQQBotAppID() string {
	if s.qqNotifier == nil {
		return ""
	}
	appID, ok := s.qqNotifier.ActiveQQBotAppID()
	if !ok {
		return ""
	}
	return strings.TrimSpace(appID)
}

func averagePoolCapacityCosts(history *PoolCapacityCostSummary, currentAccountCost, currentActualCost float64) (decimal.Decimal, decimal.Decimal, bool) {
	if history == nil || history.Count != PoolCapacityAlertSampleSize-1 || currentAccountCost <= 0 || currentActualCost <= 0 {
		return decimal.Zero, decimal.Zero, false
	}
	accountSum := history.AccountCostSum.Add(decimal.NewFromFloat(currentAccountCost))
	actualSum := history.ActualCostSum.Add(decimal.NewFromFloat(currentActualCost))
	divisor := decimal.NewFromInt(PoolCapacityAlertSampleSize)
	avgAccount := accountSum.Div(divisor)
	avgActual := actualSum.Div(divisor)
	return avgAccount, avgActual, avgAccount.IsPositive() && avgActual.IsPositive()
}

func calculatePoolUpstreamCapacity(snapshot *AccountCapacitySnapshot, average decimal.Decimal) (*int64, *float64, bool) {
	if snapshot == nil || snapshot.Mode != AccountCapacityModeUpstreamBalance || !snapshot.Authoritative {
		return nil, nil, false
	}
	if snapshot.State == AccountCapacityStateUnlimited {
		return nil, nil, true
	}
	if snapshot.State != AccountCapacityStateVerified || snapshot.Remaining == nil || *snapshot.Remaining < 0 || math.IsNaN(*snapshot.Remaining) || math.IsInf(*snapshot.Remaining, 0) {
		return nil, nil, false
	}

	remaining := *snapshot.Remaining
	switch strings.ToLower(strings.TrimSpace(snapshot.Unit)) {
	case "usd", "$":
		if !average.IsPositive() {
			return nil, nil, false
		}
		requests := decimal.NewFromFloat(remaining).Div(average).Floor().IntPart()
		return &requests, cloneFloat64Ptr(snapshot.Remaining), true
	case "request", "requests":
		requests := decimal.NewFromFloat(remaining).Floor().IntPart()
		return &requests, nil, true
	default:
		return nil, nil, false
	}
}

func calculatePoolUpstreamBalanceUSD(snapshot *AccountCapacitySnapshot) (*float64, bool) {
	if snapshot == nil || snapshot.Mode != AccountCapacityModeUpstreamBalance || !snapshot.Authoritative {
		return nil, false
	}
	if snapshot.State == AccountCapacityStateUnlimited {
		return nil, true
	}
	if snapshot.State != AccountCapacityStateVerified || snapshot.Remaining == nil || *snapshot.Remaining < 0 || math.IsNaN(*snapshot.Remaining) || math.IsInf(*snapshot.Remaining, 0) {
		return nil, false
	}
	switch strings.ToLower(strings.TrimSpace(snapshot.Unit)) {
	case "usd", "$":
		return cloneFloat64Ptr(snapshot.Remaining), true
	default:
		return nil, false
	}
}

func minimumAccountCapacity(upstreamRequests *int64, upstreamRemaining *float64, localRequests *int64, localRemaining *float64) (*int64, *float64) {
	if upstreamRequests == nil {
		return cloneCapacityInt64Ptr(localRequests), cloneFloat64Ptr(localRemaining)
	}
	if localRequests == nil || *upstreamRequests <= *localRequests {
		return cloneCapacityInt64Ptr(upstreamRequests), cloneFloat64Ptr(upstreamRemaining)
	}
	return cloneCapacityInt64Ptr(localRequests), cloneFloat64Ptr(localRemaining)
}

func calculatePoolAccountCapacity(account *Account, state *AccountQuotaState, average decimal.Decimal) (*int64, *float64, bool) {
	remaining, known := calculatePoolAccountRemainingUSD(account, state)
	if !known || !average.IsPositive() {
		return nil, nil, false
	}
	if remaining == nil {
		return nil, nil, true
	}
	requests := decimal.NewFromFloat(*remaining).Div(average).Floor().IntPart()
	return &requests, remaining, true
}

func calculatePoolAccountRemainingUSD(account *Account, state *AccountQuotaState) (*float64, bool) {
	if account == nil {
		return nil, false
	}
	type dimension struct {
		limit float64
		used  float64
	}
	dimensions := make([]dimension, 0, 3)
	if state != nil {
		dimensions = append(dimensions,
			dimension{limit: state.TotalLimit, used: state.TotalUsed},
			dimension{limit: state.DailyLimit, used: state.DailyUsed},
			dimension{limit: state.WeeklyLimit, used: state.WeeklyUsed},
		)
	} else {
		dailyUsed := account.GetQuotaDailyUsed()
		if account.IsDailyQuotaPeriodExpired() {
			dailyUsed = 0
		}
		weeklyUsed := account.GetQuotaWeeklyUsed()
		if account.IsWeeklyQuotaPeriodExpired() {
			weeklyUsed = 0
		}
		dimensions = append(dimensions,
			dimension{limit: account.GetQuotaLimit(), used: account.GetQuotaUsed()},
			dimension{limit: account.GetQuotaDailyLimit(), used: dailyUsed},
			dimension{limit: account.GetQuotaWeeklyLimit(), used: weeklyUsed},
		)
	}
	var minimumRemaining *float64
	for _, dim := range dimensions {
		if math.IsNaN(dim.limit) || math.IsInf(dim.limit, 0) {
			return nil, false
		}
		if dim.limit <= 0 {
			continue
		}
		if dim.used < 0 || math.IsNaN(dim.used) || math.IsInf(dim.used, 0) {
			return nil, false
		}
		remaining := math.Max(dim.limit-dim.used, 0)
		if minimumRemaining == nil || remaining < *minimumRemaining {
			remainingValue := remaining
			minimumRemaining = &remainingValue
		}
	}
	return minimumRemaining, true
}

func calculatePoolAPIKeyCapacity(task poolCapacityAlertTask, average decimal.Decimal) (*int64, *float64, bool) {
	remaining, known := calculatePoolAPIKeyRemainingUSD(task)
	if !known || !average.IsPositive() {
		return nil, nil, false
	}
	if remaining == nil {
		return nil, nil, true
	}
	requests := decimal.NewFromFloat(*remaining).Div(average).Floor().IntPart()
	return &requests, remaining, true
}

func calculatePoolAPIKeyRemainingUSD(task poolCapacityAlertTask) (*float64, bool) {
	if math.IsNaN(task.APIKeyQuota) || math.IsInf(task.APIKeyQuota, 0) {
		return nil, false
	}
	if task.APIKeyQuota <= 0 {
		return nil, true
	}
	if task.APIKeyQuotaState == nil || task.APIKeyQuotaState.Quota < 0 || task.APIKeyQuotaState.QuotaUsed < 0 ||
		math.IsNaN(task.APIKeyQuotaState.Quota) || math.IsInf(task.APIKeyQuotaState.Quota, 0) ||
		math.IsNaN(task.APIKeyQuotaState.QuotaUsed) || math.IsInf(task.APIKeyQuotaState.QuotaUsed, 0) {
		return nil, false
	}
	remaining := math.Max(task.APIKeyQuotaState.Quota-task.APIKeyQuotaState.QuotaUsed, 0)
	if task.APIKeyQuotaState.Status == StatusAPIKeyQuotaExhausted || task.APIKeyQuotaState.Status == StatusDisabled {
		remaining = 0
	}
	return &remaining, true
}

func (s *PoolCapacityAlertService) calculatePoolWalletCapacity(task poolCapacityAlertTask, average decimal.Decimal) (*int64, *float64, bool) {
	if task.IsSubscriptionBill {
		return nil, nil, true
	}
	if task.NewBalance == nil || !average.IsPositive() || math.IsNaN(*task.NewBalance) || math.IsInf(*task.NewBalance, 0) {
		return nil, nil, false
	}
	remainingValue := math.Max(*task.NewBalance, 0)
	remaining := &remainingValue
	requests := decimal.NewFromFloat(remainingValue).Div(average).Floor().IntPart()
	return &requests, remaining, true
}

func (s *PoolCapacityAlertService) calculatePoolWalletRemainingUSD(task poolCapacityAlertTask) (*float64, bool) {
	if task.IsSubscriptionBill {
		return nil, true
	}
	if task.NewBalance == nil || math.IsNaN(*task.NewBalance) || math.IsInf(*task.NewBalance, 0) {
		return nil, false
	}
	reserve := 0.0
	if s.cfg != nil && s.cfg.Billing.MinimumBalanceReserve > 0 {
		reserve = s.cfg.Billing.MinimumBalanceReserve
	}
	if math.IsNaN(reserve) || math.IsInf(reserve, 0) {
		return nil, false
	}
	remaining := math.Max(*task.NewBalance-reserve, 0)
	return &remaining, true
}

func minimumFiniteCapacity(named ...*int64) (*int64, string) {
	labels := []string{"account", "api_key", "wallet"}
	var minimum *int64
	bottleneck := ""
	for index, value := range named {
		if value == nil {
			continue
		}
		if minimum == nil || *value < *minimum {
			copyValue := *value
			minimum = &copyValue
			bottleneck = labels[index]
		}
	}
	return minimum, bottleneck
}

func minimumFiniteAmountValue(values ...*float64) *float64 {
	minimum, _ := minimumFiniteAmount(values...)
	return minimum
}

func minimumFiniteAmount(named ...*float64) (*float64, string) {
	labels := []string{"account", "api_key", "wallet"}
	var minimum *float64
	bottleneck := ""
	for index, value := range named {
		if value == nil {
			continue
		}
		if minimum == nil || *value < *minimum {
			copyValue := *value
			minimum = &copyValue
			if index < len(labels) {
				bottleneck = labels[index]
			}
		}
	}
	return minimum, bottleneck
}

func (s *PoolCapacityAlertService) reminderCooldown() time.Duration {
	hours := 24
	if s.cfg != nil && s.cfg.PoolCapacityAlert.ReminderCooldownHours > 0 {
		hours = s.cfg.PoolCapacityAlert.ReminderCooldownHours
	}
	return time.Duration(hours) * time.Hour
}

func (s *PoolCapacityAlertService) deliveryMaxAttempts() int {
	attempts := 6
	if s.cfg != nil && s.cfg.PoolCapacityAlert.MaxAttempts > 0 {
		attempts = s.cfg.PoolCapacityAlert.MaxAttempts
	}
	return attempts
}

func (s *PoolCapacityAlertService) deliveryLoop(ctx context.Context) {
	defer s.wg.Done()
	interval := 5 * time.Second
	if s.cfg != nil && s.cfg.PoolCapacityAlert.PollIntervalSeconds > 0 {
		interval = time.Duration(s.cfg.PoolCapacityAlert.PollIntervalSeconds) * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.dispatchDue(ctx); err != nil && ctx.Err() == nil {
				slog.Error("pool capacity alert delivery dispatch failed", "error", err)
			}
		}
	}
}

func (s *PoolCapacityAlertService) dispatchDue(ctx context.Context) error {
	lease := 90 * time.Second
	batchSize := 50
	workers := 4
	if s.cfg != nil {
		if s.cfg.PoolCapacityAlert.LeaseSeconds > 0 {
			lease = time.Duration(s.cfg.PoolCapacityAlert.LeaseSeconds) * time.Second
		}
		if s.cfg.PoolCapacityAlert.DeliveryBatchSize > 0 {
			batchSize = s.cfg.PoolCapacityAlert.DeliveryBatchSize
		}
		if s.cfg.PoolCapacityAlert.DeliveryWorkerCount > 0 {
			workers = s.cfg.PoolCapacityAlert.DeliveryWorkerCount
		}
	}
	processed := 0
	for processed < batchSize {
		claimLimit := workers
		if remaining := batchSize - processed; claimLimit > remaining {
			claimLimit = remaining
		}
		deliveries, err := s.repo.ClaimDeliveries(ctx, s.owner, time.Now().UTC(), lease, claimLimit)
		if err != nil || len(deliveries) == 0 {
			return err
		}
		var wg sync.WaitGroup
		for index := range deliveries {
			delivery := deliveries[index]
			wg.Add(1)
			go func() {
				defer wg.Done()
				s.sendDelivery(ctx, delivery)
			}()
		}
		wg.Wait()
		processed += len(deliveries)
		if len(deliveries) < claimLimit {
			return nil
		}
	}
	return nil
}

func (s *PoolCapacityAlertService) sendDelivery(parent context.Context, delivery PoolCapacityAlertDelivery) {
	timeout := 20 * time.Second
	if s.cfg != nil && s.cfg.PoolCapacityAlert.SendTimeoutSeconds > 0 {
		timeout = time.Duration(s.cfg.PoolCapacityAlert.SendTimeoutSeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	current, err := s.repo.IsDeliveryCurrent(ctx, delivery.ID, s.owner)
	if err != nil {
		s.recordDeliveryFailure(parent, delivery, err)
		return
	}
	if !current {
		if markErr := s.repo.MarkDeliveryCancelled(parent, delivery.ID, s.owner, "group generation, administrator, or recipient is no longer current"); markErr != nil {
			slog.Error("mark stale pool capacity alert delivery cancelled failed", "delivery_id", delivery.ID, "error", markErr)
		}
		return
	}

	var sendErr error
	switch delivery.Channel {
	case PoolCapacityAlertChannelEmail:
		sendErr = s.sendEmailDelivery(ctx, delivery)
	case PoolCapacityAlertChannelQQBot:
		if s.qqNotifier == nil {
			sendErr = errors.New("qqbot notifier unavailable")
		} else {
			sendErr = s.qqNotifier.SendAdminProactiveAlert(ctx, delivery.IdentityChannelID, renderPoolCapacityQQMessage(delivery.Event))
		}
	default:
		_ = s.repo.MarkDeliveryCancelled(parent, delivery.ID, s.owner, "unsupported channel")
		return
	}
	if delivery.Channel == PoolCapacityAlertChannelQQBot && errors.Is(sendErr, ErrQQBotRecipientUnavailable) {
		if markErr := s.repo.MarkDeliveryCancelled(parent, delivery.ID, s.owner, "qqbot recipient is no longer active or bound"); markErr != nil {
			slog.Error("mark unavailable QQBot pool capacity alert delivery cancelled failed", "delivery_id", delivery.ID, "error", markErr)
		}
		return
	}
	if sendErr == nil {
		if markErr := s.repo.MarkDeliverySent(parent, delivery.ID, s.owner, time.Now().UTC()); markErr != nil {
			slog.Error("mark pool capacity alert delivery sent failed", "delivery_id", delivery.ID, "error", markErr)
		}
		return
	}

	s.recordDeliveryFailure(parent, delivery, sendErr)
}

func (s *PoolCapacityAlertService) recordDeliveryFailure(ctx context.Context, delivery PoolCapacityAlertDelivery, err error) {
	class := "temporary"
	var definitive interface{ Definitive() bool }
	if errors.As(err, &definitive) && definitive.Definitive() {
		class = "permanent"
	}
	var nextAttempt *time.Time
	if class == "temporary" && delivery.AttemptCount < delivery.MaxAttempts {
		delay := s.retryDelay(delivery.AttemptCount)
		next := time.Now().UTC().Add(delay)
		nextAttempt = &next
	}
	if markErr := s.repo.MarkDeliveryFailed(ctx, delivery.ID, s.owner, class, truncatePoolCapacityAlertError(err), nextAttempt); markErr != nil {
		slog.Error("mark pool capacity alert delivery failed", "delivery_id", delivery.ID, "error", markErr)
	}
}

func (s *PoolCapacityAlertService) sendEmailDelivery(ctx context.Context, delivery PoolCapacityAlertDelivery) error {
	if s.notifications == nil {
		return errors.New("notification email service unavailable")
	}
	event := delivery.Event
	return s.notifications.Send(ctx, NotificationEmailSendInput{
		Event:          NotificationEmailEventPoolCapacityLow,
		Locale:         delivery.Locale,
		RecipientEmail: delivery.RecipientEmail,
		RecipientName:  delivery.RecipientName,
		UserID:         delivery.RecipientUserID,
		SourceType:     "pool_capacity_alert",
		SourceID:       strconv.FormatInt(event.ID, 10),
		ReminderKey:    strconv.FormatInt(event.Episode, 10),
		Variables:      poolCapacityAlertEmailVariables(event),
	})
}

func (s *PoolCapacityAlertService) retryDelay(attempt int) time.Duration {
	base := 30
	maxSeconds := 3600
	if s.cfg != nil {
		if s.cfg.PoolCapacityAlert.RetryBaseSeconds > 0 {
			base = s.cfg.PoolCapacityAlert.RetryBaseSeconds
		}
		if s.cfg.PoolCapacityAlert.MaxRetrySeconds > 0 {
			maxSeconds = s.cfg.PoolCapacityAlert.MaxRetrySeconds
		}
	}
	if attempt < 1 {
		attempt = 1
	}
	delay := time.Duration(float64(base)*math.Pow(2, float64(attempt-1))) * time.Second
	maximum := time.Duration(maxSeconds) * time.Second
	if delay > maximum {
		delay = maximum
	}
	return delay
}

func poolCapacityAlertEmailVariables(event PoolCapacityAlertEvent) map[string]string {
	metric := strings.TrimSpace(event.AlertMetric)
	if metric == "" {
		metric = PoolCapacityAlertMetricPredictedRequests
	}
	alertValue := "N/A"
	alertThreshold := "N/A"
	alertUnit := ""
	alertLabel := "Capacity"
	alertLabelZH := "容量"
	alertSummary := "The selected pool-mode capacity is below its configured threshold."
	alertSummaryZH := "本次实际命中的池模式容量已低于配置阈值。"
	disclaimer := "Only complete, authoritative capacity evaluations can change the alert state."
	disclaimerZH := "只有完整且可信的容量评估才会改变告警状态。"

	switch metric {
	case PoolCapacityAlertMetricRemainingBalanceUSD:
		alertValue = formatOptionalMetricAmount(event.RemainingBalanceUSD)
		alertThreshold = formatOptionalMetricAmount(event.ThresholdUSD)
		alertUnit = "USD"
		alertLabel = "Remaining balance"
		alertLabelZH = "剩余可用金额"
		alertSummary = fmt.Sprintf("The remaining comparable balance is $%s, below the configured $%s threshold.", alertValue, alertThreshold)
		alertSummaryZH = fmt.Sprintf("可比较的剩余金额为 $%s，已低于配置的 $%s 阈值。", alertValue, alertThreshold)
		disclaimer = "Uses only authoritative USD balances and post-billing local USD limits; unavailable or incompatible units are not treated as zero."
		disclaimerZH = "仅使用权威 USD 余额与扣费后本地 USD 限额；不可用或单位不兼容的数据不会按 0 处理。"
	case PoolCapacityAlertMetricPredictedRequests:
		alertValue = formatOptionalMetricCapacity(event.PredictedRequests)
		alertThreshold = formatOptionalMetricCapacity(event.ThresholdRequests)
		alertUnit = "requests"
		alertLabel = "Estimated remaining requests"
		alertLabelZH = "预计剩余请求数"
		alertSummary = fmt.Sprintf("The estimated remaining request capacity is %s, below the configured %s-request threshold.", alertValue, alertThreshold)
		alertSummaryZH = fmt.Sprintf("预计剩余请求数为 %s，已低于配置的 %s 次阈值。", alertValue, alertThreshold)
		disclaimer = "Estimate based on the latest 50 successfully billed requests; actual capacity may vary."
		disclaimerZH = "基于最近 50 次成功计费请求的历史均值估算，实际可用请求数可能变化。"
	}

	requestMetric := metric == PoolCapacityAlertMetricPredictedRequests
	accountRequests := "N/A"
	apiKeyRequests := "N/A"
	walletRequests := "N/A"
	if requestMetric {
		accountRequests = formatOptionalCapacity(event.AccountRequests)
		apiKeyRequests = formatOptionalCapacity(event.APIKeyRequests)
		walletRequests = formatOptionalCapacity(event.WalletRequests)
	}
	averageAccountCost := "N/A"
	averageActualCost := "N/A"
	if event.SampleCount > 0 {
		averageAccountCost = fmt.Sprintf("%.6f", event.AverageAccountCost)
		averageActualCost = fmt.Sprintf("%.6f", event.AverageActualCost)
	}

	return map[string]string{
		"group_name":               event.GroupName,
		"group_id":                 strconv.FormatInt(event.GroupID, 10),
		"account_name":             event.AccountName,
		"account_id":               strconv.FormatInt(event.AccountID, 10),
		"api_key_name":             event.APIKeyName,
		"api_key_id":               strconv.FormatInt(event.APIKeyID, 10),
		"user_id":                  strconv.FormatInt(event.UserID, 10),
		"alert_metric":             metric,
		"alert_metric_label":       alertLabel,
		"alert_metric_label_zh":    alertLabelZH,
		"alert_metric_value":       alertValue,
		"alert_metric_threshold":   alertThreshold,
		"alert_metric_unit":        alertUnit,
		"alert_summary":            alertSummary,
		"alert_summary_zh":         alertSummaryZH,
		"predicted_requests":       formatOptionalMetricCapacity(event.PredictedRequests),
		"remaining_balance_usd":    formatOptionalMetricAmount(event.RemainingBalanceUSD),
		"threshold_requests":       formatOptionalMetricCapacity(event.ThresholdRequests),
		"threshold_usd":            formatOptionalMetricAmount(event.ThresholdUSD),
		"avg_account_cost":         averageAccountCost,
		"avg_actual_cost":          averageActualCost,
		"account_requests":         accountRequests,
		"api_key_requests":         apiKeyRequests,
		"wallet_requests":          walletRequests,
		"account_remaining":        formatOptionalAmount(event.AccountRemaining),
		"api_key_remaining":        formatOptionalAmount(event.APIKeyRemaining),
		"wallet_remaining":         formatOptionalAmount(event.WalletRemaining),
		"bottleneck":               event.Bottleneck,
		"sample_count":             strconv.Itoa(event.SampleCount),
		"triggered_at":             event.CreatedAt.UTC().Format(time.RFC3339),
		"prediction_disclaimer":    disclaimer,
		"prediction_disclaimer_zh": disclaimerZH,
	}
}

func renderPoolCapacityQQMessage(event PoolCapacityAlertEvent) string {
	if event.AlertMetric == PoolCapacityAlertMetricRemainingBalanceUSD {
		return fmt.Sprintf("【池账户容量提醒】\n分组：%s (#%d)\n账户：%s (#%d)\nAPI Key：%s (#%d)\n剩余可用金额：$%s（阈值 $%s）\n账户/Key/用户余额：%s / %s / %s USD\n瓶颈：%s\n时间：%s\n仅使用权威 USD 余额与扣费后本地 USD 限额；未知或单位不兼容的数据不会按 0 处理。",
			event.GroupName, event.GroupID,
			event.AccountName, event.AccountID,
			event.APIKeyName, event.APIKeyID,
			formatOptionalMetricAmount(event.RemainingBalanceUSD), formatOptionalMetricAmount(event.ThresholdUSD),
			formatOptionalAmount(event.AccountRemaining), formatOptionalAmount(event.APIKeyRemaining), formatOptionalAmount(event.WalletRemaining),
			event.Bottleneck, event.CreatedAt.UTC().Format(time.RFC3339),
		)
	}
	return fmt.Sprintf("【池账户容量提醒】\n分组：%s (#%d)\n账户：%s (#%d)\nAPI Key：%s (#%d)\n预计剩余：%s 次（阈值 %s）\n账户/Key/用户容量：%s / %s / %s\n最近 %d 次均值：账户 $%.6f，用户 $%.6f\n瓶颈：%s\n时间：%s\n该结果基于最近 50 次成功计费请求的历史均值估算，实际容量可能变化。",
		event.GroupName, event.GroupID,
		event.AccountName, event.AccountID,
		event.APIKeyName, event.APIKeyID,
		formatOptionalMetricCapacity(event.PredictedRequests), formatOptionalMetricCapacity(event.ThresholdRequests),
		formatOptionalCapacity(event.AccountRequests), formatOptionalCapacity(event.APIKeyRequests), formatOptionalCapacity(event.WalletRequests),
		event.SampleCount, event.AverageAccountCost, event.AverageActualCost,
		event.Bottleneck, event.CreatedAt.UTC().Format(time.RFC3339),
	)
}

func formatOptionalCapacity(value *int64) string {
	if value == nil {
		return "unlimited"
	}
	return strconv.FormatInt(*value, 10)
}

func formatOptionalMetricCapacity(value *int64) string {
	if value == nil {
		return "N/A"
	}
	return strconv.FormatInt(*value, 10)
}

func formatOptionalMetricAmount(value *float64) string {
	if value == nil {
		return "N/A"
	}
	return fmt.Sprintf("%.6f", *value)
}

func formatOptionalAmount(value *float64) string {
	if value == nil {
		return "unlimited"
	}
	return fmt.Sprintf("%.6f", *value)
}

func truncatePoolCapacityAlertError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 2000 {
		message = message[:2000]
	}
	return message
}
