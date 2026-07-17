package service

import (
	"context"
	"fmt"
	"math"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionplan"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionplangroup"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func normalizeSubscriptionPlanType(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return domain.SubscriptionPlanTypeSubscription
	}
	return value
}

func validateSubscriptionPlanType(value string, allowLegacy bool) error {
	switch normalizeSubscriptionPlanType(value) {
	case domain.SubscriptionPlanTypeSubscription, domain.SubscriptionPlanTypeStandardQuota:
		return nil
	case domain.SubscriptionPlanTypeLegacySharedSubscription:
		if allowLegacy {
			return nil
		}
	}
	return infraerrors.BadRequest("PLAN_TYPE_INVALID", "plan type must be subscription or standard_quota")
}

// normalizePlanCurrency validates and normalizes the display-only currency label.
// Empty means "no label" and is kept as-is so existing plans stay unchanged.
func normalizePlanCurrency(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	currency, err := payment.NormalizePaymentCurrency(raw)
	if err != nil {
		return "", infraerrors.BadRequest("PLAN_CURRENCY_INVALID", "currency must be a 3-letter ISO currency code")
	}
	return currency, nil
}

// validatePlanRequired checks that all required fields for a plan are provided.
func validatePlanRequired(name string, groupIDs []int64, price float64, validityDays int, validityUnit string, originalPrice *float64) error {
	if strings.TrimSpace(name) == "" {
		return infraerrors.BadRequest("PLAN_NAME_REQUIRED", "plan name is required")
	}
	if len(groupIDs) == 0 {
		return infraerrors.BadRequest("PLAN_GROUP_REQUIRED", "at least one group is required")
	}
	if price <= 0 {
		return infraerrors.BadRequest("PLAN_PRICE_INVALID", "price must be > 0")
	}
	if validityDays <= 0 {
		return infraerrors.BadRequest("PLAN_VALIDITY_REQUIRED", "validity days must be > 0")
	}
	if strings.TrimSpace(validityUnit) == "" {
		return infraerrors.BadRequest("PLAN_VALIDITY_UNIT_REQUIRED", "validity unit is required")
	}
	if originalPrice != nil && *originalPrice < 0 {
		return infraerrors.BadRequest("PLAN_ORIGINAL_PRICE_INVALID", "original price must be >= 0")
	}
	return nil
}

func validatePlanPatch(req UpdatePlanRequest) error {
	if req.PlanType != nil {
		if err := validateSubscriptionPlanType(*req.PlanType, false); err != nil {
			return err
		}
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		return infraerrors.BadRequest("PLAN_NAME_REQUIRED", "plan name is required")
	}
	if req.GroupID != nil && *req.GroupID <= 0 {
		return infraerrors.BadRequest("PLAN_GROUP_REQUIRED", "group is required")
	}
	if req.GroupIDs != nil && len(req.GroupIDs) == 0 {
		return infraerrors.BadRequest("PLAN_GROUP_REQUIRED", "at least one group is required")
	}
	if req.Price != nil && *req.Price <= 0 {
		return infraerrors.BadRequest("PLAN_PRICE_INVALID", "price must be > 0")
	}
	if req.ValidityDays != nil && *req.ValidityDays <= 0 {
		return infraerrors.BadRequest("PLAN_VALIDITY_REQUIRED", "validity days must be > 0")
	}
	if req.ValidityUnit != nil && strings.TrimSpace(*req.ValidityUnit) == "" {
		return infraerrors.BadRequest("PLAN_VALIDITY_UNIT_REQUIRED", "validity unit is required")
	}
	if req.OriginalPrice != nil && *req.OriginalPrice < 0 {
		return infraerrors.BadRequest("PLAN_ORIGINAL_PRICE_INVALID", "original price must be >= 0")
	}
	if req.Currency != nil {
		if _, err := normalizePlanCurrency(*req.Currency); err != nil {
			return err
		}
	}
	return validatePlanQuotaLimits(req.DailyLimitUSD, req.WeeklyLimitUSD, req.MonthlyLimitUSD)
}

