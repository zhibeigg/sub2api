package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type batchImageResolveAccountRepoStub struct {
	service.AccountRepository
}

func (s *batchImageResolveAccountRepoStub) ListSchedulableByGroupIDAndPlatform(context.Context, int64, string) ([]service.Account, error) {
	return nil, nil
}

type batchImageSubscriptionRepoStub struct {
	service.UserSubscriptionRepository

	subscription *service.UserSubscription
	err          error
	calls        int
	userID       int64
	groupID      int64
}

func (s *batchImageSubscriptionRepoStub) GetActiveByUserIDAndGroupID(_ context.Context, userID, groupID int64) (*service.UserSubscription, error) {
	s.calls++
	s.userID = userID
	s.groupID = groupID
	return s.subscription, s.err
}

func newBatchImageOpenAIHandlerForResolveTest(subRepo service.UserSubscriptionRepository, coordinator *securityaudit.Coordinator) *OpenAIGatewayHandler {
	accountRepo := &batchImageResolveAccountRepoStub{}
	return &OpenAIGatewayHandler{
		gatewayService: service.NewOpenAIGatewayService(
			accountRepo,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		),
		subscriptionService:      service.NewSubscriptionService(nil, subRepo, nil, nil, nil),
		securityAuditCoordinator: coordinator,
	}
}

func newBatchImageSubmitContext(t *testing.T, apiKey *service.APIKey) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/batches", strings.NewReader(`{
		"model":"gemini-image-test",
		"items":[{"custom_id":"one","prompt":"draw one image"}]
	}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware2.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: apiKey.UserID, Concurrency: 1})
	return c, recorder
}

func batchImageResponseErrorCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	return body.Error.Code
}

func TestBatchImageSubmitRejectsFinalMultiGroupStandardQuotaBeforeService(t *testing.T) {
	originalGroup := &service.Group{ID: 11, SubscriptionType: service.SubscriptionTypeStandard}
	finalGroup := &service.Group{ID: 22, SubscriptionType: service.SubscriptionTypeStandard}
	user := &service.User{ID: 7}
	apiKey := &service.APIKey{
		ID:      9,
		UserID:  user.ID,
		User:    user,
		GroupID: &originalGroup.ID,
		Group:   originalGroup,
		GroupBindings: []service.APIKeyGroupBinding{{
			GroupID:  finalGroup.ID,
			Priority: 0,
			Group:    finalGroup,
		}},
	}
	subRepo := &batchImageSubscriptionRepoStub{subscription: &service.UserSubscription{
		ID:               99,
		UserID:           user.ID,
		GroupID:          finalGroup.ID,
		QuotaSnapshotted: true,
	}}
	h := &BatchImageHandler{
		service: nil,
		openAI:  newBatchImageOpenAIHandlerForResolveTest(subRepo, nil),
	}
	c, recorder := newBatchImageSubmitContext(t, apiKey)

	require.NotPanics(t, func() { h.Submit(c) }, "standard_quota must be rejected before service.Submit")
	require.Equal(t, http.StatusConflict, recorder.Code)
	require.Equal(t, "BATCH_IMAGE_SUBSCRIPTION_UNSUPPORTED", batchImageResponseErrorCode(t, recorder))
	require.Equal(t, 1, subRepo.calls)
	require.Equal(t, user.ID, subRepo.userID)
	require.Equal(t, finalGroup.ID, subRepo.groupID)

	resolvedAPIKey, ok := middleware2.GetAPIKeyFromContext(c)
	require.True(t, ok)
	require.Equal(t, finalGroup.ID, *resolvedAPIKey.GroupID)
	resolvedSubscription, ok := middleware2.GetSubscriptionFromContext(c)
	require.True(t, ok)
	require.True(t, resolvedSubscription.QuotaSnapshotted)
}

func TestBatchImageSubmitBalancePathWithoutSubscriptionIsUnchanged(t *testing.T) {
	group := &service.Group{ID: 33, SubscriptionType: service.SubscriptionTypeStandard}
	apiKey := &service.APIKey{ID: 10, UserID: 8, GroupID: &group.ID, Group: group}
	h := &BatchImageHandler{
		service: &service.BatchImagePublicService{},
		openAI:  &OpenAIGatewayHandler{},
	}
	c, recorder := newBatchImageSubmitContext(t, apiKey)

	h.Submit(c)

	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.Equal(t, "BATCH_IMAGE_DISABLED", batchImageResponseErrorCode(t, recorder))
	require.NotEqual(t, "BATCH_IMAGE_SUBSCRIPTION_UNSUPPORTED", batchImageResponseErrorCode(t, recorder))
}

