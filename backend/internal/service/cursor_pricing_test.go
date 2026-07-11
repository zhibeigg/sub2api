package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestCursorModelCatalogHasPricingForEveryExplicitModel(t *testing.T) {
	t.Parallel()

	require.Len(t, CursorModelCatalog, 40)
	for _, model := range CursorModelCatalog {
		pricing := cursorModelPricing(model)
		require.NotNilf(t, pricing, "missing Cursor pricing for %s", model)
	}
}

func TestCursorModelPricingUsesProvidedPerMillionTokenRates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		model                        string
		input, cacheWrite, cacheRead float64
		output                       float64
	}{
		{model: "claude-4-sonnet-1m", input: 6, cacheWrite: 7.5, cacheRead: 0.6, output: 22.5},
		{model: "claude-4.7-opus-fast", input: 30, cacheWrite: 37.5, cacheRead: 3, output: 150},
		{model: "composer-2.5", input: 0.5, cacheWrite: 0, cacheRead: 0.2, output: 2.5},
		{model: "gpt-5.4-mini", input: 0.75, cacheWrite: 0, cacheRead: 0.075, output: 4.5},
		{model: "gpt-5.6-terra", input: 2.5, cacheWrite: 3.125, cacheRead: 0.25, output: 15},
		{model: "kimi-k2.7-code", input: 0.95, cacheWrite: 0, cacheRead: 0.19, output: 4},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			pricing := cursorModelPricing(tt.model)
			require.NotNil(t, pricing)
			require.InDelta(t, tt.input*1e-6, pricing.InputPricePerToken, 1e-15)
			require.InDelta(t, tt.cacheWrite*1e-6, pricing.CacheCreationPricePerToken, 1e-15)
			require.InDelta(t, tt.cacheRead*1e-6, pricing.CacheReadPricePerToken, 1e-15)
			require.InDelta(t, tt.output*1e-6, pricing.OutputPricePerToken, 1e-15)
			require.True(t, pricing.CacheCreationPriceExplicit)
		})
	}
}

func TestCursorModelPricingRecognizesSyncedModelAliases(t *testing.T) {
	t.Parallel()

	require.Equal(t, cursorModelPricing("claude-4.6-opus"), cursorModelPricing("claude-4.6-opus-high-thinking"))
	require.Equal(t, cursorModelPricing("claude-4.8-opus"), cursorModelPricing("claude-4.8-opus-high-thinking"))
	require.Equal(t, cursorModelPricing("gpt-5.4"), cursorModelPricing("gpt-5.4-high"))
	require.Equal(t, cursorModelPricing("gpt-5.3-codex"), cursorModelPricing("cursor/gpt-5.3-codex-high"))
}

func TestCursorPlatformPricingOverridesGlobalDynamicPricing(t *testing.T) {
	t.Parallel()

	pricingService := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"gpt-5.5": {
			InputCostPerToken:     2.5e-6,
			OutputCostPerToken:    15e-6,
			LiteLLMProvider:       "openai",
			Mode:                  "chat",
			SupportsPromptCaching: true,
		},
	}}
	billingService := NewBillingService(&config.Config{}, pricingService)
	resolver := NewModelPricingResolver(nil, billingService)

	cursorResolved := resolver.Resolve(context.Background(), PricingInput{Model: "gpt-5.5", Platform: PlatformCursor})
	require.Equal(t, PricingSourcePlatform, cursorResolved.Source)
	require.InDelta(t, 5e-6, cursorResolved.BasePricing.InputPricePerToken, 1e-15)
	require.InDelta(t, 30e-6, cursorResolved.BasePricing.OutputPricePerToken, 1e-15)

	globalResolved := resolver.Resolve(context.Background(), PricingInput{Model: "gpt-5.5"})
	require.Equal(t, PricingSourceLiteLLM, globalResolved.Source)
	require.InDelta(t, 2.5e-6, globalResolved.BasePricing.InputPricePerToken, 1e-15)
	require.InDelta(t, 15e-6, globalResolved.BasePricing.OutputPricePerToken, 1e-15)
}

func TestCursorChannelPricingStillOverridesPlatformDefaults(t *testing.T) {
	t.Parallel()

	groupID := int64(901)
	inputPrice := 9e-6
	cache := newEmptyChannelCache()
	cache.pricingByGroupModel[channelModelKey{groupID: groupID, platform: PlatformCursor, model: "gpt-5.5"}] = &ChannelModelPricing{
		Platform:    PlatformCursor,
		Models:      []string{"gpt-5.5"},
		BillingMode: BillingModeToken,
		InputPrice:  &inputPrice,
	}
	cache.channelByGroupID[groupID] = &Channel{ID: 902, Status: StatusActive}
	cache.groupPlatform[groupID] = PlatformCursor
	cache.loadedAt = time.Now()
	channelService := &ChannelService{}
	channelService.cache.Store(cache)

	billingService := NewBillingService(&config.Config{}, nil)
	resolver := NewModelPricingResolver(channelService, billingService)
	resolved := resolver.Resolve(context.Background(), PricingInput{
		Model:    "gpt-5.5",
		Platform: PlatformCursor,
		GroupID:  &groupID,
	})

	require.Equal(t, PricingSourceChannel, resolved.Source)
	require.InDelta(t, inputPrice, resolved.BasePricing.InputPricePerToken, 1e-15)
	require.InDelta(t, 30e-6, resolved.BasePricing.OutputPricePerToken, 1e-15)
}
