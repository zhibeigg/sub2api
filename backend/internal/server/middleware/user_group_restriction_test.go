package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestValidateAPIKeyGroupAllowedDefersUnpinnedMultiGroupRestriction(t *testing.T) {
	blocked := &service.Group{ID: 1, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeStandard}
	allowed := &service.Group{ID: 2, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeStandard}
	user := &service.User{
		ID:                7,
		GroupAccessMode:   service.GroupAccessModeRestricted,
		GroupAccessGroups: []int64{allowed.ID},
	}
	key := &service.APIKey{
		User:    user,
		GroupID: &blocked.ID,
		Group:   blocked,
		GroupBindings: []service.APIKeyGroupBinding{
			{GroupID: blocked.ID, Priority: 0, Group: blocked},
			{GroupID: allowed.ID, Priority: 1, Group: allowed},
		},
	}

	require.True(t, validateAPIKeyGroupAllowed(key, nil), "an allowed fallback must survive middleware validation")

	key.GroupBindings = key.GroupBindings[:1]
	require.False(t, validateAPIKeyGroupAllowed(key, nil), "all-restricted multi-group keys must be rejected")

	key.GroupBindings = append(key.GroupBindings, service.APIKeyGroupBinding{GroupID: allowed.ID, Priority: 1, Group: allowed})
	key.ExplicitGroupSelection = true
	require.False(t, validateAPIKeyGroupAllowed(key, nil), "an explicitly selected restricted group must not fall back")
}

func TestAbortIfAPIKeyGroupNotAllowedReturnsStableErrorCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	group := &service.Group{ID: 1, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeStandard}
	key := &service.APIKey{
		User: &service.User{
			ID:                7,
			GroupAccessMode:   service.GroupAccessModeRestricted,
			GroupAccessGroups: []int64{},
		},
		GroupID: &group.ID,
		Group:   group,
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	require.True(t, abortIfAPIKeyGroupNotAllowed(c, key, nil))
	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "GROUP_NOT_ALLOWED")
}

func TestValidateAPIKeyGroupAllowedDoesNotApplyStandardRestrictionToSubscriptionGroup(t *testing.T) {
	group := &service.Group{ID: 9, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeSubscription}
	key := &service.APIKey{
		User: &service.User{
			ID:                7,
			GroupAccessMode:   service.GroupAccessModeRestricted,
			GroupAccessGroups: []int64{},
		},
		GroupID: &group.ID,
		Group:   group,
	}

	require.False(t, validateAPIKeyGroupAllowed(key, nil))
	require.True(t, validateAPIKeyGroupAllowed(key, &service.UserSubscription{UserID: 7, GroupID: group.ID}))
}
