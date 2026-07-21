package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/Wei-Shaw/sub2api/internal/provider/adobe/firefly"
)

const (
	PlaygroundCapabilityChat  = "chat"
	PlaygroundCapabilityImage = "image"
	PlaygroundCapabilityVideo = "video"
)

type PlaygroundModelFeatures struct {
	ImageInput    bool `json:"image_input"`
	Responses     bool `json:"responses"`
	WebSearch     bool `json:"web_search"`
	CodeExecution bool `json:"code_execution"`
	WebFetch      bool `json:"web_fetch"`
}

type PlaygroundModelOption struct {
	ID            string                  `json:"id"`
	GroupID       int64                   `json:"group_id"`
	GroupName     string                  `json:"group_name"`
	GroupPriority int                     `json:"group_priority"`
	Model         string                  `json:"model"`
	Platform      string                  `json:"platform"`
	Capabilities  []string                `json:"capabilities"`
	Features      PlaygroundModelFeatures `json:"features"`
}

type PlaygroundAPIKeyReader interface {
	GetByID(ctx context.Context, id int64) (*APIKey, error)
}

type PlaygroundModelLister interface {
	GetAvailableModels(ctx context.Context, groupID *int64, platform string) []string
}

type PlaygroundRoutableModelLister interface {
	GetAvailablePlaygroundModels(ctx context.Context, groupID *int64, platform string) ([]string, bool)
}

type PlaygroundGroupAccessChecker interface {
	CanUserBindGroup(ctx context.Context, user *User, group *Group) bool
}

type PlaygroundService struct {
	apiKeys    PlaygroundAPIKeyReader
	models     PlaygroundModelLister
	urlFetcher *PlaygroundURLFetcher
}

func NewPlaygroundService(apiKeys PlaygroundAPIKeyReader, models PlaygroundModelLister) *PlaygroundService {
	return &PlaygroundService{apiKeys: apiKeys, models: models, urlFetcher: newPlaygroundURLFetcher()}
}

func (s *PlaygroundService) GetModelOptions(ctx context.Context, userID, apiKeyID int64) ([]PlaygroundModelOption, error) {
	apiKey, err := s.apiKeys.GetByID(ctx, apiKeyID)
	if err != nil {
		return nil, err
	}
	if apiKey == nil || apiKey.UserID != userID {
		return nil, ErrAPIKeyNotFound
	}

	bindings := playgroundBindings(apiKey)
	optionsByModel := make(map[string]PlaygroundModelOption)
	for _, binding := range bindings {
		group := binding.Group
		if group == nil || group.ID <= 0 || !group.IsActive() {
			continue
		}
		if checker, ok := s.apiKeys.(PlaygroundGroupAccessChecker); ok && !checker.CanUserBindGroup(ctx, apiKey.User, group) {
			continue
		}
		var models []string
		if lister, ok := s.models.(PlaygroundRoutableModelLister); ok {
			var routable bool
			models, routable = lister.GetAvailablePlaygroundModels(ctx, &group.ID, group.Platform)
			if !routable {
				continue
			}
		} else {
			models = s.models.GetAvailableModels(ctx, &group.ID, group.Platform)
		}
		models = playgroundModelsForGroup(group, models)
		for _, model := range models {
			capabilities := playgroundModelCapabilities(group, model)
			if len(capabilities) == 0 {
				continue
			}
			modelKey := strings.ToLower(strings.TrimSpace(model))
			if modelKey == "" {
				continue
			}
			if _, exists := optionsByModel[modelKey]; exists {
				continue
			}
			optionsByModel[modelKey] = PlaygroundModelOption{
				ID:            fmt.Sprintf("%d::%s", group.ID, model),
				GroupID:       group.ID,
				GroupName:     group.Name,
				GroupPriority: binding.Priority,
				Model:         model,
				Platform:      group.Platform,
				Capabilities:  capabilities,
				Features:      playgroundModelFeatures(group, model, capabilities),
			}
		}
	}

	options := make([]PlaygroundModelOption, 0, len(optionsByModel))
	for _, option := range optionsByModel {
		options = append(options, option)
	}
	sort.SliceStable(options, func(i, j int) bool {
		left := strings.ToLower(options[i].Model)
		right := strings.ToLower(options[j].Model)
		if left != right {
			return left < right
		}
		return options[i].Model < options[j].Model
	})
	return options, nil
}

