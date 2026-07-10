package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type adobeVideoStoreStub struct {
	mu                   sync.Mutex
	task                 *AdobeVideoTask
	healthyErr           error
	createErr            error
	updateErr            error
	failSettledUpdates   int
	healthyCalls         int
	createCalls          int
	updateCalls          int
	settlementLockActive bool
}

func cloneAdobeVideoTaskForTest(task *AdobeVideoTask) *AdobeVideoTask {
	if task == nil {
		return nil
	}
	body, _ := json.Marshal(task)
	var cloned AdobeVideoTask
	_ = json.Unmarshal(body, &cloned)
	return &cloned
}

func (s *adobeVideoStoreStub) Healthy(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.healthyCalls++
	return s.healthyErr
}

func (s *adobeVideoStoreStub) Create(_ context.Context, task *AdobeVideoTask) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.createCalls++
	if s.createErr != nil {
		return s.createErr
	}
	s.task = cloneAdobeVideoTaskForTest(task)
	return nil
}

func (s *adobeVideoStoreStub) Get(_ context.Context, taskID string) (*AdobeVideoTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.task == nil || s.task.TaskID != taskID {
		return nil, ErrAdobeVideoTaskNotFound
	}
	return cloneAdobeVideoTaskForTest(s.task), nil
}

func (s *adobeVideoStoreStub) Update(_ context.Context, task *AdobeVideoTask) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateCalls++
	if s.updateErr != nil {
		return s.updateErr
	}
	if task.SettlementStatus == AdobeVideoSettlementSettled && s.failSettledUpdates > 0 {
		s.failSettledUpdates--
		return errors.New("redis write failed")
	}
	s.task = cloneAdobeVideoTaskForTest(task)
	return nil
}

func (s *adobeVideoStoreStub) AcquireSettlementLock(_ context.Context, _ string, _ time.Duration) (func(context.Context) error, error) {
	s.mu.Lock()
	if s.settlementLockActive {
		s.mu.Unlock()
		return nil, ErrAdobeVideoTaskSettlementLocked
	}
	s.settlementLockActive = true
	s.mu.Unlock()
	return func(context.Context) error {
		s.mu.Lock()
		s.settlementLockActive = false
		s.mu.Unlock()
		return nil
	}, nil
}

type adobeVideoUpstreamStub struct {
	submitResult *AdobeVideoSubmitResult
	submitErr    error
	pollResults  []*AdobeVideoPollResult
	pollErr      error
	submitCalls  int
	pollCalls    int
	lastRequest  AdobeVideoSubmitRequest
	lastPollURL  string
}

func (s *adobeVideoUpstreamStub) SubmitVideo(_ context.Context, _ *Account, request AdobeVideoSubmitRequest) (*AdobeVideoSubmitResult, error) {
	s.submitCalls++
	s.lastRequest = request
	return s.submitResult, s.submitErr
}

func (s *adobeVideoUpstreamStub) PollVideo(_ context.Context, _ *Account, pollURL string) (*AdobeVideoPollResult, error) {
	s.pollCalls++
	s.lastPollURL = pollURL
	if s.pollErr != nil {
		return nil, s.pollErr
	}
	if len(s.pollResults) == 0 {
		return &AdobeVideoPollResult{Status: AdobeVideoTaskProcessing}, nil
	}
	result := s.pollResults[0]
	if len(s.pollResults) > 1 {
		s.pollResults = s.pollResults[1:]
	}
	return result, nil
}

type adobeVideoAccountRepoStub struct {
	AccountRepository
	account *Account
}

func (s *adobeVideoAccountRepoStub) GetByID(context.Context, int64) (*Account, error) {
	return s.account, nil
}

type adobeVideoAPIKeyRepoStub struct {
	APIKeyRepository
	apiKey *APIKey
}

func (s *adobeVideoAPIKeyRepoStub) GetByID(context.Context, int64) (*APIKey, error) {
	return s.apiKey, nil
}

type adobeVideoUserRepoStub struct {
	UserRepository
	user *User
}

func (s *adobeVideoUserRepoStub) GetByID(context.Context, int64) (*User, error) {
	return s.user, nil
}

type adobeVideoSubscriptionRepoStub struct {
	UserSubscriptionRepository
	subscription *UserSubscription
}

func (s *adobeVideoSubscriptionRepoStub) GetByID(context.Context, int64) (*UserSubscription, error) {
	return s.subscription, nil
}