func validatePlanQuotaLimits(limits ...*float64) error {
	for _, limit := range limits {
		if limit == nil {
			continue
		}
		if math.IsNaN(*limit) || math.IsInf(*limit, 0) || *limit <= 0 {
			return infraerrors.BadRequest("PLAN_QUOTA_INVALID", "quota limits must be positive numbers or null")
		}
	}
	return nil
}

func validatePlanSemantics(planType string, groupIDs []int64, dailyLimit, weeklyLimit, monthlyLimit *float64) error {
	planType = normalizeSubscriptionPlanType(planType)
	if err := validateSubscriptionPlanType(planType, false); err != nil {
		return err
	}
	switch planType {
	case domain.SubscriptionPlanTypeSubscription:
		if len(groupIDs) != 1 {
			return infraerrors.BadRequest("PLAN_GROUP_COUNT_INVALID", "subscription plans require exactly one subscription group")
		}
	case domain.SubscriptionPlanTypeStandardQuota:
		if len(groupIDs) == 0 {
			return infraerrors.BadRequest("PLAN_GROUP_REQUIRED", "standard_quota plans require at least one standard group")
		}
		if dailyLimit == nil && weeklyLimit == nil && monthlyLimit == nil {
			return infraerrors.BadRequest("PLAN_QUOTA_REQUIRED", "standard_quota plans require at least one quota limit")
		}
	}
	return nil
}

