package service

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// CursorModelCatalog 是 Cursor Cloud Agents 可选模型目录。
// 价格单位均为 USD/token；展示层按 USD/百万 token 换算。
// 列顺序对应 Cursor 价格页：输入、缓存写入、缓存读取、输出。
var CursorModelCatalog = domain.CursorSupportedModels

var cursorModelPrices = map[string]*ModelPricing{
	"claude-4-sonnet":            cursorTokenPrice(3, 3.75, 0.3, 15),
	"claude-4-sonnet-1m":         cursorTokenPrice(6, 7.5, 0.6, 22.5),
	"claude-4.5-haiku":           cursorTokenPrice(1, 1.25, 0.1, 5),
	"claude-4.5-opus":            cursorTokenPrice(5, 6.25, 0.5, 25),
	"claude-4.5-sonnet":          cursorTokenPrice(3, 3.75, 0.3, 15),
	"claude-4.6-opus":            cursorTokenPrice(5, 6.25, 0.5, 25),
	"claude-4.6-sonnet":          cursorTokenPrice(3, 3.75, 0.3, 15),
	"claude-4.7-opus":            cursorTokenPrice(5, 6.25, 0.5, 25),
	"claude-fable-5":             cursorTokenPrice(10, 12.5, 1, 50),
	"claude-4.7-opus-fast":       cursorTokenPrice(30, 37.5, 3, 150),
	"claude-4.8-opus":            cursorTokenPrice(5, 6.25, 0.5, 25),
	"claude-sonnet-5":            cursorTokenPrice(3, 3.75, 0.3, 15),
	"composer-1":                 cursorTokenPrice(1.25, 0, 0.125, 10),
	"composer-2.5":               cursorTokenPrice(0.5, 0, 0.2, 2.5),
	"gemini-2.5-flash":           cursorTokenPrice(0.3, 0, 0.03, 2.5),
	"gemini-3-flash":             cursorTokenPrice(0.5, 0, 0.05, 3),
	"gemini-3-pro":               cursorTokenPrice(2, 0, 0.2, 12),
	"gemini-3-pro-image-preview": cursorTokenPrice(2, 0, 0.2, 12),
	"gemini-3.1-pro":             cursorTokenPrice(2, 0, 0.2, 12),
	"gemini-3.5-flash":           cursorTokenPrice(1.5, 0, 0.15, 9),
	"glm-5.2":                    cursorTokenPrice(1.4, 0, 0.26, 4.4),
	"gpt-5":                      cursorTokenPrice(1.25, 0, 0.125, 10),
	"gpt-5-fast":                 cursorTokenPrice(2.5, 0, 0.25, 20),
	"gpt-5-mini":                 cursorTokenPrice(0.25, 0, 0.025, 2),
	"gpt-5-codex":                cursorTokenPrice(1.25, 0, 0.125, 10),
	"gpt-5.1-codex":              cursorTokenPrice(1.25, 0, 0.125, 10),
	"gpt-5.1-codex-max":          cursorTokenPrice(1.25, 0, 0.125, 10),
	"gpt-5.1-codex-mini":         cursorTokenPrice(0.25, 0, 0.025, 2),
	"gpt-5.2":                    cursorTokenPrice(1.75, 0, 0.175, 14),
	"gpt-5.2-codex":              cursorTokenPrice(1.75, 0, 0.175, 14),
	"gpt-5.3-codex":              cursorTokenPrice(1.75, 0, 0.175, 14),
	"gpt-5.4":                    cursorTokenPrice(2.5, 0, 0.25, 15),
	"gpt-5.4-mini":               cursorTokenPrice(0.75, 0, 0.075, 4.5),
	"gpt-5.4-nano":               cursorTokenPrice(0.2, 0, 0.02, 1.25),
	"gpt-5.5":                    cursorTokenPrice(5, 0, 0.5, 30),
	"gpt-5.6-luna":               cursorTokenPrice(1, 1.25, 0.1, 6),
	"gpt-5.6-sol":                cursorTokenPrice(5, 6.25, 0.5, 30),
	"gpt-5.6-terra":              cursorTokenPrice(2.5, 3.125, 0.25, 15),
	"grok-4.5":                   cursorTokenPrice(2, 0, 0.5, 6),
	"kimi-k2.7-code":             cursorTokenPrice(0.95, 0, 0.19, 4),
}

