package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrAdobeMediaUpstreamUnavailable = errors.New("adobe media upstream is unavailable")
	ErrAdobeMediaSettlementTemporary = errors.New("adobe media settlement temporarily unavailable")
)

type AdobeVideoSubmitRequest struct {
	Model           string
	Prompt          string
	Resolution      string
	DurationSeconds int
	AspectRatio     string
	GenerateAudio   bool
	ReferenceMode   string
	ReferenceAssets []string
}

type AdobeVideoSubmitResult struct {
	TaskID  string
	PollURL string
	Status  AdobeVideoTaskStatus
}

type AdobeVideoPollResult struct {
	Status       AdobeVideoTaskStatus
	ResultURLs   []string
	Raw          json.RawMessage
	ErrorMessage string
}

// AdobeVideoUpstream is the intentionally small adapter boundary for the Firefly
// provider. The provider owns payload specifics and auth-refresh retry; orchestration
// owns identity, Redis durability and billing.
type AdobeVideoUpstream interface {
	SubmitVideo(ctx context.Context, account *Account, request AdobeVideoSubmitRequest) (*AdobeVideoSubmitResult, error)
	PollVideo(ctx context.Context, account *Account, pollURL string) (*AdobeVideoPollResult, error)
}

type AdobeVideoService struct {
	upstream         AdobeVideoUpstream
	store            AdobeVideoTaskStore
	gateway          *OpenAIGatewayService
	accountRepo      AccountRepository
	apiKeyRepo       APIKeyRepository
	userRepo         UserRepository
	subscriptionRepo UserSubscriptionRepository
}

func NewAdobeVideoService(upstream AdobeVideoUpstream, store AdobeVideoTaskStore, gateway *OpenAIGatewayService, accountRepo AccountRepository, apiKeyRepo APIKeyRepository, userRepo UserRepository, subscriptionRepo UserSubscriptionRepository) *AdobeVideoService {
	return &AdobeVideoService{upstream: upstream, store: store, gateway: gateway, accountRepo: accountRepo, apiKeyRepo: apiKeyRepo, userRepo: userRepo, subscriptionRepo: subscriptionRepo}
}

func (s *AdobeVideoService) Preflight(ctx context.Context) error {
	if s == nil || s.store == nil {
		return ErrAdobeMediaUpstreamUnavailable
	}
	if err := s.store.Healthy(ctx); err != nil {
		return fmt.Errorf("redis preflight: %w", err)
	}
	return nil
}

type SubmitAdobeVideoInput struct {
	Account      *Account
	APIKey       *APIKey
	User         *User
	Subscription *UserSubscription
	Request      AdobeVideoSubmitRequest
	Snapshot     AdobeMediaPricingSnapshot
}

func (s *AdobeVideoService) Submit(ctx context.Context, in *SubmitAdobeVideoInput) (*AdobeVideoTask, error) {
	if s == nil || s.upstream == nil || s.store == nil || in == nil || in.Account == nil || in.APIKey == nil || in.User == nil {
		return nil, ErrAdobeMediaUpstreamUnavailable
	}
	if err := in.Snapshot.Validate(); err != nil {
		return nil, err
	}
	if err := s.Preflight(ctx); err != nil {
		return nil, err
	}
	result, err := s.upstream.SubmitVideo(ctx, in.Account, in.Request)
	if err != nil {
		return nil, err
	}
	if result == nil || strings.TrimSpace(result.TaskID) == "" || strings.TrimSpace(result.PollURL) == "" {
		return nil, ErrAdobeMediaUpstreamUnavailable
	}
	task := &AdobeVideoTask{
		TaskID: result.TaskID, PollURL: result.PollURL, AccountID: in.Account.ID,
		UserID: in.User.ID, APIKeyID: in.APIKey.ID, GroupID: in.Snapshot.GroupID,
		RequestedModel: in.Snapshot.RequestedModel, ChannelModel: in.Snapshot.ChannelModel,
		UpstreamModel: in.Snapshot.UpstreamModel, Resolution: in.Snapshot.Tier,
		DurationSeconds: in.Snapshot.Quantity, ReferenceMode: in.Request.ReferenceMode,
		PricingSnapshot: in.Snapshot, SnapshotHash: in.Snapshot.Hash,
		Status: result.Status, SettlementStatus: AdobeVideoSettlementPending,
	}
	if in.Subscription != nil {
		id := in.Subscription.ID
		task.SubscriptionID = &id
	}
	if task.Status == "" {
		task.Status = AdobeVideoTaskPending
	}
	if err := s.store.Create(ctx, task); err != nil {
		// The task already exists upstream. Caller must return 503 and emit an orphan Ops event.
		return task, fmt.Errorf("persist submitted adobe task: %w", err)
	}
	return task, nil
}

type AdobeVideoStatusResult struct {
	TaskID  string               `json:"request_id"`
	Status  AdobeVideoTaskStatus `json:"status"`
	Model   string               `json:"model"`
	URLs    []string             `json:"urls,omitempty"`
	Error   string               `json:"error,omitempty"`
	Settled bool                 `json:"settled"`
}

type GetAdobeVideoStatusInput struct {
	TaskID           string
	CurrentUserID    int64
	CurrentGroupID   int64
	InboundEndpoint  string
	UpstreamEndpoint string
	UserAgent        string
	IPAddress        string
	APIKeyService    APIKeyQuotaUpdater
}