func normalizePlanGroupIDs(groupIDs []int64, legacyGroupID int64) []int64 {
	if len(groupIDs) == 0 && legacyGroupID > 0 {
		groupIDs = []int64{legacyGroupID}
	}
	seen := make(map[int64]struct{}, len(groupIDs))
	out := make([]int64, 0, len(groupIDs))
	for _, id := range groupIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// PlanGroupInfo holds the group details needed for subscription plan display.
type PlanGroupInfo struct {
	ID                 int64    `json:"id"`
	Platform           string   `json:"platform"`
	Name               string   `json:"name"`
	SubscriptionType   string   `json:"subscription_type"`
	RateMultiplier     float64  `json:"rate_multiplier"`
	PeakRateEnabled    bool     `json:"peak_rate_enabled"`
	PeakStart          string   `json:"peak_start"`
	PeakEnd            string   `json:"peak_end"`
	PeakRateMultiplier float64  `json:"peak_rate_multiplier"`
	DailyLimitUSD      *float64 `json:"daily_limit_usd"`
	WeeklyLimitUSD     *float64 `json:"weekly_limit_usd"`
	MonthlyLimitUSD    *float64 `json:"monthly_limit_usd"`
	ModelScopes        []string `json:"supported_model_scopes"`
}

// SubscriptionPlanResponse keeps legacy plan fields while exposing multi-group data.
type SubscriptionPlanResponse struct {
	ID              int64           `json:"id"`
	PlanType        string          `json:"plan_type"`
	GroupID         int64           `json:"group_id"`
	GroupIDs        []int64         `json:"group_ids"`
	Groups          []PlanGroupInfo `json:"groups"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	Price           float64         `json:"price"`
	OriginalPrice   *float64        `json:"original_price,omitempty"`
	Currency        string          `json:"currency"`
	DailyLimitUSD   *float64        `json:"daily_limit_usd"`
	WeeklyLimitUSD  *float64        `json:"weekly_limit_usd"`
	MonthlyLimitUSD *float64        `json:"monthly_limit_usd"`
	ValidityDays    int             `json:"validity_days"`
	ValidityUnit    string          `json:"validity_unit"`
	Features        string          `json:"features"`
	ProductName     string          `json:"product_name"`
	ForSale         bool            `json:"for_sale"`
	SortOrder       int             `json:"sort_order"`
	CreatedAt       any             `json:"created_at"`
	UpdatedAt       any             `json:"updated_at"`
}

func planGroupInfoFromEntity(g *dbent.Group) PlanGroupInfo {
	return PlanGroupInfo{
		ID:                 g.ID,
		Platform:           g.Platform,
		Name:               g.Name,
		SubscriptionType:   g.SubscriptionType,
		RateMultiplier:     g.RateMultiplier,
		PeakRateEnabled:    g.PeakRateEnabled,
		PeakStart:          g.PeakStart,
		PeakEnd:            g.PeakEnd,
		PeakRateMultiplier: g.PeakRateMultiplier,
		DailyLimitUSD:      g.DailyLimitUsd,
		WeeklyLimitUSD:     g.WeeklyLimitUsd,
		MonthlyLimitUSD:    g.MonthlyLimitUsd,
		ModelScopes:        g.SupportedModelScopes,
	}
}

// GetGroupInfoMap returns legacy primary-group information for plans.
func (s *PaymentConfigService) GetGroupInfoMap(ctx context.Context, plans []*dbent.SubscriptionPlan) map[int64]PlanGroupInfo {
	ids := make([]int64, 0, len(plans))
	seen := make(map[int64]bool)
	for _, p := range plans {
		if !seen[p.GroupID] {
			seen[p.GroupID] = true
			ids = append(ids, p.GroupID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	groups, err := s.entClient.Group.Query().Where(group.IDIn(ids...)).All(ctx)
	if err != nil {
		return nil
	}
	m := make(map[int64]PlanGroupInfo, len(groups))
	for _, g := range groups {
		m[g.ID] = planGroupInfoFromEntity(g)
	}
	return m
}

// GetPlanGroupsMap returns ordered whitelist groups for each plan.
func (s *PaymentConfigService) GetPlanGroupsMap(ctx context.Context, plans []*dbent.SubscriptionPlan) map[int64][]PlanGroupInfo {
	planIDs := make([]int64, 0, len(plans))
	for _, plan := range plans {
		planIDs = append(planIDs, plan.ID)
	}
	out := make(map[int64][]PlanGroupInfo, len(plans))
	if len(planIDs) == 0 {
		return out
	}
	bindings, err := s.entClient.SubscriptionPlanGroup.Query().
		Where(subscriptionplangroup.PlanIDIn(planIDs...)).
		WithGroup().
		Order(subscriptionplangroup.ByPlanID(), subscriptionplangroup.ByPriority(), subscriptionplangroup.ByGroupID()).
		All(ctx)
	if err == nil {
		for _, binding := range bindings {
			if binding.Edges.Group != nil {
				out[binding.PlanID] = append(out[binding.PlanID], planGroupInfoFromEntity(binding.Edges.Group))
			}
		}
	}
	primary := s.GetGroupInfoMap(ctx, plans)
	for _, plan := range plans {
		if len(out[plan.ID]) == 0 {
			if info, ok := primary[plan.GroupID]; ok {
				out[plan.ID] = []PlanGroupInfo{info}
			}
		}
	}
	return out
}

func (s *PaymentConfigService) PlanResponses(ctx context.Context, plans []*dbent.SubscriptionPlan) []SubscriptionPlanResponse {
	groupsByPlan := s.GetPlanGroupsMap(ctx, plans)
	out := make([]SubscriptionPlanResponse, 0, len(plans))
	for _, plan := range plans {
		groups := groupsByPlan[plan.ID]
		groupIDs := make([]int64, 0, len(groups))
		for _, item := range groups {
			groupIDs = append(groupIDs, item.ID)
		}
		out = append(out, SubscriptionPlanResponse{
			ID: plan.ID, PlanType: normalizeSubscriptionPlanType(plan.PlanType), GroupID: plan.GroupID, GroupIDs: groupIDs, Groups: groups,
			Name: plan.Name, Description: plan.Description, Price: plan.Price, OriginalPrice: plan.OriginalPrice, Currency: plan.Currency,
			DailyLimitUSD: plan.DailyLimitUsd, WeeklyLimitUSD: plan.WeeklyLimitUsd, MonthlyLimitUSD: plan.MonthlyLimitUsd,
			ValidityDays: plan.ValidityDays, ValidityUnit: plan.ValidityUnit, Features: plan.Features,
			ProductName: plan.ProductName, ForSale: plan.ForSale, SortOrder: plan.SortOrder,
			CreatedAt: plan.CreatedAt, UpdatedAt: plan.UpdatedAt,
		})
	}
	return out
}

func (s *PaymentConfigService) ListPlans(ctx context.Context) ([]*dbent.SubscriptionPlan, error) {
	return s.entClient.SubscriptionPlan.Query().Order(subscriptionplan.BySortOrder()).All(ctx)
}

func (s *PaymentConfigService) ListPlansForSale(ctx context.Context) ([]*dbent.SubscriptionPlan, error) {
	return s.entClient.SubscriptionPlan.Query().Where(
		subscriptionplan.ForSaleEQ(true),
		subscriptionplan.PlanTypeNEQ(domain.SubscriptionPlanTypeLegacySharedSubscription),
	).Order(subscriptionplan.BySortOrder()).All(ctx)
}

func (s *PaymentConfigService) validatePlanGroups(ctx context.Context, planType string, groupIDs []int64) error {
	if len(groupIDs) == 0 {
		return infraerrors.BadRequest("PLAN_GROUP_REQUIRED", "at least one group is required")
	}
	expectedType := domain.SubscriptionTypeSubscription
	if normalizeSubscriptionPlanType(planType) == domain.SubscriptionPlanTypeStandardQuota {
		expectedType = domain.SubscriptionTypeStandard
	}
	count, err := s.entClient.Group.Query().Where(
		group.IDIn(groupIDs...),
		group.StatusEQ(domain.StatusActive),
		group.SubscriptionTypeEQ(expectedType),
	).Count(ctx)
	if err != nil {
		return fmt.Errorf("validate plan groups: %w", err)
	}
	if count != len(groupIDs) {
		return infraerrors.BadRequest("PLAN_GROUP_INVALID", fmt.Sprintf("all groups must exist, be active, and use %s billing", expectedType))
	}
	return nil
}

func (s *PaymentConfigService) loadPlanGroupIDs(ctx context.Context, plan *dbent.SubscriptionPlan) ([]int64, error) {
	if plan == nil {
		return nil, infraerrors.NotFound("PLAN_NOT_FOUND", "subscription plan not found")
	}
	bindings, err := s.entClient.SubscriptionPlanGroup.Query().
		Where(subscriptionplangroup.PlanIDEQ(plan.ID)).
		Order(subscriptionplangroup.ByPriority(), subscriptionplangroup.ByGroupID()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("load plan groups: %w", err)
	}
	groupIDs := make([]int64, 0, len(bindings))
	for _, binding := range bindings {
		groupIDs = append(groupIDs, binding.GroupID)
	}
	if len(groupIDs) == 0 && plan.GroupID > 0 {
		groupIDs = []int64{plan.GroupID}
	}
	return normalizePlanGroupIDs(groupIDs, plan.GroupID), nil
}

func syncPlanGroups(ctx context.Context, client *dbent.Client, planID int64, groupIDs []int64) error {
	if _, err := client.SubscriptionPlanGroup.Delete().Where(subscriptionplangroup.PlanIDEQ(planID)).Exec(ctx); err != nil {
		return err
	}
	builders := make([]*dbent.SubscriptionPlanGroupCreate, 0, len(groupIDs))
	for priority, groupID := range groupIDs {
		builders = append(builders, client.SubscriptionPlanGroup.Create().SetPlanID(planID).SetGroupID(groupID).SetPriority(priority))
	}
	if len(builders) == 0 {
		return nil
	}
	return client.SubscriptionPlanGroup.CreateBulk(builders...).Exec(ctx)
}

func (s *PaymentConfigService) CreatePlan(ctx context.Context, req CreatePlanRequest) (*dbent.SubscriptionPlan, error) {
	planType := normalizeSubscriptionPlanType(req.PlanType)
	groupIDs := normalizePlanGroupIDs(req.GroupIDs, req.GroupID)
	if err := validatePlanRequired(req.Name, groupIDs, req.Price, req.ValidityDays, req.ValidityUnit, req.OriginalPrice); err != nil {
		return nil, err
	}
	if err := validatePlanQuotaLimits(req.DailyLimitUSD, req.WeeklyLimitUSD, req.MonthlyLimitUSD); err != nil {
		return nil, err
	}
	if planType == domain.SubscriptionPlanTypeSubscription {
		req.DailyLimitUSD = nil
		req.WeeklyLimitUSD = nil
		req.MonthlyLimitUSD = nil
	}
	if err := validatePlanSemantics(planType, groupIDs, req.DailyLimitUSD, req.WeeklyLimitUSD, req.MonthlyLimitUSD); err != nil {
		return nil, err
	}
	if err := s.validatePlanGroups(ctx, planType, groupIDs); err != nil {
		return nil, err
	}
	currency, err := normalizePlanCurrency(req.Currency)
	if err != nil {
		return nil, err
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin create plan transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	b := tx.SubscriptionPlan.Create().
		SetPlanType(planType).
		SetGroupID(groupIDs[0]).SetName(req.Name).SetDescription(req.Description).
		SetPrice(req.Price).SetCurrency(currency).SetValidityDays(req.ValidityDays).SetValidityUnit(req.ValidityUnit).
		SetFeatures(req.Features).SetProductName(req.ProductName).
		SetForSale(req.ForSale).SetSortOrder(req.SortOrder).
		SetNillableOriginalPrice(req.OriginalPrice).
		SetNillableDailyLimitUsd(req.DailyLimitUSD).
		SetNillableWeeklyLimitUsd(req.WeeklyLimitUSD).
		SetNillableMonthlyLimitUsd(req.MonthlyLimitUSD)
	plan, err := b.Save(ctx)
	if err != nil {
		return nil, err
	}
	if err := syncPlanGroups(ctx, tx.Client(), plan.ID, groupIDs); err != nil {
		return nil, fmt.Errorf("sync plan groups: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit create plan transaction: %w", err)
	}
	return s.GetPlan(ctx, plan.ID)
}

func (s *PaymentConfigService) UpdatePlan(ctx context.Context, id int64, req UpdatePlanRequest) (*dbent.SubscriptionPlan, error) {
	if err := validatePlanPatch(req); err != nil {
		return nil, err
	}
	existing, err := s.GetPlan(ctx, id)
	if err != nil {
		return nil, err
	}
	existingType := normalizeSubscriptionPlanType(existing.PlanType)
	if existingType == domain.SubscriptionPlanTypeLegacySharedSubscription && req.PlanType == nil {
		return nil, infraerrors.Conflict("PLAN_LEGACY_READ_ONLY", "legacy shared subscription plans must be converted before editing")
	}

	planType := existingType
	if req.PlanType != nil {
		planType = normalizeSubscriptionPlanType(*req.PlanType)
	}
	groupIDs, err := s.loadPlanGroupIDs(ctx, existing)
	if err != nil {
		return nil, err
	}
	if req.GroupIDs != nil {
		groupIDs = normalizePlanGroupIDs(req.GroupIDs, 0)
	} else if req.GroupID != nil {
		groupIDs = normalizePlanGroupIDs(nil, *req.GroupID)
	}

	dailyLimit := existing.DailyLimitUsd
	weeklyLimit := existing.WeeklyLimitUsd
	monthlyLimit := existing.MonthlyLimitUsd
	if req.QuotaLimitsSet || req.DailyLimitUSD != nil || req.WeeklyLimitUSD != nil || req.MonthlyLimitUSD != nil {
		dailyLimit = req.DailyLimitUSD
		weeklyLimit = req.WeeklyLimitUSD
		monthlyLimit = req.MonthlyLimitUSD
	}
	if planType == domain.SubscriptionPlanTypeSubscription {
		dailyLimit = nil
		weeklyLimit = nil
		monthlyLimit = nil
	}
	if err := validatePlanQuotaLimits(dailyLimit, weeklyLimit, monthlyLimit); err != nil {
		return nil, err
	}
	if err := validatePlanSemantics(planType, groupIDs, dailyLimit, weeklyLimit, monthlyLimit); err != nil {
		return nil, err
	}
	if err := s.validatePlanGroups(ctx, planType, groupIDs); err != nil {
		return nil, err
	}

	forSale := existing.ForSale
	if req.ForSale != nil {
		forSale = *req.ForSale
	}
	if planType == domain.SubscriptionPlanTypeLegacySharedSubscription && forSale {
		return nil, infraerrors.Conflict("PLAN_LEGACY_NOT_FOR_SALE", "legacy shared subscription plans cannot be put on sale")
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin update plan transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	u := tx.SubscriptionPlan.UpdateOneID(id).
		SetPlanType(planType).
		SetGroupID(groupIDs[0]).
		SetForSale(forSale)
	if req.Name != nil {
		u.SetName(*req.Name)
	}
	if req.Description != nil {
		u.SetDescription(*req.Description)
	}
	if req.Price != nil {
		u.SetPrice(*req.Price)
	}
	if req.OriginalPrice != nil {
		u.SetOriginalPrice(*req.OriginalPrice)
	}
	if req.Currency != nil {
		currency, err := normalizePlanCurrency(*req.Currency)
		if err != nil {
			return nil, err
		}
		u.SetCurrency(currency)
	}
	if req.ValidityDays != nil {
		u.SetValidityDays(*req.ValidityDays)
	}
	if req.ValidityUnit != nil {
		u.SetValidityUnit(*req.ValidityUnit)
	}
	if req.Features != nil {
		u.SetFeatures(*req.Features)
	}
	if req.ProductName != nil {
		u.SetProductName(*req.ProductName)
	}
	if req.SortOrder != nil {
		u.SetSortOrder(*req.SortOrder)
	}
	if dailyLimit == nil {
		u.ClearDailyLimitUsd()
	} else {
		u.SetDailyLimitUsd(*dailyLimit)
	}
	if weeklyLimit == nil {
		u.ClearWeeklyLimitUsd()
	} else {
		u.SetWeeklyLimitUsd(*weeklyLimit)
	}
	if monthlyLimit == nil {
		u.ClearMonthlyLimitUsd()
	} else {
		u.SetMonthlyLimitUsd(*monthlyLimit)
	}
	if _, err := u.Save(ctx); err != nil {
		return nil, err
	}
	if err := syncPlanGroups(ctx, tx.Client(), id, groupIDs); err != nil {
		return nil, fmt.Errorf("sync plan groups: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit update plan transaction: %w", err)
	}
	return s.GetPlan(ctx, id)
}

func (s *PaymentConfigService) DeletePlan(ctx context.Context, id int64) error {
	count, err := s.countPendingOrdersByPlan(ctx, id)
	if err != nil {
		return fmt.Errorf("check pending orders: %w", err)
	}
	if count > 0 {
		return infraerrors.Conflict("PENDING_ORDERS", fmt.Sprintf("this plan has %d in-progress orders and cannot be deleted — wait for orders to complete first", count))
	}
	return s.entClient.SubscriptionPlan.DeleteOneID(id).Exec(ctx)
}

func (s *PaymentConfigService) GetPlan(ctx context.Context, id int64) (*dbent.SubscriptionPlan, error) {
	plan, err := s.entClient.SubscriptionPlan.Get(ctx, id)
	if err != nil {
		return nil, infraerrors.NotFound("PLAN_NOT_FOUND", "subscription plan not found")
	}
	return plan, nil
}
