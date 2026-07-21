//go:build unit

package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUserAvailableChannel_Unauthenticated401(t *testing.T) {
	// 没有 AuthSubject 注入时，handler 应返回 401 且不触达 service 依赖。
	gin.SetMode(gin.TestMode)
	h := &AvailableChannelHandler{} // nil services — 401 路径不会调用它们
	for _, test := range []struct {
		path   string
		handle func(*gin.Context)
	}{
		{path: "/api/v1/channels/available", handle: h.List},
		{path: "/api/v1/models/available", handle: h.ListModelSquare},
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, test.path, nil)

		test.handle(c)

		require.Equal(t, http.StatusUnauthorized, w.Code)
	}
}

func TestAvailableChannelHandler_FeatureGatesAreIndependent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := service.NewSettingService(&settingHandlerPublicRepoStub{values: map[string]string{
		service.SettingKeyAvailableChannelsEnabled: "true",
		service.SettingKeyModelSquareEnabled:       "false",
	}}, &config.Config{})
	h := &AvailableChannelHandler{settingService: svc}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	require.True(t, h.availableChannelsEnabled(c))
	require.False(t, h.modelSquareEnabled(c))

	svc = service.NewSettingService(&settingHandlerPublicRepoStub{values: map[string]string{
		service.SettingKeyAvailableChannelsEnabled: "false",
		service.SettingKeyModelSquareEnabled:       "true",
	}}, &config.Config{})
	h.settingService = svc
	require.False(t, h.availableChannelsEnabled(c))
	require.True(t, h.modelSquareEnabled(c))
}

