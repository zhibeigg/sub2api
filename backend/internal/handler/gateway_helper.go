package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync"
	"time"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// claudeCodeValidator is a singleton validator for Claude Code client detection
var claudeCodeValidator = service.NewClaudeCodeValidator()

// SetClaudeCodeClientContext 检查请求是否来自 Claude Code 客户端，并设置到 context 中
// 返回更新后的 context
func SetClaudeCodeClientContext(c *gin.Context, body []byte, parsedReq *service.ParsedRequest) {
	if c == nil || c.Request == nil {
		return
	}
	ua := c.GetHeader("User-Agent")
	// Fast path：非 Claude CLI UA 直接判定 false，避免热路径二次 JSON 反序列化。
	if !claudeCodeValidator.ValidateUserAgent(ua) {
		ctx := service.SetClaudeCodeClient(c.Request.Context(), false)
		c.Request = c.Request.WithContext(ctx)
		return
	}

	isClaudeCode := false
	if !strings.Contains(c.Request.URL.Path, "messages") {
		// 与 Validate 行为一致：非 messages 路径 UA 命中即可视为 Claude Code 客户端。
		isClaudeCode = true
	} else {
		// 仅在确认为 Claude CLI 且 messages 路径时再做 body 解析。
		bodyMap := claudeCodeBodyMapFromParsedRequest(parsedReq)
		if bodyMap == nil && len(body) > 0 {
			_ = json.Unmarshal(body, &bodyMap)
		}
		isClaudeCode = claudeCodeValidator.Validate(c.Request, bodyMap)
	}

	// 更新 request context
	ctx := service.SetClaudeCodeClient(c.Request.Context(), isClaudeCode)

	// 仅在确认为 Claude Code 客户端时提取版本号写入 context
	if isClaudeCode {
		if version := claudeCodeValidator.ExtractVersion(ua); version != "" {
			ctx = service.SetClaudeCodeVersion(ctx, version)
		}
	}

	c.Request = c.Request.WithContext(ctx)
}

func claudeCodeBodyMapFromParsedRequest(parsedReq *service.ParsedRequest) map[string]any {
	if parsedReq == nil {
		return nil
	}
	bodyMap := map[string]any{
		"model": parsedReq.Model,
	}
	if parsedReq.HasSystem {
		if system, ok := parsedReq.SystemValue(); ok {
			bodyMap["system"] = system
		} else {
			bodyMap["system"] = nil
		}
	}
	if parsedReq.MetadataUserID != "" {
		bodyMap["metadata"] = map[string]any{"user_id": parsedReq.MetadataUserID}
	}
	return bodyMap
}

// 并发槽位等待相关常量
//
// 性能优化说明：
// 原实现使用固定间隔（100ms）轮询并发槽位，存在以下问题：
// 1. 高并发时频繁轮询增加 Redis 压力
// 2. 固定间隔可能导致多个请求同时重试（惊群效应）
//
// 新实现使用指数退避 + 抖动算法：
// 1. 初始退避 100ms，每次乘以 1.5，最大 2s
// 2. 添加 ±20% 的随机抖动，分散重试时间点
// 3. 减少 Redis 压力，避免惊群效应
const (
	// maxConcurrencyWait 等待并发槽位的最大时间
	maxConcurrencyWait = 30 * time.Second
	// defaultPingInterval 流式响应等待时发送 ping 的默认间隔
	defaultPingInterval = 10 * time.Second
	// initialBackoff 初始退避时间
	initialBackoff = 100 * time.Millisecond
	// backoffMultiplier 退避时间乘数（指数退避）
	backoffMultiplier = 1.5
	// maxBackoff 最大退避时间
	maxBackoff = 2 * time.Second
)

// SSEPingFormat defines the format of SSE ping events for different platforms
type SSEPingFormat string

const (
	// SSEPingFormatClaude is the Claude/Anthropic SSE ping format
	SSEPingFormatClaude SSEPingFormat = "data: {\"type\": \"ping\"}\n\n"
	// SSEPingFormatNone indicates no ping should be sent (e.g., OpenAI has no ping spec)
	SSEPingFormatNone SSEPingFormat = ""
	// SSEPingFormatComment is an SSE comment ping for OpenAI/Codex CLI clients
	SSEPingFormatComment SSEPingFormat = ":\n\n"
)

