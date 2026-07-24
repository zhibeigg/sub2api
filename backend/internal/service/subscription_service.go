package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionplangroup"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/dgraph-io/ristretto"
	"golang.org/x/sync/singleflight"
)

// MaxExpiresAt is the maximum allowed expiration date (year 2099)
// This prevents time.Time JSON serialization errors (RFC 3339 requires year <= 9999)
var MaxExpiresAt = time.Date(2099, 12, 31, 23, 59, 59, 0, time.UTC)

// noActiveSubscriptionCacheEntry 是标准分组余额回退路径使用的 L1 负缓存哨兵。
// 套餐分配/续期会按所有授权 group_ids 主动失效该 key，因此不会延迟已购买套餐生效。
var noActiveSubscriptionCacheEntry = &UserSubscription{}

// MaxValidityDays is the maximum allowed validity days for subscriptions (100 years)
const MaxValidityDays = 36500

var (
	ErrSubscriptionNotFound        = infraerrors.NotFound("SUBSCRIPTION_NOT_FOUND", "subscription not found")
	ErrSubscriptionExpired         = infraerrors.Forbidden("SUBSCRIPTION_EXPIRED", "subscription has expired")
	ErrSubscriptionSuspended       = infraerrors.Forbidden("SUBSCRIPTION_SUSPENDED", "subscription is suspended")
	ErrSubscriptionAlreadyExists   = infraerrors.Conflict("SUBSCRIPTION_ALREADY_EXISTS", "subscription already exists for this user and group")
	ErrSubscriptionAssignConflict  = infraerrors.Conflict("SUBSCRIPTION_ASSIGN_CONFLICT", "subscription exists but request conflicts with existing assignment semantics")
	ErrSubscriptionNotRevoked      = infraerrors.Conflict("SUBSCRIPTION_NOT_REVOKED", "subscription is not revoked")
	ErrSubscriptionRestoreConflict = infraerrors.Conflict("SUBSCRIPTION_RESTORE_CONFLICT", "subscription already exists for this user and group")
	ErrGroupNotSubscriptionType    = infraerrors.BadRequest("GROUP_NOT_SUBSCRIPTION_TYPE", "group is not a subscription type")
	ErrInvalidInput                = infraerrors.BadRequest("INVALID_INPUT", "at least one of resetDaily, resetWeekly, or resetMonthly must be true")
	ErrDailyLimitExceeded          = infraerrors.TooManyRequests("DAILY_LIMIT_EXCEEDED", "daily usage limit exceeded")
	ErrWeeklyLimitExceeded         = infraerrors.TooManyRequests("WEEKLY_LIMIT_EXCEEDED", "weekly usage limit exceeded")
	ErrMonthlyLimitExceeded        = infraerrors.TooManyRequests("MONTHLY_LIMIT_EXCEEDED", "monthly usage limit exceeded")
	ErrSubscriptionNilInput        = infraerrors.BadRequest("SUBSCRIPTION_NIL_INPUT", "subscription input cannot be nil")
	ErrAdjustWouldExpire           = infraerrors.BadRequest("ADJUST_WOULD_EXPIRE", "adjustment would result in expired subscription (remaining days must be > 0)")
)

// IsSubscriptionUsageLimitExceeded reports whether err is a daily, weekly, or
// monthly subscription usage-limit rejection. Callers use it to distinguish a
// standard-group balance fallback from invalid subscription states.
func IsSubscriptionUsageLimitExceeded(err error) bool {
	return errors.Is(err, ErrDailyLimitExceeded) ||
		errors.Is(err, ErrWeeklyLimitExceeded) ||
		errors.Is(err, ErrMonthlyLimitExceeded)
}

// SubscriptionService 订阅服务
type SubscriptionService struct {
	groupRepo           GroupRepository
	userSubRepo         UserSubscriptionRepository
	billingCacheService *BillingCacheService
	entClient           *dbent.Client

	// L1 缓存：加速中间件热路径的订阅查询
	subCacheL1     *ristretto.Cache
	subCacheGroup  singleflight.Group
	subCacheTTL    time.Duration
	subCacheJitter int // 抖动百分比

	maintenanceQueue *SubscriptionMaintenanceQueue
}

// NewSubscriptionService 创建订阅服务
func NewSubscriptionService(groupRepo GroupRepository, userSubRepo UserSubscriptionRepository, billingCacheService *BillingCacheService, entClient *dbent.Client, cfg *config.Config) *SubscriptionService {
	svc := &SubscriptionService{
		groupRepo:           groupRepo,
		userSubRepo:         userSubRepo,
		billingCacheService: billingCacheService,
		entClient:           entClient,
	}
	svc.initSubCache(cfg)
	svc.initMaintenanceQueue(cfg)
	svc.StartSubCacheInvalidationSubscriber(context.Background())
	return svc
}

func (s *SubscriptionService) initMaintenanceQueue(cfg *config.Config) {
	if cfg == nil {
		return
	}
	mc := cfg.SubscriptionMaintenance
	if mc.WorkerCount <= 0 || mc.QueueSize <= 0 {
		return
	}
	s.maintenanceQueue = NewSubscriptionMaintenanceQueue(mc.WorkerCount, mc.QueueSize)
}

// Stop stops the maintenance worker pool.
func (s *SubscriptionService) Stop() {
	if s == nil {
		return
	}
	if s.maintenanceQueue != nil {
		s.maintenanceQueue.Stop()
	}
}

// initSubCache 初始化订阅 L1 缓存
func (s *SubscriptionService) initSubCache(cfg *config.Config) {
	if cfg == nil {
		return
	}
	sc := cfg.SubscriptionCache
	if sc.L1Size <= 0 || sc.L1TTLSeconds <= 0 {
		return
	}
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: int64(sc.L1Size) * 10,
		MaxCost:     int64(sc.L1Size),
		BufferItems: 64,
	})
	if err != nil {
		log.Printf("Warning: failed to init subscription L1 cache: %v", err)
		return
	}
	s.subCacheL1 = cache
	s.subCacheTTL = time.Duration(sc.L1TTLSeconds) * time.Second
	s.subCacheJitter = sc.JitterPercent
}

// subCacheKey 生成订阅缓存 key（热路径，避免 fmt.Sprintf 开销）
func subCacheKey(userID, groupID int64) string {
	return "sub:" + strconv.FormatInt(userID, 10) + ":" + strconv.FormatInt(groupID, 10)
}

// jitteredTTL 为 TTL 添加抖动，避免集中过期
func (s *SubscriptionService) jitteredTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 || s.subCacheJitter <= 0 {
		return ttl
	}
	pct := s.subCacheJitter
	if pct > 100 {
		pct = 100
	}
	delta := float64(pct) / 100
	factor := 1 - delta + rand.Float64()*(2*delta)
	if factor <= 0 {
		return ttl
	}
	return time.Duration(float64(ttl) * factor)
}

func (s *SubscriptionService) negativeSubCacheTTL() time.Duration {
	ttl := s.subCacheTTL / 10
	if ttl <= 0 || ttl > 15*time.Second {
		ttl = 15 * time.Second
	}
	if ttl < time.Second {
		ttl = time.Second
	}
	return s.jitteredTTL(ttl)
}

