package service

import "errors"

func (s *OpenAIGatewayService) requireOpenCodeGatewayService() (*OpenCodeGatewayService, error) {
	if s == nil || s.openCodeGatewayService == nil {
		return nil, errors.New("OpenCode Go gateway service is not configured")
	}
	return s.openCodeGatewayService, nil
}

func openCodeForwardResultToOpenAI(result *ForwardResult) *OpenAIForwardResult {
	if result == nil {
		return nil
	}
	return &OpenAIForwardResult{
		RequestID:  result.RequestID,
		ResponseID: result.RequestID,
		Usage: OpenAIUsage{
			InputTokens:              result.Usage.InputTokens,
			OutputTokens:             result.Usage.OutputTokens,
			CacheCreationInputTokens: result.Usage.CacheCreationInputTokens,
			CacheReadInputTokens:     result.Usage.CacheReadInputTokens,
			ImageOutputTokens:        result.Usage.ImageOutputTokens,
		},
		Model:            result.Model,
		BillingModel:     result.BillingModel,
		UpstreamModel:    result.UpstreamModel,
		UpstreamEndpoint: result.UpstreamEndpoint,
		ReasoningEffort:  result.ReasoningEffort,
		Stream:           result.Stream,
		Duration:         result.Duration,
		FirstTokenMs:     result.FirstTokenMs,
		ClientDisconnect: result.ClientDisconnect,
	}
}