func TestBatchImageSubmitNativeSubscriptionGroupWithoutSubscriptionRemainsFailClosed(t *testing.T) {
	originalGroup := &service.Group{ID: 44, SubscriptionType: service.SubscriptionTypeStandard}
	finalGroup := &service.Group{ID: 55, SubscriptionType: service.SubscriptionTypeSubscription}
	user := &service.User{ID: 12}
	apiKey := &service.APIKey{
		ID:      13,
		UserID:  user.ID,
		User:    user,
		GroupID: &originalGroup.ID,
		Group:   originalGroup,
		GroupBindings: []service.APIKeyGroupBinding{{
			GroupID: finalGroup.ID,
			Group:   finalGroup,
		}},
	}
	subRepo := &batchImageSubscriptionRepoStub{err: service.ErrSubscriptionNotFound}
	h := &BatchImageHandler{openAI: newBatchImageOpenAIHandlerForResolveTest(subRepo, nil)}
	c, recorder := newBatchImageSubmitContext(t, apiKey)

	require.NotPanics(t, func() { h.Submit(c) })
	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Equal(t, "SUBSCRIPTION_NOT_FOUND", batchImageResponseErrorCode(t, recorder))
}

func TestBatchImageSecurityAuditUsesFinalMultiGroupAPIKey(t *testing.T) {
	originalGroup := &service.Group{ID: 66, Name: "original", SubscriptionType: service.SubscriptionTypeStandard}
	finalGroup := &service.Group{ID: 77, Name: "final", SubscriptionType: service.SubscriptionTypeStandard}
	user := &service.User{ID: 14, Username: "batch-user"}
	apiKey := &service.APIKey{
		ID:      15,
		UserID:  user.ID,
		User:    user,
		Name:    "batch-key",
		GroupID: &originalGroup.ID,
		Group:   originalGroup,
		GroupBindings: []service.APIKeyGroupBinding{{
			GroupID: finalGroup.ID,
			Group:   finalGroup,
		}},
	}
	subRepo := &batchImageSubscriptionRepoStub{err: service.ErrSubscriptionNotFound}
	engine := blockingHandlerPromptEngine()
	coordinator := securityaudit.NewCoordinator(nil, engine)
	h := &BatchImageHandler{
		service: nil,
		openAI:  newBatchImageOpenAIHandlerForResolveTest(subRepo, coordinator),
	}
	c, recorder := newBatchImageSubmitContext(t, apiKey)

	require.NotPanics(t, func() { h.Submit(c) }, "blocking audit must run before service.Submit")
	require.Equal(t, http.StatusForbidden, recorder.Code)
	evaluated, _, requests := engine.snapshot()
	require.Equal(t, 1, evaluated)
	require.Len(t, requests, 1)
	require.NotNil(t, requests[0].GroupID)
	require.Equal(t, finalGroup.ID, *requests[0].GroupID)
	require.Equal(t, apiKey.ID, requests[0].APIKeyID)
	_, hasSubscription := middleware2.GetSubscriptionFromContext(c)
	require.False(t, hasSubscription)
}

func TestBatchImageSubscriptionUnsupportedErrorCodeIsStable(t *testing.T) {
	group := &service.Group{ID: 88, SubscriptionType: service.SubscriptionTypeStandard}
	apiKey := &service.APIKey{ID: 16, UserID: 17, GroupID: &group.ID, Group: group}
	h := &BatchImageHandler{openAI: &OpenAIGatewayHandler{}}
	c, recorder := newBatchImageSubmitContext(t, apiKey)
	c.Set(string(middleware2.ContextKeySubscription), &service.UserSubscription{ID: 18, GroupID: group.ID, QuotaSnapshotted: true})

	require.NotPanics(t, func() { h.Submit(c) })
	require.Equal(t, http.StatusConflict, recorder.Code)
	require.Equal(t, "BATCH_IMAGE_SUBSCRIPTION_UNSUPPORTED", batchImageResponseErrorCode(t, recorder))
}
