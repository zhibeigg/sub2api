package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestAccountSupportsOpenAIImageRequestByExecutionPath(t *testing.T) {
	oauth := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	apiKey := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	require.True(t, oauth.SupportsOpenAIImageRequest("gpt-image-2", openAIImagesGenerationsEndpoint, OpenAIImagesCapabilityNative))
	require.True(t, oauth.SupportsOpenAIImageRequest("gpt-image-2", openAIImagesEditsEndpoint, OpenAIImagesCapabilityBasic))
	require.False(t, oauth.SupportsOpenAIImageRequest("dall-e-3", openAIImagesGenerationsEndpoint, OpenAIImagesCapabilityNative))

	require.True(t, apiKey.SupportsOpenAIImageRequest("dall-e-3", openAIImagesGenerationsEndpoint, OpenAIImagesCapabilityNative))
	require.True(t, apiKey.SupportsOpenAIImageRequest("dall-e-2", openAIImagesEditsEndpoint, OpenAIImagesCapabilityNative))
	require.False(t, apiKey.SupportsOpenAIImageRequest("dall-e-3", openAIImagesEditsEndpoint, OpenAIImagesCapabilityNative))
}

func TestAccountSupportsOpenAIImageRequestAllowsCompatibleGrokAPIKeyRoutes(t *testing.T) {
	apiKey := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	oauth := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	require.True(t, apiKey.SupportsOpenAIImageRequest("grok-imagine", openAIImagesGenerationsEndpoint, OpenAIImagesCapabilityNative))
	require.True(t, apiKey.SupportsOpenAIImageRequest("grok-imagine-edit", openAIImagesEditsEndpoint, OpenAIImagesCapabilityNative))
	require.False(t, oauth.SupportsOpenAIImageRequest("grok-imagine", openAIImagesGenerationsEndpoint, OpenAIImagesCapabilityNative))
}

func TestAccountSupportsOpenAIImageRequestAppliesMappings(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"model_mapping": map[string]any{"gpt-image-*": "gpt-image-2"},
		},
	}
	require.True(t, account.SupportsOpenAIImageRequest("gpt-image-custom", openAIImagesEditsEndpoint, OpenAIImagesCapabilityNative))
	require.False(t, account.SupportsOpenAIImageRequest("dall-e-2", openAIImagesEditsEndpoint, OpenAIImagesCapabilityNative))

	account = &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"model_mapping": map[string]any{"gpt-image-2": "dall-e-3"},
		},
	}
	require.True(t, account.SupportsOpenAIImageRequest("gpt-image-2", openAIImagesGenerationsEndpoint, OpenAIImagesCapabilityNative))
	require.False(t, account.SupportsOpenAIImageRequest("gpt-image-2", openAIImagesEditsEndpoint, OpenAIImagesCapabilityNative))
}

func TestResolveEffectiveImageGroupBindingSkipsNonRoutableGroups(t *testing.T) {
	first := &Group{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, AllowImageGeneration: true}
	second := &Group{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, AllowImageGeneration: true}
	repo := schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: []Account{
		{ID: 10, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, GroupIDs: []int64{1}, Credentials: map[string]any{"model_mapping": map[string]any{"dall-e-3": "dall-e-3"}}},
		{ID: 20, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, GroupIDs: []int64{2}, Credentials: map[string]any{"model_mapping": map[string]any{"dall-e-3": "dall-e-3"}}},
	}}}
	svc := &OpenAIGatewayService{accountRepo: repo}
	key := &APIKey{GroupBindings: []APIKeyGroupBinding{
		{GroupID: 1, Priority: 0, Group: first},
		{GroupID: 2, Priority: 1, Group: second},
	}}

	probeCtx := context.WithValue(context.Background(), ctxkey.Group, second)
	probeCtx = WithEndpointProtocol(probeCtx, EndpointProtocolOpenAIImages)
	secondID := second.ID
	selection, _, err := svc.SelectAccountWithSchedulerForImages(probeCtx, &secondID, "", "dall-e-3", nil, OpenAIImagesCapabilityNative, openAIImagesGenerationsEndpoint)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, int64(20), selection.Account.ID)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}

	selected := svc.ResolveEffectiveImageGroupBinding(context.Background(), key, "dall-e-3", openAIImagesGenerationsEndpoint, OpenAIImagesCapabilityNative)
	require.Same(t, second, selected)

	key.ExplicitGroupSelection = true
	key.Group = first
	selected = svc.ResolveEffectiveImageGroupBinding(context.Background(), key, "dall-e-3", openAIImagesGenerationsEndpoint, OpenAIImagesCapabilityNative)
	require.Same(t, first, selected)
}

