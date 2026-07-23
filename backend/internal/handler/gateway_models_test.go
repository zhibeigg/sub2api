package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type gatewayModelsAccountRepoStub struct {
	service.AccountRepository

	byGroup map[int64][]service.Account
}

type gatewayModelsResponseForTest struct {
	Object string                    `json:"object"`
	Data   []gatewayModelItemForTest `json:"data"`
}

type gatewayModelItemForTest struct {
	ID                      string                                `json:"id"`
	Object                  string                                `json:"object"`
	Created                 int64                                 `json:"created"`
	OwnedBy                 string                                `json:"owned_by"`
	CreatedAt               string                                `json:"created_at"`
	SupportsReasoningEffort bool                                  `json:"supportsReasoningEffort"`
	ReasoningEffort         string                                `json:"reasoningEffort"`
	ReasoningEfforts        []gatewayReasoningEffortOptionForTest `json:"reasoningEfforts"`
}

type gatewayReasoningEffortOptionForTest struct {
	Value   string `json:"value"`
	Label   string `json:"label"`
	Default bool   `json:"default"`
}

func (s *gatewayModelsAccountRepoStub) ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]service.Account, error) {
	accounts, ok := s.byGroup[groupID]
	if !ok {
		return nil, nil
	}
	out := make([]service.Account, len(accounts))
	copy(out, accounts)
	return out, nil
}

func (s *gatewayModelsAccountRepoStub) ListSchedulableByGroupIDAndPlatforms(_ context.Context, groupID int64, platforms []string) ([]service.Account, error) {
	allowedPlatforms := make(map[string]struct{}, len(platforms))
	for _, platform := range platforms {
		allowedPlatforms[platform] = struct{}{}
	}
	accounts := s.byGroup[groupID]
	out := make([]service.Account, 0, len(accounts))
	for _, account := range accounts {
		if _, ok := allowedPlatforms[account.Platform]; !ok || !account.IsSchedulable() {
			continue
		}
		out = append(out, account)
	}
	return out, nil
}

func newGatewayModelsHandlerForTest(repo service.AccountRepository) *GatewayHandler {
	return &GatewayHandler{
		gatewayService: service.NewGatewayService(
			repo,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		),
	}
}

func TestDefaultModelIDsForCompositeIncludesAntigravityDefaults(t *testing.T) {
	antigravityIDs := defaultModelIDsForPlatform(service.PlatformAntigravity)
	require.NotEmpty(t, antigravityIDs)

	compositeIDs := defaultModelIDsForPlatform(service.PlatformComposite)
	require.Contains(t, compositeIDs, antigravityIDs[0])
}

func TestGatewayModels_GeminiGroupFallsBackToGeminiModels(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(20)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				groupID: {
					{ID: 1, Platform: service.PlatformGemini},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{ID: groupID, Platform: service.PlatformGemini},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)

	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "list", got.Object)
	require.Contains(t, modelIDsForTest(got.Data), "gemini-2.5-flash")
	require.NotContains(t, modelIDsForTest(got.Data), "claude-sonnet-4-6")
}

func TestGatewayModels_Grok45AdvertisesReasoningEffortForGrokBuild(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(4409)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				groupID: {
					{
						ID:       1,
						Platform: service.PlatformGrok,
						Credentials: map[string]any{
							"model_mapping": map[string]any{"grok-4.5": "grok-4.5"},
						},
					},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{ID: groupID, Platform: service.PlatformGrok},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)
	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got.Data, 1)
	model := got.Data[0]
	require.Equal(t, "grok-4.5", model.ID)
	require.True(t, model.SupportsReasoningEffort)
	require.Equal(t, "high", model.ReasoningEffort)
	require.Equal(t, []gatewayReasoningEffortOptionForTest{
		{Value: "low", Label: "Low"},
		{Value: "medium", Label: "Medium"},
		{Value: "high", Label: "High", Default: true},
	}, model.ReasoningEfforts)
}