// ConcurrencyError represents a concurrency limit error with context
type ConcurrencyError struct {
	SlotType  string
	IsTimeout bool
}

func (e *ConcurrencyError) Error() string {
	if e.IsTimeout {
		return fmt.Sprintf("timeout waiting for %s concurrency slot", e.SlotType)
	}
	return fmt.Sprintf("%s concurrency limit reached", e.SlotType)
}

type WaitQueueFullError struct {
	SlotType string
}

func (e *WaitQueueFullError) Error() string {
	return "Too many pending requests, please retry later"
}

const subscriptionConcurrencyErrorCode = "SUBSCRIPTION_CONCURRENCY_LIMIT_EXCEEDED"

func markConcurrencyResponseHeaders(c *gin.Context, err error) {
	if c == nil || err == nil {
		return
	}
	slotType := ""
	var concurrencyErr *ConcurrencyError
	if errors.As(err, &concurrencyErr) {
		slotType = concurrencyErr.SlotType
	}
	var waitQueueErr *WaitQueueFullError
	if errors.As(err, &waitQueueErr) && waitQueueErr.SlotType != "" {
		slotType = waitQueueErr.SlotType
	}
	if slotType != service.ConcurrencyScopeSubscription {
		return
	}
	c.Header("Retry-After", "1")
	c.Header("X-Sub2API-Error-Code", subscriptionConcurrencyErrorCode)
}

type requestSlotsWithLeaseLossAcquireFunc func(
	context.Context,
	int64,
	int,
	int64,
	int,
	func(error),
) (*service.AcquireResult, error)

// ConcurrencyHelper provides common concurrency slot management for gateway handlers
type ConcurrencyHelper struct {
	concurrencyService *service.ConcurrencyService
	pingFormat         SSEPingFormat
	pingInterval       time.Duration

	// Optional test seam; production falls back to ConcurrencyService directly.
	requestSlotsAcquire requestSlotsWithLeaseLossAcquireFunc
}

// NewConcurrencyHelper creates a new ConcurrencyHelper
func NewConcurrencyHelper(concurrencyService *service.ConcurrencyService, pingFormat SSEPingFormat, pingInterval time.Duration) *ConcurrencyHelper {
	if pingInterval <= 0 {
		pingInterval = defaultPingInterval
	}
	return &ConcurrencyHelper{
		concurrencyService: concurrencyService,
		pingFormat:         pingFormat,
		pingInterval:       pingInterval,
	}
}

// wrapReleaseOnDone ensures release runs at most once and still triggers on context cancellation.
// 用于避免客户端断开或上游超时导致的并发槽位泄漏。
// 优化：基于 context.AfterFunc 注册回调，避免每请求额外守护 goroutine。
func wrapReleaseOnDone(ctx context.Context, releaseFunc func()) func() {
	if releaseFunc == nil {
		return nil
	}
	var once sync.Once
	releaseOnce := func() {
		once.Do(releaseFunc)
	}
	stop := context.AfterFunc(ctx, releaseOnce)

	return func() {
		_ = stop()
		releaseOnce()
	}
}

// IncrementWaitCount increments the wait count for a user
func (h *ConcurrencyHelper) IncrementWaitCount(ctx context.Context, userID int64, maxWait int) (bool, error) {
	return h.concurrencyService.IncrementWaitCount(ctx, userID, maxWait)
}

// DecrementWaitCount decrements the wait count for a user
func (h *ConcurrencyHelper) DecrementWaitCount(ctx context.Context, userID int64) {
	h.concurrencyService.DecrementWaitCount(ctx, userID)
}

// IncrementAccountWaitCount increments the wait count for an account
func (h *ConcurrencyHelper) IncrementAccountWaitCount(ctx context.Context, accountID int64, maxWait int) (bool, error) {
	return h.concurrencyService.IncrementAccountWaitCount(ctx, accountID, maxWait)
}