func TestResolveEffectiveImageGroupBindingFailsOverPastModelRateLimitedGroup(t *testing.T) {
	first := &Group{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, AllowImageGeneration: true}
	second := &Group{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, AllowImageGeneration: true}
	resetAt := time.Now().Add(time.Minute).UTC().Format(time.RFC3339)
	repo := schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: []Account{
		{ID: 10, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, GroupIDs: []int64{1}, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-image-2": "gpt-image-2"}}, Extra: map[string]any{
			"model_rate_limits": map[string]any{"gpt-image-2": map[string]any{"rate_limit_reset_at": resetAt}},
		}},
		{ID: 20, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, GroupIDs: []int64{2}, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-image-2": "gpt-image-2"}}},
	}}}
	svc := &OpenAIGatewayService{accountRepo: repo}
	key := &APIKey{GroupBindings: []APIKeyGroupBinding{
		{GroupID: 1, Priority: 0, Group: first},
		{GroupID: 2, Priority: 1, Group: second},
	}}

	selected := svc.ResolveEffectiveImageGroupBinding(context.Background(), key, "gpt-image-2", openAIImagesGenerationsEndpoint, OpenAIImagesCapabilityNative)
	require.Same(t, second, selected)
}

func TestResolveEffectiveImageGroupBindingSkipsNonOpenAIGroups(t *testing.T) {
	first := &Group{ID: 1, Platform: PlatformAnthropic, Status: StatusActive, AllowImageGeneration: true}
	second := &Group{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, AllowImageGeneration: true}
	repo := schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: []Account{
		{ID: 10, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, GroupIDs: []int64{1}, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-image-2": "gpt-image-2"}}},
		{ID: 20, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, GroupIDs: []int64{2}, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-image-2": "gpt-image-2"}}},
	}}}
	svc := &OpenAIGatewayService{accountRepo: repo}
	key := &APIKey{GroupBindings: []APIKeyGroupBinding{
		{GroupID: 1, Priority: 0, Group: first},
		{GroupID: 2, Priority: 1, Group: second},
	}}

	selected := svc.ResolveEffectiveImageGroupBinding(context.Background(), key, "gpt-image-2", openAIImagesGenerationsEndpoint, OpenAIImagesCapabilityNative)
	require.Same(t, second, selected)
}

type openAIPlaygroundCatalogRepo struct {
	AccountRepository
	accounts []Account
}

func (r openAIPlaygroundCatalogRepo) ListSchedulableByGroupIDAndPlatforms(ctx context.Context, groupID int64, platforms []string) ([]Account, error) {
	return r.ListModelAvailabilityCandidates(ctx, &groupID, platforms, false)
}

func (r openAIPlaygroundCatalogRepo) ListModelAvailabilityCandidates(_ context.Context, groupID *int64, platforms []string, _ bool) ([]Account, error) {
	allowedPlatforms := make(map[string]struct{}, len(platforms))
	for _, platform := range platforms {
		allowedPlatforms[platform] = struct{}{}
	}
	result := make([]Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		if _, ok := allowedPlatforms[account.Platform]; !ok || !openAIStickyAccountMatchesGroup(&account, groupID) {
			continue
		}
		result = append(result, account)
	}
	return result, nil
}