type adobeVideoServiceFixture struct {
	service     *AdobeVideoService
	store       *adobeVideoStoreStub
	upstream    *adobeVideoUpstreamStub
	usageRepo   *openAIRecordUsageLogRepoStub
	billingRepo *openAIRecordUsageBillingRepoStub
	account     *Account
	apiKey      *APIKey
	user        *User
}

func newAdobeVideoServiceFixture(t *testing.T, task *AdobeVideoTask) *adobeVideoServiceFixture {
	t.Helper()
	if task == nil {
		task = newAdobeVideoTaskForTest(t)
	}
	groupID := task.GroupID
	account := &Account{ID: task.AccountID, Platform: PlatformAdobe, Type: AccountTypeOAuth, Status: StatusDisabled}
	user := &User{ID: task.UserID, Balance: 100}
	apiKey := &APIKey{ID: task.APIKeyID, UserID: user.ID, GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformAdobe, RateMultiplier: 1}}
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
	gateway := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	store := &adobeVideoStoreStub{task: cloneAdobeVideoTaskForTest(task)}
	upstream := &adobeVideoUpstreamStub{}
	service := NewAdobeVideoService(
		upstream,
		store,
		gateway,
		&adobeVideoAccountRepoStub{account: account},
		&adobeVideoAPIKeyRepoStub{apiKey: apiKey},
		&adobeVideoUserRepoStub{user: user},
		&adobeVideoSubscriptionRepoStub{},
	)
	return &adobeVideoServiceFixture{service: service, store: store, upstream: upstream, usageRepo: usageRepo, billingRepo: billingRepo, account: account, apiKey: apiKey, user: user}
}

func TestAdobeVideoServiceSubmitPreflightsRedisAndDoesNotBill(t *testing.T) {
	taskTemplate := newAdobeVideoTaskForTest(t)
	fixture := newAdobeVideoServiceFixture(t, nil)
	fixture.store.task = nil
	fixture.store.healthyErr = errors.New("redis unavailable")
	fixture.upstream.submitResult = &AdobeVideoSubmitResult{TaskID: taskTemplate.TaskID, PollURL: taskTemplate.PollURL, Status: AdobeVideoTaskPending}
	input := &SubmitAdobeVideoInput{
		Account:  fixture.account,
		APIKey:   fixture.apiKey,
		User:     fixture.user,
		Snapshot: taskTemplate.PricingSnapshot,
		Request:  AdobeVideoSubmitRequest{Model: "veo3", Prompt: "hello", Resolution: VideoBillingResolution720P, DurationSeconds: 5, ReferenceAssets: []string{"asset-1"}},
	}

	_, err := fixture.service.Submit(context.Background(), input)
	require.ErrorContains(t, err, "redis preflight")
	require.Zero(t, fixture.upstream.submitCalls)
	require.Zero(t, fixture.billingRepo.calls)
	require.Zero(t, fixture.usageRepo.calls)

	fixture.store.healthyErr = nil
	submitted, err := fixture.service.Submit(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, taskTemplate.TaskID, submitted.TaskID)
	require.Equal(t, []string{"asset-1"}, fixture.upstream.lastRequest.ReferenceAssets)
	require.Equal(t, 1, fixture.upstream.submitCalls)
	require.Equal(t, 1, fixture.store.createCalls)
	require.Zero(t, fixture.billingRepo.calls)
	require.Zero(t, fixture.usageRepo.calls)
}

func TestAdobeVideoServiceOwnerAndFailedPollDoNotSettle(t *testing.T) {
	fixture := newAdobeVideoServiceFixture(t, nil)
	_, err := fixture.service.GetStatus(context.Background(), &GetAdobeVideoStatusInput{TaskID: fixture.store.task.TaskID, CurrentUserID: fixture.user.ID + 1, CurrentGroupID: fixture.store.task.GroupID})
	require.ErrorIs(t, err, ErrAdobeVideoTaskOwnerMismatch)
	require.Zero(t, fixture.upstream.pollCalls)

	fixture.upstream.pollResults = []*AdobeVideoPollResult{{Status: AdobeVideoTaskFailed, ErrorMessage: "generation failed"}}
	result, err := fixture.service.GetStatus(context.Background(), &GetAdobeVideoStatusInput{TaskID: fixture.store.task.TaskID, CurrentUserID: fixture.user.ID, CurrentGroupID: fixture.store.task.GroupID})
	require.NoError(t, err)
	require.Equal(t, AdobeVideoTaskFailed, result.Status)
	require.Equal(t, "generation failed", result.Error)
	require.Equal(t, 1, fixture.upstream.pollCalls)
	require.Zero(t, fixture.billingRepo.calls)
	require.Zero(t, fixture.usageRepo.calls)
}