var cursorModelPriceAliases = map[string]string{
	"claude-sonnet-4":               "claude-4-sonnet",
	"claude-sonnet-4-1m":            "claude-4-sonnet-1m",
	"claude-haiku-4-5":              "claude-4.5-haiku",
	"claude-opus-4-5":               "claude-4.5-opus",
	"claude-sonnet-4-5":             "claude-4.5-sonnet",
	"claude-opus-4-6":               "claude-4.6-opus",
	"claude-sonnet-4-6":             "claude-4.6-sonnet",
	"claude-opus-4-7":               "claude-4.7-opus",
	"claude-opus-4-7-fast":          "claude-4.7-opus-fast",
	"claude-opus-4-7-fast-mode":     "claude-4.7-opus-fast",
	"claude-opus-4-8":               "claude-4.8-opus",
	"gpt-5.1":                       "gpt-5",
	"claude-4.5-haiku-thinking":     "claude-4.5-haiku",
	"claude-4.5-opus-thinking":      "claude-4.5-opus",
	"claude-4.5-sonnet-thinking":    "claude-4.5-sonnet",
	"claude-4.6-opus-high-thinking": "claude-4.6-opus",
	"claude-4.6-sonnet-thinking":    "claude-4.6-sonnet",
	"claude-4.7-opus-thinking":      "claude-4.7-opus",
	"gpt-5.1-codex-high":            "gpt-5.1-codex",
	"gpt-5.1-codex-max-high":        "gpt-5.1-codex-max",
	"gpt-5.2-high":                  "gpt-5.2",
	"gpt-5.2-codex-high":            "gpt-5.2-codex",
	"gpt-5.3-codex-high":            "gpt-5.3-codex",
	"gpt-5.4-high":                  "gpt-5.4",
}

func cursorTokenPrice(inputPerMillion, cacheWritePerMillion, cacheReadPerMillion, outputPerMillion float64) *ModelPricing {
	return &ModelPricing{
		InputPricePerToken:         inputPerMillion * 1e-6,
		OutputPricePerToken:        outputPerMillion * 1e-6,
		CacheCreationPricePerToken: cacheWritePerMillion * 1e-6,
		CacheCreationPriceExplicit: true,
		CacheReadPricePerToken:     cacheReadPerMillion * 1e-6,
		SupportsCacheBreakdown:     false,
	}
}

func canonicalCursorPricingModel(model string) string {
	if canonical, ok := cursorModelPriceAliases[model]; ok {
		return canonical
	}
	for _, suffix := range []string{"-high-thinking", "-medium-thinking", "-low-thinking", "-thinking", "-high", "-medium", "-low"} {
		base := strings.TrimSuffix(model, suffix)
		if base == model {
			continue
		}
		if canonical, ok := cursorModelPriceAliases[base]; ok {
			return canonical
		}
		if _, ok := cursorModelPrices[base]; ok {
			return base
		}
	}
	return model
}

func cursorModelPricing(model string) *ModelPricing {
	model = strings.ToLower(strings.TrimSpace(model))
	model = strings.TrimPrefix(model, "cursor/")
	model = canonicalCursorPricingModel(model)
	pricing := cursorModelPrices[model]
	if pricing == nil {
		return nil
	}
	cloned := *pricing
	return &cloned
}

func (s *BillingService) GetPlatformModelPricing(platform, model string) *ModelPricing {
	if strings.EqualFold(strings.TrimSpace(platform), PlatformCursor) {
		return cursorModelPricing(model)
	}
	return nil
}
