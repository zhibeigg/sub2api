package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

// ConcurrencyCache 定义并发控制的缓存接口
// 使用有序集合存储槽位，按时间戳清理过期条目
type ConcurrencyCache interface {
	// 账号槽位管理
	// 键格式: concurrency:account:{accountID}（有序集合，成员为 requestID）
	AcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int, requestID string) (bool, error)
	ReleaseAccountSlot(ctx context.Context, accountID int64, requestID string) error
	GetAccountConcurrency(ctx context.Context, accountID int64) (int, error)
	GetAccountConcurrencyBatch(ctx context.Context, accountIDs []int64) (map[int64]int, error)

	// 账号等待队列（账号级）
	IncrementAccountWaitCount(ctx context.Context, accountID int64, maxWait int) (bool, error)
	DecrementAccountWaitCount(ctx context.Context, accountID int64) error
	GetAccountWaitingCount(ctx context.Context, accountID int64) (int, error)

	// 用户槽位管理
	// 键格式: concurrency:user:{userID}（有序集合，成员为 requestID）
	AcquireUserSlot(ctx context.Context, userID int64, maxConcurrency int, requestID string) (bool, error)
	ReleaseUserSlot(ctx context.Context, userID int64, requestID string) error
	GetUserConcurrency(ctx context.Context, userID int64) (int, error)

	// 等待队列计数（每次入队都会刷新 TTL，避免长时间排队时计数提前过期）
	IncrementWaitCount(ctx context.Context, userID int64, maxWait int) (bool, error)
	DecrementWaitCount(ctx context.Context, userID int64) error

	// 批量负载查询（只读）
	GetAccountsLoadBatch(ctx context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error)
	GetUsersLoadBatch(ctx context.Context, users []UserWithConcurrency) (map[int64]*UserLoadInfo, error)

	// 清理过期槽位（后台任务）
	CleanupExpiredAccountSlots(ctx context.Context, accountID int64) error
	CleanupExpiredAccountSlotKeys(ctx context.Context) error

	// 启动时清理旧进程遗留槽位与等待计数
	CleanupStaleProcessSlots(ctx context.Context, activeRequestPrefix string) error
}

type APIKeyConcurrencyCache interface {
	TrackAPIKeySlot(ctx context.Context, apiKeyID int64, requestID string) error
	ReleaseAPIKeySlot(ctx context.Context, apiKeyID int64, requestID string) error
	GetAPIKeyConcurrencyBatch(ctx context.Context, apiKeyIDs []int64) (map[int64]int, error)
}

// SubscriptionConcurrencyCache is an optional capability implemented by caches
// that can enforce request concurrency per user-subscription instance. Keeping
// it separate avoids widening ConcurrencyCache for callers that only need the
// existing user/account scopes.
type SubscriptionConcurrencyCache interface {
	AcquireSubscriptionSlot(ctx context.Context, subscriptionID int64, maxConcurrency int, requestID string) (bool, error)
	RefreshSubscriptionSlot(ctx context.Context, subscriptionID int64, requestID string) (bool, error)
	ReleaseSubscriptionSlot(ctx context.Context, subscriptionID int64, requestID string) error
	IncrementSubscriptionWaitCount(ctx context.Context, subscriptionID int64, maxWait int) (bool, error)
	DecrementSubscriptionWaitCount(ctx context.Context, subscriptionID int64) error
}

// OpenAIWSIngressLeaseCache owns the short-lived distributed lease used to
// bound live client WebSocket sessions. It is deliberately independent of the
// request-slot namespace: idle ingress connections do not occupy turn slots.
type OpenAIWSIngressLeaseCache interface {
	AcquireOpenAIWSIngressLease(ctx context.Context, apiKeyID int64, maxConnections int, leaseID string) (bool, error)
	RefreshOpenAIWSIngressLease(ctx context.Context, apiKeyID int64, leaseID string) (bool, error)
	ReleaseOpenAIWSIngressLease(ctx context.Context, apiKeyID int64, leaseID string) error
}

const (
	openAIWSIngressLeaseTTL             = 60 * time.Second
	openAIWSIngressLeaseRefreshInterval = 20 * time.Second
	openAIWSIngressLeaseOperationTO     = 2 * time.Second
)

var ErrOpenAIWSIngressLeaseLost = errors.New("openai websocket ingress lease lost")

// OpenAIWSIngressLease keeps a Redis-backed ingress lease alive and cancels
// its context if Redis cannot confirm ownership for a full lease lifetime.
// Call Release on every handler exit to reclaim capacity immediately.
type OpenAIWSIngressLease struct {
	ctx      context.Context
	cancel   context.CancelCauseFunc
	cache    OpenAIWSIngressLeaseCache
	apiKeyID int64
	leaseID  string

	stopOnce    sync.Once
	stopCh      chan struct{}
	refreshDone chan struct{}
}

func (l *OpenAIWSIngressLease) Context() context.Context {
	if l == nil || l.ctx == nil {
		return context.Background()
	}
	return l.ctx
}