func TestGatewayModels_GeminiGroupFiltersMappedModelsByPlatform(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(21)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				groupID: {
					{
						ID:       1,
						Platform: service.PlatformAnthropic,
						Credentials: map[string]any{
							"model_mapping": map[string]any{
								"claude-sonnet-4-6": "claude-sonnet-4-6",
							},
						},
					},
					{
						ID:       2,
						Platform: service.PlatformGemini,
						Credentials: map[string]any{
							"model_mapping": map[string]any{
								"gemini-2.5-flash": "gemini-2.5-flash",
							},
						},
					},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{ID: groupID, Platform: service.PlatformGemini},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)

	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, []string{"gemini-2.5-flash"}, modelIDsForTest(got.Data))
}

func TestGatewayModels_CustomModelsListDisabledKeepsOriginalModels(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(22)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				groupID: {
					{
						ID:       1,
						Platform: service.PlatformOpenAI,
						Credentials: map[string]any{
							"model_mapping": map[string]any{
								"gpt-5.5": "gpt-5.5",
								"gpt-5.4": "gpt-5.4",
							},
						},
					},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{
			ID:       groupID,
			Platform: service.PlatformOpenAI,
			ModelsListConfig: service.GroupModelsListConfig{
				Enabled: false,
				Models:  []string{"gpt-5.5"},
			},
		},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)

	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, []string{"gpt-5.4", "gpt-5.5"}, modelIDsForTest(got.Data))
}

func TestGatewayModels_CustomModelsListFiltersAndOrdersMappedModels(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(23)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				groupID: {
					{
						ID:       1,
						Platform: service.PlatformOpenAI,
						Credentials: map[string]any{
							"model_mapping": map[string]any{
								"gpt-5.4":         "gpt-5.4",
								"gpt-5.5":         "gpt-5.5",
								"legacy-gpt-2024": "legacy-gpt-2024",
							},
						},
					},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{
			ID:       groupID,
			Platform: service.PlatformOpenAI,
			ModelsListConfig: service.GroupModelsListConfig{
				Enabled: true,
				Models:  []string{"gpt-5.5", "missing-model", "gpt-5.4"},
			},
		},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)

	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, []string{"gpt-5.5", "gpt-5.4"}, modelIDsForTest(got.Data))
}

func TestGatewayModels_CompositeCustomModelsListFiltersAcrossConcretePlatforms(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(33)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				groupID: {
					{
						ID:       1,
						Platform: service.PlatformOpenAI,
						Credentials: map[string]any{
							"model_mapping": map[string]any{
								"gpt-5.4": "gpt-5.4",
								"gpt-5.5": "gpt-5.5",
							},
						},
					},
					{
						ID:       2,
						Platform: service.PlatformGemini,
						Credentials: map[string]any{
							"model_mapping": map[string]any{
								"gemini-2.5-flash": "gemini-2.5-flash",
							},
						},
					},
					{
						ID:       3,
						Platform: service.PlatformAntigravity,
						Credentials: map[string]any{
							"model_mapping": map[string]any{
								"ag-custom-model": "ag-custom-model",
							},
						},
					},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{
			ID:       groupID,
			Platform: service.PlatformComposite,
			ModelsListConfig: service.GroupModelsListConfig{
				Enabled: true,
				Models:  []string{"gemini-2.5-flash", "missing-model", "ag-custom-model", "gpt-5.5"},
			},
		},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)

	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, []string{"gemini-2.5-flash", "ag-custom-model", "gpt-5.5"}, modelIDsForTest(got.Data))
}

func TestGatewayModels_CompositeUnmappedAccountsFallbackToLinkedPlatformsOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(34)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				groupID: {
					{ID: 1, Platform: service.PlatformOpenAI},
					{ID: 2, Platform: service.PlatformGrok},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{ID: groupID, Platform: service.PlatformComposite},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)

	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))

	ids := modelIDsForTest(got.Data)
	require.Contains(t, ids, "gpt-5.5")
	require.Contains(t, ids, "grok-4.3")
	require.NotContains(t, ids, "claude-sonnet-4-6")
	require.NotContains(t, ids, "gemini-2.5-flash")
}

func TestGatewayModels_CustomModelsListKeepsConcreteModelAllowedByWildcardMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(26)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				groupID: {
					{
						ID:       1,
						Platform: service.PlatformAnthropic,
						Credentials: map[string]any{
							"model_mapping": map[string]any{
								"claude-*": "claude-sonnet-4-6",
							},
						},
					},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{
			ID:       groupID,
			Platform: service.PlatformAnthropic,
			ModelsListConfig: service.GroupModelsListConfig{
				Enabled: true,
				Models:  []string{"claude-sonnet-4-6"},
			},
		},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)

	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, []string{"claude-sonnet-4-6"}, modelIDsForTest(got.Data))
}