func TestOpenAIPlaygroundCatalogMatchesImageSchedulerCapabilities(t *testing.T) {
	groupID := int64(1)
	accounts := []Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, GroupIDs: []int64{groupID}},
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, GroupIDs: []int64{groupID}, Credentials: map[string]any{
			"model_mapping": map[string]any{"dall-e-3": "dall-e-3"},
		}},
	}
	svc := &GatewayService{accountRepo: openAIPlaygroundCatalogRepo{accounts: accounts}}

	models, routable := svc.GetAvailablePlaygroundModels(context.Background(), &groupID, PlatformOpenAI)
	require.True(t, routable)
	require.Contains(t, models, "gpt-image-2")
	require.Contains(t, models, "dall-e-3")
	require.NotContains(t, models, "dall-e-2")

	for _, model := range models {
		if ModelMediaType(model) != PlaygroundCapabilityImage {
			continue
		}
		hasSchedulerCandidate := false
		for i := range accounts {
			if accounts[i].SupportsOpenAIImageRequest(model, openAIImagesGenerationsEndpoint, OpenAIImagesCapabilityNative) {
				hasSchedulerCandidate = true
				break
			}
		}
		require.True(t, hasSchedulerCandidate, model)
	}
}

func TestPlaygroundCatalogExpandsWildcardMappingsForNonOpenAIPlatforms(t *testing.T) {
	groupID := int64(1)
	svc := &GatewayService{accountRepo: openAIPlaygroundCatalogRepo{accounts: []Account{
		{ID: 1, Platform: PlatformGrok, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, GroupIDs: []int64{groupID}, Credentials: map[string]any{
			"model_mapping": map[string]any{"grok-*": "grok-4.5", "grok-imagine": "grok-imagine"},
		}},
	}}}

	models, routable := svc.GetAvailablePlaygroundModels(context.Background(), &groupID, PlatformGrok)
	require.True(t, routable)
	require.Contains(t, models, "grok-4.5")
	require.Contains(t, models, "grok-imagine")
}

func TestPlaygroundCatalogUsesModelSquareChannelModelsAsAuthoritativeSet(t *testing.T) {
	groupID := int64(28)
	account := Account{
		ID:          138,
		Platform:    PlatformCursor,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		GroupIDs:    []int64{groupID},
		AccountGroups: []AccountGroup{{
			AccountID: 138, GroupID: groupID, EndpointCompatibilityEnabled: true,
		}},
		Credentials: map[string]any{"model_mapping": map[string]any{
			"grok-4.3":         "grok-4.3",
			"grok-4.5":         "grok-4.5",
			"gpt-5.5":          "gpt-5.5",
			"gemini-3.5-flash": "gemini-3.5-flash",
		}},
		Extra: map[string]any{"mixed_scheduling": true},
	}
	channel := &Channel{
		ID:     6,
		Name:   "Cursor",
		Status: StatusActive,
		ModelPricing: []ChannelModelPricing{
			{Platform: PlatformGrok, Models: []string{"grok-4.3", "grok-4.5"}},
			{Platform: PlatformGemini, Models: []string{"gemini-3.5-flash"}},
		},
	}
	channelService := &ChannelService{}
	channelService.cache.Store(&channelCache{
		channelByGroupID: map[int64]*Channel{groupID: channel},
		byID:             map[int64]*Channel{channel.ID: channel},
		groupPlatform:    map[int64]string{groupID: PlatformGrok},
		loadedAt:         time.Now(),
	})
	svc := &GatewayService{
		accountRepo:    openAIPlaygroundCatalogRepo{accounts: []Account{account}},
		channelService: channelService,
		cfg: &config.Config{Gateway: config.GatewayConfig{
			CrossProviderCompatibilityEnabled: true,
		}},
	}

	models, routable := svc.GetAvailablePlaygroundModels(context.Background(), &groupID, PlatformGrok)
	require.True(t, routable)
	require.Equal(t, []string{"grok-4.3", "grok-4.5"}, models)
}

func TestOpenAIPlaygroundCatalogOAuthDoesNotAdvertiseDALLERoutes(t *testing.T) {
	groupID := int64(1)
	svc := &GatewayService{accountRepo: openAIPlaygroundCatalogRepo{accounts: []Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, GroupIDs: []int64{groupID}},
	}}}

	models, routable := svc.GetAvailablePlaygroundModels(context.Background(), &groupID, PlatformOpenAI)
	require.True(t, routable)
	require.Contains(t, models, "gpt-image-2")
	require.NotContains(t, models, "dall-e-2")
	require.NotContains(t, models, "dall-e-3")
}
