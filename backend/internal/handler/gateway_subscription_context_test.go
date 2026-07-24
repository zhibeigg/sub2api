package handler

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type activeSubscriptionResolverStub struct {
	subscription    *service.UserSubscription
	err             error
	validateErr     error
	calls           int
	validationCalls int
	userID          int64
	groupID         int64
}

func (s *activeSubscriptionResolverStub) GetActiveSubscription(_ context.Context, userID, groupID int64) (*service.UserSubscription, error) {
	s.calls++
	s.userID = userID
	s.groupID = groupID
	return s.subscription, s.err
}

func (s *activeSubscriptionResolverStub) ValidateAndCheckLimits(_ *service.UserSubscription, _ *service.Group) (bool, error) {
	s.validationCalls++
	return false, s.validateErr
}

func newResolvedGroupTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	return c
}

func TestApplyResolvedAPIKeyContext(t *testing.T) {
	standardGroup := &service.Group{ID: 2, SubscriptionType: service.SubscriptionTypeStandard}
	originalGroup := &service.Group{ID: 1, SubscriptionType: service.SubscriptionTypeStandard}
	user := &service.User{ID: 7}
	original := &service.APIKey{User: user, GroupID: &originalGroup.ID, Group: originalGroup}
	selected := cloneAPIKeyWithGroup(original, standardGroup)

	t.Run("changed group loads and stores the final subscription", func(t *testing.T) {
		limit := 2
		resolved := &service.UserSubscription{ID: 99, UserID: user.ID, GroupID: standardGroup.ID, ConcurrencyLimit: &limit}
		resolver := &activeSubscriptionResolverStub{subscription: resolved}
		c := newResolvedGroupTestContext()
		c.Set(string(middleware2.ContextKeySubscription), &service.UserSubscription{ID: 88, GroupID: originalGroup.ID})

		err := applyResolvedAPIKeyContext(c, original, selected, resolver, nil)
		require.NoError(t, err)
		require.Equal(t, 1, resolver.calls)
		require.Equal(t, user.ID, resolver.userID)
		require.Equal(t, standardGroup.ID, resolver.groupID)

		contextKey, ok := middleware2.GetAPIKeyFromContext(c)
		require.True(t, ok)
		require.Equal(t, standardGroup.ID, *contextKey.GroupID)
		contextSubscription, ok := middleware2.GetSubscriptionFromContext(c)
		require.True(t, ok)
		require.Equal(t, resolved.ID, contextSubscription.ID)
		contextGroup, ok := c.Request.Context().Value(ctxkey.Group).(*service.Group)
		require.True(t, ok)
		require.Equal(t, standardGroup.ID, contextGroup.ID)
	})

	t.Run("standard group without subscription clears stale context for balance billing", func(t *testing.T) {
		resolver := &activeSubscriptionResolverStub{err: service.ErrSubscriptionNotFound}
		c := newResolvedGroupTestContext()
		c.Set(string(middleware2.ContextKeySubscription), &service.UserSubscription{ID: 88, GroupID: originalGroup.ID})

		err := applyResolvedAPIKeyContext(c, original, selected, resolver, nil)
		require.NoError(t, err)
		contextSubscription, ok := middleware2.GetSubscriptionFromContext(c)
		require.False(t, ok)
		require.Nil(t, contextSubscription)
	})

	t.Run("exhausted standard group subscription clears context for balance billing", func(t *testing.T) {
		resolver := &activeSubscriptionResolverStub{
			subscription: &service.UserSubscription{ID: 100, UserID: user.ID, GroupID: standardGroup.ID},
			validateErr:  service.ErrDailyLimitExceeded,
		}
		c := newResolvedGroupTestContext()
		c.Set(string(middleware2.ContextKeySubscription), &service.UserSubscription{ID: 88, GroupID: originalGroup.ID})

		err := applyResolvedAPIKeyContext(c, original, selected, resolver, nil)
		require.NoError(t, err)
		require.Equal(t, 1, resolver.validationCalls)
		contextSubscription, ok := middleware2.GetSubscriptionFromContext(c)
		require.False(t, ok)
		require.Nil(t, contextSubscription)
	})

	t.Run("native subscription group remains fail closed", func(t *testing.T) {
		nativeGroup := &service.Group{ID: 3, SubscriptionType: service.SubscriptionTypeSubscription}
		nativeSelected := cloneAPIKeyWithGroup(original, nativeGroup)
		resolver := &activeSubscriptionResolverStub{err: service.ErrSubscriptionNotFound}
		c := newResolvedGroupTestContext()

		err := applyResolvedAPIKeyContext(c, original, nativeSelected, resolver, nil)
		require.ErrorIs(t, err, service.ErrSubscriptionNotFound)
	})

	t.Run("repository failures do not silently fall back to balance", func(t *testing.T) {
		lookupErr := errors.New("database unavailable")
		resolver := &activeSubscriptionResolverStub{err: lookupErr}
		c := newResolvedGroupTestContext()

		err := applyResolvedAPIKeyContext(c, original, selected, resolver, nil)
		require.Error(t, err)
		require.ErrorIs(t, err, lookupErr)
	})

	t.Run("unchanged group reuses authentication context", func(t *testing.T) {
		resolver := &activeSubscriptionResolverStub{err: errors.New("must not be called")}
		c := newResolvedGroupTestContext()
		current := &service.UserSubscription{ID: 77, GroupID: originalGroup.ID}
		c.Set(string(middleware2.ContextKeySubscription), current)

		err := applyResolvedAPIKeyContext(c, original, cloneAPIKeyWithGroup(original, originalGroup), resolver, nil)
		require.NoError(t, err)
		require.Zero(t, resolver.calls)
		contextSubscription, ok := middleware2.GetSubscriptionFromContext(c)
		require.True(t, ok)
		require.Same(t, current, contextSubscription)
	})

	t.Run("simple mode updates key and group while clearing stale subscription", func(t *testing.T) {
		resolver := &activeSubscriptionResolverStub{err: errors.New("must not be called")}
		c := newResolvedGroupTestContext()
		c.Set(string(middleware2.ContextKeySubscription), &service.UserSubscription{ID: 88, GroupID: originalGroup.ID})

		err := applyResolvedAPIKeyContext(c, original, selected, resolver, &config.Config{RunMode: config.RunModeSimple})
		require.NoError(t, err)
		require.Zero(t, resolver.calls)
		contextKey, ok := middleware2.GetAPIKeyFromContext(c)
		require.True(t, ok)
		require.Equal(t, standardGroup.ID, *contextKey.GroupID)
		contextGroup, ok := c.Request.Context().Value(ctxkey.Group).(*service.Group)
		require.True(t, ok)
		require.Equal(t, standardGroup.ID, contextGroup.ID)
		contextSubscription, ok := middleware2.GetSubscriptionFromContext(c)
		require.False(t, ok)
		require.Nil(t, contextSubscription)
	})
}
