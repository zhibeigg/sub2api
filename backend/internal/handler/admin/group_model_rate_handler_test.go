package admin

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type groupModelRateCapturingAdminService struct {
	*stubAdminService
	createInput *service.CreateGroupInput
	updateInput *service.UpdateGroupInput
}

func (s *groupModelRateCapturingAdminService) CreateGroup(_ context.Context, input *service.CreateGroupInput) (*service.Group, error) {
	s.createInput = input
	return &service.Group{
		ID:                   26,
		Name:                 input.Name,
		Platform:             input.Platform,
		Status:               service.StatusActive,
		RateMultiplier:       input.RateMultiplier,
		ModelRateMultipliers: input.ModelRateMultipliers,
	}, nil
}

func (s *groupModelRateCapturingAdminService) UpdateGroup(_ context.Context, id int64, input *service.UpdateGroupInput) (*service.Group, error) {
	s.updateInput = input
	group := &service.Group{ID: id, Name: input.Name, Status: service.StatusActive}
	if input.ModelRateMultipliers != nil {
		group.ModelRateMultipliers = *input.ModelRateMultipliers
	}
	return group, nil
}

func setupGroupModelRateHandler() (*gin.Engine, *groupModelRateCapturingAdminService) {
	gin.SetMode(gin.TestMode)
	svc := &groupModelRateCapturingAdminService{stubAdminService: newStubAdminService()}
	handler := NewGroupHandler(svc, nil, nil, nil)
	router := gin.New()
	router.POST("/api/v1/admin/groups", handler.Create)
	router.PUT("/api/v1/admin/groups/:id", handler.Update)
	return router, svc
}

func TestGroupHandlerMapsModelRateMultipliers(t *testing.T) {
	router, svc := setupGroupModelRateHandler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/groups", bytes.NewBufferString(`{
		"name":"cursor",
		"platform":"anthropic",
		"rate_multiplier":0.65,
		"model_rate_multipliers":{"grok-4.5":0.6,"gpt-*":0.65}
	}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, svc.createInput)
	require.Equal(t, map[string]float64{"grok-4.5": 0.6, "gpt-*": 0.65}, svc.createInput.ModelRateMultipliers)
	require.Contains(t, rec.Body.String(), `"model_rate_multipliers"`)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/admin/groups/26", bytes.NewBufferString(`{"model_rate_multipliers":{}}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, svc.updateInput)
	require.NotNil(t, svc.updateInput.ModelRateMultipliers)
	require.Empty(t, *svc.updateInput.ModelRateMultipliers)
}

func TestGroupHandlerMapsNullSubscriptionLimitsToUnlimited(t *testing.T) {
	router, svc := setupGroupModelRateHandler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/groups/26", bytes.NewBufferString(`{
		"subscription_type":"subscription",
		"daily_limit_usd":60,
		"weekly_limit_usd":null,
		"monthly_limit_usd":null
	}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, svc.updateInput)
	require.NotNil(t, svc.updateInput.DailyLimitUSD)
	require.Equal(t, 60.0, *svc.updateInput.DailyLimitUSD)
	require.NotNil(t, svc.updateInput.WeeklyLimitUSD)
	require.Less(t, *svc.updateInput.WeeklyLimitUSD, 0.0)
	require.NotNil(t, svc.updateInput.MonthlyLimitUSD)
	require.Less(t, *svc.updateInput.MonthlyLimitUSD, 0.0)
}

func TestGroupHandlerPreservesExplicitZeroSubscriptionLimit(t *testing.T) {
	router, svc := setupGroupModelRateHandler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/groups/26", bytes.NewBufferString(`{"weekly_limit_usd":0}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, svc.updateInput)
	require.NotNil(t, svc.updateInput.WeeklyLimitUSD)
	require.Equal(t, 0.0, *svc.updateInput.WeeklyLimitUSD)
}

func TestGroupHandlerRejectsInvalidModelRateMultiplierJSONType(t *testing.T) {
	router, svc := setupGroupModelRateHandler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/groups", bytes.NewBufferString(`{
		"name":"cursor",
		"platform":"anthropic",
		"model_rate_multipliers":{"gpt-*":"invalid"}
	}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Nil(t, svc.createInput)
}