func TestAvailableChannelHandler_ClosedGateReturnsEmptyAfterAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name   string
		path   string
		values map[string]string
		handle func(*AvailableChannelHandler, *gin.Context)
	}{
		{
			name: "available channels ignores enabled model square",
			path: "/api/v1/channels/available",
			values: map[string]string{
				service.SettingKeyAvailableChannelsEnabled: "false",
				service.SettingKeyModelSquareEnabled:       "true",
			},
			handle: func(h *AvailableChannelHandler, c *gin.Context) { h.List(c) },
		},
		{
			name: "model square ignores enabled available channels",
			path: "/api/v1/models/available",
			values: map[string]string{
				service.SettingKeyAvailableChannelsEnabled: "true",
				service.SettingKeyModelSquareEnabled:       "false",
			},
			handle: func(h *AvailableChannelHandler, c *gin.Context) { h.ListModelSquare(c) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &AvailableChannelHandler{settingService: service.NewSettingService(
				&settingHandlerPublicRepoStub{values: tt.values},
				&config.Config{},
			)}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, tt.path, nil)
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1})

			tt.handle(h, c)

			require.Equal(t, http.StatusOK, w.Code)
			var resp struct {
				Code int   `json:"code"`
				Data []any `json:"data"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, 0, resp.Code)
			require.Empty(t, resp.Data)
		})
	}
}

func TestFilterUserVisibleGroups_IntersectionOnly(t *testing.T) {
	// 渠道挂在 {g1, g2, g3}，用户只允许 {g1, g3} —— 响应必须仅含 g1/g3。
	imagePrice1K := 0.12
	groups := []service.AvailableGroupRef{
		{ID: 1, Name: "g1", Platform: "anthropic", AllowImageGeneration: true, AllowMessagesDispatch: true, ImageBillingEnabled: true, ImageRateIndependent: true, ImageRateMultiplier: 0.8, ImagePrice1K: &imagePrice1K},
		{ID: 2, Name: "g2", Platform: "anthropic"},
		{ID: 3, Name: "g3", Platform: "openai"},
	}
	allowed := map[int64]struct{}{1: {}, 3: {}}

	visible := filterUserVisibleGroups(groups, allowed)
	require.Len(t, visible, 2)
	ids := []int64{visible[0].ID, visible[1].ID}
	require.ElementsMatch(t, []int64{1, 3}, ids)
	require.True(t, visible[0].AllowImageGeneration)
	require.True(t, visible[0].AllowMessagesDispatch)
	require.True(t, visible[0].ImageBillingEnabled)
	require.True(t, visible[0].ImageRateIndependent)
	require.InDelta(t, 0.8, visible[0].ImageRateMultiplier, 1e-12)
	require.Same(t, &imagePrice1K, visible[0].ImagePrice1K)
}

func TestToUserSupportedModels_FiltersByAllowedPlatforms(t *testing.T) {
	// 用户可访问分组只覆盖 anthropic；anthropic 平台的模型保留，openai 模型被剔除。
	src := []service.SupportedModel{
		{Name: "claude-sonnet-4-6", Platform: "anthropic", Pricing: nil},
		{Name: "gpt-4o", Platform: "openai", Pricing: nil},
	}
	allowed := map[string]struct{}{"anthropic": {}}
	out := toUserSupportedModels(src, allowed)
	require.Len(t, out, 1)
	require.Equal(t, "claude-sonnet-4-6", out[0].Name)
	require.Empty(t, out[0].MediaType)
}

func TestToUserSupportedModels_MapsMediaType(t *testing.T) {
	src := []service.SupportedModel{
		{Name: "gpt-image-2", Platform: service.PlatformOpenAI},
		{Name: "grok-imagine-video-1.5", Platform: service.PlatformGrok},
		{Name: "gpt-5.4", Platform: service.PlatformOpenAI},
	}

	out := toUserSupportedModels(src, nil)
	require.Len(t, out, 3)
	require.Equal(t, "image", out[0].MediaType)
	require.Equal(t, "video", out[1].MediaType)
	require.Empty(t, out[2].MediaType)
}

func TestToUserSupportedModels_UsesBillingModelForMediaType(t *testing.T) {
	out := toUserSupportedModels([]service.SupportedModel{{
		Name:         "public-image-alias",
		BillingModel: "gpt-image-2",
		Platform:     service.PlatformOpenAI,
	}}, nil)

	require.Len(t, out, 1)
	require.Equal(t, "public-image-alias", out[0].Name)
	require.Equal(t, "image", out[0].MediaType)
}

func TestToUserSupportedModels_NilAllowedPlatformsKeepsAll(t *testing.T) {
	// 显式传 nil allowedPlatforms 表示不做过滤。
	src := []service.SupportedModel{
		{Name: "a", Platform: "anthropic"},
		{Name: "b", Platform: "openai"},
	}
	require.Len(t, toUserSupportedModels(src, nil), 2)
}

func TestUserAvailableChannel_FieldWhitelist(t *testing.T) {
	// 通过序列化 userAvailableChannel 结构体验证响应形状：
	// 只有 name / description / platforms；不含管理端字段。
	imagePrice2K := 0.23
	videoPrice720P := 0.12
	row := userAvailableChannel{
		Name:        "ch",
		Description: "d",
		Platforms: []userChannelPlatformSection{
			{
				Platform: "anthropic",
				Groups: []userAvailableGroup{{
					ID: 1, Name: "g1", Platform: "anthropic",
					AllowImageGeneration:  true,
					AllowMessagesDispatch: true,
					ImageBillingEnabled:   true,
					ImageRateIndependent:  true,
					ImageRateMultiplier:   0.75,
					ImagePrice2K:          &imagePrice2K,
					VideoBillingEnabled:   true,
					VideoRateIndependent:  true,
					VideoRateMultiplier:   0.4,
					VideoPrice720P:        &videoPrice720P,
				}},
				SupportedModels: []userSupportedModel{{
					Name: "claude-sonnet-4-6", Platform: "anthropic",
					GroupRates: []userSupportedModelGroupRate{{
						GroupID: 1, TokenRateMultiplier: 0.9, ImageRateMultiplier: 0.75, VideoRateMultiplier: 0.4,
					}},
				}},
			},
		},
	}
	raw, err := json.Marshal(row)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))

	for _, key := range []string{"id", "status", "billing_model_source", "restrict_models"} {
		_, exists := decoded[key]
		require.Falsef(t, exists, "user DTO must not expose %q", key)
	}
	for _, key := range []string{"name", "description", "platforms"} {
		_, exists := decoded[key]
		require.Truef(t, exists, "user DTO must expose %q", key)
	}

	// 验证 section 的字段（platform / groups / supported_models）。
	rawSection, err := json.Marshal(row.Platforms[0])
	require.NoError(t, err)
	var sectionDecoded map[string]any
	require.NoError(t, json.Unmarshal(rawSection, &sectionDecoded))
	for _, key := range []string{"platform", "groups", "supported_models"} {
		_, exists := sectionDecoded[key]
		require.Truef(t, exists, "platform section must expose %q", key)
	}

	// Group DTO 暴露区分专属/公开、订阅类型、默认倍率和高峰倍率规则所需的字段，
	// 前端据此渲染 GroupBadge 并与 API 密钥页保持一致的视觉。
	rawGroup, err := json.Marshal(row.Platforms[0].Groups[0])
	require.NoError(t, err)
	var groupDecoded map[string]any
	require.NoError(t, json.Unmarshal(rawGroup, &groupDecoded))
	for _, key := range []string{
		"id", "name", "platform", "subscription_type", "rate_multiplier",
		"peak_rate_enabled", "peak_start", "peak_end", "peak_rate_multiplier", "is_exclusive",
		"allow_image_generation", "allow_video_generation", "allow_messages_dispatch", "image_billing_enabled", "image_rate_independent",
		"image_rate_multiplier", "image_price_1k", "image_price_2k", "image_price_4k",
		"video_billing_enabled", "video_rate_independent", "video_rate_multiplier",
		"video_price_480p", "video_price_720p", "video_price_1080p",
	} {
		_, exists := groupDecoded[key]
		require.Truef(t, exists, "group DTO must expose %q", key)
	}
	require.Equal(t, true, groupDecoded["allow_messages_dispatch"])

	rawModel, err := json.Marshal(row.Platforms[0].SupportedModels[0])
	require.NoError(t, err)
	var modelDecoded map[string]any
	require.NoError(t, json.Unmarshal(rawModel, &modelDecoded))
	for _, key := range []string{
		"name", "platform", "media_type", "pricing",
		"default_video_price_480p", "default_video_price_720p", "default_video_price_1080p", "group_rates",
	} {
		_, exists := modelDecoded[key]
		require.Truef(t, exists, "supported model DTO must expose %q", key)
	}
	groupRates, ok := modelDecoded["group_rates"].([]any)
	require.True(t, ok)
	require.Len(t, groupRates, 1)
	rateDecoded, ok := groupRates[0].(map[string]any)
	require.True(t, ok)
	for _, key := range []string{"group_id", "token_rate_multiplier", "image_rate_multiplier", "video_rate_multiplier"} {
		_, exists := rateDecoded[key]
		require.Truef(t, exists, "model group rate DTO must expose %q", key)
	}

	// pricing interval 白名单：不应暴露 id / sort_order。
	pricing := toUserPricing(&service.ChannelModelPricing{
		BillingMode: service.BillingModeToken,
		Intervals: []service.PricingInterval{
			{ID: 7, MinTokens: 0, MaxTokens: nil, SortOrder: 3},
		},
	})
	require.NotNil(t, pricing)
	require.Len(t, pricing.Intervals, 1)
	rawIv, err := json.Marshal(pricing.Intervals[0])
	require.NoError(t, err)
	var ivDecoded map[string]any
	require.NoError(t, json.Unmarshal(rawIv, &ivDecoded))
	for _, key := range []string{"id", "pricing_id", "sort_order"} {
		_, exists := ivDecoded[key]
		require.Falsef(t, exists, "user pricing interval must not expose %q", key)
	}
}

func TestBuildPlatformSections_GroupsByPlatform(t *testing.T) {
	// 一个渠道横跨 anthropic / openai / 空平台：应该生成 2 个 section，
	// 按 platform 字母序排序，各自 groups 和 supported_models 只含同平台条目。
	ch := service.AvailableChannel{
		Name: "ch",
		SupportedModels: []service.SupportedModel{
			{Name: "claude-sonnet-4-6", Platform: "anthropic"},
			{Name: "gpt-4o", Platform: "openai"},
		},
	}
	visible := []userAvailableGroup{
		{ID: 1, Name: "g-openai", Platform: "openai"},
		{ID: 2, Name: "g-ant", Platform: "anthropic"},
		{ID: 3, Name: "g-empty", Platform: ""},
	}
	sections := buildPlatformSections(ch, visible)
	require.Len(t, sections, 2)
	require.Equal(t, "anthropic", sections[0].Platform)
	require.Equal(t, "openai", sections[1].Platform)
	require.Len(t, sections[0].Groups, 1)
	require.Equal(t, int64(2), sections[0].Groups[0].ID)
	require.Len(t, sections[0].SupportedModels, 1)
	require.Equal(t, "claude-sonnet-4-6", sections[0].SupportedModels[0].Name)
}

func TestBuildPlatformSectionsWithRates_UsesEffectiveModelAndMediaMultipliers(t *testing.T) {
	ch := service.AvailableChannel{
		SupportedModels: []service.SupportedModel{{Name: "kimi-k3", Platform: service.PlatformOpenCode}},
	}
	visible := []userAvailableGroup{{ID: 7, Name: "Kimi", Platform: service.PlatformOpenCode}}
	groups := map[int64]service.Group{7: {
		ID:                   7,
		Platform:             service.PlatformOpenCode,
		RateMultiplier:       1.2,
		ModelRateMultipliers: map[string]float64{"kimi-*": 0.8},
		ImageRateIndependent: true,
		ImageRateMultiplier:  0.5,
		VideoRateIndependent: true,
		VideoRateMultiplier:  0.25,
	}}
	userRates := map[int64]float64{7: 0.6}

	sections := buildPlatformSectionsWithRates(ch, visible, groups, userRates, time.Time{})
	require.Len(t, sections, 1)
	require.Len(t, sections[0].SupportedModels, 1)
	require.Len(t, sections[0].SupportedModels[0].GroupRates, 1)
	rate := sections[0].SupportedModels[0].GroupRates[0]
	require.InDelta(t, 0.6, rate.TokenRateMultiplier, 1e-12)
	require.InDelta(t, 0.5, rate.ImageRateMultiplier, 1e-12)
	require.InDelta(t, 0.25, rate.VideoRateMultiplier, 1e-12)
}

func TestBuildPlatformSectionsWithRates_UsesBillingModelForModelMultiplier(t *testing.T) {
	ch := service.AvailableChannel{
		SupportedModels: []service.SupportedModel{{
			Name:         "public-alias",
			BillingModel: "served-model",
			Platform:     service.PlatformOpenCode,
		}},
	}
	visible := []userAvailableGroup{{ID: 9, Name: "Alias", Platform: service.PlatformOpenCode}}
	groups := map[int64]service.Group{9: {
		ID:                   9,
		Platform:             service.PlatformOpenCode,
		RateMultiplier:       1.2,
		ModelRateMultipliers: map[string]float64{"served-*": 0.75},
	}}

	sections := buildPlatformSectionsWithRates(ch, visible, groups, nil, time.Time{})

	require.Len(t, sections, 1)
	require.Len(t, sections[0].SupportedModels, 1)
	require.Len(t, sections[0].SupportedModels[0].GroupRates, 1)
	require.InDelta(t, 0.75, sections[0].SupportedModels[0].GroupRates[0].TokenRateMultiplier, 1e-12)
}