// InvalidateSubCache 失效指定用户+分组的订阅 L1 缓存
func (s *SubscriptionService) InvalidateSubCache(userID, groupID int64) {
	if s.subCacheL1 == nil {
		return
	}
	s.subCacheL1.Del(subCacheKey(userID, groupID))
}

// InvalidateSubCacheSync 失效订阅 L1 缓存并等待 Ristretto 删除操作生效。
func (s *SubscriptionService) InvalidateSubCacheSync(userID, groupID int64) {
	s.invalidateSubCacheKeySync(subCacheKey(userID, groupID))
}

func (s *SubscriptionService) invalidateSubCacheKeySync(key string) {
	if s.subCacheL1 == nil {
		return
	}
	s.subCacheL1.Del(key)
	s.subCacheL1.Wait()
}

// StartSubCacheInvalidationSubscriber 启动跨实例订阅 L1 缓存失效订阅。
func (s *SubscriptionService) StartSubCacheInvalidationSubscriber(ctx context.Context) {
	if s.billingCacheService == nil || s.subCacheL1 == nil {
		return
	}
	if err := s.billingCacheService.SubscribeSubscriptionCacheInvalidation(ctx, func(cacheKey string) {
		s.invalidateSubCacheKeySync(cacheKey)
	}); err != nil {
		log.Printf("Warning: failed to start subscription cache invalidation subscriber: %v", err)
	}
}

func (s *SubscriptionService) invalidateSubscriptionGroupCaches(userID int64, groupIDs []int64) error {
	groupIDs = uniqueSubscriptionGroupIDs(groupIDs)
	for _, groupID := range groupIDs {
		s.InvalidateSubCacheSync(userID, groupID)
	}
	if s.billingCacheService == nil {
		return nil
	}
	cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, groupID := range groupIDs {
		if err := s.billingCacheService.InvalidateSubscription(cacheCtx, userID, groupID); err != nil {
			return fmt.Errorf("invalidate billing subscription cache: %w", err)
		}
		if err := s.billingCacheService.PublishSubscriptionCacheInvalidation(cacheCtx, subCacheKey(userID, groupID)); err != nil {
			return fmt.Errorf("publish subscription cache invalidation: %w", err)
		}
	}
	return nil
}

func uniqueSubscriptionGroupIDs(groupIDs []int64) []int64 {
	seen := make(map[int64]struct{}, len(groupIDs))
	out := make([]int64, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		if groupID <= 0 {
			continue
		}
		if _, ok := seen[groupID]; ok {
			continue
		}
		seen[groupID] = struct{}{}
		out = append(out, groupID)
	}
	return out
}

// AssignSubscriptionInput 分配订阅输入
type AssignSubscriptionInput struct {
	UserID           int64
	GroupID          int64
	GroupIDs         []int64
	SourcePlanID     *int64
	QuotaSnapshotted bool
	DailyLimitUSD    *float64
	WeeklyLimitUSD   *float64
	MonthlyLimitUSD  *float64
	ConcurrencyLimit *int
	ValidityDays     int
	AssignedBy       int64
	Notes            string
}

func (s *SubscriptionService) BuildPlanAssignmentInput(ctx context.Context, userID, planID int64, validityDays int, assignedBy int64, notes string) (*AssignSubscriptionInput, error) {
	if s.entClient == nil || planID <= 0 {
		return nil, infraerrors.BadRequest("PLAN_REQUIRED", "subscription plan is required")
	}
	plan, err := s.entClient.SubscriptionPlan.Get(ctx, planID)
	if err != nil {
		return nil, infraerrors.NotFound("PLAN_NOT_FOUND", "subscription plan not found")
	}
	bindings, err := s.entClient.SubscriptionPlanGroup.Query().
		Where(subscriptionplangroup.PlanIDEQ(planID)).
		Order(subscriptionplangroup.ByPriority(), subscriptionplangroup.ByGroupID()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("load subscription plan groups: %w", err)
	}
	groupIDs := make([]int64, 0, len(bindings))
	for _, binding := range bindings {
		groupIDs = append(groupIDs, binding.GroupID)
	}
	if len(groupIDs) == 0 {
		groupIDs = []int64{plan.GroupID}
	}
	planType := normalizeSubscriptionPlanType(plan.PlanType)
	if planType == domain.SubscriptionPlanTypeLegacySharedSubscription {
		return nil, infraerrors.Conflict("PLAN_LEGACY_READ_ONLY", "legacy shared subscription plans must be converted before assignment")
	}
	dailyLimit := plan.DailyLimitUsd
	weeklyLimit := plan.WeeklyLimitUsd
	monthlyLimit := plan.MonthlyLimitUsd
	concurrencyLimit := plan.ConcurrencyLimit
	quotaSnapshotted := planType == domain.SubscriptionPlanTypeStandardQuota
	if !quotaSnapshotted {
		dailyLimit = nil
		weeklyLimit = nil
		monthlyLimit = nil
		concurrencyLimit = nil
	}
	if err := validatePlanSemantics(planType, groupIDs, dailyLimit, weeklyLimit, monthlyLimit); err != nil {
		return nil, err
	}
	if validityDays <= 0 {
		validityDays = psComputeValidityDays(plan.ValidityDays, plan.ValidityUnit)
	}
	return &AssignSubscriptionInput{
		UserID: userID, GroupID: groupIDs[0], GroupIDs: groupIDs, SourcePlanID: &plan.ID,
		QuotaSnapshotted: quotaSnapshotted, DailyLimitUSD: dailyLimit,
		WeeklyLimitUSD: weeklyLimit, MonthlyLimitUSD: monthlyLimit,
		ConcurrencyLimit: concurrencyLimit,
		ValidityDays:     validityDays, AssignedBy: assignedBy, Notes: notes,
	}, nil
}

func (s *SubscriptionService) AssignPlanSubscription(ctx context.Context, userID, planID int64, validityDays int, assignedBy int64, notes string) (*UserSubscription, error) {
	input, err := s.BuildPlanAssignmentInput(ctx, userID, planID, validityDays, assignedBy, notes)
	if err != nil {
		return nil, err
	}
	return s.AssignSubscription(ctx, input)
}

func (s *SubscriptionService) prepareAssignmentInput(ctx context.Context, input *AssignSubscriptionInput) (*AssignSubscriptionInput, error) {
	if input == nil || input.UserID <= 0 {
		return nil, ErrSubscriptionNilInput
	}
	prepared := *input
	prepared.GroupIDs = uniqueSubscriptionGroupIDs(input.GroupIDs)
	if len(prepared.GroupIDs) == 0 && input.GroupID > 0 {
		prepared.GroupIDs = []int64{input.GroupID}
	}
	if len(prepared.GroupIDs) == 0 {
		return nil, infraerrors.BadRequest("SUBSCRIPTION_GROUP_REQUIRED", "at least one subscription group is required")
	}
	prepared.GroupID = prepared.GroupIDs[0]
	hasStandardGroup := false
	hasSubscriptionGroup := false
	for _, groupID := range prepared.GroupIDs {
		item, err := s.groupRepo.GetByID(ctx, groupID)
		if err != nil {
			return nil, fmt.Errorf("group %d not found: %w", groupID, err)
		}
		switch item.SubscriptionType {
		case domain.SubscriptionTypeSubscription:
			hasSubscriptionGroup = true
		case domain.SubscriptionTypeStandard:
			hasStandardGroup = true
		default:
			return nil, ErrGroupNotSubscriptionType
		}
	}
	if hasStandardGroup && hasSubscriptionGroup {
		return nil, infraerrors.BadRequest("SUBSCRIPTION_GROUP_TYPE_MIXED", "subscription assignments cannot mix standard and subscription groups")
	}
	if hasStandardGroup {
		if !prepared.QuotaSnapshotted || prepared.SourcePlanID == nil {
			return nil, ErrGroupNotSubscriptionType
		}
		if prepared.DailyLimitUSD == nil && prepared.WeeklyLimitUSD == nil && prepared.MonthlyLimitUSD == nil {
			return nil, infraerrors.BadRequest("PLAN_QUOTA_REQUIRED", "standard group subscriptions require at least one quota limit")
		}
		if err := validatePlanConcurrencyLimit(prepared.ConcurrencyLimit); err != nil {
			return nil, err
		}
	} else {
		prepared.ConcurrencyLimit = nil
		if !prepared.QuotaSnapshotted && len(prepared.GroupIDs) != 1 {
			return nil, infraerrors.BadRequest("SUBSCRIPTION_GROUP_COUNT_INVALID", "native subscription assignments require exactly one group")
		}
	}
	return &prepared, nil
}