// DecrementAccountWaitCount decrements the wait count for an account
func (h *ConcurrencyHelper) DecrementAccountWaitCount(ctx context.Context, accountID int64) {
	h.concurrencyService.DecrementAccountWaitCount(ctx, accountID)
}

// TryAcquireUserSlot 尝试立即获取用户并发槽位。
// 返回值: (releaseFunc, acquired, error)
func (h *ConcurrencyHelper) TryAcquireUserSlot(ctx context.Context, userID int64, maxConcurrency int) (func(), bool, error) {
	result, err := h.concurrencyService.AcquireUserSlot(ctx, userID, maxConcurrency)
	if err != nil {
		return nil, false, err
	}
	if !result.Acquired {
		return nil, false, nil
	}
	return result.ReleaseFunc, true, nil
}

func (h *ConcurrencyHelper) acquireRequestSlotsWithLeaseLoss(
	ctx context.Context,
	userID int64,
	userMaxConcurrency int,
	subscriptionID int64,
	subscriptionMaxConcurrency int,
	onLeaseLoss func(error),
) (*service.AcquireResult, error) {
	if h == nil || h.concurrencyService == nil {
		return nil, fmt.Errorf("concurrency service is unavailable")
	}
	if h.requestSlotsAcquire != nil {
		return h.requestSlotsAcquire(
			ctx,
			userID,
			userMaxConcurrency,
			subscriptionID,
			subscriptionMaxConcurrency,
			onLeaseLoss,
		)
	}
	return h.concurrencyService.AcquireUserAndSubscriptionSlotsWithLeaseLoss(
		ctx,
		userID,
		userMaxConcurrency,
		subscriptionID,
		subscriptionMaxConcurrency,
		onLeaseLoss,
	)
}

func (h *ConcurrencyHelper) tryAcquireRequestSlotsWithLeaseLoss(
	ctx context.Context,
	userID int64,
	userMaxConcurrency int,
	subscriptionID int64,
	subscriptionMaxConcurrency int,
	onLeaseLoss func(error),
) (func(), bool, string, error) {
	result, err := h.acquireRequestSlotsWithLeaseLoss(
		ctx,
		userID,
		userMaxConcurrency,
		subscriptionID,
		subscriptionMaxConcurrency,
		onLeaseLoss,
	)
	if err != nil {
		return nil, false, "", err
	}
	if !result.Acquired {
		return nil, false, result.BlockedScope, nil
	}
	return result.ReleaseFunc, true, "", nil
}

func (h *ConcurrencyHelper) TryAcquireUserSlotForAPIKey(ctx context.Context, userID int64, maxConcurrency int, apiKeyID int64) (func(), bool, error) {
	return h.TryAcquireRequestSlotsForAPIKey(ctx, userID, maxConcurrency, apiKeyID, 0, 0)
}

func (h *ConcurrencyHelper) TryAcquireRequestSlotsForAPIKey(
	ctx context.Context,
	userID int64,
	maxConcurrency int,
	apiKeyID int64,
	subscriptionID int64,
	subscriptionMaxConcurrency int,
) (func(), bool, error) {
	return h.TryAcquireRequestSlotsForAPIKeyWithLeaseLoss(
		ctx,
		userID,
		maxConcurrency,
		apiKeyID,
		subscriptionID,
		subscriptionMaxConcurrency,
		nil,
	)
}

// TryAcquireRequestSlotsForAPIKeyWithLeaseLoss is the callback-aware immediate
// acquisition entry used by WebSocket request turns. The callback is invoked at
// most once if an active subscription lease is confirmed lost and cannot be restored.
func (h *ConcurrencyHelper) TryAcquireRequestSlotsForAPIKeyWithLeaseLoss(
	ctx context.Context,
	userID int64,
	maxConcurrency int,
	apiKeyID int64,
	subscriptionID int64,
	subscriptionMaxConcurrency int,
	onLeaseLoss func(error),
) (func(), bool, error) {
	releaseFunc, acquired, _, err := h.tryAcquireRequestSlotsWithLeaseLoss(
		ctx,
		userID,
		maxConcurrency,
		subscriptionID,
		subscriptionMaxConcurrency,
		onLeaseLoss,
	)
	if err != nil || !acquired {
		return releaseFunc, acquired, err
	}
	return h.withAPIKeySlot(ctx, apiKeyID, releaseFunc), true, nil
}