func TestGatewayModels_AnthropicCustomModelsListIncludesOAuthClaudeAndMappedDeepSeek(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(28)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				groupID: {
					{
						ID:       1,
						Platform: service.PlatformAnthropic,
						Type:     service.AccountTypeOAuth,
					},
					{
						ID:       2,
						Platform: service.PlatformAnthropic,
						Type:     service.AccountTypeAPIKey,
						Credentials: map[string]any{
							"model_mapping": map[string]any{
								"deepseek-v4-pro": "deepseek-v4-pro",
							},
						},
					},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{
			ID:       groupID,
			Platform: service.PlatformAnthropic,
			ModelsListConfig: service.GroupModelsListConfig{
				Enabled: true,
				Models:  []string{"claude-fable-5", "claude-opus-4-8", "deepseek-v4-pro"},
			},
		},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)

	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, []string{"claude-fable-5", "claude-opus-4-8", "deepseek-v4-pro"}, modelIDsForTest(got.Data))
}

func TestGatewayModels_AnthropicCustomModelsListDisabledKeepsMappedModelList(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(29)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				groupID: {
					{
						ID:       1,
						Platform: service.PlatformAnthropic,
						Type:     service.AccountTypeOAuth,
					},
					{
						ID:       2,
						Platform: service.PlatformAnthropic,
						Type:     service.AccountTypeAPIKey,
						Credentials: map[string]any{
							"model_mapping": map[string]any{
								"deepseek-v4-pro": "deepseek-v4-pro",
							},
						},
					},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{
			ID:       groupID,
			Platform: service.PlatformAnthropic,
			ModelsListConfig: service.GroupModelsListConfig{
				Enabled: false,
				Models:  []string{"claude-fable-5", "deepseek-v4-pro"},
			},
		},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)

	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, []string{"deepseek-v4-pro"}, modelIDsForTest(got.Data))
}

func TestGatewayModels_AnthropicCustomModelsListIncludesOAuthClaudeWithoutMappings(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(30)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				groupID: {
					{
						ID:       1,
						Platform: service.PlatformAnthropic,
						Type:     service.AccountTypeOAuth,
					},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{
			ID:       groupID,
			Platform: service.PlatformAnthropic,
			ModelsListConfig: service.GroupModelsListConfig{
				Enabled: true,
				Models:  []string{"claude-opus-4-6-thinking", "claude-sonnet-4-5"},
			},
		},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)

	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, []string{"claude-opus-4-6-thinking", "claude-sonnet-4-5"}, modelIDsForTest(got.Data))
}

func TestGatewayModels_CustomModelsListCanReturnEmptyWhenSelectionsUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(24)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				groupID: {
					{
						ID:       1,
						Platform: service.PlatformOpenAI,
						Credentials: map[string]any{
							"model_mapping": map[string]any{
								"gpt-5.4": "gpt-5.4",
							},
						},
					},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{
			ID:       groupID,
			Platform: service.PlatformOpenAI,
			ModelsListConfig: service.GroupModelsListConfig{
				Enabled: true,
				Models:  []string{"gpt-5.5"},
			},
		},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)

	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Empty(t, modelIDsForTest(got.Data))
}

func TestGatewayModels_CustomModelsListFiltersDefaultFallbackModels(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(25)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				groupID: {
					{ID: 1, Platform: service.PlatformOpenAI},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{
			ID:       groupID,
			Platform: service.PlatformOpenAI,
			ModelsListConfig: service.GroupModelsListConfig{
				Enabled: true,
				Models:  []string{"gpt-5.5", "legacy-gpt-2024", "gpt-5.4"},
			},
		},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)

	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, []string{"gpt-5.5", "gpt-5.4"}, modelIDsForTest(got.Data))
}