func (s *SubscriptionService) findExistingAssignment(ctx context.Context, userID int64, groupIDs []int64) (*UserSubscription, error) {
	var existing *UserSubscription
	for _, groupID := range groupIDs {
		sub, err := s.userSubRepo.GetByUserIDAndGroupID(ctx, userID, groupID)
		if err != nil {
			if errors.Is(err, ErrSubscriptionNotFound) {
				continue
			}
			return nil, err
		}
		if existing != nil && existing.ID != sub.ID {
			return nil, ErrSubscriptionAssignConflict.WithMetadata(map[string]string{"conflict_reason": "groups_overlap_multiple_subscriptions"})
		}
		existing = sub
	}
	return existing, nil
}

func subscriptionGroupIDs(sub *UserSubscription) []int64 {
	if sub == nil {
		return nil
	}
	if len(sub.GroupIDs) > 0 {
		return sub.GroupIDs
	}
	if sub.GroupID > 0 {
		return []int64{sub.GroupID}
	}
	return nil
}

func equalSubscriptionGroupIDs(left, right []int64) bool {
	left = uniqueSubscriptionGroupIDs(left)
	right = uniqueSubscriptionGroupIDs(right)
	if len(left) != len(right) {
		return false
	}
	seen := make(map[int64]struct{}, len(left))
	for _, id := range left {
		seen[id] = struct{}{}
	}
	for _, id := range right {
		if _, ok := seen[id]; !ok {
			return false
		}
	}
	return true
}

func equalOptionalFloat(left, right *float64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return math.Abs(*left-*right) < 1e-9
}

func equalOptionalInt(left, right *int) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