func (l *OpenAIWSIngressLease) Release() {
	if l == nil {
		return
	}
	l.stopOnce.Do(func() {
		if l.stopCh != nil {
			close(l.stopCh)
		}
		if l.cancel != nil {
			l.cancel(nil)
		}
		if l.refreshDone != nil {
			<-l.refreshDone
		}
		if l.cache == nil || l.apiKeyID <= 0 || l.leaseID == "" {
			return
		}
		releaseCtx, releaseCancel := context.WithTimeout(context.Background(), openAIWSIngressLeaseOperationTO)
		defer releaseCancel()
		if err := l.cache.ReleaseOpenAIWSIngressLease(releaseCtx, l.apiKeyID, l.leaseID); err != nil {
			logger.L().Warn("openai_ws_ingress_lease_release_failed",
				zap.Int64("api_key_id", l.apiKeyID),
				zap.Error(err),
			)
		}
	})
}

func (l *OpenAIWSIngressLease) refreshLoop() {
	defer func() {
		if l != nil && l.refreshDone != nil {
			close(l.refreshDone)
		}
	}()
	if l == nil || l.cache == nil {
		return
	}
	ticker := time.NewTicker(openAIWSIngressLeaseRefreshInterval)
	defer ticker.Stop()
	lastConfirmedAt := time.Now()
	for {
		select {
		case <-l.ctx.Done():
			return
		case <-l.stopCh:
			return
		case <-ticker.C:
			var lost bool
			lastConfirmedAt, lost = l.refresh(lastConfirmedAt)
			if lost {
				l.cancel(ErrOpenAIWSIngressLeaseLost)
				return
			}
		}
	}
}

// refresh confirms the lease is still owned. A missing member is an immediate
// lease loss; transient Redis errors are tolerated only for one full lease TTL.
func (l *OpenAIWSIngressLease) refresh(lastConfirmedAt time.Time) (time.Time, bool) {
	refreshCtx, refreshCancel := context.WithTimeout(context.Background(), openAIWSIngressLeaseOperationTO)
	owned, err := l.cache.RefreshOpenAIWSIngressLease(refreshCtx, l.apiKeyID, l.leaseID)
	refreshCancel()
	if err == nil && owned {
		return time.Now(), false
	}
	if err == nil {
		err = ErrOpenAIWSIngressLeaseLost
	}
	elapsed := time.Since(lastConfirmedAt)
	logger.L().Warn("openai_ws_ingress_lease_refresh_failed",
		zap.Int64("api_key_id", l.apiKeyID),
		zap.Duration("unconfirmed_for", elapsed),
		zap.Error(err),
	)
	if errors.Is(err, ErrOpenAIWSIngressLeaseLost) || elapsed >= openAIWSIngressLeaseTTL {
		logger.L().Error("openai_ws_ingress_lease_lost",
			zap.Int64("api_key_id", l.apiKeyID),
			zap.Duration("unconfirmed_for", elapsed),
			zap.Error(err),
		)
		return lastConfirmedAt, true
	}
	return lastConfirmedAt, false
}

var (
	requestIDPrefix  = initRequestIDPrefix()
	requestIDCounter atomic.Uint64
)

func initRequestIDPrefix() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err == nil {
		return "r" + strconv.FormatUint(binary.BigEndian.Uint64(b), 36)
	}
	fallback := uint64(time.Now().UnixNano()) ^ (uint64(os.Getpid()) << 16)
	return "r" + strconv.FormatUint(fallback, 36)
}

func RequestIDPrefix() string {
	return requestIDPrefix
}

func generateRequestID() string {
	seq := requestIDCounter.Add(1)
	return requestIDPrefix + "-" + strconv.FormatUint(seq, 36)
}

func (s *ConcurrencyService) CleanupStaleProcessSlots(ctx context.Context) error {
	if s == nil || s.cache == nil {
		return nil
	}
	return s.cache.CleanupStaleProcessSlots(ctx, RequestIDPrefix())
}

const (
	// 默认等待队列额外槽位
	defaultExtraWaitSlots = 20

	defaultAccountLoadBatchCacheTTL = 200 * time.Millisecond
	accountLoadBatchFetchTimeout    = 3 * time.Second
	maxAccountLoadBatchCacheEntries = 256
	apiKeyConcurrencyFetchTimeout   = 3 * time.Second
	apiKeySlotTrackTimeout          = 2 * time.Second

	// Subscription slots use a minimum one-minute Redis TTL in the repository.
	// Refreshing every 20 seconds leaves multiple retry opportunities before expiry.
	defaultSubscriptionLeaseRefreshInterval = 20 * time.Second
	defaultSubscriptionLeaseRetryInterval   = 2 * time.Second
	subscriptionLeaseOperationTimeout       = 2 * time.Second
	subscriptionLeaseReleaseTimeout         = 5 * time.Second
)

var errSubscriptionConcurrencyLeaseLost = errors.New("subscription concurrency lease lost")

// ConcurrencyService 管理账号和用户的并发限制。
type ConcurrencyService struct {
	cache ConcurrencyCache

	// These timings are copied into each subscription lease at acquisition time.
	// Tests in this package may shorten them without changing production defaults.
	subscriptionLeaseRefreshInterval time.Duration
	subscriptionLeaseRetryInterval   time.Duration
	subscriptionLeaseOperationTO     time.Duration

	accountLoadCacheTTL atomic.Int64
	accountLoadCacheMu  sync.RWMutex
	accountLoadCache    map[string]cachedAccountLoadBatch
	accountLoadGroup    singleflight.Group
}