// AcquireOpenAIWSIngressLease bounds the whole client WebSocket lifecycle,
// independently from per-turn user and account slots.
func (h *ConcurrencyHelper) AcquireOpenAIWSIngressLease(ctx context.Context, apiKeyID int64, maxConnections int) (*service.OpenAIWSIngressLease, bool, error) {
	if h == nil || h.concurrencyService == nil {
		return nil, false, fmt.Errorf("concurrency service is unavailable")
	}
	return h.concurrencyService.AcquireOpenAIWSIngressLease(ctx, apiKeyID, maxConnections)
}

// TryAcquireAccountSlot 尝试立即获取账号并发槽位。
// 返回值: (releaseFunc, acquired, error)
func (h *ConcurrencyHelper) TryAcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int) (func(), bool, error) {
	result, err := h.concurrencyService.AcquireAccountSlot(ctx, accountID, maxConcurrency)
	if err != nil {
		return nil, false, err
	}
	if !result.Acquired {
		return nil, false, nil
	}
	return result.ReleaseFunc, true, nil
}

func subscriptionConcurrencyScopeFromGin(c *gin.Context) (int64, int) {
	if c == nil {
		return 0, 0
	}
	subscription, ok := middleware2.GetSubscriptionFromContext(c)

	if !ok || subscription == nil || subscription.ID <= 0 || subscription.ConcurrencyLimit == nil || *subscription.ConcurrencyLimit <= 0 {
		return 0, 0
	}
	return subscription.ID, *subscription.ConcurrencyLimit
}

func attachLeaseLossCancellation(c *gin.Context) (func(error), func()) {
	if c == nil || c.Request == nil {
		return func(error) {}, func() {}
	}
	requestCtx, cancel := context.WithCancelCause(c.Request.Context())
	c.Request = c.Request.WithContext(requestCtx)
	return func(err error) {
			cancel(err)
		}, func() {
			cancel(nil)
		}
}

func releaseWithContextCleanup(releaseFunc func(), cleanup func()) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			if releaseFunc != nil {
				releaseFunc()
			}
			if cleanup != nil {
				cleanup()
			}
		})
	}
}

// AcquireUserSlotWithWait acquires the user-global slot and, when the resolved
// request is backed by a limited standard-quota subscription, its instance slot.
// For streaming requests, it sends ping events during the shared wait deadline.
func (h *ConcurrencyHelper) AcquireUserSlotWithWait(c *gin.Context, userID int64, maxConcurrency int, isStream bool, streamStarted *bool) (func(), error) {
	return h.acquireUserSlotWithWaitTimeout(c, userID, maxConcurrency, maxConcurrencyWait, isStream, streamStarted)
}