func TestGatewayModels_OpenAICustomModelsListKeepsOpenAIResponseShapeForDefaultFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(27)
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				groupID: {
					{ID: 1, Platform: service.PlatformOpenAI},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{
			ID:       groupID,
			Platform: service.PlatformOpenAI,
			ModelsListConfig: service.GroupModelsListConfig{
				Enabled: true,
				Models:  []string{"gpt-5.5", "gpt-5.4"},
			},
		},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)

	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, []string{"gpt-5.5", "gpt-5.4"}, modelIDsForTest(got.Data))
	require.Equal(t, "model", got.Data[0].Object)
	require.NotZero(t, got.Data[0].Created)
	require.Equal(t, "openai", got.Data[0].OwnedBy)
	require.Empty(t, got.Data[0].CreatedAt)
}

func TestGatewayModels_MultiGroupAggregatesMappedModelsAndDeduplicates(t *testing.T) {
	gin.SetMode(gin.TestMode)

	anthropicGroup := &service.Group{ID: 31, Platform: service.PlatformAnthropic, Status: service.StatusActive}
	openAIGroup := &service.Group{ID: 32, Platform: service.PlatformOpenAI, Status: service.StatusActive}
	geminiGroup := &service.Group{ID: 33, Platform: service.PlatformGemini, Status: service.StatusActive}
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				anthropicGroup.ID: {
					{
						ID:       1,
						Platform: service.PlatformAnthropic,
						Credentials: map[string]any{
							"model_mapping": map[string]any{
								"claude-sonnet-4-6": "claude-sonnet-4-6",
								"shared-model":      "shared-model",
							},
						},
					},
				},
				openAIGroup.ID: {
					{
						ID:       2,
						Platform: service.PlatformOpenAI,
						Credentials: map[string]any{
							"model_mapping": map[string]any{
								"gpt-5.4":      "gpt-5.4",
								"shared-model": "shared-model",
							},
						},
					},
				},
				geminiGroup.ID: {
					{
						ID:       3,
						Platform: service.PlatformGemini,
						Credentials: map[string]any{
							"model_mapping": map[string]any{
								"gemini-2.5-flash": "gemini-2.5-flash",
							},
						},
					},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		GroupID: &anthropicGroup.ID,
		Group:   anthropicGroup,
		GroupBindings: []service.APIKeyGroupBinding{
			{GroupID: anthropicGroup.ID, Priority: 0, Group: anthropicGroup},
			{GroupID: openAIGroup.ID, Priority: 1, Group: openAIGroup},
			{GroupID: geminiGroup.ID, Priority: 2, Group: geminiGroup},
		},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)
	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, []string{
		"claude-sonnet-4-6",
		"shared-model",
		"gpt-5.4",
		"gemini-2.5-flash",
	}, modelIDsForTest(got.Data))
}

func TestGatewayModels_MultiGroupAppliesEachGroupsCustomModelList(t *testing.T) {
	gin.SetMode(gin.TestMode)

	anthropicGroup := &service.Group{
		ID:       34,
		Platform: service.PlatformAnthropic,
		Status:   service.StatusActive,
		ModelsListConfig: service.GroupModelsListConfig{
			Enabled: true,
			Models:  []string{"claude-opus-4-8", "claude-sonnet-4-6"},
		},
	}
	openAIGroup := &service.Group{ID: 35, Platform: service.PlatformOpenAI, Status: service.StatusActive}
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				anthropicGroup.ID: {
					{
						ID:       1,
						Platform: service.PlatformAnthropic,
						Credentials: map[string]any{
							"model_mapping": map[string]any{
								"claude-sonnet-4-6": "claude-sonnet-4-6",
								"claude-opus-4-8":   "claude-opus-4-8",
							},
						},
					},
				},
				openAIGroup.ID: {
					{
						ID:       2,
						Platform: service.PlatformOpenAI,
						Credentials: map[string]any{
							"model_mapping": map[string]any{"gpt-5.4": "gpt-5.4"},
						},
					},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		GroupID: &anthropicGroup.ID,
		Group:   anthropicGroup,
		GroupBindings: []service.APIKeyGroupBinding{
			{GroupID: anthropicGroup.ID, Priority: 0, Group: anthropicGroup},
			{GroupID: openAIGroup.ID, Priority: 1, Group: openAIGroup},
		},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)
	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, []string{"claude-opus-4-8", "claude-sonnet-4-6", "gpt-5.4"}, modelIDsForTest(got.Data))
}

