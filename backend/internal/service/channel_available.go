package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// AvailableGroupRef 渠道视图中关联分组的简要信息。
//
// 用户侧「可用渠道」页面据此展示：专属分组 vs 公开分组（IsExclusive）、
// 订阅 vs 标准（SubscriptionType）、默认倍率（RateMultiplier）与高峰倍率规则。
// 用户专属、模型级、高峰和媒体独立倍率由 handler 按模型生成 group_rates 快照。
type AvailableGroupRef struct {
	ID                    int64
	Name                  string
	Platform              string
	SubscriptionType      string
	RateMultiplier        float64
	PeakRateEnabled       bool
	PeakStart             string
	PeakEnd               string
	PeakRateMultiplier    float64
	IsExclusive           bool
	AllowImageGeneration  bool
	AllowVideoGeneration  bool
	AllowMessagesDispatch bool
	ImageBillingEnabled   bool
	ImageRateIndependent  bool
	ImageRateMultiplier   float64
	ImagePrice1K          *float64
	ImagePrice2K          *float64
	ImagePrice4K          *float64
	VideoBillingEnabled   bool
	VideoRateIndependent  bool
	VideoRateMultiplier   float64
	VideoPrice480P        *float64
	VideoPrice720P        *float64
	VideoPrice1080P       *float64
}

// AvailableChannel 可用渠道视图：用于「可用渠道」页面展示渠道基础信息 +
// 关联的分组 + 推导出的支持模型列表（无通配符）。
type AvailableChannel struct {
	ID                 int64
	Name               string
	Description        string
	Status             string
	BillingModelSource string
	RestrictModels     bool
	Groups             []AvailableGroupRef
	SupportedModels    []SupportedModel
}

