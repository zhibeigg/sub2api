package service

import "context"

func imagePriceConfigFromAPIKey(apiKey *APIKey) *ImagePriceConfig {
	if apiKey == nil || apiKey.Group == nil {
		return nil
	}
	return &ImagePriceConfig{
		Price1K: apiKey.Group.ImagePrice1K,
		Price2K: apiKey.Group.ImagePrice2K,
		Price4K: apiKey.Group.ImagePrice4K,
	}
}

func apiKeyHasImageBillingPrice(apiKey *APIKey) bool {
	return apiKey != nil && apiKey.Group != nil && apiKey.Group.HasImageBillingPrice()
}

func refreshAPIKeyGroupMediaPricing(ctx context.Context, apiKey *APIKey, groupRepo GroupRepository) *APIKey {
	if apiKey == nil || apiKey.GroupID == nil || *apiKey.GroupID <= 0 || groupRepo == nil {
		return apiKey
	}
	if !groupMediaPricingLooksIncomplete(apiKey.Group) {
		return apiKey
	}
	group, err := groupRepo.GetByIDLite(ctx, *apiKey.GroupID)
	if err != nil || group == nil {
		return apiKey
	}
	clone := *apiKey
	clone.Group = group
	return &clone
}

// groupMediaPricingLooksIncomplete 判断分组对象是否可能缺失媒体计费字段（例如由不含
// 这些字段的旧快照或手工构造的上下文对象生成）。image/video 独立倍率在数据库中的
// 默认值均为 1.0，正常加载的分组不可能两个倍率同时为 0 且未开启独立倍率、全部媒体
// 价为 nil——只有这种情况才回源查库，避免对未配置覆盖价的分组每条媒体用量都多打一次 DB 查询。
func groupMediaPricingLooksIncomplete(group *Group) bool {
	if group == nil {
		return true
	}
	if group.ImageRateIndependent || group.VideoRateIndependent {
		return false
	}
	if group.ImageRateMultiplier != 0 || group.VideoRateMultiplier != 0 {
		return false
	}
	return !group.HasImageBillingPrice() &&
		group.VideoPrice480P == nil && group.VideoPrice720P == nil && group.VideoPrice1080P == nil
}

func videoPriceConfigFromAPIKey(apiKey *APIKey) *VideoPriceConfig {
	if apiKey == nil || apiKey.Group == nil {
		return nil
	}
	return &VideoPriceConfig{
		Price480P:  apiKey.Group.VideoPrice480P,
		Price720P:  apiKey.Group.VideoPrice720P,
		Price1080P: apiKey.Group.VideoPrice1080P,
	}
}

func apiKeyHasConfiguredVideoPrice(apiKey *APIKey, resolution string) bool {
	return apiKey != nil && apiKey.Group != nil && apiKey.Group.GetVideoPrice(resolution) != nil
}

func webSearchPricePerCallFromAPIKey(apiKey *APIKey) *float64 {
	if apiKey == nil || apiKey.Group == nil {
		return nil
	}
	return apiKey.Group.WebSearchPricePerCall
}
