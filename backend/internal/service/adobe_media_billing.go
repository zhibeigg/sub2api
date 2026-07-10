package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const (
	adobeMediaSnapshotVersion = 1
	adobePlatformName         = "adobe"
	AdobeMediaUnitImage       = "image"
	AdobeMediaUnitSecond      = "second"
)

var (
	ErrAdobeMediaPricingMissing    = errors.New("adobe media pricing is not configured for the requested tier")
	ErrAdobeMediaSnapshotInvalid   = errors.New("invalid adobe media pricing snapshot")
	ErrAdobeMediaSnapshotConflict  = errors.New("adobe media pricing snapshot hash mismatch")
	ErrAdobeMediaInsufficientFunds = errors.New("insufficient balance or quota for adobe media settlement")
)

// AdobeMediaPricingSnapshot is created only after model/size resolution and is persisted
// with async tasks. Hash is calculated from every other field and makes later settlement
// independent from mutable Group/Channel pricing.
type AdobeMediaPricingSnapshot struct {
	Version     int    `json:"version"`
	Platform    string `json:"platform"`
	BillingMode string `json:"billing_mode"`
	Tier        string `json:"tier"`
	Unit        string `json:"unit"`
	Quantity    int    `json:"quantity"`

	GroupID     int64   `json:"group_id"`
	ChannelID   int64   `json:"channel_id,omitempty"`
	PriceSource string  `json:"price_source"`
	UnitPrice   float64 `json:"unit_price"`

	RequestedModel string `json:"requested_model"`
	ChannelModel   string `json:"channel_model,omitempty"`
	UpstreamModel  string `json:"upstream_model"`

	GroupMultiplier        float64 `json:"group_multiplier"`
	PeakMultiplier         float64 `json:"peak_multiplier"`
	MediaMultiplier        float64 `json:"media_multiplier"`
	AccountMultiplier      float64 `json:"account_multiplier"`
	SubscriptionMultiplier float64 `json:"subscription_multiplier"`

	BaseCost         float64   `json:"base_cost"`
	ActualCost       float64   `json:"actual_cost"`
	QuotaCost        float64   `json:"quota_cost"`
	AccountQuotaCost float64   `json:"account_quota_cost"`
	CreatedAt        time.Time `json:"created_at"`
	Hash             string    `json:"hash"`
}

func (s AdobeMediaPricingSnapshot) canonicalBytes() ([]byte, error) {
	s.Hash = ""
	return json.Marshal(s)
}

func (s AdobeMediaPricingSnapshot) ComputeHash() (string, error) {
	body, err := s.canonicalBytes()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

func adobeCostEqual(a, b float64) bool {
	return math.Abs(a-b) <= 0.000000001*math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
}

func (s *AdobeMediaPricingSnapshot) Seal() error {
	if s == nil {
		return ErrAdobeMediaSnapshotInvalid
	}
	hash, err := s.ComputeHash()
	if err != nil {
		return err
	}
	s.Hash = hash
	return s.Validate()
}

func (s AdobeMediaPricingSnapshot) Validate() error {
	if s.Version != adobeMediaSnapshotVersion || s.Platform != adobePlatformName || s.GroupID <= 0 {
		return ErrAdobeMediaSnapshotInvalid
	}
	if s.BillingMode != string(BillingModeImage) && s.BillingMode != string(BillingModeVideo) {
		return ErrAdobeMediaSnapshotInvalid
	}
	if s.Unit != AdobeMediaUnitImage && s.Unit != AdobeMediaUnitSecond {
		return ErrAdobeMediaSnapshotInvalid
	}
	if s.Quantity <= 0 || strings.TrimSpace(s.Tier) == "" || strings.TrimSpace(s.UpstreamModel) == "" {
		return ErrAdobeMediaSnapshotInvalid
	}
	for _, n := range []float64{s.UnitPrice, s.GroupMultiplier, s.PeakMultiplier, s.MediaMultiplier, s.AccountMultiplier, s.SubscriptionMultiplier, s.BaseCost, s.ActualCost, s.QuotaCost, s.AccountQuotaCost} {
		if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
			return ErrAdobeMediaSnapshotInvalid
		}
	}
	expectedBase := s.UnitPrice * float64(s.Quantity)
	expectedActual := expectedBase * s.MediaMultiplier * s.SubscriptionMultiplier
	expectedAccountQuota := expectedBase * s.AccountMultiplier
	if !adobeCostEqual(s.BaseCost, expectedBase) || !adobeCostEqual(s.ActualCost, expectedActual) ||
		!adobeCostEqual(s.QuotaCost, expectedActual) || !adobeCostEqual(s.AccountQuotaCost, expectedAccountQuota) ||
		!adobeCostEqual(s.PeakMultiplier, 1) {
		return ErrAdobeMediaSnapshotInvalid
	}
	expected, err := s.ComputeHash()
	if err != nil {
		return err
	}
	if strings.TrimSpace(s.Hash) == "" || !strings.EqualFold(expected, s.Hash) {
		return ErrAdobeMediaSnapshotConflict
	}
	return nil
}