func (s *AdobeVideoService) GetStatus(ctx context.Context, in *GetAdobeVideoStatusInput) (*AdobeVideoStatusResult, error) {
	if s == nil || s.store == nil || s.gateway == nil || in == nil {
		return nil, ErrAdobeMediaUpstreamUnavailable
	}
	task, err := s.store.Get(ctx, in.TaskID)
	if err != nil {
		return nil, err
	}
	if task.UserID != in.CurrentUserID || task.GroupID != in.CurrentGroupID {
		return nil, ErrAdobeVideoTaskOwnerMismatch
	}
	if task.Status == AdobeVideoTaskCompleted {
		return s.settleAndRender(ctx, task, in)
	}
	if task.Status == AdobeVideoTaskFailed || task.Status == AdobeVideoTaskCanceled {
		return renderAdobeVideoTask(task), nil
	}

	account, err := s.accountRepo.GetByID(ctx, task.AccountID)
	if err != nil {
		return nil, err
	}
	poll, err := s.upstream.PollVideo(ctx, account, task.PollURL)
	if err != nil {
		return nil, err
	}
	if poll == nil {
		return nil, ErrAdobeMediaUpstreamUnavailable
	}
	task.Status = poll.Status
	task.UpstreamResponse = cloneRawJSON(poll.Raw)
	task.LastError = strings.TrimSpace(poll.ErrorMessage)
	if poll.Status == AdobeVideoTaskCompleted {
		task.ResultURLs = nonEmptyAdobeResultURLs(poll.ResultURLs)
		if len(task.ResultURLs) == 0 {
			task.Status = AdobeVideoTaskFailed
			if task.LastError == "" {
				task.LastError = "generation completed without an output URL"
			}
		}
	}
	if err := s.store.Update(ctx, task); err != nil {
		return nil, err
	}
	if task.Status == AdobeVideoTaskCompleted {
		return s.settleAndRender(ctx, task, in)
	}
	return renderAdobeVideoTask(task), nil
}

func (s *AdobeVideoService) settleAndRender(ctx context.Context, task *AdobeVideoTask, in *GetAdobeVideoStatusInput) (*AdobeVideoStatusResult, error) {
	if task.CanExposeResult() {
		return renderAdobeVideoTask(task), nil
	}
	unlock, err := s.store.AcquireSettlementLock(ctx, task.TaskID, 15*time.Second)
	if errors.Is(err, ErrAdobeVideoTaskSettlementLocked) {
		// Another poll is settling. Do not leak result URLs; caller may retry.
		return nil, ErrAdobeMediaSettlementTemporary
	}
	if err != nil {
		return nil, ErrAdobeMediaSettlementTemporary
	}
	defer func() { _ = unlock(context.Background()) }()

	fresh, err := s.store.Get(ctx, task.TaskID)
	if err != nil {
		return nil, err
	}
	if fresh.CanExposeResult() {
		return renderAdobeVideoTask(fresh), nil
	}
	apiKey, err := s.apiKeyRepo.GetByID(ctx, fresh.APIKeyID)
	if err != nil {
		return nil, err
	}
	user, err := s.userRepo.GetByID(ctx, fresh.UserID)
	if err != nil {
		return nil, err
	}
	account, err := s.accountRepo.GetByID(ctx, fresh.AccountID)
	if err != nil {
		return nil, err
	}
	var subscription *UserSubscription
	if fresh.SubscriptionID != nil {
		subscription, err = s.subscriptionRepo.GetByID(ctx, *fresh.SubscriptionID)
		if err != nil {
			return nil, err
		}
	}
	applied, err := s.gateway.RecordMediaUsageFromSnapshot(ctx, &RecordMediaUsageFromSnapshotInput{
		Snapshot: fresh.PricingSnapshot, RequestID: fresh.TaskID, APIKey: apiKey, User: user,
		Account: account, Subscription: subscription, InboundEndpoint: in.InboundEndpoint,
		UpstreamEndpoint: in.UpstreamEndpoint, UserAgent: in.UserAgent, IPAddress: in.IPAddress,
		APIKeyService: in.APIKeyService,
	})
	if err != nil {
		fresh.SettlementStatus = AdobeVideoSettlementFailed
		fresh.LastError = safeAdobeSettlementError(err)
		_ = s.store.Update(ctx, fresh)
		if errors.Is(err, ErrAdobeMediaInsufficientFunds) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", ErrAdobeMediaSettlementTemporary, err)
	}
	_ = applied // false is a valid dedup hit.
	now := time.Now().UTC()
	fresh.SettlementStatus = AdobeVideoSettlementSettled
	fresh.SettledAt = &now
	fresh.LastError = ""
	if err := s.store.Update(ctx, fresh); err != nil {
		return nil, ErrAdobeMediaSettlementTemporary
	}
	return renderAdobeVideoTask(fresh), nil
}

func renderAdobeVideoTask(task *AdobeVideoTask) *AdobeVideoStatusResult {
	out := &AdobeVideoStatusResult{TaskID: task.TaskID, Status: task.Status, Model: task.RequestedModel, Settled: task.SettlementStatus == AdobeVideoSettlementSettled}
	if task.CanExposeResult() {
		out.URLs = append([]string(nil), task.ResultURLs...)
	}
	if task.Status == AdobeVideoTaskFailed || task.Status == AdobeVideoTaskCanceled {
		out.Error = task.LastError
	}
	return out
}

func cloneRawJSON(in json.RawMessage) json.RawMessage { return append(json.RawMessage(nil), in...) }

func nonEmptyAdobeResultURLs(urls []string) []string {
	out := make([]string, 0, len(urls))
	for _, raw := range urls {
		if value := strings.TrimSpace(raw); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func safeAdobeSettlementError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	if len(msg) > 256 {
		msg = msg[:256]
	}
	return msg
}