type cachedAccountLoadBatch struct {
	loadMap   map[int64]*AccountLoadInfo
	expiresAt time.Time
}

// NewConcurrencyService 创建并发控制服务。
func NewConcurrencyService(cache ConcurrencyCache) *ConcurrencyService {
	svc := &ConcurrencyService{
		cache:                            cache,
		subscriptionLeaseRefreshInterval: defaultSubscriptionLeaseRefreshInterval,
		subscriptionLeaseRetryInterval:   defaultSubscriptionLeaseRetryInterval,
		subscriptionLeaseOperationTO:     subscriptionLeaseOperationTimeout,
		accountLoadCache:                 make(map[string]cachedAccountLoadBatch),
	}
	svc.SetAccountLoadBatchCacheTTL(defaultAccountLoadBatchCacheTTL)
	return svc
}

// AcquireOpenAIWSIngressLease atomically reserves one live ingress connection
// for an API key. A non-positive limit explicitly disables this protection.
func (s *ConcurrencyService) AcquireOpenAIWSIngressLease(ctx context.Context, apiKeyID int64, maxConnections int) (*OpenAIWSIngressLease, bool, error) {
	if maxConnections <= 0 {
		return nil, true, nil
	}
	if s == nil || s.cache == nil || apiKeyID <= 0 {
		return nil, false, errors.New("openai websocket ingress lease cache is unavailable")
	}
	cache, ok := s.cache.(OpenAIWSIngressLeaseCache)
	if !ok {
		return nil, false, errors.New("openai websocket ingress lease cache is unsupported")
	}
	leaseID := generateRequestID()
	baseCtx := context.Background()
	if ctx != nil {
		baseCtx = context.WithoutCancel(ctx)
	}
	acquireCtx, acquireCancel := context.WithTimeout(baseCtx, openAIWSIngressLeaseOperationTO)
	acquired, err := cache.AcquireOpenAIWSIngressLease(acquireCtx, apiKeyID, maxConnections, leaseID)
	acquireCancel()
	if err != nil || !acquired {
		return nil, acquired, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	leaseCtx, leaseCancel := context.WithCancelCause(ctx)
	lease := &OpenAIWSIngressLease{
		ctx:         leaseCtx,
		cancel:      leaseCancel,
		cache:       cache,
		apiKeyID:    apiKeyID,
		leaseID:     leaseID,
		stopCh:      make(chan struct{}),
		refreshDone: make(chan struct{}),
	}
	go lease.refreshLoop()
	return lease, true, nil
}

// SetAccountLoadBatchCacheTTL 设置账号负载批量读取的极短 TTL 缓存；非正数表示禁用缓存。
func (s *ConcurrencyService) SetAccountLoadBatchCacheTTL(ttl time.Duration) {
	if s == nil {
		return
	}
	s.accountLoadCacheTTL.Store(int64(ttl))
	if ttl <= 0 {
		s.accountLoadCacheMu.Lock()
		s.accountLoadCache = make(map[string]cachedAccountLoadBatch)
		s.accountLoadCacheMu.Unlock()
	}
}

const (
	ConcurrencyScopeUser         = "user"
	ConcurrencyScopeSubscription = "subscription"
)

// AcquireResult represents the result of acquiring one or more concurrency slots.
type AcquireResult struct {
	Acquired     bool
	ReleaseFunc  func() // Must be called when done (typically via defer)
	BlockedScope string // Set when Acquired is false for a scoped request acquisition.
}

type AccountWithConcurrency struct {
	ID             int64
	MaxConcurrency int
}

type UserWithConcurrency struct {
	ID             int64
	MaxConcurrency int
}

type AccountLoadInfo struct {
	AccountID          int64
	CurrentConcurrency int
	WaitingCount       int
	LoadRate           int // 0-100+ (percent)
}

type UserLoadInfo struct {
	UserID             int64
	CurrentConcurrency int
	WaitingCount       int
	LoadRate           int // 0-100+ (percent)
}

// AcquireAccountSlot attempts to acquire a concurrency slot for an account.
// If the account is at max concurrency, it waits until a slot is available or timeout.
// Returns a release function that MUST be called when the request completes.
func (s *ConcurrencyService) AcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int) (*AcquireResult, error) {
	// If maxConcurrency is 0 or negative, no limit
	if maxConcurrency <= 0 {
		return &AcquireResult{
			Acquired:    true,
			ReleaseFunc: func() {}, // no-op
		}, nil
	}

	// Generate unique request ID for this slot
	requestID := generateRequestID()

	acquired, err := s.cache.AcquireAccountSlot(ctx, accountID, maxConcurrency, requestID)
	if err != nil {
		return nil, err
	}

	if acquired {
		return &AcquireResult{
			Acquired: true,
			ReleaseFunc: func() {
				bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := s.cache.ReleaseAccountSlot(bgCtx, accountID, requestID); err != nil {
					logger.LegacyPrintf("service.concurrency", "Warning: failed to release account slot for %d (req=%s): %v", accountID, requestID, err)
				}
			},
		}, nil
	}

	return &AcquireResult{
		Acquired:    false,
		ReleaseFunc: nil,
	}, nil
}

