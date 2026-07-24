//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGatewayServiceRecordUsage_StandardGroupWithSubscriptionUsesSubscriptionCost(t *testing.T) {
	groupID := int64(41)
	subscription := &UserSubscription{ID: 501, GroupID: groupID, GroupIDs: []int64{groupID}, QuotaSnapshotted: true}
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
	svc := newGatewayRecordUsageServiceWithBillingRepoForTest(
		usageRepo,
		billingRepo,
		&openAIRecordUsageUserRepoStub{},
		&openAIRecordUsageSubRepoStub{},
	)

	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "gateway_standard_subscription",
			Usage: ClaudeUsage{
				InputTokens:  1000,
				OutputTokens: 500,
			},
			Model:    "claude-sonnet-4",
			Duration: time.Second,
		},
		APIKey: &APIKey{
			ID:      601,
			GroupID: &groupID,
			Group: &Group{
				ID:               groupID,
				SubscriptionType: SubscriptionTypeStandard,
				RateMultiplier:   1,
			},
		},
		User:         &User{ID: 701},
		Account:      &Account{ID: 801},
		Subscription: subscription,
	})

	require.NoError(t, err)
	require.NotNil(t, billingRepo.lastCmd)
	require.Greater(t, billingRepo.lastCmd.SubscriptionCost, 0.0)
	require.Zero(t, billingRepo.lastCmd.BalanceCost)
	require.NotNil(t, billingRepo.lastCmd.SubscriptionID)
	require.Equal(t, subscription.ID, *billingRepo.lastCmd.SubscriptionID)
}

func TestGatewayServiceRecordUsage_StandardGroupWithoutSubscriptionUsesBalanceCost(t *testing.T) {
	groupID := int64(43)
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
	svc := newGatewayRecordUsageServiceWithBillingRepoForTest(
		usageRepo,
		billingRepo,
		&openAIRecordUsageUserRepoStub{},
		&openAIRecordUsageSubRepoStub{},
	)

	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{
			RequestID: "gateway_standard_balance_fallback",
			Usage: ClaudeUsage{
				InputTokens:  1000,
				OutputTokens: 500,
			},
			Model:    "claude-sonnet-4",
			Duration: time.Second,
		},
		APIKey: &APIKey{
			ID:      603,
			GroupID: &groupID,
			Group: &Group{
				ID:               groupID,
				SubscriptionType: SubscriptionTypeStandard,
				RateMultiplier:   1,
			},
		},
		User:    &User{ID: 703},
		Account: &Account{ID: 803},
	})

	require.NoError(t, err)
	require.NotNil(t, billingRepo.lastCmd)
	require.Zero(t, billingRepo.lastCmd.SubscriptionCost)
	require.Greater(t, billingRepo.lastCmd.BalanceCost, 0.0)
	require.Nil(t, billingRepo.lastCmd.SubscriptionID)
}

func TestOpenAIGatewayServiceRecordUsage_StandardGroupWithSubscriptionUsesSubscriptionCost(t *testing.T) {
	groupID := int64(42)
	subscription := &UserSubscription{ID: 502, GroupID: groupID, GroupIDs: []int64{groupID}, QuotaSnapshotted: true}
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(
		usageRepo,
		billingRepo,
		&openAIRecordUsageUserRepoStub{},
		&openAIRecordUsageSubRepoStub{},
		nil,
	)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "openai_standard_subscription",
			Usage: OpenAIUsage{
				InputTokens:  1000,
				OutputTokens: 500,
			},
			Model:    "gpt-5.1",
			Duration: time.Second,
		},
		APIKey: &APIKey{
			ID:      602,
			GroupID: &groupID,
			Group: &Group{
				ID:               groupID,
				SubscriptionType: SubscriptionTypeStandard,
				RateMultiplier:   1,
			},
		},
		User:         &User{ID: 702},
		Account:      &Account{ID: 802},
		Subscription: subscription,
	})

	require.NoError(t, err)
	require.NotNil(t, billingRepo.lastCmd)
	require.Greater(t, billingRepo.lastCmd.SubscriptionCost, 0.0)
	require.Zero(t, billingRepo.lastCmd.BalanceCost)
	require.NotNil(t, billingRepo.lastCmd.SubscriptionID)
	require.Equal(t, subscription.ID, *billingRepo.lastCmd.SubscriptionID)
}

func TestOpenAIGatewayServiceRecordUsage_StandardGroupWithoutSubscriptionUsesBalanceCost(t *testing.T) {
	groupID := int64(44)
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(
		usageRepo,
		billingRepo,
		&openAIRecordUsageUserRepoStub{},
		&openAIRecordUsageSubRepoStub{},
		nil,
	)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "openai_standard_balance_fallback",
			Usage: OpenAIUsage{
				InputTokens:  1000,
				OutputTokens: 500,
			},
			Model:    "gpt-5.1",
			Duration: time.Second,
		},
		APIKey: &APIKey{
			ID:      604,
			GroupID: &groupID,
			Group: &Group{
				ID:               groupID,
				SubscriptionType: SubscriptionTypeStandard,
				RateMultiplier:   1,
			},
		},
		User:    &User{ID: 704},
		Account: &Account{ID: 804},
	})

	require.NoError(t, err)
	require.NotNil(t, billingRepo.lastCmd)
	require.Zero(t, billingRepo.lastCmd.SubscriptionCost)
	require.Greater(t, billingRepo.lastCmd.BalanceCost, 0.0)
	require.Nil(t, billingRepo.lastCmd.SubscriptionID)
}