func (h *ConcurrencyHelper) acquireUserSlotWithWaitTimeout(c *gin.Context, userID int64, maxConcurrency int, timeout time.Duration, isStream bool, streamStarted *bool) (func(), error) {
	onLeaseLoss, cleanupRequestContext := attachLeaseLossCancellation(c)
	keepRequestContext := false
	defer func() {
		if !keepRequestContext {
			cleanupRequestContext()
		}
	}()

	ctx := c.Request.Context()
	subscriptionID, subscriptionMaxConcurrency := subscriptionConcurrencyScopeFromGin(c)
	acquireSlots := func(acquireCtx context.Context) (*service.AcquireResult, error) {
		return h.acquireRequestSlotsWithLeaseLoss(
			acquireCtx,
			userID,
			maxConcurrency,
			subscriptionID,
			subscriptionMaxConcurrency,
			onLeaseLoss,
		)
	}

	result, err := acquireSlots(ctx)
	if err != nil {
		return nil, err
	}
	if result.Acquired {
		keepRequestContext = true
		return releaseWithContextCleanup(
			h.withAPIKeySlotFromGin(c, result.ReleaseFunc),
			cleanupRequestContext,
		), nil
	}

	blockedScope := result.BlockedScope
	if blockedScope == "" {
		blockedScope = service.ConcurrencyScopeUser
	}
	queueConcurrency := maxConcurrency
	canWait := false
	if blockedScope == service.ConcurrencyScopeSubscription {
		queueConcurrency = subscriptionMaxConcurrency
		queueLimit := service.CalculateMaxWait(queueConcurrency) - queueConcurrency
		if queueLimit < 1 {
			queueLimit = 1
		}
		canWait, err = h.concurrencyService.IncrementSubscriptionWaitCount(ctx, subscriptionID, queueLimit)
		if err == nil && canWait {
			defer h.concurrencyService.DecrementSubscriptionWaitCount(ctx, subscriptionID)
		}
	} else {
		queueLimit := service.CalculateMaxWait(queueConcurrency) - queueConcurrency
		if queueLimit < 1 {
			queueLimit = 1
		}
		canWait, err = h.IncrementWaitCount(ctx, userID, queueLimit)
		if err == nil && canWait {
			defer h.DecrementWaitCount(ctx, userID)
		}
	}
	if err != nil {
		markConcurrencyResponseHeaders(c, &ConcurrencyError{SlotType: blockedScope})
		return nil, err
	}
	if !canWait {
		err = &WaitQueueFullError{SlotType: blockedScope}
		markConcurrencyResponseHeaders(c, err)
		return nil, err
	}

	resultRelease, err := h.waitForAcquireWithPingTimeout(c, blockedScope, timeout, isStream, streamStarted, false, acquireSlots)
	if err != nil {
		markConcurrencyResponseHeaders(c, err)
		return nil, err
	}
	keepRequestContext = true
	return releaseWithContextCleanup(
		h.withAPIKeySlotFromGin(c, resultRelease),
		cleanupRequestContext,
	), nil
}

func (h *ConcurrencyHelper) withAPIKeySlotFromGin(c *gin.Context, releaseFunc func()) func() {
	if c == nil {
		return releaseFunc
	}
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		return releaseFunc
	}
	return h.withAPIKeySlot(c.Request.Context(), apiKey.ID, releaseFunc)
}

func (h *ConcurrencyHelper) withAPIKeySlot(ctx context.Context, apiKeyID int64, releaseFunc func()) func() {
	if h == nil || h.concurrencyService == nil || apiKeyID <= 0 {
		return releaseFunc
	}
	apiKeyReleaseFunc := h.concurrencyService.TrackAPIKeySlot(ctx, apiKeyID)
	return func() {
		if releaseFunc != nil {
			releaseFunc()
		}
		if apiKeyReleaseFunc != nil {
			apiKeyReleaseFunc()
		}
	}
}

// AcquireAccountSlotWithWait acquires an account concurrency slot, waiting if necessary.
// For streaming requests, sends ping events during the wait.
// streamStarted is updated if streaming response has begun.
func (h *ConcurrencyHelper) AcquireAccountSlotWithWait(c *gin.Context, accountID int64, maxConcurrency int, isStream bool, streamStarted *bool) (func(), error) {
	ctx := c.Request.Context()

	// Try to acquire immediately
	releaseFunc, acquired, err := h.TryAcquireAccountSlot(ctx, accountID, maxConcurrency)
	if err != nil {
		return nil, err
	}

	if acquired {
		return releaseFunc, nil
	}

	// Need to wait - handle streaming ping if needed
	return h.waitForSlotWithPing(c, "account", accountID, maxConcurrency, isStream, streamStarted)
}

// waitForSlotWithPing waits for a concurrency slot, sending ping events for streaming requests.
// streamStarted pointer is updated when streaming begins (for proper error handling by caller).
func (h *ConcurrencyHelper) waitForSlotWithPing(c *gin.Context, slotType string, id int64, maxConcurrency int, isStream bool, streamStarted *bool) (func(), error) {
	return h.waitForSlotWithPingTimeout(c, slotType, id, maxConcurrency, maxConcurrencyWait, isStream, streamStarted, false)
}