// AcquireUserSlot attempts to acquire a concurrency slot for a user.
// If the user is at max concurrency, it waits until a slot is available or timeout.
// Returns a release function that MUST be called when the request completes.
func (s *ConcurrencyService) AcquireUserSlot(ctx context.Context, userID int64, maxConcurrency int) (*AcquireResult, error) {
	// If maxConcurrency is 0 or negative, no limit
	if maxConcurrency <= 0 {
		return &AcquireResult{
			Acquired:    true,
			ReleaseFunc: func() {}, // no-op
		}, nil
	}

	// Generate unique request ID for this slot
	requestID := generateRequestID()

	acquired, err := s.cache.AcquireUserSlot(ctx, userID, maxConcurrency, requestID)
	if err != nil {
		return nil, err
	}

	if acquired {
		return &AcquireResult{
			Acquired: true,
			ReleaseFunc: func() {
				bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := s.cache.ReleaseUserSlot(bgCtx, userID, requestID); err != nil {
					logger.LegacyPrintf("service.concurrency", "Warning: failed to release user slot for %d (req=%s): %v", userID, requestID, err)
				}
			},
		}, nil
	}

	return &AcquireResult{
		Acquired:     false,
		ReleaseFunc:  nil,
		BlockedScope: ConcurrencyScopeUser,
	}, nil
}

const (
	subscriptionLeaseActive uint32 = iota
	subscriptionLeaseLossNotified
	subscriptionLeaseStopping
)

type subscriptionConcurrencyLease struct {
	cache          SubscriptionConcurrencyCache
	subscriptionID int64
	maxConcurrency int
	requestID      string
	userRelease    func()

	refreshInterval time.Duration
	retryInterval   time.Duration
	operationTO     time.Duration

	cancel      context.CancelFunc
	refreshDone chan struct{}
	releaseOnce sync.Once
	lifecycle   atomic.Uint32
	onLeaseLoss func(error)
}

func (l *subscriptionConcurrencyLease) Release() {
	if l == nil {
		return
	}
	l.releaseOnce.Do(func() {
		l.markStopping()
		// Stop and join the refresher before removing the Redis member. Besides
		// preventing leaks, this ordering guarantees an in-flight refresh cannot
		// race with release and extend a lease after the caller is done.
		if l.cancel != nil {
			l.cancel()
		}
		if l.refreshDone != nil {
			<-l.refreshDone
		}

		if l.cache != nil && l.subscriptionID > 0 && l.requestID != "" {
			releaseCtx, releaseCancel := context.WithTimeout(context.Background(), subscriptionLeaseReleaseTimeout)
			if err := l.cache.ReleaseSubscriptionSlot(releaseCtx, l.subscriptionID, l.requestID); err != nil {
				logger.L().Warn("subscription_concurrency_lease_release_failed",
					zap.Int64("subscription_id", l.subscriptionID),
					zap.String("request_id", l.requestID),
					zap.Error(err),
				)
			}
			releaseCancel()
		}
		if l.userRelease != nil {
			l.userRelease()
		}
	})
}

func (l *subscriptionConcurrencyLease) markStopping() {
	if l == nil {
		return
	}
	for {
		state := l.lifecycle.Load()
		if state == subscriptionLeaseStopping || l.lifecycle.CompareAndSwap(state, subscriptionLeaseStopping) {
			return
		}
	}
}

func (l *subscriptionConcurrencyLease) notifyLeaseLoss() {
	if l == nil || l.onLeaseLoss == nil {
		return
	}
	// Compete atomically with normal Release. Whichever transition wins defines
	// the outcome: a release that started first suppresses a false loss report;
	// a confirmed loss that won first invokes the callback exactly once.
	if !l.lifecycle.CompareAndSwap(subscriptionLeaseActive, subscriptionLeaseLossNotified) {
		return
	}
	l.onLeaseLoss(errSubscriptionConcurrencyLeaseLost)
}

func (l *subscriptionConcurrencyLease) refreshLoop(ctx context.Context) {
	defer close(l.refreshDone)
	timer := time.NewTimer(l.refreshInterval)
	defer timer.Stop()

	consecutiveFailures := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			confirmed, err := l.refreshOrReacquire(ctx)
			if ctx.Err() != nil {
				return
			}

			nextDelay := l.refreshInterval
			if err == nil && confirmed {
				if consecutiveFailures > 0 {
					logger.L().Info("subscription_concurrency_lease_refresh_recovered",
						zap.Int64("subscription_id", l.subscriptionID),
						zap.String("request_id", l.requestID),
						zap.Int("previous_failures", consecutiveFailures),
					)
				}
				consecutiveFailures = 0
			} else {
				consecutiveFailures++
				nextDelay = l.retryInterval
				l.logRefreshFailure(err, consecutiveFailures)
			}
			timer.Reset(nextDelay)
		}
	}
}

