//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGatewayRecordUsageUsesGroupModelRateMultiplier(t *testing.T) {
	groupID := int64(2601)
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	svc := newGatewayRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{})

	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "gateway_model_rate",
			Usage: ClaudeUsage{
				InputTokens:  1000,
				OutputTokens: 100,
			},
			Model:    "gpt-5.4",
			Duration: time.Second,
		},
		APIKey: &APIKey{
			ID:      501,
			Quota:   100,
			GroupID: i64p(groupID),
			Group: &Group{
				ID:                   groupID,
				RateMultiplier:       0.6,
				ModelRateMultipliers: map[string]float64{"gpt-*": 0.65},
			},
		},
		User:    &User{ID: 601},
		Account: &Account{ID: 701},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.InDelta(t, 0.65, usageRepo.lastLog.RateMultiplier, 1e-12)
	require.InDelta(t, usageRepo.lastLog.TotalCost*0.65, usageRepo.lastLog.ActualCost, 1e-12)
	require.InDelta(t, usageRepo.lastLog.ActualCost, userRepo.lastAmount, 1e-12)
}

func TestOpenAIRecordUsageUsesGroupModelRateMultiplier(t *testing.T) {
	groupID := int64(2602)
	usage := OpenAIUsage{InputTokens: 1000, OutputTokens: 100}
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{}, nil)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "openai_model_rate",
			Usage:     usage,
			Model:     "gpt-5.4",
			Duration:  time.Second,
		},
		APIKey: &APIKey{
			ID:      1001,
			Quota:   100,
			GroupID: i64p(groupID),
			Group: &Group{
				ID:                   groupID,
				RateMultiplier:       0.6,
				ModelRateMultipliers: map[string]float64{"gpt-*": 0.65},
			},
		},
		User:    &User{ID: 2001},
		Account: &Account{ID: 3001},
	})

	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.InDelta(t, 0.65, usageRepo.lastLog.RateMultiplier, 1e-12)
	require.InDelta(t, usageRepo.lastLog.TotalCost*0.65, usageRepo.lastLog.ActualCost, 1e-12)
	require.InDelta(t, usageRepo.lastLog.ActualCost, userRepo.lastAmount, 1e-12)
}

func TestOpenAIRecordUsageModelDefaultsDoNotBleedThroughUserRateCache(t *testing.T) {
	groupID := int64(2604)
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	rateRepo := &openAIUserGroupRateRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{}, rateRepo)
	apiKey := &APIKey{
		ID:      1004,
		Quota:   100,
		GroupID: i64p(groupID),
		Group: &Group{
			ID:             groupID,
			RateMultiplier: 0.65,
			ModelRateMultipliers: map[string]float64{
				"grok-4.5": 0.6,
				"gpt-*":    0.65,
			},
		},
	}
	user := &User{ID: 2004}
	account := &Account{ID: 3004}

	for _, testCase := range []struct {
		model string
		want  float64
	}{
		{model: "grok-4.5", want: 0.6},
		{model: "gpt-5.4", want: 0.65},
	} {
		err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
			Result: &OpenAIForwardResult{
				RequestID: "openai_model_rate_cache_" + testCase.model,
				Usage:     OpenAIUsage{InputTokens: 1000, OutputTokens: 100},
				Model:     testCase.model,
				Duration:  time.Second,
			},
			APIKey:  apiKey,
			User:    user,
			Account: account,
		})
		require.NoError(t, err)
		require.NotNil(t, usageRepo.lastLog)
		require.InDelta(t, testCase.want, usageRepo.lastLog.RateMultiplier, 1e-12)
	}
	require.Equal(t, 1, rateRepo.calls)
}

func TestOpenAIRecordUsageUserRateOverridesGroupModelRate(t *testing.T) {
	groupID := int64(2603)
	userRate := 0.8
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	rateRepo := &openAIUserGroupRateRepoStub{rate: &userRate}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{}, rateRepo)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "openai_user_rate_over_model_rate",
			Usage:     OpenAIUsage{InputTokens: 1000, OutputTokens: 100},
			Model:     "gpt-5.4",
			Duration:  time.Second,
		},
		APIKey: &APIKey{
			ID:      1002,
			Quota:   100,
			GroupID: i64p(groupID),
			Group: &Group{
				ID:                   groupID,
				RateMultiplier:       0.6,
				ModelRateMultipliers: map[string]float64{"gpt-*": 0.65},
			},
		},
		User:    &User{ID: 2002},
		Account: &Account{ID: 3002},
	})

	require.NoError(t, err)
	require.Equal(t, 1, rateRepo.calls)
	require.NotNil(t, usageRepo.lastLog)
	require.InDelta(t, userRate, usageRepo.lastLog.RateMultiplier, 1e-12)
	require.InDelta(t, usageRepo.lastLog.TotalCost*userRate, usageRepo.lastLog.ActualCost, 1e-12)
}