// waitForSlotWithPingTimeout waits for one legacy user/account scope with a custom timeout.
func (h *ConcurrencyHelper) waitForSlotWithPingTimeout(c *gin.Context, slotType string, id int64, maxConcurrency int, timeout time.Duration, isStream bool, streamStarted *bool, tryImmediate bool) (func(), error) {
	acquireSlot := func(ctx context.Context) (*service.AcquireResult, error) {
		if slotType == service.ConcurrencyScopeUser {
			return h.concurrencyService.AcquireUserSlot(ctx, id, maxConcurrency)
		}
		return h.concurrencyService.AcquireAccountSlot(ctx, id, maxConcurrency)
	}
	return h.waitForAcquireWithPingTimeout(c, slotType, timeout, isStream, streamStarted, tryImmediate, acquireSlot)
}

type concurrencyAcquireFunc func(context.Context) (*service.AcquireResult, error)

func (h *ConcurrencyHelper) waitForAcquireWithPingTimeout(
	c *gin.Context,
	slotType string,
	timeout time.Duration,
	isStream bool,
	streamStarted *bool,
	tryImmediate bool,
	acquireSlot concurrencyAcquireFunc,
) (func(), error) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()

	if tryImmediate {
		result, err := acquireSlot(ctx)
		if err != nil {
			return nil, err
		}
		if result.Acquired {
			return result.ReleaseFunc, nil
		}
		if result.BlockedScope != "" {
			slotType = result.BlockedScope
		}
	}

	needPing := isStream && h.pingFormat != ""
	var flusher http.Flusher
	if needPing {
		var ok bool
		flusher, ok = c.Writer.(http.Flusher)
		if !ok {
			return nil, fmt.Errorf("streaming not supported")
		}
	}

	var pingCh <-chan time.Time
	if needPing {
		pingTicker := time.NewTicker(h.pingInterval)
		defer pingTicker.Stop()
		pingCh = pingTicker.C
	}

	backoff := initialBackoff
	timer := time.NewTimer(backoff)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			if parentErr := c.Request.Context().Err(); parentErr != nil {
				return nil, parentErr
			}
			return nil, &ConcurrencyError{SlotType: slotType, IsTimeout: true}

		case <-pingCh:
			if !*streamStarted {
				c.Header("Content-Type", "text/event-stream")
				c.Header("Cache-Control", "no-cache")
				c.Header("Connection", "keep-alive")
				c.Header("X-Accel-Buffering", "no")
				*streamStarted = true
			}
			if _, err := fmt.Fprint(c.Writer, string(h.pingFormat)); err != nil {
				return nil, err
			}
			flusher.Flush()

		case <-timer.C:
			result, err := acquireSlot(ctx)
			if err != nil {
				return nil, err
			}
			if result.Acquired {
				return result.ReleaseFunc, nil
			}
			if result.BlockedScope != "" {
				slotType = result.BlockedScope
			}
			backoff = nextBackoff(backoff)
			timer.Reset(backoff)
		}
	}
}

// AcquireAccountSlotWithWaitTimeout acquires an account slot with a custom timeout (keeps SSE ping).
func (h *ConcurrencyHelper) AcquireAccountSlotWithWaitTimeout(c *gin.Context, accountID int64, maxConcurrency int, timeout time.Duration, isStream bool, streamStarted *bool) (func(), error) {
	return h.waitForSlotWithPingTimeout(c, "account", accountID, maxConcurrency, timeout, isStream, streamStarted, true)
}

// nextBackoff 计算下一次退避时间
// 性能优化：使用指数退避 + 随机抖动，避免惊群效应
// current: 当前退避时间
// 返回值：下一次退避时间（100ms ~ 2s 之间）
func nextBackoff(current time.Duration) time.Duration {
	// 指数退避：当前时间 * 1.5
	next := time.Duration(float64(current) * backoffMultiplier)
	if next > maxBackoff {
		next = maxBackoff
	}
	// 添加 ±20% 的随机抖动（jitter 范围 0.8 ~ 1.2）
	// 抖动可以分散多个请求的重试时间点，避免同时冲击 Redis
	jitter := 0.8 + rand.Float64()*0.4
	jittered := time.Duration(float64(next) * jitter)
	if jittered < initialBackoff {
		return initialBackoff
	}
	if jittered > maxBackoff {
		return maxBackoff
	}
	return jittered
}