type ResolveAdobeMediaPricingInput struct {
	APIKey         *APIKey
	User           *User
	Account        *Account
	Subscription   *UserSubscription
	BillingMode    BillingMode
	Tier           string
	Quantity       int
	RequestedModel string
	ChannelModel   string
	UpstreamModel  string
}

// ResolveAdobeMediaPricingSnapshot rejects missing Group pricing before looking at
// Channel pricing. A matching Adobe Channel row is authoritative and must contain the
// exact tier; otherwise the already-validated Group tier is used.
func (s *OpenAIGatewayService) ResolveAdobeMediaPricingSnapshot(ctx context.Context, in ResolveAdobeMediaPricingInput) (*AdobeMediaPricingSnapshot, error) {
	if in.APIKey == nil || in.APIKey.Group == nil || in.User == nil || in.Account == nil || in.Quantity <= 0 {
		return nil, ErrAdobeMediaSnapshotInvalid
	}
	group := in.APIKey.Group
	if group.Platform != adobePlatformName || group.ID <= 0 {
		return nil, ErrAdobeMediaSnapshotInvalid
	}
	tier, err := normalizeAdobeMediaTier(in.BillingMode, in.Tier)
	if err != nil {
		return nil, err
	}
	groupPrice := adobeGroupTierPrice(group, in.BillingMode, tier)
	if groupPrice == nil {
		return nil, ErrAdobeMediaPricingMissing
	}
	unitPrice := *groupPrice
	priceSource := "group"
	channelID := int64(0)
	channelModel := strings.TrimSpace(in.ChannelModel)
	if channelModel == "" {
		channelModel = strings.TrimSpace(in.RequestedModel)
	}
	if s != nil && s.channelService != nil {
		if pricing := s.channelService.GetChannelModelPricing(ctx, group.ID, channelModel); pricing != nil {
			if pricing.Platform != adobePlatformName || pricing.BillingMode != in.BillingMode {
				return nil, ErrAdobeMediaPricingMissing
			}
			price, ok := adobeChannelTierPrice(pricing, tier)
			if !ok {
				return nil, ErrAdobeMediaPricingMissing
			}
			unitPrice, priceSource, channelID = price, "channel", pricing.ChannelID
		}
	}

	baseMultiplier := group.RateMultiplier
	if s != nil && s.userGroupRateResolver != nil && in.APIKey.GroupID != nil {
		baseMultiplier = s.userGroupRateResolver.Resolve(ctx, in.User.ID, *in.APIKey.GroupID, group.RateMultiplier)
	}
	// Existing media semantics intentionally exclude peak multipliers. Freeze 1 so
	// the snapshot is self-consistent and can be recomputed during settlement.
	peakMultiplier := 1.0
	mediaMultiplier := resolveImageRateMultiplier(in.APIKey, baseMultiplier)
	unit := AdobeMediaUnitImage
	if in.BillingMode == BillingModeVideo {
		mediaMultiplier = resolveVideoRateMultiplier(in.APIKey, baseMultiplier)
		unit = AdobeMediaUnitSecond
	}
	accountMultiplier := in.Account.BillingRateMultiplier()
	subscriptionMultiplier := 1.0
	baseCost := unitPrice * float64(in.Quantity)
	actualCost := baseCost * mediaMultiplier * subscriptionMultiplier

	snapshot := &AdobeMediaPricingSnapshot{
		Version: adobeMediaSnapshotVersion, Platform: adobePlatformName,
		BillingMode: string(in.BillingMode), Tier: tier, Unit: unit, Quantity: in.Quantity,
		GroupID: group.ID, ChannelID: channelID, PriceSource: priceSource, UnitPrice: unitPrice,
		RequestedModel: strings.TrimSpace(in.RequestedModel), ChannelModel: channelModel,
		UpstreamModel: strings.TrimSpace(in.UpstreamModel), GroupMultiplier: baseMultiplier,
		PeakMultiplier: peakMultiplier, MediaMultiplier: mediaMultiplier,
		AccountMultiplier: accountMultiplier, SubscriptionMultiplier: subscriptionMultiplier,
		BaseCost: baseCost, ActualCost: actualCost, QuotaCost: actualCost,
		AccountQuotaCost: baseCost * accountMultiplier, CreatedAt: time.Now().UTC(),
	}
	if err := snapshot.Seal(); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func normalizeAdobeMediaTier(mode BillingMode, tier string) (string, error) {
	if mode == BillingModeImage {
		switch strings.ToUpper(strings.TrimSpace(tier)) {
		case "1K", "2K", "4K":
			return strings.ToUpper(strings.TrimSpace(tier)), nil
		default:
			return "", ErrAdobeMediaPricingMissing
		}
	}
	if mode == BillingModeVideo {
		switch strings.ToLower(strings.TrimSpace(tier)) {
		case VideoBillingResolution720P:
			return VideoBillingResolution720P, nil
		case VideoBillingResolution1080P:
			return VideoBillingResolution1080P, nil
		default:
			return "", ErrAdobeMediaPricingMissing
		}
	}
	return "", ErrAdobeMediaSnapshotInvalid
}

func adobeGroupTierPrice(group *Group, mode BillingMode, tier string) *float64 {
	if group == nil {
		return nil
	}
	if mode == BillingModeImage {
		switch tier {
		case "1K":
			return group.ImagePrice1K
		case "2K":
			return group.ImagePrice2K
		case "4K":
			return group.ImagePrice4K
		}
		return nil
	}
	switch tier {
	case VideoBillingResolution720P:
		return group.VideoPrice720P
	case VideoBillingResolution1080P:
		return group.VideoPrice1080P
	}
	return nil
}

func adobeChannelTierPrice(pricing *ChannelModelPricing, tier string) (float64, bool) {
	if pricing == nil {
		return 0, false
	}
	for _, interval := range pricing.Intervals {
		if strings.EqualFold(strings.TrimSpace(interval.TierLabel), tier) && interval.PerRequestPrice != nil {
			return *interval.PerRequestPrice, true
		}
	}
	return 0, false
}

type RecordMediaUsageFromSnapshotInput struct {
	Snapshot         AdobeMediaPricingSnapshot
	RequestID        string
	APIKey           *APIKey
	User             *User
	Account          *Account
	Subscription     *UserSubscription
	InboundEndpoint  string
	UpstreamEndpoint string
	UserAgent        string
	IPAddress        string
	APIKeyService    APIKeyQuotaUpdater
}

// RecordMediaUsageFromSnapshot is the only Adobe settlement entry point. It never
// resolves mutable pricing and uses the Firefly task ID verbatim for usage and dedup.
func (s *OpenAIGatewayService) RecordMediaUsageFromSnapshot(ctx context.Context, in *RecordMediaUsageFromSnapshotInput) (bool, error) {
	if s == nil || in == nil || in.APIKey == nil || in.User == nil || in.Account == nil {
		return false, ErrAdobeMediaSnapshotInvalid
	}
	if err := in.Snapshot.Validate(); err != nil {
		return false, err
	}
	requestID := strings.TrimSpace(in.RequestID)
	if requestID == "" || in.APIKey.GroupID == nil || *in.APIKey.GroupID != in.Snapshot.GroupID || in.APIKey.UserID != in.User.ID {
		return false, ErrAdobeMediaSnapshotInvalid
	}
	if in.Account.ID <= 0 || in.Account.Platform != adobePlatformName {
		return false, ErrAdobeMediaSnapshotInvalid
	}
	billingType := BillingTypeBalance
	isSubscription := in.Subscription != nil
	if isSubscription {
		billingType = BillingTypeSubscription
	}
	mode := in.Snapshot.BillingMode
	durationMs := 0
	usageLog := &UsageLog{
		UserID: in.User.ID, APIKeyID: in.APIKey.ID, AccountID: in.Account.ID,
		RequestID: requestID, Model: in.Snapshot.RequestedModel,
		RequestedModel:  in.Snapshot.RequestedModel,
		UpstreamModel:   optionalNonEqualStringPtr(in.Snapshot.UpstreamModel, in.Snapshot.RequestedModel),
		InboundEndpoint: optionalTrimmedStringPtr(in.InboundEndpoint), UpstreamEndpoint: optionalTrimmedStringPtr(in.UpstreamEndpoint),
		TotalCost: in.Snapshot.BaseCost, ActualCost: in.Snapshot.ActualCost,
		RateMultiplier: in.Snapshot.MediaMultiplier, AccountRateMultiplier: &in.Snapshot.AccountMultiplier,
		BillingType: billingType, BillingMode: &mode, DurationMs: &durationMs,
		ChannelID: optionalInt64Ptr(in.Snapshot.ChannelID), UserAgent: optionalTrimmedStringPtr(in.UserAgent),
		IPAddress: optionalTrimmedStringPtr(in.IPAddress), GroupID: in.APIKey.GroupID,
		SubscriptionID: optionalSubscriptionID(in.Subscription), CreatedAt: time.Now(),
	}
	if in.Snapshot.BillingMode == string(BillingModeImage) {
		usageLog.ImageCount = in.Snapshot.Quantity
		usageLog.ImageSize = optionalTrimmedStringPtr(in.Snapshot.Tier)
	} else {
		usageLog.VideoCount = 1
		usageLog.VideoResolution = optionalTrimmedStringPtr(in.Snapshot.Tier)
		seconds := in.Snapshot.Quantity
		usageLog.VideoDurationSeconds = &seconds
	}
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		writeUsageLogBestEffort(ctx, s.usageLogRepo, usageLog, "service.adobe_media")
		return true, nil
	}
	if s.usageBillingRepo == nil {
		if err := precheckAdobeSnapshotFunds(in); err != nil {
			return false, err
		}
	}
	cost := &CostBreakdown{TotalCost: in.Snapshot.BaseCost, ActualCost: in.Snapshot.ActualCost, BillingMode: mode}
	applied, err := applyUsageBilling(ctx, requestID, usageLog, &postUsageBillingParams{
		Cost: cost, User: in.User, APIKey: in.APIKey, Account: in.Account, Subscription: in.Subscription,
		RequestPayloadHash: in.Snapshot.Hash, IsSubscriptionBill: isSubscription,
		AccountRateMultiplier: in.Snapshot.AccountMultiplier, APIKeyService: in.APIKeyService, Platform: adobePlatformName,
		StrictFunds: true,
	}, s.billingDeps(), s.usageBillingRepo)
	if err != nil {
		return false, err
	}
	if applied {
		writeUsageLogBestEffort(ctx, s.usageLogRepo, usageLog, "service.adobe_media")
	}
	return applied, nil
}

func precheckAdobeSnapshotFunds(in *RecordMediaUsageFromSnapshotInput) error {
	cost := in.Snapshot.ActualCost
	if cost <= 0 {
		return nil
	}
	if in.Subscription != nil {
		group := in.APIKey.Group
		d, w, m := in.Subscription.CheckAllLimits(group, cost)
		if !d || !w || !m {
			return ErrAdobeMediaInsufficientFunds
		}
	} else if in.User.Balance < cost {
		return ErrAdobeMediaInsufficientFunds
	}
	if in.APIKey.Quota > 0 && in.APIKey.QuotaUsed+cost > in.APIKey.Quota {
		return ErrAdobeMediaInsufficientFunds
	}
	return nil
}

func AdobeMediaSnapshotIntegrityError(taskID string, expected, actual AdobeMediaPricingSnapshot) error {
	return fmt.Errorf("%w: task=%s expected=%s actual=%s", ErrAdobeMediaSnapshotConflict, strings.TrimSpace(taskID), expected.Hash, actual.Hash)
}
