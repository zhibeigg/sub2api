package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveAdobeMediaPricingSnapshot_MissingTierRejectedAndZeroAllowed(t *testing.T) {
	groupID := int64(9)
	zero := 0.0
	apiKey := &APIKey{ID: 3, UserID: 4, GroupID: &groupID, Group: &Group{ID: groupID, Platform: adobePlatformName, RateMultiplier: 1, ImagePrice1K: &zero}}
	account := &Account{ID: 5, Platform: adobePlatformName}
	svc := &OpenAIGatewayService{}

	_, err := svc.ResolveAdobeMediaPricingSnapshot(context.Background(), ResolveAdobeMediaPricingInput{
		APIKey: apiKey, User: &User{ID: 4}, Account: account, BillingMode: BillingModeImage,
		Tier: "2K", Quantity: 1, RequestedModel: "nano-banana", UpstreamModel: "firefly-image",
	})
	require.ErrorIs(t, err, ErrAdobeMediaPricingMissing)
	_, err = svc.ResolveAdobeMediaPricingSnapshot(context.Background(), ResolveAdobeMediaPricingInput{
		APIKey: apiKey, User: &User{ID: 4}, Account: account, BillingMode: BillingModeVideo,
		Tier: VideoBillingResolution480P, Quantity: 4, RequestedModel: "veo3", UpstreamModel: "veo3",
	})
	require.ErrorIs(t, err, ErrAdobeMediaPricingMissing)

	snapshot, err := svc.ResolveAdobeMediaPricingSnapshot(context.Background(), ResolveAdobeMediaPricingInput{
		APIKey: apiKey, User: &User{ID: 4}, Account: account, BillingMode: BillingModeImage,
		Tier: "1K", Quantity: 1, RequestedModel: "nano-banana", UpstreamModel: "firefly-image",
	})
	require.NoError(t, err)
	require.Zero(t, snapshot.UnitPrice)
	require.Zero(t, snapshot.ActualCost)
	require.NoError(t, snapshot.Validate())
}

func TestAdobeMediaPricingSnapshot_TamperDetected(t *testing.T) {
	snapshot := AdobeMediaPricingSnapshot{
		Version: adobeMediaSnapshotVersion, Platform: adobePlatformName, BillingMode: string(BillingModeVideo),
		Tier: VideoBillingResolution720P, Unit: AdobeMediaUnitSecond, Quantity: 5,
		GroupID: 1, PriceSource: "group", UnitPrice: 0.1, RequestedModel: "veo3",
		UpstreamModel: "firefly-video", GroupMultiplier: 1, PeakMultiplier: 1,
		MediaMultiplier: 1, AccountMultiplier: 1, SubscriptionMultiplier: 1,
		BaseCost: 0.5, ActualCost: 0.5, QuotaCost: 0.5, AccountQuotaCost: 0.5,
	}
	require.NoError(t, snapshot.Seal())
	snapshot.ActualCost = 0.1
	require.ErrorIs(t, snapshot.Validate(), ErrAdobeMediaSnapshotInvalid)
}

func TestRecordMediaUsageFromSnapshot_DedupHitDoesNotWriteDuplicateUsage(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	groupID := int64(12)
	zero := 0.0
	apiKey := &APIKey{ID: 3, UserID: 4, GroupID: &groupID, Group: &Group{ID: groupID, Platform: adobePlatformName, RateMultiplier: 1, VideoPrice720P: &zero}}
	user := &User{ID: 4, Balance: 100}
	account := &Account{ID: 5, Platform: adobePlatformName}
	snapshot, err := svc.ResolveAdobeMediaPricingSnapshot(context.Background(), ResolveAdobeMediaPricingInput{APIKey: apiKey, User: user, Account: account, BillingMode: BillingModeVideo, Tier: VideoBillingResolution720P, Quantity: 4, RequestedModel: "veo3", UpstreamModel: "veo3"})
	require.NoError(t, err)
	input := &RecordMediaUsageFromSnapshotInput{Snapshot: *snapshot, RequestID: "raw-firefly-task", APIKey: apiKey, User: user, Account: account}
	applied, err := svc.RecordMediaUsageFromSnapshot(context.Background(), input)
	require.NoError(t, err)
	require.True(t, applied)
	require.Equal(t, 1, usageRepo.calls)
	require.Equal(t, "raw-firefly-task", usageRepo.lastLog.RequestID)

	billingRepo.result = &UsageBillingApplyResult{Applied: false}
	applied, err = svc.RecordMediaUsageFromSnapshot(context.Background(), input)
	require.NoError(t, err)
	require.False(t, applied)
	require.Equal(t, 1, usageRepo.calls)
	require.Equal(t, "raw-firefly-task", billingRepo.lastCmd.RequestID)
	require.Equal(t, snapshot.Hash, billingRepo.lastCmd.RequestPayloadHash)
}

func TestIsVideoUsageResult_DoesNotRequireGrokModel(t *testing.T) {
	require.True(t, isVideoUsageResult(&OpenAIForwardResult{Model: "veo3", VideoCount: 1}))
	require.False(t, isVideoUsageResult(&OpenAIForwardResult{Model: "grok-imagine-video", VideoCount: 0}))
}