func (l *subscriptionConcurrencyLease) refreshOrReacquire(ctx context.Context) (bool, error) {
	refreshCtx, refreshCancel := context.WithTimeout(ctx, l.operationTO)
	owned, err := l.cache.RefreshSubscriptionSlot(refreshCtx, l.subscriptionID, l.requestID)
	refreshCancel()
	if err != nil || owned {
		return owned, err
	}

	// A confirmed missing member means the distributed constraint has already
	// lapsed. While the request is still active, conservatively try to reserve
	// the same lease again. Release cancels and joins this loop before ZREM, so
	// this recovery path cannot recreate a lease after an explicit release.
	reacquireCtx, reacquireCancel := context.WithTimeout(ctx, l.operationTO)
	reacquired, reacquireErr := l.cache.AcquireSubscriptionSlot(
		reacquireCtx,
		l.subscriptionID,
		l.maxConcurrency,
		l.requestID,
	)
	reacquireCancel()
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if reacquireErr != nil {
		l.notifyLeaseLoss()
		return false, reacquireErr
	}
	if !reacquired {
		l.notifyLeaseLoss()
		return false, errSubscriptionConcurrencyLeaseLost
	}
	logger.L().Warn("subscription_concurrency_lease_reacquired_after_loss",
		zap.Int64("subscription_id", l.subscriptionID),
		zap.String("request_id", l.requestID),
	)
	return true, nil
}

func (l *subscriptionConcurrencyLease) logRefreshFailure(err error, consecutiveFailures int) {
	// Continuous Redis failures are retried at a much shorter interval. Log the
	// first failure and periodic reminders to avoid silently losing enforcement
	// without flooding logs for every long-running request.
	if consecutiveFailures != 1 && consecutiveFailures%15 != 0 {
		return
	}
	if errors.Is(err, errSubscriptionConcurrencyLeaseLost) {
		logger.L().Error("subscription_concurrency_lease_lost",
			zap.Int64("subscription_id", l.subscriptionID),
			zap.String("request_id", l.requestID),
			zap.Int("consecutive_failures", consecutiveFailures),
			zap.Error(err),
		)
		return
	}
	logger.L().Warn("subscription_concurrency_lease_refresh_failed",
		zap.Int64("subscription_id", l.subscriptionID),
		zap.String("request_id", l.requestID),
		zap.Int("consecutive_failures", consecutiveFailures),
		zap.Error(err),
	)
}

func (s *ConcurrencyService) subscriptionLeaseTimings() (time.Duration, time.Duration, time.Duration) {
	refreshInterval := s.subscriptionLeaseRefreshInterval
	if refreshInterval <= 0 {
		refreshInterval = defaultSubscriptionLeaseRefreshInterval
	}
	retryInterval := s.subscriptionLeaseRetryInterval
	if retryInterval <= 0 || retryInterval >= refreshInterval {
		retryInterval = refreshInterval / 10
		if retryInterval <= 0 {
			retryInterval = time.Millisecond
		}
	}
	operationTO := s.subscriptionLeaseOperationTO
	if operationTO <= 0 {
		operationTO = subscriptionLeaseOperationTimeout
	}
	return refreshInterval, retryInterval, operationTO
}

// AcquireUserAndSubscriptionSlots preserves the existing API for callers that
// do not need an active notification when the distributed subscription lease is lost.
func (s *ConcurrencyService) AcquireUserAndSubscriptionSlots(
	ctx context.Context,
	userID int64,
	userMaxConcurrency int,
	subscriptionID int64,
	subscriptionMaxConcurrency int,
) (*AcquireResult, error) {
	return s.AcquireUserAndSubscriptionSlotsWithLeaseLoss(
		ctx,
		userID,
		userMaxConcurrency,
		subscriptionID,
		subscriptionMaxConcurrency,
		nil,
	)
}

// AcquireUserAndSubscriptionSlotsWithLeaseLoss acquires the existing user-global
// slot and an optional subscription-instance slot. If Redis confirms that the
// active subscription member disappeared and it cannot be reacquired under the
// original limit, onLeaseLoss is invoked exactly once for this acquisition.
func (s *ConcurrencyService) AcquireUserAndSubscriptionSlotsWithLeaseLoss(
	ctx context.Context,
	userID int64,
	userMaxConcurrency int,
	subscriptionID int64,
	subscriptionMaxConcurrency int,
	onLeaseLoss func(error),
) (*AcquireResult, error) {
	userResult, err := s.AcquireUserSlot(ctx, userID, userMaxConcurrency)
	if err != nil {
		return nil, err
	}
	if !userResult.Acquired {
		userResult.BlockedScope = ConcurrencyScopeUser
		return userResult, nil
	}

	if subscriptionID <= 0 || subscriptionMaxConcurrency <= 0 {
		return userResult, nil
	}
	if s == nil || s.cache == nil {
		userResult.ReleaseFunc()
		return nil, errors.New("subscription concurrency cache is unavailable")
	}
	cache, ok := s.cache.(SubscriptionConcurrencyCache)
	if !ok {
		userResult.ReleaseFunc()
		return nil, errors.New("subscription concurrency cache is unsupported")
	}

	requestID := generateRequestID()
	acquired, err := cache.AcquireSubscriptionSlot(ctx, subscriptionID, subscriptionMaxConcurrency, requestID)
	if err != nil {
		userResult.ReleaseFunc()
		return nil, err
	}
	if !acquired {
		userResult.ReleaseFunc()
		return &AcquireResult{
			Acquired:     false,
			BlockedScope: ConcurrencyScopeSubscription,
		}, nil
	}

	refreshInterval, retryInterval, operationTO := s.subscriptionLeaseTimings()
	refreshCtx, refreshCancel := context.WithCancel(context.Background())
	lease := &subscriptionConcurrencyLease{
		cache:           cache,
		subscriptionID:  subscriptionID,
		maxConcurrency:  subscriptionMaxConcurrency,
		requestID:       requestID,
		userRelease:     userResult.ReleaseFunc,
		refreshInterval: refreshInterval,
		retryInterval:   retryInterval,
		operationTO:     operationTO,
		cancel:          refreshCancel,
		refreshDone:     make(chan struct{}),
		onLeaseLoss:     onLeaseLoss,
	}
	go lease.refreshLoop(refreshCtx)

	return &AcquireResult{
		Acquired:    true,
		ReleaseFunc: lease.Release,
	}, nil
}