func playgroundBindings(apiKey *APIKey) []APIKeyGroupBinding {
	if apiKey == nil {
		return nil
	}
	if len(apiKey.GroupBindings) > 0 {
		bindings := append([]APIKeyGroupBinding(nil), apiKey.GroupBindings...)
		sort.SliceStable(bindings, func(i, j int) bool {
			if bindings[i].Priority != bindings[j].Priority {
				return bindings[i].Priority < bindings[j].Priority
			}
			return bindings[i].GroupID < bindings[j].GroupID
		})
		return bindings
	}
	if apiKey.Group == nil || apiKey.GroupID == nil {
		return nil
	}
	return []APIKeyGroupBinding{{GroupID: *apiKey.GroupID, Priority: 0, Group: apiKey.Group}}
}

func playgroundModelsForGroup(group *Group, available []string) []string {
	if group == nil {
		return nil
	}
	source := normalizePlaygroundModels(available)
	if group.CustomModelsListEnabled() {
		return filterPlaygroundCustomModels(source, group.ModelsListConfig.Models)
	}
	return source
}

func filterPlaygroundCustomModels(available, selected []string) []string {
	allowed := normalizePlaygroundModels(available)
	seen := make(map[string]struct{}, len(selected))
	out := make([]string, 0, len(selected))
	for _, model := range selected {
		model = strings.TrimSpace(model)
		if model == "" || !playgroundModelAllowed(allowed, model) {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		out = append(out, model)
	}
	return out
}

func playgroundModelAllowed(patterns []string, model string) bool {
	for _, pattern := range patterns {
		if pattern == model || (strings.HasSuffix(pattern, "*") && strings.HasPrefix(model, strings.TrimSuffix(pattern, "*"))) {
			return true
		}
	}
	return false
}

func normalizePlaygroundModels(models []string) []string {
	seen := make(map[string]struct{}, len(models))
	out := make([]string, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		out = append(out, model)
	}
	return out
}

func mergePlaygroundModelIDs(primary, secondary []string) []string {
	return normalizePlaygroundModels(append(append([]string(nil), primary...), secondary...))
}

func playgroundDefaultModelIDs(platform string) []string {
	switch NormalizePlatform(platform) {
	case PlatformOpenAI:
		return mergePlaygroundModelIDs(openai.DefaultModelIDs(), []string{"dall-e-2", "dall-e-3"})
	case PlatformGemini:
		ids := make([]string, 0, len(geminicli.DefaultModels))
		for _, model := range geminicli.DefaultModels {
			ids = append(ids, model.ID)
		}
		return ids
	case PlatformAntigravity:
		models := antigravity.DefaultModels()
		ids := make([]string, 0, len(models))
		for _, model := range models {
			ids = append(ids, model.ID)
		}
		return ids
	case PlatformAnthropic:
		ids := make([]string, 0, len(claude.DefaultModels)+len(antigravity.DefaultModels()))
		for _, model := range claude.DefaultModels {
			ids = append(ids, model.ID)
		}
		for _, model := range antigravity.DefaultModels() {
			ids = append(ids, model.ID)
		}
		return normalizePlaygroundModels(ids)
	case PlatformGrok:
		return xai.DefaultModelIDs()
	case PlatformAdobe:
		return firefly.PublicModelIDs()
	case PlatformCursor:
		ids := make([]string, 0, len(domain.DefaultCursorModelMapping))
		for id := range domain.DefaultCursorModelMapping {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		return ids
	case PlatformKiro:
		ids := make([]string, 0, len(domain.DefaultKiroModelMapping))
		for id := range domain.DefaultKiroModelMapping {
			ids = append(ids, id)
		}
		return mergePlaygroundModelIDs(ids, KiroDiscoveredModelIDs())
	default:
		return nil
	}
}

func playgroundModelFeatures(group *Group, model string, capabilities []string) PlaygroundModelFeatures {
	features := PlaygroundModelFeatures{}
	if group == nil || len(capabilities) != 1 || capabilities[0] != PlaygroundCapabilityChat {
		return features
	}

	platform := NormalizePlatform(group.Platform)
	modelLower := strings.ToLower(strings.TrimSpace(model))
	features.WebFetch = true

	switch platform {
	case PlatformOpenAI:
		features.Responses = true
		features.ImageInput = hasAnyModelPrefix(modelLower,
			"gpt-4o", "gpt-4.1", "gpt-5", "chatgpt-4o", "o1", "o3", "o4")
		features.WebSearch = hasAnyModelPrefix(modelLower, "gpt-4.1", "gpt-5", "o3", "o4")
		features.CodeExecution = features.WebSearch
	case PlatformAnthropic:
		features.Responses = true
		features.WebSearch = true
		features.ImageInput = strings.HasPrefix(modelLower, "claude-3") ||
			strings.Contains(modelLower, "claude-sonnet-4") ||
			strings.Contains(modelLower, "claude-opus-4") ||
			strings.Contains(modelLower, "claude-haiku-4")
	case PlatformGemini, PlatformAntigravity:
		features.ImageInput = strings.HasPrefix(modelLower, "gemini-")
	case PlatformGrok:
		features.Responses = true
		features.WebSearch = true
		features.CodeExecution = true
		features.ImageInput = strings.Contains(modelLower, "vision")
	}
	return features
}

func hasAnyModelPrefix(model string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(model, prefix) {
			return true
		}
	}
	return false
}

// ModelMediaType 根据模型名识别图片或视频生成模型；普通模型返回空字符串。
func ModelMediaType(model string) string {
	modelLower := strings.ToLower(strings.TrimSpace(model))
	if modelLower == "" {
		return ""
	}
	if firefly.IsVideoAlias(modelLower) || strings.Contains(modelLower, "video") || strings.HasPrefix(modelLower, "veo") || strings.HasPrefix(modelLower, "sora") || strings.Contains(modelLower, "seedance") {
		return PlaygroundCapabilityVideo
	}
	if firefly.IsImageAlias(modelLower) || isOpenAIPlatformImageModel(modelLower) || strings.Contains(modelLower, "imagine-image") || strings.HasSuffix(modelLower, "-image") || strings.Contains(modelLower, "image-preview") || strings.Contains(modelLower, "imagine-edit") || modelLower == "grok-imagine" {
		return PlaygroundCapabilityImage
	}
	return ""
}

func playgroundModelCapabilities(group *Group, model string) []string {
	platform := NormalizePlatform(group.Platform)
	mediaType := ModelMediaType(model)
	if strings.TrimSpace(model) == "" {
		return nil
	}

	isVideo := mediaType == PlaygroundCapabilityVideo
	isImage := mediaType == PlaygroundCapabilityImage

	switch platform {
	case PlatformAdobe:
		if isVideo && group.AllowImageGeneration {
			return []string{PlaygroundCapabilityVideo}
		}
		if isImage && group.AllowImageGeneration {
			return []string{PlaygroundCapabilityImage}
		}
		return nil
	case PlatformGrok:
		if isVideo && group.AllowImageGeneration {
			return []string{PlaygroundCapabilityVideo}
		}
		if isImage && group.AllowImageGeneration {
			return []string{PlaygroundCapabilityImage}
		}
		return []string{PlaygroundCapabilityChat}
	case PlatformOpenAI:
		if isVideo {
			return []string{PlaygroundCapabilityVideo}
		}
		if isImage && group.AllowImageGeneration {
			return []string{PlaygroundCapabilityImage}
		}
		if isImage {
			return nil
		}
		return []string{PlaygroundCapabilityChat}
	case PlatformAnthropic, PlatformGemini, PlatformAntigravity, PlatformCursor, PlatformOpenCode, PlatformKiro:
		if isImage || isVideo {
			return nil
		}
		return []string{PlaygroundCapabilityChat}
	default:
		return nil
	}
}