// ListAvailable 返回所有渠道的可用视图：每个渠道附带关联分组信息与支持模型列表。
//
// 支持模型通过 (*Channel).SupportedModels() 计算（mapping ∪ pricing 并联）。
// 对于渠道未配置定价的模型，Cursor 先用平台专属目录，其它平台再用 PricingService
// 的全局 LiteLLM 数据合成展示用定价，让用户看到默认价格而非"未配置"。
//
// 关联分组信息通过 groupRepo.ListActive 查询后按 ID 映射；渠道 GroupIDs 中未在活跃列表中
// 的分组（已停用或删除）会被忽略。
//
// 前置条件：s.groupRepo 必须非 nil（由 wire DI 保证）。直接 nil-deref 用于 fail-fast，
// 避免静默掩盖注入缺失。
func (s *ChannelService) ListAvailable(ctx context.Context) ([]AvailableChannel, error) {
	channels, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}

	groups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active groups: %w", err)
	}
	groupByID := make(map[int64]AvailableGroupRef, len(groups))
	for i := range groups {
		g := groups[i]
		groupByID[g.ID] = AvailableGroupRef{
			ID:                   g.ID,
			Name:                 g.Name,
			Platform:             g.Platform,
			SubscriptionType:     g.SubscriptionType,
			RateMultiplier:       g.RateMultiplier,
			PeakRateEnabled:      g.PeakRateEnabled,
			PeakStart:            g.PeakStart,
			PeakEnd:              g.PeakEnd,
			PeakRateMultiplier:   g.PeakRateMultiplier,
			IsExclusive:          g.IsExclusive,
			AllowImageGeneration: g.AllowImageGeneration,
			AllowVideoGeneration: GroupAllowsVideoGeneration(&g) &&
				(NormalizePlatform(g.Platform) != PlatformAdobe || g.VideoPrice720P != nil || g.VideoPrice1080P != nil),
			AllowMessagesDispatch: g.AllowMessagesDispatch,
			ImageBillingEnabled:   g.HasImageBillingPrice(),
			ImageRateIndependent:  g.ImageRateIndependent,
			ImageRateMultiplier:   g.ImageRateMultiplier,
			ImagePrice1K:          g.ImagePrice1K,
			ImagePrice2K:          g.ImagePrice2K,
			ImagePrice4K:          g.ImagePrice4K,
			VideoBillingEnabled:   g.VideoPrice480P != nil || g.VideoPrice720P != nil || g.VideoPrice1080P != nil,
			VideoRateIndependent:  g.VideoRateIndependent,
			VideoRateMultiplier:   g.VideoRateMultiplier,
			VideoPrice480P:        g.VideoPrice480P,
			VideoPrice720P:        g.VideoPrice720P,
			VideoPrice1080P:       g.VideoPrice1080P,
		}
	}

	out := make([]AvailableChannel, 0, len(channels))
	for i := range channels {
		ch := &channels[i]
		groups := make([]AvailableGroupRef, 0, len(ch.GroupIDs))
		for _, gid := range ch.GroupIDs {
			if ref, ok := groupByID[gid]; ok {
				groups = append(groups, ref)
			}
		}
		sort.SliceStable(groups, func(i, j int) bool { return groups[i].Name < groups[j].Name })

		ch.normalizeBillingModelSource()

		supported := ch.SupportedModels()
		s.fillGlobalPricingFallback(supported)
		s.fillDefaultVideoPricing(supported)

		out = append(out, AvailableChannel{
			ID:                 ch.ID,
			Name:               ch.Name,
			Description:        ch.Description,
			Status:             ch.Status,
			BillingModelSource: ch.BillingModelSource,
			RestrictModels:     ch.RestrictModels,
			Groups:             groups,
			SupportedModels:    supported,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// fillGlobalPricingFallback 对未命中渠道定价的支持模型，从全局 LiteLLM 数据合成一份
// 展示用定价。仅用于「可用渠道」展示，不影响真实计费链路。
//
// 触发条件：
//  1. Pricing == nil（渠道完全没声明该模型的定价条目）
//  2. Pricing 非 nil 但所有价格字段为空（admin UI 建了条目但没填价格）
//
// Cursor 平台优先使用内置平台价格；其它平台在 s.pricingService 为 nil 时跳过回落。
func (s *ChannelService) fillGlobalPricingFallback(models []SupportedModel) {
	for i := range models {
		if !pricingNeedsFallback(models[i].Pricing) {
			continue
		}
		billingModel := strings.TrimSpace(models[i].BillingModel)
		if billingModel == "" {
			billingModel = models[i].Name
		}
		if strings.EqualFold(models[i].Platform, PlatformCursor) {
			if pricing := cursorModelPricing(billingModel); pricing != nil {
				models[i].Pricing = synthesizePricingFromModelPricing(pricing, models[i].Pricing)
				continue
			}
		}
		if s.pricingService == nil {
			continue
		}
		lp := s.pricingService.GetModelPricing(billingModel)
		if lp == nil {
			continue
		}
		models[i].Pricing = synthesizePricingFromLiteLLM(lp, models[i].Pricing)
	}
}

// fillDefaultVideoPricing exposes the same per-second fallback rate card used by
// OpenAI/Grok settlement when a group has not configured a resolution price.
// Adobe is excluded because Firefly settlement requires an explicit group tier
// (and may then replace it with a channel interval price).
func (s *ChannelService) fillDefaultVideoPricing(models []SupportedModel) {
	billing := &BillingService{pricingService: s.pricingService}
	for i := range models {
		platform := NormalizePlatform(models[i].Platform)
		if platform == PlatformAdobe {
			continue
		}
		billingModel := strings.TrimSpace(models[i].BillingModel)
		if billingModel == "" {
			billingModel = strings.TrimSpace(models[i].Name)
		}
		if ModelMediaType(billingModel) != PlaygroundCapabilityVideo {
			continue
		}
		price480P := billing.getDefaultVideoPrice(billingModel, VideoBillingResolution480P)
		price720P := billing.getDefaultVideoPrice(billingModel, VideoBillingResolution720P)
		price1080P := billing.getDefaultVideoPrice(billingModel, VideoBillingResolution1080P)
		models[i].DefaultVideoPrice480P = &price480P
		models[i].DefaultVideoPrice720P = &price720P
		models[i].DefaultVideoPrice1080P = &price1080P
	}
}

// pricingNeedsFallback 判定一个 ChannelModelPricing 是否需要走全局回落。
// 价格全部缺失（无 flat 字段且无任何带价 interval）即视为未配置。
func pricingNeedsFallback(p *ChannelModelPricing) bool {
	if p == nil {
		return true
	}
	if p.InputPrice != nil || p.OutputPrice != nil ||
		p.CacheWritePrice != nil || p.CacheReadPrice != nil ||
		p.ImageInputPrice != nil || p.ImageOutputPrice != nil || p.PerRequestPrice != nil {
		return false
	}
	for _, iv := range p.Intervals {
		if iv.InputPrice != nil || iv.OutputPrice != nil ||
			iv.CacheWritePrice != nil || iv.CacheReadPrice != nil ||
			iv.PerRequestPrice != nil {
			return false
		}
	}
	return true
}

// synthesizePricingFromLiteLLM 把 LiteLLM 的定价数据转成 ChannelModelPricing 形态，
// 仅用于展示。
//
// 计费模式优先级：
//  1. 渠道已选 BillingMode（admin 在 UI 里选了 image / per_request 但没填价的场景，
//     按选定模式合成对应字段）
//  2. LiteLLM mode="image_generation" → image
//  3. 默认 token
//
// LiteLLM 中字段 0 视为未配置，不带入展示。
func synthesizePricingFromLiteLLM(lp *LiteLLMModelPricing, existing *ChannelModelPricing) *ChannelModelPricing {
	if lp == nil {
		return existing
	}

	mode := BillingModeToken
	switch {
	case existing != nil && existing.BillingMode != "":
		mode = existing.BillingMode
	case lp.Mode == "image_generation":
		mode = BillingModeImage
	}

	if mode == BillingModeImage || mode == BillingModePerRequest {
		return &ChannelModelPricing{
			BillingMode:      mode,
			PerRequestPrice:  nonZeroPtr(lp.OutputCostPerImage),
			ImageOutputPrice: nonZeroPtr(lp.OutputCostPerImageToken),
			InputPrice:       nonZeroPtr(lp.InputCostPerToken),
			OutputPrice:      nonZeroPtr(lp.OutputCostPerToken),
		}
	}
	return &ChannelModelPricing{
		BillingMode:      mode,
		InputPrice:       nonZeroPtr(lp.InputCostPerToken),
		OutputPrice:      nonZeroPtr(lp.OutputCostPerToken),
		CacheWritePrice:  nonZeroPtr(lp.CacheCreationInputTokenCost),
		CacheReadPrice:   nonZeroPtr(lp.CacheReadInputTokenCost),
		ImageOutputPrice: nonZeroPtr(lp.OutputCostPerImageToken),
	}
}

func synthesizePricingFromModelPricing(pricing *ModelPricing, existing *ChannelModelPricing) *ChannelModelPricing {
	if pricing == nil {
		return existing
	}
	mode := BillingModeToken
	if existing != nil && existing.BillingMode != "" {
		mode = existing.BillingMode
	}
	return &ChannelModelPricing{
		BillingMode:      mode,
		InputPrice:       nonZeroPtr(pricing.InputPricePerToken),
		OutputPrice:      nonZeroPtr(pricing.OutputPricePerToken),
		CacheWritePrice:  nonZeroPtr(pricing.CacheCreationPricePerToken),
		CacheReadPrice:   nonZeroPtr(pricing.CacheReadPricePerToken),
		ImageOutputPrice: nonZeroPtr(pricing.ImageOutputPricePerToken),
	}
}

func nonZeroPtr(v float64) *float64 {
	if v == 0 {
		return nil
	}
	return &v
}