// TrackAPIKeySlot records one active request slot for an API key without
// applying key-level concurrency limits. It is fail-open: Redis errors are
// logged and return a no-op release function.
func (s *ConcurrencyService) TrackAPIKeySlot(ctx context.Context, apiKeyID int64) func() {
	if s == nil || s.cache == nil || apiKeyID <= 0 {
		return func() {}
	}
	cache, ok := s.cache.(APIKeyConcurrencyCache)
	if !ok {
		return func() {}
	}

	requestID := generateRequestID()
	baseCtx := context.Background()
	if ctx != nil {
		baseCtx = context.WithoutCancel(ctx)
	}
	trackCtx, cancel := context.WithTimeout(baseCtx, apiKeySlotTrackTimeout)
	err := cache.TrackAPIKeySlot(trackCtx, apiKeyID, requestID)
	cancel()
	if err != nil {
		logger.LegacyPrintf("service.concurrency", "Warning: failed to track api key slot for %d (req=%s): %v", apiKeyID, requestID, err)
		return func() {}
	}

	return func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := cache.ReleaseAPIKeySlot(bgCtx, apiKeyID, requestID); err != nil {
			logger.LegacyPrintf("service.concurrency", "Warning: failed to release api key slot for %d (req=%s): %v", apiKeyID, requestID, err)
		}
	}
}

// GetAPIKeyConcurrencyBatch gets real-time active request counts for API keys.
// Stats are best-effort: missing Redis support or Redis errors return zeroes.
func (s *ConcurrencyService) GetAPIKeyConcurrencyBatch(ctx context.Context, apiKeyIDs []int64) (map[int64]int, error) {
	result := zeroAPIKeyConcurrencyMap(apiKeyIDs)
	if len(apiKeyIDs) == 0 {
		return result, nil
	}
	if s == nil || s.cache == nil {
		return result, nil
	}
	cache, ok := s.cache.(APIKeyConcurrencyCache)
	if !ok {
		return result, nil
	}

	redisCtx, cancel := context.WithTimeout(context.Background(), apiKeyConcurrencyFetchTimeout)
	defer cancel()

	counts, err := cache.GetAPIKeyConcurrencyBatch(redisCtx, apiKeyIDs)
	if err != nil {
		logger.LegacyPrintf("service.concurrency", "Warning: get api key concurrency batch failed: %v", err)
		return result, nil
	}
	for _, apiKeyID := range apiKeyIDs {
		result[apiKeyID] = counts[apiKeyID]
	}
	return result, nil
}

func zeroAPIKeyConcurrencyMap(apiKeyIDs []int64) map[int64]int {
	result := make(map[int64]int, len(apiKeyIDs))
	for _, apiKeyID := range apiKeyIDs {
		result[apiKeyID] = 0
	}
	return result
}

// ============================================
// Wait Queue Count Methods
// ============================================

// IncrementWaitCount attempts to increment the wait queue counter for a user.
// Returns true if successful, false if the wait queue is full.
// maxWait should be user.Concurrency + defaultExtraWaitSlots
func (s *ConcurrencyService) IncrementWaitCount(ctx context.Context, userID int64, maxWait int) (bool, error) {
	if s.cache == nil {
		// Redis not available, allow request
		return true, nil
	}

	result, err := s.cache.IncrementWaitCount(ctx, userID, maxWait)
	if err != nil {
		// On error, allow the request to proceed (fail open)
		logger.LegacyPrintf("service.concurrency", "Warning: increment wait count failed for user %d: %v", userID, err)
		return true, nil
	}
	return result, nil
}

// DecrementWaitCount decrements the wait queue counter for a user.
// Should be called when a request completes or exits the wait queue.
func (s *ConcurrencyService) DecrementWaitCount(ctx context.Context, userID int64) {
	if s.cache == nil {
		return
	}

	// Use background context to ensure decrement even if original context is cancelled
	bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.cache.DecrementWaitCount(bgCtx, userID); err != nil {
		logger.LegacyPrintf("service.concurrency", "Warning: decrement wait count failed for user %d: %v", userID, err)
	}
}