// AssignSubscription 分配订阅给用户（不允许重复分配）
func (s *SubscriptionService) AssignSubscription(ctx context.Context, input *AssignSubscriptionInput) (*UserSubscription, error) {
	prepared, err := s.prepareAssignmentInput(ctx, input)
	if err != nil {
		return nil, err
	}
	sub, _, err := s.assignSubscriptionWithReuse(ctx, prepared)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

// AssignOrExtendSubscription 分配或续期订阅（用于兑换码等场景）
// 如果用户已有同分组的订阅：
//   - 未过期：从当前过期时间累加天数
//   - 已过期：从当前时间开始计算新的过期时间，并激活订阅
//
// 如果没有订阅：创建新订阅
func (s *SubscriptionService) AssignOrExtendSubscription(ctx context.Context, input *AssignSubscriptionInput) (*UserSubscription, bool, error) {
	prepared, err := s.prepareAssignmentInput(ctx, input)
	if err != nil {
		return nil, false, err
	}
	return s.assignOrExtendSubscription(ctx, prepared, false)
}

func (s *SubscriptionService) assignOrExtendSubscription(ctx context.Context, input *AssignSubscriptionInput, deferCacheInvalidation bool) (*UserSubscription, bool, error) {
	prepared, err := s.prepareAssignmentInput(ctx, input)
	if err != nil {
		return nil, false, err
	}
	input = prepared
	existingSub, err := s.findExistingAssignment(ctx, input.UserID, input.GroupIDs)
	if err != nil {
		return nil, false, err
	}

	validityDays := input.ValidityDays
	if validityDays <= 0 {
		validityDays = 30
	}
	if validityDays > MaxValidityDays {
		validityDays = MaxValidityDays
	}

	// 已有订阅，执行续期（在事务中完成所有更新）
	if existingSub != nil {
		now := time.Now()
		var newExpiresAt time.Time

		isExpired := !existingSub.ExpiresAt.After(now)
		if !isExpired {
			// 未过期：从当前过期时间累加
			newExpiresAt = existingSub.ExpiresAt.AddDate(0, 0, validityDays)
		} else {
			// 已过期：从当前时间开始计算
			newExpiresAt = now.AddDate(0, 0, validityDays)
		}

		// 确保不超过最大过期时间
		if newExpiresAt.After(MaxExpiresAt) {
			newExpiresAt = MaxExpiresAt
		}

		if err := s.updateExistingSubscriptionTerm(ctx, existingSub, input, now, newExpiresAt, isExpired); err != nil {
			return nil, false, err
		}

		// 失效订阅缓存
		s.maybeInvalidateAssignmentGroupCaches(input.UserID, input.GroupIDs, deferCacheInvalidation)

		// 返回更新后的订阅
		sub, err := s.userSubRepo.GetByID(ctx, existingSub.ID)
		return sub, true, err // true 表示是续期
	}

	// 没有订阅，创建新订阅
	sub, err := s.createSubscription(ctx, input)
	if err != nil {
		return nil, false, err
	}

	// 失效订阅缓存
	s.maybeInvalidateAssignmentGroupCaches(input.UserID, input.GroupIDs, deferCacheInvalidation)

	return sub, false, nil // false 表示是新建
}

func (s *SubscriptionService) maybeInvalidateAssignmentCaches(userID, groupID int64, deferred bool) {
	s.maybeInvalidateAssignmentGroupCaches(userID, []int64{groupID}, deferred)
}

func (s *SubscriptionService) maybeInvalidateAssignmentGroupCaches(userID int64, groupIDs []int64, deferred bool) {
	if deferred {
		return
	}
	groupIDs = uniqueSubscriptionGroupIDs(groupIDs)
	for _, groupID := range groupIDs {
		s.InvalidateSubCache(userID, groupID)
	}
	if s.billingCacheService != nil {
		go func(ids []int64) {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			for _, groupID := range ids {
				_ = s.billingCacheService.InvalidateSubscription(cacheCtx, userID, groupID)
			}
		}(append([]int64(nil), groupIDs...))
	}
}

func (s *SubscriptionService) updateExistingSubscriptionTerm(
	ctx context.Context,
	existingSub *UserSubscription,
	input *AssignSubscriptionInput,
	startsAt time.Time,
	newExpiresAt time.Time,
	isExpired bool,
) error {
	return s.withSubscriptionUpdateTx(ctx, func(txCtx context.Context) error {
		updated := *existingSub
		if isExpired {
			updated = *renewedSubscriptionTerm(existingSub, input.Notes, startsAt, newExpiresAt)
		} else {
			updated.ExpiresAt = newExpiresAt
			updated.Status = SubscriptionStatusActive
			updated.Notes = appendSubscriptionNotes(existingSub.Notes, input.Notes)
		}
		if input.QuotaSnapshotted {
			updated.GroupID = input.GroupID
			updated.GroupIDs = append([]int64(nil), input.GroupIDs...)
			updated.SourcePlanID = input.SourcePlanID
			updated.QuotaSnapshotted = true
			updated.DailyLimitUSD = input.DailyLimitUSD
			updated.WeeklyLimitUSD = input.WeeklyLimitUSD
			updated.MonthlyLimitUSD = input.MonthlyLimitUSD
			updated.ConcurrencyLimit = input.ConcurrencyLimit
		}
		if err := s.userSubRepo.Update(txCtx, &updated); err != nil {
			return fmt.Errorf("update subscription term: %w", err)
		}
		return nil
	})
}

func (s *SubscriptionService) withSubscriptionUpdateTx(ctx context.Context, fn func(context.Context) error) error {
	if dbent.TxFromContext(ctx) != nil {
		return fn(ctx)
	}
	if s.entClient == nil {
		return fn(ctx)
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	txCtx := dbent.NewTxContext(ctx, tx)

	if err := fn(txCtx); err != nil {
		_ = tx.Rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func renewedSubscriptionTerm(existingSub *UserSubscription, notes string, startsAt, expiresAt time.Time) *UserSubscription {
	renewed := *existingSub
	windowStart := startsAt
	renewed.StartsAt = startsAt
	renewed.ExpiresAt = expiresAt
	renewed.Status = SubscriptionStatusActive
	renewed.DailyWindowStart = &windowStart
	renewed.WeeklyWindowStart = &windowStart
	renewed.MonthlyWindowStart = &windowStart
	renewed.DailyUsageUSD = 0
	renewed.WeeklyUsageUSD = 0
	renewed.MonthlyUsageUSD = 0
	renewed.Notes = appendSubscriptionNotes(existingSub.Notes, notes)
	return &renewed
}

func appendSubscriptionNotes(existingNotes, newNotes string) string {
	if newNotes == "" {
		return existingNotes
	}
	if existingNotes == "" {
		return newNotes
	}
	return existingNotes + "\n" + newNotes
}

// createSubscription 创建新订阅（内部方法）
func (s *SubscriptionService) createSubscription(ctx context.Context, input *AssignSubscriptionInput) (*UserSubscription, error) {
	validityDays := input.ValidityDays
	if validityDays <= 0 {
		validityDays = 30
	}
	if validityDays > MaxValidityDays {
		validityDays = MaxValidityDays
	}

	now := time.Now()
	windowStart := now
	expiresAt := now.AddDate(0, 0, validityDays)
	if expiresAt.After(MaxExpiresAt) {
		expiresAt = MaxExpiresAt
	}

	sub := &UserSubscription{
		UserID: input.UserID, GroupID: input.GroupID, GroupIDs: append([]int64(nil), input.GroupIDs...),
		SourcePlanID: input.SourcePlanID, QuotaSnapshotted: input.QuotaSnapshotted,
		DailyLimitUSD: input.DailyLimitUSD, WeeklyLimitUSD: input.WeeklyLimitUSD, MonthlyLimitUSD: input.MonthlyLimitUSD,
		ConcurrencyLimit:   input.ConcurrencyLimit,
		StartsAt:           now,
		ExpiresAt:          expiresAt,
		Status:             SubscriptionStatusActive,
		DailyWindowStart:   &windowStart,
		WeeklyWindowStart:  &windowStart,
		MonthlyWindowStart: &windowStart,
		AssignedAt:         now,
		Notes:              input.Notes,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	// 只有当 AssignedBy > 0 时才设置（0 表示系统分配，如兑换码）
	if input.AssignedBy > 0 {
		sub.AssignedBy = &input.AssignedBy
	}

	var created *UserSubscription
	if err := s.withSubscriptionUpdateTx(ctx, func(txCtx context.Context) error {
		if err := s.userSubRepo.Create(txCtx, sub); err != nil {
			return err
		}
		var err error
		created, err = s.userSubRepo.GetByID(txCtx, sub.ID)
		return err
	}); err != nil {
		return nil, err
	}
	return created, nil
}

// BulkAssignSubscriptionInput 批量分配订阅输入
type BulkAssignSubscriptionInput struct {
	UserIDs      []int64
	PlanID       int64
	GroupID      int64
	ValidityDays int
	AssignedBy   int64
	Notes        string
}

// BulkAssignResult 批量分配结果
type BulkAssignResult struct {
	SuccessCount  int
	CreatedCount  int
	ReusedCount   int
	FailedCount   int
	Subscriptions []UserSubscription
	Errors        []string
	Statuses      map[int64]string
}

// BulkAssignSubscription 批量分配订阅
func (s *SubscriptionService) BulkAssignSubscription(ctx context.Context, input *BulkAssignSubscriptionInput) (*BulkAssignResult, error) {
	result := &BulkAssignResult{
		Subscriptions: make([]UserSubscription, 0),
		Errors:        make([]string, 0),
		Statuses:      make(map[int64]string),
	}

	for _, userID := range input.UserIDs {
		assignment := &AssignSubscriptionInput{
			UserID: userID, GroupID: input.GroupID, ValidityDays: input.ValidityDays,
			AssignedBy: input.AssignedBy, Notes: input.Notes,
		}
		if input.PlanID > 0 {
			var buildErr error
			assignment, buildErr = s.BuildPlanAssignmentInput(ctx, userID, input.PlanID, input.ValidityDays, input.AssignedBy, input.Notes)
			if buildErr != nil {
				result.FailedCount++
				result.Errors = append(result.Errors, fmt.Sprintf("user %d: %v", userID, buildErr))
				result.Statuses[userID] = "failed"
				continue
			}
		}
		sub, reused, err := s.assignSubscriptionWithReuse(ctx, assignment)
		if err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, fmt.Sprintf("user %d: %v", userID, err))
			result.Statuses[userID] = "failed"
		} else {
			result.SuccessCount++
			result.Subscriptions = append(result.Subscriptions, *sub)
			if reused {
				result.ReusedCount++
				result.Statuses[userID] = "reused"
			} else {
				result.CreatedCount++
				result.Statuses[userID] = "created"
			}
		}
	}

	return result, nil
}

func (s *SubscriptionService) assignSubscriptionWithReuse(ctx context.Context, input *AssignSubscriptionInput) (*UserSubscription, bool, error) {
	prepared, err := s.prepareAssignmentInput(ctx, input)
	if err != nil {
		return nil, false, err
	}
	input = prepared
	existing, err := s.findExistingAssignment(ctx, input.UserID, input.GroupIDs)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		now := time.Now()
		if existing.Status == SubscriptionStatusExpired ||
			(existing.Status != SubscriptionStatusSuspended && !existing.ExpiresAt.After(now)) {
			validityDays := normalizeAssignValidityDays(input.ValidityDays)
			newExpiresAt := now.AddDate(0, 0, validityDays)
			if newExpiresAt.After(MaxExpiresAt) {
				newExpiresAt = MaxExpiresAt
			}

			renewalInput := *input
			if strings.TrimSpace(existing.Notes) == strings.TrimSpace(input.Notes) {
				renewalInput.Notes = ""
			}
			if err := s.updateExistingSubscriptionTerm(ctx, existing, &renewalInput, now, newExpiresAt, true); err != nil {
				return nil, false, err
			}

			affectedGroupIDs := append([]int64(nil), subscriptionGroupIDs(existing)...)
			affectedGroupIDs = append(affectedGroupIDs, input.GroupIDs...)
			s.maybeInvalidateAssignmentGroupCaches(input.UserID, affectedGroupIDs, false)
			renewed, getErr := s.userSubRepo.GetByID(ctx, existing.ID)
			return renewed, true, getErr
		}
		if conflictReason, conflict := detectAssignSemanticConflict(existing, input); conflict {
			return nil, false, ErrSubscriptionAssignConflict.WithMetadata(map[string]string{"conflict_reason": conflictReason})
		}
		return existing, true, nil
	}
	sub, err := s.createSubscription(ctx, input)
	if err != nil {
		if errors.Is(err, ErrSubscriptionAlreadyExists) || errors.Is(err, ErrSubscriptionAssignConflict) {
			existing, lookupErr := s.findExistingAssignment(ctx, input.UserID, input.GroupIDs)
			if lookupErr == nil && existing != nil {
				if conflictReason, conflict := detectAssignSemanticConflict(existing, input); !conflict {
					return existing, true, nil
				} else {
					return nil, false, ErrSubscriptionAssignConflict.WithMetadata(map[string]string{"conflict_reason": conflictReason})
				}
			}
		}
		return nil, false, err
	}
	s.maybeInvalidateAssignmentGroupCaches(input.UserID, input.GroupIDs, false)
	return sub, false, nil
}

func detectAssignSemanticConflict(existing *UserSubscription, input *AssignSubscriptionInput) (string, bool) {
	if existing == nil || input == nil {
		return "", false
	}

	if existing.QuotaSnapshotted != input.QuotaSnapshotted {
		return "quota_mode_mismatch", true
	}
	inputGroupIDs := input.GroupIDs
	if len(inputGroupIDs) == 0 && input.GroupID > 0 {
		inputGroupIDs = []int64{input.GroupID}
	}
	if !equalSubscriptionGroupIDs(subscriptionGroupIDs(existing), inputGroupIDs) {
		return "group_ids_mismatch", true
	}
	if input.QuotaSnapshotted {
		if (existing.SourcePlanID == nil) != (input.SourcePlanID == nil) ||
			(existing.SourcePlanID != nil && input.SourcePlanID != nil && *existing.SourcePlanID != *input.SourcePlanID) {
			return "source_plan_mismatch", true
		}
		if !equalOptionalFloat(existing.DailyLimitUSD, input.DailyLimitUSD) ||
			!equalOptionalFloat(existing.WeeklyLimitUSD, input.WeeklyLimitUSD) ||
			!equalOptionalFloat(existing.MonthlyLimitUSD, input.MonthlyLimitUSD) {
			return "quota_limits_mismatch", true
		}
		if !equalOptionalInt(existing.ConcurrencyLimit, input.ConcurrencyLimit) {
			return "concurrency_limit_mismatch", true
		}
	}

	normalizedDays := normalizeAssignValidityDays(input.ValidityDays)
	if !existing.StartsAt.IsZero() {
		expectedExpiresAt := existing.StartsAt.AddDate(0, 0, normalizedDays)
		if expectedExpiresAt.After(MaxExpiresAt) {
			expectedExpiresAt = MaxExpiresAt
		}
		if !existing.ExpiresAt.Equal(expectedExpiresAt) {
			return "validity_days_mismatch", true
		}
	}

	existingNotes := strings.TrimSpace(existing.Notes)
	inputNotes := strings.TrimSpace(input.Notes)
	if existingNotes != inputNotes {
		return "notes_mismatch", true
	}

	return "", false
}

func normalizeAssignValidityDays(days int) int {
	if days <= 0 {
		days = 30
	}
	if days > MaxValidityDays {
		days = MaxValidityDays
	}
	return days
}

// RevokeSubscription 撤销订阅
func (s *SubscriptionService) RevokeSubscription(ctx context.Context, subscriptionID int64) error {
	// 先获取订阅信息用于失效缓存
	sub, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return err
	}

	if err := s.withSubscriptionUpdateTx(ctx, func(txCtx context.Context) error {
		return s.userSubRepo.Delete(txCtx, subscriptionID)
	}); err != nil {
		return err
	}

	if err := s.invalidateSubscriptionGroupCaches(sub.UserID, subscriptionGroupIDs(sub)); err != nil {
		return err
	}

	return nil
}

// RestoreSubscription 恢复已撤销订阅
func (s *SubscriptionService) RestoreSubscription(ctx context.Context, subscriptionID int64) (*UserSubscription, error) {
	sub, err := s.userSubRepo.GetByIDIncludeDeleted(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}
	if sub.DeletedAt == nil {
		return nil, ErrSubscriptionNotRevoked
	}

	for _, groupID := range subscriptionGroupIDs(sub) {
		exists, err := s.userSubRepo.ExistsActiveByUserIDAndGroupID(ctx, sub.UserID, groupID)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, ErrSubscriptionRestoreConflict
		}
	}

	restoredStatus := sub.Status
	now := time.Now()
	if restoredStatus == SubscriptionStatusActive && !sub.ExpiresAt.After(now) {
		restoredStatus = SubscriptionStatusExpired
	}

	var restored *UserSubscription
	if err := s.withSubscriptionUpdateTx(ctx, func(txCtx context.Context) error {
		var restoreErr error
		restored, restoreErr = s.userSubRepo.Restore(txCtx, subscriptionID, restoredStatus)
		return restoreErr
	}); err != nil {
		return nil, err
	}

	if err := s.invalidateSubscriptionGroupCaches(restored.UserID, subscriptionGroupIDs(restored)); err != nil {
		return nil, err
	}
	return restored, nil
}

