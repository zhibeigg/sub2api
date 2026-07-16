package service

import (
	"context"
	"strings"
)

type openAIImageEndpointContextKey struct{}

func withOpenAIImageEndpoint(ctx context.Context, endpoint string) context.Context {
	return context.WithValue(ctx, openAIImageEndpointContextKey{}, normalizeOpenAIImagesEndpointPath(endpoint))
}

func openAIImageEndpointFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	endpoint, _ := ctx.Value(openAIImageEndpointContextKey{}).(string)
	return endpoint
}

var openAIImagesCatalogDefaults = []string{
	"gpt-image-1",
	"gpt-image-1.5",
	"gpt-image-2",
	"dall-e-2",
	"dall-e-3",
}

func isOpenAIPlatformImageModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(model, "gpt-image-") || model == "dall-e-2" || model == "dall-e-3"
}

func openAIImageModelSupportsEndpoint(model, endpoint string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if !isOpenAIPlatformImageModel(model) {
		return false
	}
	switch normalizeOpenAIImagesEndpointPath(endpoint) {
	case openAIImagesEditsEndpoint:
		return strings.HasPrefix(model, "gpt-image-") || model == "dall-e-2"
	case openAIImagesGenerationsEndpoint, "":
		return true
	default:
		return false
	}
}

// SupportsOpenAIImageRequest reports whether this account can execute the
// concrete OpenAI Images request after applying account model mapping.
func (a *Account) SupportsOpenAIImageRequest(model, endpoint string, capability OpenAIImagesCapability) bool {
	if a == nil || !a.IsOpenAI() || !a.SupportsOpenAIImageCapability(capability) {
		return false
	}
	if a.Type != AccountTypeOAuth && a.Type != AccountTypeAPIKey {
		return false
	}
	model = strings.TrimSpace(model)
	if model == "" || !a.IsModelSupported(model) {
		return false
	}
	mappedModel := strings.TrimSpace(a.GetMappedModel(model))
	if isOpenAIPlatformImageModel(mappedModel) {
		if !openAIImageModelSupportsEndpoint(mappedModel, endpoint) {
			return false
		}
		// OAuth uses ChatGPT Responses image_generation rather than the public
		// Images endpoint. That bridge executes GPT Image models; DALL-E routing is
		// reserved for API-key/upstream accounts that expose the real Images API.
		return a.Type == AccountTypeAPIKey || strings.HasPrefix(strings.ToLower(mappedModel), "gpt-image-")
	}
	// OpenAI-compatible API-key accounts can also expose Grok image aliases.
	// Keep those routes available while preventing OAuth accounts from entering
	// a forwarding path that only knows how to execute GPT Image models.
	return a.Type == AccountTypeAPIKey && isGrokImageGenerationModel(mappedModel)
}

func accountCanRoutePlaygroundModel(account *Account, platform, model string) bool {
	if account == nil || strings.TrimSpace(model) == "" {
		return false
	}
	if NormalizePlatform(platform) == PlatformOpenAI && ModelMediaType(model) == PlaygroundCapabilityImage {
		return account.SupportsOpenAIImageRequest(model, openAIImagesGenerationsEndpoint, OpenAIImagesCapabilityNative)
	}
	return account.IsModelSupported(model)
}

func openAIPlaygroundCatalogCandidates(accounts []Account) []string {
	models := make([]string, 0, len(openAIImagesCatalogDefaults)+16)
	models = append(models, playgroundDefaultModelIDs(PlatformOpenAI)...)
	models = append(models, openAIImagesCatalogDefaults...)
	for i := range accounts {
		for model := range accounts[i].GetModelMapping() {
			model = strings.TrimSpace(model)
			if model != "" && !strings.Contains(model, "*") {
				models = append(models, model)
			}
		}
	}
	return normalizePlaygroundModels(models)
}