// IncrementSubscriptionWaitCount bounds pending requests for one subscription
// instance without sharing queue capacity with the user's balance traffic or
// other subscription instances.
func (s *ConcurrencyService) IncrementSubscriptionWaitCount(ctx context.Context, subscriptionID int64, maxWait int) (bool, error) {
	if s == nil || s.cache == nil || subscriptionID <= 0 {
		return true, nil
	}
	cache, ok := s.cache.(SubscriptionConcurrencyCache)
	if !ok {
		return false, errors.New("subscription concurrency cache is unsupported")
	}
	result, err := cache.IncrementSubscriptionWaitCount(ctx, subscriptionID, maxWait)
	if err != nil {
		logger.LegacyPrintf("service.concurrency", "Warning: increment wait count failed for subscription %d: %v", subscriptionID, err)
		return true, nil
	}
	return result, nil
}

// DecrementSubscriptionWaitCount removes one pending request from the scoped
// wait counter, even when the original request context has been cancelled.
func (s *ConcurrencyService) DecrementSubscriptionWaitCount(ctx context.Context, subscriptionID int64) {
	if s == nil || s.cache == nil || subscriptionID <= 0 {
		return
	}
	cache, ok := s.cache.(SubscriptionConcurrencyCache)
	if !ok {
		return
	}
	bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := cache.DecrementSubscriptionWaitCount(bgCtx, subscriptionID); err != nil {
		logger.LegacyPrintf("service.concurrency", "Warning: decrement wait count failed for subscription %d: %v", subscriptionID, err)
	}
}

// IncrementAccountWaitCount increments the wait queue counter for an account.
func (s *ConcurrencyService) IncrementAccountWaitCount(ctx context.Context, accountID int64, maxWait int) (bool, error) {
	if s.cache == nil {
		return true, nil
	}

	result, err := s.cache.IncrementAccountWaitCount(ctx, accountID, maxWait)
	if err != nil {
		logger.LegacyPrintf("service.concurrency", "Warning: increment wait count failed for account %d: %v", accountID, err)
		return true, nil
	}
	return result, nil
}

// DecrementAccountWaitCount decrements the wait queue counter for an account.
func (s *ConcurrencyService) DecrementAccountWaitCount(ctx context.Context, accountID int64) {
	if s.cache == nil {
		return
	}

	bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.cache.DecrementAccountWaitCount(bgCtx, accountID); err != nil {
		logger.LegacyPrintf("service.concurrency", "Warning: decrement wait count failed for account %d: %v", accountID, err)
	}
}

// GetAccountWaitingCount gets current wait queue count for an account.
func (s *ConcurrencyService) GetAccountWaitingCount(ctx context.Context, accountID int64) (int, error) {
	if s.cache == nil {
		return 0, nil
	}
	return s.cache.GetAccountWaitingCount(ctx, accountID)
}

// CalculateMaxWait calculates the maximum wait queue size for a user
// maxWait = userConcurrency + defaultExtraWaitSlots
func CalculateMaxWait(userConcurrency int) int {
	if userConcurrency <= 0 {
		userConcurrency = 1
	}
	return userConcurrency + defaultExtraWaitSlots
}

// GetAccountsLoadBatch 批量获取账号负载信息。
func (s *ConcurrencyService) GetAccountsLoadBatch(ctx context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	return s.getAccountsLoadBatch(ctx, accounts, true)
}

// GetAccountsLoadBatchFresh 绕过极短 TTL 缓存，用于抢槽失败后的实时刷新兜底。
func (s *ConcurrencyService) GetAccountsLoadBatchFresh(ctx context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	return s.getAccountsLoadBatch(ctx, accounts, false)
}

func (s *ConcurrencyService) getAccountsLoadBatch(ctx context.Context, accounts []AccountWithConcurrency, allowCache bool) (map[int64]*AccountLoadInfo, error) {
	if len(accounts) == 0 {
		return map[int64]*AccountLoadInfo{}, nil
	}
	if s.cache == nil {
		return map[int64]*AccountLoadInfo{}, nil
	}

	ttl := time.Duration(s.accountLoadCacheTTL.Load())
	if !allowCache || ttl <= 0 {
		return s.fetchAccountsLoadBatch(ctx, accounts)
	}

	key := accountLoadBatchCacheKey(accounts)
	if cached, ok := s.getCachedAccountLoadBatch(key, time.Now()); ok {
		return cached, nil
	}

	value, err, _ := s.accountLoadGroup.Do(key, func() (any, error) {
		now := time.Now()
		if cached, ok := s.getCachedAccountLoadBatch(key, now); ok {
			return cached, nil
		}
		loadMap, fetchErr := s.fetchAccountsLoadBatch(ctx, accounts)
		if fetchErr != nil {
			return nil, fetchErr
		}
		cached := cloneAccountLoadMap(loadMap)
		s.storeCachedAccountLoadBatch(key, cached, now.Add(ttl))
		return cached, nil
	})
	if err != nil {
		return nil, err
	}
	loadMap, _ := value.(map[int64]*AccountLoadInfo)
	if loadMap == nil {
		return map[int64]*AccountLoadInfo{}, nil
	}
	return loadMap, nil
}

