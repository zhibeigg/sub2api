package handler

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type multiGroupEntryOrderCase struct {
	file              string
	function          string
	modelToken        string
	resolutionToken   string
	concurrencyTokens []string
}

func TestMultiGroupEntryResolvesFinalContextBeforePolicyAndConcurrency(t *testing.T) {
	tests := []multiGroupEntryOrderCase{
		{file: "gateway_handler_chat_completions.go", function: "ChatCompletions", modelToken: "reqModel :=", resolutionToken: "resolveMultiGroupAPIKey(", concurrencyTokens: []string{"AcquireUserSlotWithWait("}},
		{file: "gateway_handler_responses.go", function: "Responses", modelToken: "reqModel :=", resolutionToken: "resolveMultiGroupAPIKey(", concurrencyTokens: []string{"AcquireUserSlotWithWait("}},
		{file: "openai_embeddings.go", function: "Embeddings", modelToken: "reqModel :=", resolutionToken: "resolveMultiGroupAPIKey(", concurrencyTokens: []string{"acquireResponsesUserSlot("}},
		{file: "openai_alpha_search.go", function: "AlphaSearch", modelToken: "requestedModel :=", resolutionToken: "resolveMultiGroupAPIKey(", concurrencyTokens: []string{"acquireResponsesUserSlot("}},
		{file: "ark_video.go", function: "handleArkVideo", modelToken: "requestModel :=", resolutionToken: "resolveMultiGroupAPIKey(", concurrencyTokens: []string{"acquireImageGenerationSlot(", "acquireResponsesUserSlot("}},
		{file: "grok_media.go", function: "handleGrokMedia", modelToken: "requestModel :=", resolutionToken: "resolveMultiGroupAPIKey(", concurrencyTokens: []string{"acquireImageGenerationSlot(", "acquireResponsesUserSlot("}},
		{file: "gemini_v1beta_handler.go", function: "GeminiV1BetaModels", modelToken: "modelName, action", resolutionToken: "resolveMultiGroupAPIKey(", concurrencyTokens: []string{"AcquireUserSlotWithWait("}},
		{file: "adobe_media.go", function: "Images", modelToken: "requestedModel :=", resolutionToken: "resolveMultiGroupAPIKey(", concurrencyTokens: []string{"acquireUserSlot("}},
		{file: "adobe_media.go", function: "VideoGeneration", modelToken: "requestedModel :=", resolutionToken: "resolveMultiGroupAPIKey(", concurrencyTokens: []string{"acquireUserSlot("}},
		{file: "openai_images.go", function: "Images", modelToken: "requestModel :=", resolutionToken: "resolveAndApplyImageGroup(", concurrencyTokens: []string{"acquireImageGenerationSlot(", "acquireResponsesUserSlot("}},
	}
	policyAndBillingTokens := []string{
		"checkSecurityAudit(",
		"checkContentModeration(",
		"CheckBillingEligibility(",
		"SelectAccount",
	}

	for _, tt := range tests {
		t.Run(tt.file+"/"+tt.function, func(t *testing.T) {
			source := stripGoComments(goFunctionSource(t, tt.file, tt.function))
			modelIndex := strings.Index(source, tt.modelToken)
			resolveIndex := strings.Index(source, tt.resolutionToken)
			require.NotEqual(t, -1, modelIndex, "missing final request model parse")
			require.NotEqual(t, -1, resolveIndex, "missing multi-group final context resolution")
			require.Less(t, modelIndex, resolveIndex, "multi-group resolution must use the parsed request model")

			for _, token := range append(policyAndBillingTokens, tt.concurrencyTokens...) {
				index := strings.Index(source, token)
				if index >= 0 {
					require.Lessf(t, resolveIndex, index, "%s must run before %s", tt.resolutionToken, token)
				}
			}
			for _, token := range tt.concurrencyTokens {
				require.Contains(t, source, token, "coverage case must include its real concurrency entry")
			}
		})
	}
}

func TestOpenAIImagesUsesSharedResolvedContextApplication(t *testing.T) {
	source := stripGoComments(goFunctionSource(t, "openai_images.go", "resolveAndApplyImageGroup"))
	require.Contains(t, source, "applyResolvedAPIKeyContext(")
	require.NotContains(t, source, ".GetActiveSubscription(", "image routing must not reintroduce divergent subscription lookup semantics")
}
