//go:build unit

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyAuth_StandardGroupSubscriptionAndFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limit := 1.0
	now := time.Now()

	tests := []struct {
		name             string
		groupType        string
		balance          float64
		subscription     *service.UserSubscription
		wantStatus       int
		wantBody         string
		wantSubscription bool
	}{
		{
			name:      "standard group with active subscription bypasses zero balance",
			groupType: service.SubscriptionTypeStandard,
			balance:   0,
			subscription: &service.UserSubscription{
				ID: 101, Status: service.SubscriptionStatusActive, ExpiresAt: now.Add(time.Hour),
				QuotaSnapshotted: true, DailyLimitUSD: &limit, DailyWindowStart: &now,
			},
			wantStatus: http.StatusOK, wantSubscription: true,
		},
		{
			name:       "standard group without subscription falls back to balance",
			groupType:  service.SubscriptionTypeStandard,
			balance:    0,
			wantStatus: http.StatusForbidden,
			wantBody:   "INSUFFICIENT_BALANCE",
		},
		{
			name:       "subscription group without subscription is rejected",
			groupType:  service.SubscriptionTypeSubscription,
			balance:    10,
			wantStatus: http.StatusForbidden,
			wantBody:   "SUBSCRIPTION_NOT_FOUND",
		},
		{
			name:      "standard quota plan exhaustion returns 429",
			groupType: service.SubscriptionTypeStandard,
			balance:   10,
			subscription: &service.UserSubscription{
				ID: 102, Status: service.SubscriptionStatusActive, ExpiresAt: now.Add(time.Hour),
				QuotaSnapshotted: true, DailyLimitUSD: &limit, DailyUsageUSD: 2, DailyWindowStart: &now,
			},
			wantStatus: http.StatusTooManyRequests,
			wantBody:   "USAGE_LIMIT_EXCEEDED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			group := &service.Group{ID: 77, Name: "dual-type", Status: service.StatusActive, Hydrated: true, SubscriptionType: tt.groupType}
			user := &service.User{ID: 88, Role: service.RoleUser, Status: service.StatusActive, Balance: tt.balance, Concurrency: 3}
			apiKey := &service.APIKey{ID: 99, UserID: user.ID, Key: "dual-key", Status: service.StatusActive, User: user, Group: group, GroupID: &group.ID}
			apiKeyService := service.NewAPIKeyService(&stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) {
				clone := *apiKey
				return &clone, nil
			}}, nil, nil, nil, nil, nil, &config.Config{RunMode: config.RunModeStandard})

			subscriptionRepo := &stubUserSubscriptionRepo{getActive: func(context.Context, int64, int64) (*service.UserSubscription, error) {
				if tt.subscription == nil {
					return nil, service.ErrSubscriptionNotFound
				}
				clone := *tt.subscription
				clone.UserID = user.ID
				clone.GroupID = group.ID
				return &clone, nil
			}}
			subscriptionService := service.NewSubscriptionService(nil, subscriptionRepo, nil, nil, &config.Config{RunMode: config.RunModeStandard})

			router := gin.New()
			router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, subscriptionService, &config.Config{RunMode: config.RunModeStandard})))
			seenSubscription := false
			router.GET("/t", func(c *gin.Context) {
				_, seenSubscription = GetSubscriptionFromContext(c)
				c.JSON(http.StatusOK, gin.H{"ok": true})
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/t", nil)
			req.Header.Set("x-api-key", apiKey.Key)
			router.ServeHTTP(w, req)

			require.Equal(t, tt.wantStatus, w.Code)
			require.Equal(t, tt.wantSubscription, seenSubscription)
			if tt.wantBody != "" {
				require.Contains(t, w.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestAPIKeyAuthGoogle_StandardGroupSubscriptionAndFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limit := 1.0
	now := time.Now()

	tests := []struct {
		name             string
		groupType        string
		balance          float64
		subscription     *service.UserSubscription
		wantStatus       int
		wantMessage      string
		wantSubscription bool
	}{
		{
			name:      "standard group with active subscription bypasses zero balance",
			groupType: service.SubscriptionTypeStandard,
			balance:   0,
			subscription: &service.UserSubscription{
				ID: 201, Status: service.SubscriptionStatusActive, ExpiresAt: now.Add(time.Hour),
				QuotaSnapshotted: true, DailyLimitUSD: &limit, DailyWindowStart: &now,
			},
			wantStatus: http.StatusOK, wantSubscription: true,
		},
		{
			name:        "standard group without subscription falls back to balance",
			groupType:   service.SubscriptionTypeStandard,
			balance:     0,
			wantStatus:  http.StatusForbidden,
			wantMessage: "account balance is insufficient",
		},
		{
			name:        "subscription group without subscription is rejected",
			groupType:   service.SubscriptionTypeSubscription,
			balance:     10,
			wantStatus:  http.StatusForbidden,
			wantMessage: "No active subscription is available for this group",
		},
		{
			name:      "standard quota plan exhaustion returns 429",
			groupType: service.SubscriptionTypeStandard,
			balance:   10,
			subscription: &service.UserSubscription{
				ID: 202, Status: service.SubscriptionStatusActive, ExpiresAt: now.Add(time.Hour),
				QuotaSnapshotted: true, DailyLimitUSD: &limit, DailyUsageUSD: 2, DailyWindowStart: &now,
			},
			wantStatus:  http.StatusTooManyRequests,
			wantMessage: "subscription usage limit has been reached",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			group := &service.Group{ID: 177, Name: "google-dual-type", Status: service.StatusActive, Platform: service.PlatformGemini, Hydrated: true, SubscriptionType: tt.groupType}
			user := &service.User{ID: 188, Role: service.RoleUser, Status: service.StatusActive, Balance: tt.balance, Concurrency: 3}
			apiKey := &service.APIKey{ID: 199, UserID: user.ID, Key: "google-dual-key", Status: service.StatusActive, User: user, Group: group, GroupID: &group.ID}
			apiKeyService := service.NewAPIKeyService(&stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) {
				clone := *apiKey
				return &clone, nil
			}}, nil, nil, nil, nil, nil, &config.Config{RunMode: config.RunModeStandard})

			subscriptionRepo := &stubUserSubscriptionRepo{getActive: func(context.Context, int64, int64) (*service.UserSubscription, error) {
				if tt.subscription == nil {
					return nil, service.ErrSubscriptionNotFound
				}
				clone := *tt.subscription
				clone.UserID = user.ID
				clone.GroupID = group.ID
				return &clone, nil
			}}
			subscriptionService := service.NewSubscriptionService(nil, subscriptionRepo, nil, nil, &config.Config{RunMode: config.RunModeStandard})

			router := gin.New()
			router.Use(APIKeyAuthWithSubscriptionGoogle(apiKeyService, subscriptionService, &config.Config{RunMode: config.RunModeStandard}))
			seenSubscription := false
			router.GET("/v1beta/test", func(c *gin.Context) {
				_, seenSubscription = GetSubscriptionFromContext(c)
				c.JSON(http.StatusOK, gin.H{"ok": true})
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/v1beta/test", nil)
			req.Header.Set("x-goog-api-key", apiKey.Key)
			router.ServeHTTP(w, req)

			require.Equal(t, tt.wantStatus, w.Code)
			require.Equal(t, tt.wantSubscription, seenSubscription)
			if tt.wantMessage != "" {
				require.Contains(t, w.Body.String(), tt.wantMessage)
			}
		})
	}
}
