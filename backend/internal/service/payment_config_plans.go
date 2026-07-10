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
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

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
	GroupID         int64           `json:"group_id"`
	GroupIDs        []int64         `json:"group_ids"`
	Groups          []PlanGroupInfo `json:"groups"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	Price           float64         `json:"price"`
	OriginalPrice   *float64        `json:"original_price,omitempty"`
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
			ID: plan.ID, GroupID: plan.GroupID, GroupIDs: groupIDs, Groups: groups,
			Name: plan.Name, Description: plan.Description, Price: plan.Price, OriginalPrice: plan.OriginalPrice,
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
	return s.entClient.SubscriptionPlan.Query().Where(subscriptionplan.ForSaleEQ(true)).Order(subscriptionplan.BySortOrder()).All(ctx)
}

func (s *PaymentConfigService) validatePlanGroups(ctx context.Context, groupIDs []int64) error {
	if len(groupIDs) == 0 {
		return infraerrors.BadRequest("PLAN_GROUP_REQUIRED", "at least one group is required")
	}
	count, err := s.entClient.Group.Query().Where(
		group.IDIn(groupIDs...),
		group.StatusEQ(domain.StatusActive),
		group.SubscriptionTypeEQ(domain.SubscriptionTypeSubscription),
	).Count(ctx)
	if err != nil {
		return fmt.Errorf("validate plan groups: %w", err)
	}
	if count != len(groupIDs) {
		return infraerrors.BadRequest("PLAN_GROUP_INVALID", "all groups must exist, be active, and use subscription billing")
	}
	return nil
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
	groupIDs := normalizePlanGroupIDs(req.GroupIDs, req.GroupID)
	if err := validatePlanRequired(req.Name, groupIDs, req.Price, req.ValidityDays, req.ValidityUnit, req.OriginalPrice); err != nil {
		return nil, err
	}
	if err := validatePlanQuotaLimits(req.DailyLimitUSD, req.WeeklyLimitUSD, req.MonthlyLimitUSD); err != nil {
		return nil, err
	}
	if err := s.validatePlanGroups(ctx, groupIDs); err != nil {
		return nil, err
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin create plan transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	b := tx.SubscriptionPlan.Create().
		SetGroupID(groupIDs[0]).SetName(req.Name).SetDescription(req.Description).
		SetPrice(req.Price).SetValidityDays(req.ValidityDays).SetValidityUnit(req.ValidityUnit).
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
	var groupIDs []int64
	if req.GroupIDs != nil {
		groupIDs = normalizePlanGroupIDs(req.GroupIDs, 0)
	} else if req.GroupID != nil {
		groupIDs = normalizePlanGroupIDs(nil, *req.GroupID)
	}
	if groupIDs != nil {
		if err := s.validatePlanGroups(ctx, groupIDs); err != nil {
			return nil, err
		}
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin update plan transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	u := tx.SubscriptionPlan.UpdateOneID(id)
	if groupIDs != nil {
		u.SetGroupID(groupIDs[0])
	}
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
	if req.ForSale != nil {
		u.SetForSale(*req.ForSale)
	}
	if req.SortOrder != nil {
		u.SetSortOrder(*req.SortOrder)
	}
	if req.QuotaLimitsSet || req.DailyLimitUSD != nil || req.WeeklyLimitUSD != nil || req.MonthlyLimitUSD != nil {
		if req.DailyLimitUSD == nil {
			u.ClearDailyLimitUsd()
		} else {
			u.SetDailyLimitUsd(*req.DailyLimitUSD)
		}
		if req.WeeklyLimitUSD == nil {
			u.ClearWeeklyLimitUsd()
		} else {
			u.SetWeeklyLimitUsd(*req.WeeklyLimitUSD)
		}
		if req.MonthlyLimitUSD == nil {
			u.ClearMonthlyLimitUsd()
		} else {
			u.SetMonthlyLimitUsd(*req.MonthlyLimitUSD)
		}
	}
	if _, err := u.Save(ctx); err != nil {
		return nil, err
	}
	if groupIDs != nil {
		if err := syncPlanGroups(ctx, tx.Client(), id, groupIDs); err != nil {
			return nil, fmt.Errorf("sync plan groups: %w", err)
		}
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
