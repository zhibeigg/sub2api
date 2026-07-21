package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUsageUnrestrictedIncludesWeeklyWindowStart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/usage", nil)

	weeklyWindowStart := time.Date(2026, time.July, 13, 0, 30, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	c.Set(string(middleware.ContextKeySubscription), &service.UserSubscription{
		WeeklyWindowStart: &weeklyWindowStart,
	})

	handler := &GatewayHandler{}
	handler.usageUnrestricted(
		c,
		context.Background(),
		&service.APIKey{Group: &service.Group{
			Name:             "Weekly plan",
			SubscriptionType: service.SubscriptionTypeSubscription,
		}},
		middleware.AuthSubject{},
		nil,
		nil,
		nil,
	)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Object        string `json:"object"`
		SchemaVersion int    `json:"schema_version"`
		Subscription  struct {
			WeeklyWindowStart *time.Time `json:"weekly_window_start"`
		} `json:"subscription"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "sub2api.key_usage", response.Object)
	require.Equal(t, 1, response.SchemaVersion)
	require.NotNil(t, response.Subscription.WeeklyWindowStart)
	require.True(t, weeklyWindowStart.Equal(*response.Subscription.WeeklyWindowStart))
}

func TestUsageQuotaLimitedIncludesStrongContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/usage", nil)

	handler := &GatewayHandler{}
	handler.usageQuotaLimited(c, context.Background(), &service.APIKey{
		Quota:     100,
		QuotaUsed: 40,
		Status:    service.StatusAPIKeyActive,
	}, nil, nil, nil)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Object        string  `json:"object"`
		SchemaVersion int     `json:"schema_version"`
		Mode          string  `json:"mode"`
		Remaining     float64 `json:"remaining"`
		Quota         struct {
			Limit     float64 `json:"limit"`
			Used      float64 `json:"used"`
			Remaining float64 `json:"remaining"`
		} `json:"quota"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "sub2api.key_usage", response.Object)
	require.Equal(t, 1, response.SchemaVersion)
	require.Equal(t, "quota_limited", response.Mode)
	require.InDelta(t, 60, response.Remaining, 1e-12)
	require.InDelta(t, 100, response.Quota.Limit, 1e-12)
	require.InDelta(t, 40, response.Quota.Used, 1e-12)
	require.InDelta(t, 60, response.Quota.Remaining, 1e-12)
}

func TestUsageDisablesCachingBeforeAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/usage", nil)

	(&GatewayHandler{}).Usage(c)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
}