// ExtendSubscription 调整订阅时长（正数延长，负数缩短）
func (s *SubscriptionService) ExtendSubscription(ctx context.Context, subscriptionID int64, days int) (*UserSubscription, error) {
	sub, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, ErrSubscriptionNotFound
	}

	// 限制调整天数范围
	if days > MaxValidityDays {
		days = MaxValidityDays
	}
	if days < -MaxValidityDays {
		days = -MaxValidityDays
	}

	now := time.Now()
	isExpired := !sub.ExpiresAt.After(now)

	// 如果订阅已过期，不允许负向调整
	if isExpired && days < 0 {
		return nil, infraerrors.BadRequest("CANNOT_SHORTEN_EXPIRED", "cannot shorten an expired subscription")
	}

	// 计算新的过期时间
	var newExpiresAt time.Time
	if isExpired {
		// 已过期：从当前时间开始增加天数
		newExpiresAt = now.AddDate(0, 0, days)
	} else {
		// 未过期：从原过期时间增加/减少天数
		newExpiresAt = sub.ExpiresAt.AddDate(0, 0, days)
	}

	if newExpiresAt.After(MaxExpiresAt) {
		newExpiresAt = MaxExpiresAt
	}

	// 检查新的过期时间必须大于当前时间
	if !newExpiresAt.After(now) {
		return nil, ErrAdjustWouldExpire
	}

	if err := s.userSubRepo.ExtendExpiry(ctx, subscriptionID, newExpiresAt); err != nil {
		return nil, err
	}

	// 如果订阅已过期，恢复为active状态
	if sub.Status == SubscriptionStatusExpired {
		if err := s.userSubRepo.UpdateStatus(ctx, subscriptionID, SubscriptionStatusActive); err != nil {
			return nil, err
		}
	}

	// 失效该共享订阅的全部分组缓存。
	s.maybeInvalidateAssignmentGroupCaches(sub.UserID, subscriptionGroupIDs(sub), false)

	return s.userSubRepo.GetByID(ctx, subscriptionID)
}