func TestAdobeVideoServiceCompletedWithoutOutputDoesNotSettle(t *testing.T) {
	fixture := newAdobeVideoServiceFixture(t, nil)
	fixture.upstream.pollResults = []*AdobeVideoPollResult{{Status: AdobeVideoTaskCompleted}}
	input := &GetAdobeVideoStatusInput{TaskID: fixture.store.task.TaskID, CurrentUserID: fixture.user.ID, CurrentGroupID: fixture.store.task.GroupID}

	result, err := fixture.service.GetStatus(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, AdobeVideoTaskFailed, result.Status)
	require.Contains(t, result.Error, "without an output URL")
	require.False(t, result.Settled)
	require.Empty(t, result.URLs)
	require.Zero(t, fixture.billingRepo.calls)
	require.Zero(t, fixture.usageRepo.calls)
}

func TestAdobeVideoServiceCompletedPollSettlesOnceAndCachesResult(t *testing.T) {
	fixture := newAdobeVideoServiceFixture(t, nil)
	fixture.upstream.pollResults = []*AdobeVideoPollResult{{Status: AdobeVideoTaskCompleted, ResultURLs: []string{"https://cdn.example/result.mp4"}}}
	input := &GetAdobeVideoStatusInput{TaskID: fixture.store.task.TaskID, CurrentUserID: fixture.user.ID, CurrentGroupID: fixture.store.task.GroupID}

	result, err := fixture.service.GetStatus(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, AdobeVideoTaskCompleted, result.Status)
	require.True(t, result.Settled)
	require.Equal(t, []string{"https://cdn.example/result.mp4"}, result.URLs)
	require.Equal(t, 1, fixture.upstream.pollCalls)
	require.Equal(t, 1, fixture.billingRepo.calls)
	require.Equal(t, 1, fixture.usageRepo.calls)
	require.Equal(t, fixture.store.task.TaskID, fixture.usageRepo.lastLog.RequestID)

	result, err = fixture.service.GetStatus(context.Background(), input)
	require.NoError(t, err)
	require.True(t, result.Settled)
	require.Equal(t, 1, fixture.upstream.pollCalls, "settled retries must not poll upstream")
	require.Equal(t, 1, fixture.billingRepo.calls, "settled retries must not bill again")
	require.Equal(t, 1, fixture.usageRepo.calls)
}

func TestAdobeVideoServiceDedupRetryAfterSettlementCacheWriteFailure(t *testing.T) {
	task := newAdobeVideoTaskForTest(t)
	task.Status = AdobeVideoTaskCompleted
	task.ResultURLs = []string{"https://cdn.example/result.mp4"}
	fixture := newAdobeVideoServiceFixture(t, task)
	fixture.store.failSettledUpdates = 1
	input := &GetAdobeVideoStatusInput{TaskID: task.TaskID, CurrentUserID: fixture.user.ID, CurrentGroupID: task.GroupID}

	_, err := fixture.service.GetStatus(context.Background(), input)
	require.ErrorIs(t, err, ErrAdobeMediaSettlementTemporary)
	require.Equal(t, 1, fixture.billingRepo.calls)
	require.Equal(t, 1, fixture.usageRepo.calls)
	require.Zero(t, fixture.upstream.pollCalls, "cached completed tasks must not poll upstream")

	fixture.billingRepo.result = &UsageBillingApplyResult{Applied: false}
	fixture.user.Balance = 0 // already charged: dedup retry must not require funds again
	result, err := fixture.service.GetStatus(context.Background(), input)
	require.NoError(t, err)
	require.True(t, result.Settled)
	require.Equal(t, []string{"https://cdn.example/result.mp4"}, result.URLs)
	require.Equal(t, 2, fixture.billingRepo.calls, "retry reaches database dedup")
	require.Equal(t, 1, fixture.usageRepo.calls, "dedup hit must not create a second usage row")

	_, err = fixture.service.GetStatus(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 2, fixture.billingRepo.calls)
	require.Equal(t, 1, fixture.usageRepo.calls)
}