func (s *ConcurrencyService) fetchAccountsLoadBatch(ctx context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	if s.cache == nil {
		return map[int64]*AccountLoadInfo{}, nil
	}
	baseCtx := context.Background()
	if ctx != nil {
		baseCtx = context.WithoutCancel(ctx)
	}
	redisCtx, cancel := context.WithTimeout(baseCtx, accountLoadBatchFetchTimeout)
	defer cancel()
	return s.cache.GetAccountsLoadBatch(redisCtx, accounts)
}

func (s *ConcurrencyService) getCachedAccountLoadBatch(key string, now time.Time) (map[int64]*AccountLoadInfo, bool) {
	s.accountLoadCacheMu.RLock()
	cached, ok := s.accountLoadCache[key]
	s.accountLoadCacheMu.RUnlock()
	if !ok {
		return nil, false
	}
	if !now.Before(cached.expiresAt) {
		s.accountLoadCacheMu.Lock()
		if current, exists := s.accountLoadCache[key]; exists && !now.Before(current.expiresAt) {
			delete(s.accountLoadCache, key)
		}
		s.accountLoadCacheMu.Unlock()
		return nil, false
	}
	return cached.loadMap, true
}

func (s *ConcurrencyService) storeCachedAccountLoadBatch(key string, loadMap map[int64]*AccountLoadInfo, expiresAt time.Time) {
	s.accountLoadCacheMu.Lock()
	if s.accountLoadCache == nil {
		s.accountLoadCache = make(map[string]cachedAccountLoadBatch)
	}
	if len(s.accountLoadCache) >= maxAccountLoadBatchCacheEntries {
		now := time.Now()
		for cacheKey, cached := range s.accountLoadCache {
			if !now.Before(cached.expiresAt) {
				delete(s.accountLoadCache, cacheKey)
			}
		}
		for len(s.accountLoadCache) >= maxAccountLoadBatchCacheEntries {
			for cacheKey := range s.accountLoadCache {
				delete(s.accountLoadCache, cacheKey)
				break
			}
		}
	}
	s.accountLoadCache[key] = cachedAccountLoadBatch{
		loadMap:   loadMap,
		expiresAt: expiresAt,
	}
	s.accountLoadCacheMu.Unlock()
}

func accountLoadBatchCacheKey(accounts []AccountWithConcurrency) string {
	hash := sha256.New()
	var buf [16]byte
	for _, account := range accounts {
		binary.LittleEndian.PutUint64(buf[:8], uint64(account.ID))
		binary.LittleEndian.PutUint64(buf[8:], uint64(int64(account.MaxConcurrency)))
		_, _ = hash.Write(buf[:])
	}
	sum := hash.Sum(nil)
	return strconv.Itoa(len(accounts)) + ":" + hex.EncodeToString(sum)
}

func cloneAccountLoadMap(loadMap map[int64]*AccountLoadInfo) map[int64]*AccountLoadInfo {
	if len(loadMap) == 0 {
		return map[int64]*AccountLoadInfo{}
	}
	clone := make(map[int64]*AccountLoadInfo, len(loadMap))
	for accountID, loadInfo := range loadMap {
		if loadInfo == nil {
			clone[accountID] = nil
			continue
		}
		copied := *loadInfo
		clone[accountID] = &copied
	}
	return clone
}

// GetUsersLoadBatch returns load info for multiple users.
func (s *ConcurrencyService) GetUsersLoadBatch(ctx context.Context, users []UserWithConcurrency) (map[int64]*UserLoadInfo, error) {
	if s.cache == nil {
		return map[int64]*UserLoadInfo{}, nil
	}
	return s.cache.GetUsersLoadBatch(ctx, users)
}

// CleanupExpiredAccountSlots removes expired slots for one account (background task).
func (s *ConcurrencyService) CleanupExpiredAccountSlots(ctx context.Context, accountID int64) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.CleanupExpiredAccountSlots(ctx, accountID)
}

// StartSlotCleanupWorker starts a background cleanup worker for expired account slots.
func (s *ConcurrencyService) StartSlotCleanupWorker(_ AccountRepository, interval time.Duration) {
	if s == nil || s.cache == nil || interval <= 0 {
		return
	}

	runCleanup := func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := s.cache.CleanupExpiredAccountSlotKeys(cleanupCtx)
		cancel()
		if err != nil {
			logger.LegacyPrintf("service.concurrency", "Warning: cleanup expired account slots failed: %v", err)
			return
		}
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		runCleanup()
		for range ticker.C {
			runCleanup()
		}
	}()
}

// GetAccountConcurrencyBatch gets current concurrency counts for multiple accounts.
// Uses a detached context with timeout to prevent HTTP request cancellation from
// causing the entire batch to fail (which would show all concurrency as 0).
func (s *ConcurrencyService) GetAccountConcurrencyBatch(ctx context.Context, accountIDs []int64) (map[int64]int, error) {
	if len(accountIDs) == 0 {
		return map[int64]int{}, nil
	}
	if s.cache == nil {
		result := make(map[int64]int, len(accountIDs))
		for _, accountID := range accountIDs {
			result[accountID] = 0
		}
		return result, nil
	}

	// Use a detached context so that a cancelled HTTP request doesn't cause
	// the Redis pipeline to fail and return all-zero concurrency counts.
	redisCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	return s.cache.GetAccountConcurrencyBatch(redisCtx, accountIDs)
}