// GetByID 根据ID获取订阅
func (s *SubscriptionService) GetByID(ctx context.Context, id int64) (*UserSubscription, error) {
	return s.userSubRepo.GetByID(ctx, id)
}

// GetActiveSubscription 获取用户对特定分组的有效订阅
// 使用 L1 缓存 + singleflight 加速中间件热路径。
// 返回缓存对象的浅拷贝，调用方可安全修改字段而不会污染缓存或触发 data race。
func (s *SubscriptionService) GetActiveSubscription(ctx context.Context, userID, groupID int64) (*UserSubscription, error) {
	key := subCacheKey(userID, groupID)

	// L1 缓存命中：返回浅拷贝
	if s.subCacheL1 != nil {
		if v, ok := s.subCacheL1.Get(key); ok {
			if sub, ok := v.(*UserSubscription); ok {
				if sub.ID == 0 {
					return nil, ErrSubscriptionNotFound
				}
				cp := *sub
				return &cp, nil
			}
		}
	}

	// singleflight 防止并发击穿
	value, err, _ := s.subCacheGroup.Do(key, func() (any, error) {
		sub, err := s.userSubRepo.GetActiveByUserIDAndGroupID(ctx, userID, groupID)
		if err != nil {
			if errors.Is(err, ErrSubscriptionNotFound) && s.subCacheL1 != nil {
				_ = s.subCacheL1.SetWithTTL(key, noActiveSubscriptionCacheEntry, 1, s.negativeSubCacheTTL())
			}
			return nil, err // 直接透传 repo 已翻译的错误（NotFound → ErrSubscriptionNotFound，其他错误原样返回）
		}
		// 写入 L1 缓存
		if s.subCacheL1 != nil {
			_ = s.subCacheL1.SetWithTTL(key, sub, 1, s.jitteredTTL(s.subCacheTTL))
		}
		return sub, nil
	})
	if err != nil {
		return nil, err
	}
	// singleflight 返回的也是缓存指针，需要浅拷贝
	sub, ok := value.(*UserSubscription)
	if !ok || sub == nil {
		return nil, ErrSubscriptionNotFound
	}
	cp := *sub
	return &cp, nil
}

// ListUserSubscriptions 获取用户的所有订阅
func (s *SubscriptionService) ListUserSubscriptions(ctx context.Context, userID int64) ([]UserSubscription, error) {
	subs, err := s.userSubRepo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	normalizeExpiredWindows(subs)
	normalizeSubscriptionStatus(subs)
	return subs, nil
}

// ListActiveUserSubscriptions 获取用户的所有有效订阅
func (s *SubscriptionService) ListActiveUserSubscriptions(ctx context.Context, userID int64) ([]UserSubscription, error) {
	subs, err := s.userSubRepo.ListActiveByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	normalizeExpiredWindows(subs)
	return subs, nil
}

// ListGroupSubscriptions 获取分组的所有订阅
func (s *SubscriptionService) ListGroupSubscriptions(ctx context.Context, groupID int64, page, pageSize int) ([]UserSubscription, *pagination.PaginationResult, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	subs, pag, err := s.userSubRepo.ListByGroupID(ctx, groupID, params)
	if err != nil {
		return nil, nil, err
	}
	normalizeExpiredWindows(subs)
	normalizeSubscriptionStatus(subs)
	return subs, pag, nil
}

// List 获取所有订阅（分页，支持筛选和排序）
func (s *SubscriptionService) List(ctx context.Context, page, pageSize int, userID, groupID *int64, status, platform, sortBy, sortOrder string) ([]UserSubscription, *pagination.PaginationResult, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	subs, pag, err := s.userSubRepo.List(ctx, params, userID, groupID, status, platform, sortBy, sortOrder)
	if err != nil {
		return nil, nil, err
	}
	normalizeExpiredWindows(subs)
	normalizeSubscriptionStatus(subs)
	return subs, pag, nil
}

// normalizeExpiredWindows 将返回数据对齐到以订阅生效时间为锚点的当前滚动窗口（不写 DB）。
func normalizeExpiredWindows(subs []UserSubscription) {
	now := time.Now()
	for i := range subs {
		if normalized := subs[i].NormalizedWindowSnapshotAt(now); normalized != nil {
			subs[i] = *normalized
		}
	}
}

// normalizeSubscriptionStatus 根据实际过期时间修正状态（仅影响返回数据，不影响数据库）
// 这确保前端显示正确的状态，即使定时任务尚未更新数据库
func normalizeSubscriptionStatus(subs []UserSubscription) {
	now := time.Now()
	for i := range subs {
		sub := &subs[i]
		if sub.Status == SubscriptionStatusActive && !sub.ExpiresAt.After(now) {
			sub.Status = SubscriptionStatusExpired
		}
	}
}

// CheckAndActivateWindow 检查并激活窗口（兼容尚未初始化窗口的存量订阅）。
func (s *SubscriptionService) CheckAndActivateWindow(ctx context.Context, sub *UserSubscription) error {
	if sub == nil {
		return ErrSubscriptionNilInput
	}
	if sub.IsWindowActivated() {
		return nil
	}

	starts := sub.WindowStartsAt(time.Now())
	if err := s.userSubRepo.ActivateWindows(ctx, sub.ID, starts); err != nil {
		return err
	}
	sub.DailyWindowStart = &starts.Daily
	sub.WeeklyWindowStart = &starts.Weekly
	sub.MonthlyWindowStart = &starts.Monthly
	return nil
}

// AdminResetQuota manually resets selected usage counters without changing the
// purchase-time anchor of their automatic rolling windows.
func (s *SubscriptionService) AdminResetQuota(ctx context.Context, subscriptionID int64, resetDaily, resetWeekly, resetMonthly bool) (*UserSubscription, error) {
	if !resetDaily && !resetWeekly && !resetMonthly {
		return nil, ErrInvalidInput
	}
	sub, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}
	starts := sub.WindowStartsAt(time.Now())
	if err := s.userSubRepo.ResetUsageWindows(ctx, sub.ID, resetDaily, resetWeekly, resetMonthly, starts); err != nil {
		return nil, err
	}
	// Invalidate L1 ristretto cache. Ristretto's Del() is asynchronous by design,
	// so call Wait() immediately after to flush pending operations and guarantee
	// the deleted key is not returned on the very next Get() call.
	for _, groupID := range subscriptionGroupIDs(sub) {
		s.InvalidateSubCacheSync(sub.UserID, groupID)
		if s.billingCacheService != nil {
			_ = s.billingCacheService.InvalidateSubscription(ctx, sub.UserID, groupID)
		}
	}
	// Return the refreshed subscription from DB
	return s.userSubRepo.GetByID(ctx, subscriptionID)
}

