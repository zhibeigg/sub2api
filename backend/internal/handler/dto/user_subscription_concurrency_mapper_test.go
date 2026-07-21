package dto

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserSubscriptionFromServiceMapsConcurrencyLimit(t *testing.T) {
	limit := 4
	mapped := UserSubscriptionFromService(&service.UserSubscription{
		ID:               10,
		UserID:           20,
		GroupID:          30,
		GroupIDs:         []int64{30, 31},
		ConcurrencyLimit: &limit,
	})

	require.NotNil(t, mapped)
	require.Equal(t, &limit, mapped.ConcurrencyLimit)
}

func TestUserSubscriptionFromServiceKeepsNullConcurrencyLimit(t *testing.T) {
	mapped := UserSubscriptionFromService(&service.UserSubscription{ID: 10})

	require.NotNil(t, mapped)
	require.Nil(t, mapped.ConcurrencyLimit)
}
