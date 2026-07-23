package repository

import (
	"context"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupEntityToService_PreservesMessagesDispatchModelConfig(t *testing.T) {
	predictionUnitCostUSD := 0.125
	group := &dbent.Group{
		ID:                          1,
		Name:                        "openai-dispatch",
		Platform:                    service.PlatformOpenAI,
		Status:                      service.StatusActive,
		SubscriptionType:            service.SubscriptionTypeStandard,
		RateMultiplier:              1,
		AllowMessagesDispatch:       true,
		DefaultMappedModel:          "gpt-5.4",
		PoolCapacityAlertEnabled:    true,
		PoolCapacityAlertGeneration: 17,
		PredictedCapacityMode:       service.PredictedCapacityModeFixedImageCost,
		PredictedImageUnitCostUsd:   &predictionUnitCostUSD,
		MessagesDispatchModelConfig: service.OpenAIMessagesDispatchModelConfig{
			OpusMappedModel:   "gpt-5.4-nano",
			SonnetMappedModel: "gpt-5.3-codex",
			HaikuMappedModel:  "gpt-5.4-mini",
			ExactModelMappings: map[string]string{
				"claude-sonnet-4.5": "gpt-5.4-nano",
			},
		},
	}

	got := groupEntityToService(group)
	require.NotNil(t, got)
	require.Equal(t, group.MessagesDispatchModelConfig, got.MessagesDispatchModelConfig)
	require.True(t, got.PoolCapacityAlertEnabled)
	require.Equal(t, int64(17), got.PoolCapacityAlertGeneration)
	require.Equal(t, service.PredictedCapacityModeFixedImageCost, got.PredictedCapacityMode)
	require.NotNil(t, got.PredictedImageUnitCostUSD)
	require.Equal(t, predictionUnitCostUSD, *got.PredictedImageUnitCostUSD)
}

func TestAPIKeyRepository_GetByKeyForAuth_PreservesMessagesDispatchModelConfig_SQLite(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "getbykey-auth-dispatch-unit@test.com")

	modelRateMultipliers := map[string]float64{
		"grok-4.5": 0.60,
		"gpt-*":    0.65,
	}
	endpointProtocols := []string{
		string(service.EndpointProtocolOpenAIChatCompletions),
		string(service.EndpointProtocolOpenAIResponses),
	}
	group, err := client.Group.Create().
		SetName("g-auth-dispatch-unit").
		SetPlatform(service.PlatformOpenAI).
		SetEndpointProtocols(endpointProtocols).
		SetStatus(service.StatusActive).
		SetSubscriptionType(service.SubscriptionTypeStandard).
		SetRateMultiplier(1).
		SetModelRateMultipliers(modelRateMultipliers).
		SetPoolCapacityAlertEnabled(true).
		SetPoolCapacityAlertGeneration(23).
		SetAllowMessagesDispatch(true).
		SetDefaultMappedModel("gpt-5.4").
		SetMessagesDispatchModelConfig(service.OpenAIMessagesDispatchModelConfig{
			OpusMappedModel:   "gpt-5.4-nano",
			SonnetMappedModel: "gpt-5.3-codex",
			HaikuMappedModel:  "gpt-5.4-mini",
			ExactModelMappings: map[string]string{
				"claude-sonnet-4.5": "gpt-5.4-nano",
			},
		}).
		Save(ctx)
	require.NoError(t, err)

	key := &service.APIKey{
		UserID:  user.ID,
		Key:     "sk-getbykey-auth-dispatch-unit",
		Name:    "Dispatch Key Unit",
		GroupID: &group.ID,
		Status:  service.StatusActive,
	}
	require.NoError(t, repo.Create(ctx, key))

	got, err := repo.GetByKeyForAuth(ctx, key.Key)
	require.NoError(t, err)
	require.Equal(t, key.Name, got.Name)
	require.NotNil(t, got.Group)
	require.Equal(t, endpointProtocols, got.Group.EndpointProtocols)
	require.True(t, service.GroupAllowsEndpoint(got.Group, service.EndpointProtocolOpenAIResponses))
	require.Equal(t, group.MessagesDispatchModelConfig, got.Group.MessagesDispatchModelConfig)
	require.Equal(t, modelRateMultipliers, got.Group.ModelRateMultipliers)
	require.True(t, got.Group.PoolCapacityAlertEnabled)
	require.Equal(t, int64(23), got.Group.PoolCapacityAlertGeneration)
}