// CheckAndResetWindows advances expired windows to the current period derived
// from StartsAt, preserving the exact purchase/effective-time anchor.
func (s *SubscriptionService) CheckAndResetWindows(ctx context.Context, sub *UserSubscription) error {
	return s.checkAndResetWindowsAt(ctx, sub, time.Now())
}

func (s *SubscriptionService) checkAndResetWindowsAt(ctx context.Context, sub *UserSubscription, now time.Time) error {
	if sub == nil {
		return ErrSubscriptionNilInput
	}
	needsInvalidateCache := false

	if sub.NeedsDailyResetAt(now) {
		windowStart := sub.CurrentDailyWindowStartAt(now)
		expectedWindowStart := sub.DailyWindowStart
		if err := s.userSubRepo.ResetDailyUsage(ctx, sub.ID, expectedWindowStart, windowStart); err != nil {
			return err
		}
		sub.DailyWindowStart = &windowStart
		sub.DailyUsageUSD = 0
		needsInvalidateCache = true
	}

	if sub.NeedsWeeklyResetAt(now) {
		windowStart := sub.CurrentWeeklyWindowStartAt(now)
		expectedWindowStart := sub.WeeklyWindowStart
		if err := s.userSubRepo.ResetWeeklyUsage(ctx, sub.ID, expectedWindowStart, windowStart); err != nil {
			return err
		}
		sub.WeeklyWindowStart = &windowStart
		sub.WeeklyUsageUSD = 0
		needsInvalidateCache = true
	}

	if sub.NeedsMonthlyResetAt(now) {
		windowStart := sub.CurrentMonthlyWindowStartAt(now)
		expectedWindowStart := sub.MonthlyWindowStart
		if err := s.userSubRepo.ResetMonthlyUsage(ctx, sub.ID, expectedWindowStart, windowStart); err != nil {
			return err
		}
		sub.MonthlyWindowStart = &windowStart
		sub.MonthlyUsageUSD = 0
		needsInvalidateCache = true
	}

	if needsInvalidateCache {
		for _, groupID := range subscriptionGroupIDs(sub) {
			s.InvalidateSubCache(sub.UserID, groupID)
			if s.billingCacheService != nil {
				_ = s.billingCacheService.InvalidateSubscription(ctx, sub.UserID, groupID)
			}
		}
	}

	return nil
}

// EnsureWindowMaintenance advances expired usage windows before a request is
// allowed to proceed. It returns a fresh database snapshot because a competing
// request may have won one of the conditional resets.
func (s *SubscriptionService) EnsureWindowMaintenance(ctx context.Context, sub *UserSubscription) (*UserSubscription, error) {
	if sub == nil {
		return nil, ErrSubscriptionNilInput
	}
	if !sub.IsWindowActivated() {
		if err := s.CheckAndActivateWindow(ctx, sub); err != nil {
			return nil, err
		}
	}
	if err := s.CheckAndResetWindows(ctx, sub); err != nil {
		return nil, err
	}

	// GetByID bypasses the service caches. This prevents a stale loser of the
	// CAS from validating limits against zeroed in-memory usage.
	refreshed, err := s.userSubRepo.GetByID(ctx, sub.ID)
	if err != nil {
		return nil, err
	}
	for _, groupID := range subscriptionGroupIDs(sub) {
		s.InvalidateSubCacheSync(sub.UserID, groupID)
	}
	return refreshed, nil
}

// CheckUsageLimits 检查使用限额（返回错误如果超限）
// 用于中间件的快速预检查，additionalCost 通常为 0
func (s *SubscriptionService) CheckUsageLimits(ctx context.Context, sub *UserSubscription, group *Group, additionalCost float64) error {
	if !sub.CheckDailyLimit(group, additionalCost) {
		return ErrDailyLimitExceeded
	}
	if !sub.CheckWeeklyLimit(group, additionalCost) {
		return ErrWeeklyLimitExceeded
	}
	if !sub.CheckMonthlyLimit(group, additionalCost) {
		return ErrMonthlyLimitExceeded
	}
	return nil
}

// ValidateAndCheckLimits 合并验证+限额检查（中间件热路径专用）
// 仅做内存检查，不触发 DB 写入。调用方必须在放行请求前同步完成窗口维护。
// 返回 needsMaintenance 表示是否需要执行窗口维护并回读数据库快照。
func (s *SubscriptionService) ValidateAndCheckLimits(sub *UserSubscription, group *Group) (needsMaintenance bool, err error) {
	// 1. 验证订阅状态
	if sub.Status == SubscriptionStatusExpired {
		return false, ErrSubscriptionExpired
	}
	if sub.Status == SubscriptionStatusSuspended {
		return false, ErrSubscriptionSuspended
	}
	if sub.IsExpired() {
		return false, ErrSubscriptionExpired
	}

	// 2. 内存中修正过期窗口的用量，确保预检查不会误拒绝用户。
	//    调用方随后同步推进 DB 窗口，并用回读快照重新校验。
	now := time.Now()
	if sub.NeedsDailyResetAt(now) {
		sub.DailyUsageUSD = 0
		needsMaintenance = true
	}
	if sub.NeedsWeeklyResetAt(now) {
		sub.WeeklyUsageUSD = 0
		needsMaintenance = true
	}
	if sub.NeedsMonthlyResetAt(now) {
		sub.MonthlyUsageUSD = 0
		needsMaintenance = true
	}
	if !sub.IsWindowActivated() {
		needsMaintenance = true
	}

	// 3. 检查用量限额
	if !sub.CheckDailyLimit(group, 0) {
		return needsMaintenance, ErrDailyLimitExceeded
	}
	if !sub.CheckWeeklyLimit(group, 0) {
		return needsMaintenance, ErrWeeklyLimitExceeded
	}
	if !sub.CheckMonthlyLimit(group, 0) {
		return needsMaintenance, ErrMonthlyLimitExceeded
	}

	return needsMaintenance, nil
}

// DoWindowMaintenance 异步执行窗口维护（激活+重置）
// 使用独立 context，不受请求取消影响。
// 注意：此方法仅在 ValidateAndCheckLimits 返回 needsMaintenance=true 时调用，
// 而 IsExpired()=true 的订阅在 ValidateAndCheckLimits 中已被拦截返回错误，
// 因此进入此方法的订阅一定未过期，无需处理过期状态同步。
func (s *SubscriptionService) DoWindowMaintenance(sub *UserSubscription) {
	if s == nil {
		return
	}
	if s.maintenanceQueue != nil {
		err := s.maintenanceQueue.TryEnqueue(func() {
			s.doWindowMaintenance(sub)
		})
		if err != nil {
			log.Printf("Subscription maintenance enqueue failed: %v", err)
		}
		return
	}

	s.doWindowMaintenance(sub)
}

func (s *SubscriptionService) doWindowMaintenance(sub *UserSubscription) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 激活窗口（首次使用时）
	if !sub.IsWindowActivated() {
		if err := s.CheckAndActivateWindow(ctx, sub); err != nil {
			log.Printf("Failed to activate subscription windows: %v", err)
		}
	}

	// 重置过期窗口
	if err := s.CheckAndResetWindows(ctx, sub); err != nil {
		log.Printf("Failed to reset subscription windows: %v", err)
	}

	// 失效 L1 缓存，确保全部白名单分组拿到更新后的数据。
	for _, groupID := range subscriptionGroupIDs(sub) {
		s.InvalidateSubCache(sub.UserID, groupID)
	}
}