func TestGatewayModels_ExplicitGroupSelectionDoesNotAggregateOtherBindings(t *testing.T) {
	gin.SetMode(gin.TestMode)

	anthropicGroup := &service.Group{ID: 36, Platform: service.PlatformAnthropic, Status: service.StatusActive}
	openAIGroup := &service.Group{ID: 37, Platform: service.PlatformOpenAI, Status: service.StatusActive}
	h := newGatewayModelsHandlerForTest(
		&gatewayModelsAccountRepoStub{
			byGroup: map[int64][]service.Account{
				anthropicGroup.ID: {
					{
						ID:       1,
						Platform: service.PlatformAnthropic,
						Credentials: map[string]any{
							"model_mapping": map[string]any{"claude-sonnet-4-6": "claude-sonnet-4-6"},
						},
					},
				},
				openAIGroup.ID: {
					{
						ID:       2,
						Platform: service.PlatformOpenAI,
						Credentials: map[string]any{
							"model_mapping": map[string]any{"gpt-5.4": "gpt-5.4"},
						},
					},
				},
			},
		},
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		GroupID:                &openAIGroup.ID,
		Group:                  openAIGroup,
		ExplicitGroupSelection: true,
		GroupBindings: []service.APIKeyGroupBinding{
			{GroupID: anthropicGroup.ID, Priority: 0, Group: anthropicGroup},
			{GroupID: openAIGroup.ID, Priority: 1, Group: openAIGroup},
		},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)
	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, []string{"gpt-5.4"}, modelIDsForTest(got.Data))
}

func TestGatewayModels_MultiGroupHidesDisabledOpenAIImageModels(t *testing.T) {
	gin.SetMode(gin.TestMode)

	anthropicGroup := &service.Group{ID: 38, Platform: service.PlatformAnthropic, Status: service.StatusActive}
	openAIGroup := &service.Group{ID: 39, Platform: service.PlatformOpenAI, Status: service.StatusActive, AllowImageGeneration: false}
	h := newGatewayModelsHandlerForTest(&gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{
		anthropicGroup.ID: {
			{ID: 1, Platform: service.PlatformAnthropic, Credentials: map[string]any{
				"model_mapping": map[string]any{"claude-sonnet-4-6": "claude-sonnet-4-6"},
			}},
		},
		openAIGroup.ID: {
			{ID: 2, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Credentials: map[string]any{
				"model_mapping": map[string]any{"gpt-5.4": "gpt-5.4", "gpt-image-2": "gpt-image-2"},
			}},
		},
	}})

	got := requestGatewayModelsForTest(t, h, &service.APIKey{
		GroupID: &anthropicGroup.ID,
		Group:   anthropicGroup,
		GroupBindings: []service.APIKeyGroupBinding{
			{GroupID: anthropicGroup.ID, Priority: 0, Group: anthropicGroup},
			{GroupID: openAIGroup.ID, Priority: 1, Group: openAIGroup},
		},
	})
	ids := modelIDsForTest(got.Data)
	require.Contains(t, ids, "claude-sonnet-4-6")
	require.Contains(t, ids, "gpt-5.4")
	require.NotContains(t, ids, "gpt-image-2")
}

func TestGatewayModels_MultiGroupNonOpenAIGroupCannotContributeOpenAIImageModels(t *testing.T) {
	gin.SetMode(gin.TestMode)

	anthropicGroup := &service.Group{ID: 40, Platform: service.PlatformAnthropic, Status: service.StatusActive}
	grokGroup := &service.Group{ID: 41, Platform: service.PlatformGrok, Status: service.StatusActive, AllowImageGeneration: true}
	h := newGatewayModelsHandlerForTest(&gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{
		anthropicGroup.ID: {
			{ID: 1, Platform: service.PlatformAnthropic, Credentials: map[string]any{
				"model_mapping": map[string]any{"claude-sonnet-4-6": "claude-sonnet-4-6"},
			}},
		},
		grokGroup.ID: {
			{ID: 2, Platform: service.PlatformGrok, Credentials: map[string]any{
				"model_mapping": map[string]any{"gpt-image-2": "gpt-image-2", "grok-imagine": "grok-imagine"},
			}},
		},
	}})

	got := requestGatewayModelsForTest(t, h, &service.APIKey{
		GroupID: &anthropicGroup.ID,
		Group:   anthropicGroup,
		GroupBindings: []service.APIKeyGroupBinding{
			{GroupID: anthropicGroup.ID, Priority: 0, Group: anthropicGroup},
			{GroupID: grokGroup.ID, Priority: 1, Group: grokGroup},
		},
	})
	ids := modelIDsForTest(got.Data)
	require.Contains(t, ids, "grok-imagine")
	require.NotContains(t, ids, "gpt-image-2")
}

