package repository

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserSubscriptionEntityToServiceMapsConcurrencyLimit(t *testing.T) {
	limit := 8
	mapped := userSubscriptionEntityToService(&dbent.UserSubscription{
		ID:               1,
		UserID:           2,
		GroupID:          3,
		ConcurrencyLimit: &limit,
	})

	require.NotNil(t, mapped)
	require.Equal(t, &limit, mapped.ConcurrencyLimit)
}

func TestUserSubscriptionEntityToServiceKeepsNullConcurrencyLimit(t *testing.T) {
	mapped := userSubscriptionEntityToService(&dbent.UserSubscription{ID: 1})

	require.NotNil(t, mapped)
	require.Nil(t, mapped.ConcurrencyLimit)
}

func TestUserSubscriptionRepositoryUpdateExplicitlyClearsConcurrencyLimit(t *testing.T) {
	ctx := context.Background()
	client := newSecuritySecretTestClient(t)
	user, err := client.User.Create().
		SetEmail("subscription-concurrency-clear@example.com").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	group, err := client.Group.Create().
		SetName("subscription-concurrency-clear").
		SetSubscriptionType(domain.SubscriptionTypeStandard).
		Save(ctx)
	require.NoError(t, err)

	limit := 5
	now := time.Now()
	sub := &service.UserSubscription{
		UserID:           user.ID,
		GroupID:          group.ID,
		GroupIDs:         []int64{group.ID},
		ConcurrencyLimit: &limit,
		StartsAt:         now,
		ExpiresAt:        now.Add(24 * time.Hour),
		Status:           service.SubscriptionStatusActive,
		AssignedAt:       now,
	}
	repo := &userSubscriptionRepository{client: client}
	require.NoError(t, repo.Create(ctx, sub))

	sub.ConcurrencyLimit = nil
	require.NoError(t, repo.Update(ctx, sub))
	reloaded, err := repo.GetByID(ctx, sub.ID)
	require.NoError(t, err)
	require.Nil(t, reloaded.ConcurrencyLimit)
}