// RecordUsage 记录使用量到订阅
func (s *SubscriptionService) RecordUsage(ctx context.Context, subscriptionID int64, costUSD float64) error {
	return s.userSubRepo.IncrementUsage(ctx, subscriptionID, costUSD)
}

// SubscriptionProgress 订阅进度
type SubscriptionProgress struct {
	ID            int64                `json:"id"`
	GroupName     string               `json:"group_name"`
	ExpiresAt     time.Time            `json:"expires_at"`
	ExpiresInDays int                  `json:"expires_in_days"`
	Daily         *UsageWindowProgress `json:"daily,omitempty"`
	Weekly        *UsageWindowProgress `json:"weekly,omitempty"`
	Monthly       *UsageWindowProgress `json:"monthly,omitempty"`
}

// UsageWindowProgress 使用窗口进度
type UsageWindowProgress struct {
	LimitUSD        float64   `json:"limit_usd"`
	UsedUSD         float64   `json:"used_usd"`
	RemainingUSD    float64   `json:"remaining_usd"`
	Percentage      float64   `json:"percentage"`
	WindowStart     time.Time `json:"window_start"`
	ResetsAt        time.Time `json:"resets_at"`
	ResetsInSeconds int64     `json:"resets_in_seconds"`
}

// GetSubscriptionProgress 获取订阅使用进度
func (s *SubscriptionService) GetSubscriptionProgress(ctx context.Context, subscriptionID int64) (*SubscriptionProgress, error) {
	sub, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, ErrSubscriptionNotFound
	}

	group := sub.Group
	if group == nil {
		group, err = s.groupRepo.GetByID(ctx, sub.GroupID)
		if err != nil {
			return nil, err
		}
	}

	return s.calculateProgress(sub, group), nil
}

// calculateProgress 根据已加载的订阅和分组数据计算使用进度（纯内存计算，无 DB 查询）
func (s *SubscriptionService) calculateProgress(sub *UserSubscription, group *Group) *SubscriptionProgress {
	now := time.Now()
	if normalized := sub.NormalizedWindowSnapshotAt(now); normalized != nil {
		sub = normalized
	}
	progress := &SubscriptionProgress{
		ID:            sub.ID,
		GroupName:     group.Name,
		ExpiresAt:     sub.ExpiresAt,
		ExpiresInDays: sub.daysRemainingAt(now),
	}

	dailyStart := sub.DailyWindowStart
	dailyUsage := sub.DailyUsageUSD
	if sub.NeedsDailyResetAt(now) {
		current := sub.CurrentDailyWindowStartAt(now)
		dailyStart = &current
		dailyUsage = 0
	} else if dailyStart == nil && !sub.StartsAt.IsZero() {
		current := sub.CurrentDailyWindowStartAt(now)
		dailyStart = &current
	}
	if dailyLimit := sub.EffectiveDailyLimit(group); dailyLimit != nil && dailyStart != nil {
		resetsAt := dailyStart.Add(subscriptionDayDuration)
		if resetTime := sub.DailyResetTimeAt(now); resetTime != nil {
			resetsAt = *resetTime
		}
		progress.Daily = newUsageWindowProgress(*dailyLimit, dailyUsage, *dailyStart, resetsAt, now)
	}

	weeklyStart := sub.WeeklyWindowStart
	weeklyUsage := sub.WeeklyUsageUSD
	if sub.NeedsWeeklyResetAt(now) {
		current := sub.CurrentWeeklyWindowStartAt(now)
		weeklyStart = &current
		weeklyUsage = 0
	} else if weeklyStart == nil && !sub.StartsAt.IsZero() {
		current := sub.CurrentWeeklyWindowStartAt(now)
		weeklyStart = &current
	}
	if weeklyLimit := sub.EffectiveWeeklyLimit(group); weeklyLimit != nil && weeklyStart != nil {
		resetsAt := weeklyStart.Add(subscriptionWeekDuration)
		if resetTime := sub.WeeklyResetTimeAt(now); resetTime != nil {
			resetsAt = *resetTime
		}
		progress.Weekly = newUsageWindowProgress(*weeklyLimit, weeklyUsage, *weeklyStart, resetsAt, now)
	}

	monthlyStart := sub.MonthlyWindowStart
	monthlyUsage := sub.MonthlyUsageUSD
	if sub.NeedsMonthlyResetAt(now) {
		current := sub.CurrentMonthlyWindowStartAt(now)
		monthlyStart = &current
		monthlyUsage = 0
	} else if monthlyStart == nil && !sub.StartsAt.IsZero() {
		current := sub.CurrentMonthlyWindowStartAt(now)
		monthlyStart = &current
	}
	if monthlyLimit := sub.EffectiveMonthlyLimit(group); monthlyLimit != nil && monthlyStart != nil {
		resetsAt := monthlyStart.Add(subscriptionMonthDuration)
		if resetTime := sub.MonthlyResetTimeAt(now); resetTime != nil {
			resetsAt = *resetTime
		}
		progress.Monthly = newUsageWindowProgress(*monthlyLimit, monthlyUsage, *monthlyStart, resetsAt, now)
	}

	return progress
}

func newUsageWindowProgress(limit, usage float64, windowStart, resetsAt, now time.Time) *UsageWindowProgress {
	progress := &UsageWindowProgress{
		LimitUSD:        limit,
		UsedUSD:         usage,
		RemainingUSD:    limit - usage,
		Percentage:      (usage / limit) * 100,
		WindowStart:     windowStart,
		ResetsAt:        resetsAt,
		ResetsInSeconds: int64(resetsAt.Sub(now).Seconds()),
	}
	if progress.RemainingUSD < 0 {
		progress.RemainingUSD = 0
	}
	if progress.Percentage > 100 {
		progress.Percentage = 100
	}
	if progress.ResetsInSeconds < 0 {
		progress.ResetsInSeconds = 0
	}
	return progress
}

// GetUserSubscriptionsWithProgress 获取用户所有订阅及进度
func (s *SubscriptionService) GetUserSubscriptionsWithProgress(ctx context.Context, userID int64) ([]SubscriptionProgress, error) {
	// ListActiveByUserID 已使用 .WithGroup() eager-load Group 关联，1 次查询获取所有数据
	subs, err := s.userSubRepo.ListActiveByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	progresses := make([]SubscriptionProgress, 0, len(subs))
	for i := range subs {
		sub := &subs[i]
		group := sub.Group
		if group == nil {
			continue
		}
		progresses = append(progresses, *s.calculateProgress(sub, group))
	}

	return progresses, nil
}

// ValidateSubscription 验证订阅是否有效
func (s *SubscriptionService) ValidateSubscription(ctx context.Context, sub *UserSubscription) error {
	if sub.Status == SubscriptionStatusExpired {
		return ErrSubscriptionExpired
	}
	if sub.Status == SubscriptionStatusSuspended {
		return ErrSubscriptionSuspended
	}
	if sub.IsExpired() {
		// 更新状态
		_ = s.userSubRepo.UpdateStatus(ctx, sub.ID, SubscriptionStatusExpired)
		return ErrSubscriptionExpired
	}
	return nil
}