func TestGatewayModels_MultiGroupKeepsRoutableOpenAIImageModels(t *testing.T) {
	gin.SetMode(gin.TestMode)

	anthropicGroup := &service.Group{ID: 42, Platform: service.PlatformAnthropic, Status: service.StatusActive}
	openAIGroup := &service.Group{ID: 43, Platform: service.PlatformOpenAI, Status: service.StatusActive, AllowImageGeneration: true}
	h := newGatewayModelsHandlerForTest(&gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{
		anthropicGroup.ID: {
			{ID: 1, Platform: service.PlatformAnthropic, Credentials: map[string]any{
				"model_mapping": map[string]any{"claude-sonnet-4-6": "claude-sonnet-4-6"},
			}},
		},
		openAIGroup.ID: {
			{ID: 2, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Credentials: map[string]any{
				"model_mapping": map[string]any{"gpt-5.4": "gpt-5.4", "gpt-image-2": "gpt-image-2"},
			}},
		},
	}})

	got := requestGatewayModelsForTest(t, h, &service.APIKey{
		GroupID: &anthropicGroup.ID,
		Group:   anthropicGroup,
		GroupBindings: []service.APIKeyGroupBinding{
			{GroupID: anthropicGroup.ID, Priority: 0, Group: anthropicGroup},
			{GroupID: openAIGroup.ID, Priority: 1, Group: openAIGroup},
		},
	})
	ids := modelIDsForTest(got.Data)
	require.Contains(t, ids, "gpt-5.4")
	require.Contains(t, ids, "gpt-image-2")
}

func TestGatewayModels_ExplicitGroupFiltersCustomOpenAIImageModels(t *testing.T) {
	gin.SetMode(gin.TestMode)

	group := &service.Group{
		ID:                   44,
		Platform:             service.PlatformOpenAI,
		Status:               service.StatusActive,
		AllowImageGeneration: false,
		ModelsListConfig: service.GroupModelsListConfig{
			Enabled: true,
			Models:  []string{"gpt-image-2", "gpt-5.4"},
		},
	}
	h := newGatewayModelsHandlerForTest(&gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{
		group.ID: {
			{ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Credentials: map[string]any{
				"model_mapping": map[string]any{"gpt-5.4": "gpt-5.4", "gpt-image-2": "gpt-image-2"},
			}},
		},
	}})

	got := requestGatewayModelsForTest(t, h, &service.APIKey{
		GroupID:                &group.ID,
		Group:                  group,
		ExplicitGroupSelection: true,
	})
	require.Equal(t, []string{"gpt-5.4"}, modelIDsForTest(got.Data))
}

func TestGatewayModels_ExplicitGroupFiltersDefaultOpenAIImageFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	group := &service.Group{
		ID:                   45,
		Platform:             service.PlatformOpenAI,
		Status:               service.StatusActive,
		AllowImageGeneration: false,
	}
	h := newGatewayModelsHandlerForTest(&gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{
		group.ID: {
			{ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true},
		},
	}})

	got := requestGatewayModelsForTest(t, h, &service.APIKey{
		GroupID:                &group.ID,
		Group:                  group,
		ExplicitGroupSelection: true,
	})
	ids := modelIDsForTest(got.Data)
	require.Contains(t, ids, "gpt-5.4")
	require.NotContains(t, ids, "gpt-image-1")
	require.NotContains(t, ids, "gpt-image-1.5")
	require.NotContains(t, ids, "gpt-image-2")
	require.NotContains(t, ids, "dall-e-2")
	require.NotContains(t, ids, "dall-e-3")
}

func requestGatewayModelsForTest(t *testing.T, h *GatewayHandler, apiKey *service.APIKey) gatewayModelsResponseForTest {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), apiKey)

	h.Models(c)

	require.Equal(t, http.StatusOK, rec.Code)
	var got gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	return got
}

func modelIDsForTest(models []gatewayModelItemForTest) []string {
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	return ids
}
