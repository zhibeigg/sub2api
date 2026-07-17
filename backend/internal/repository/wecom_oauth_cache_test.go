//go:build unit

package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestWeComOAuthCache_AccessTokenAndTicketLifecycle(t *testing.T) {
	cache, rdb, _ := newWeComOAuthCacheTestRepository(t)
	ctx := context.Background()
	scope := "instance-42:" + strings.Repeat("a", 64)

	accessToken, found, err := cache.GetAccessToken(ctx, scope)
	require.NoError(t, err)
	require.False(t, found)
	require.Empty(t, accessToken)

	require.NoError(t, cache.SetAccessToken(ctx, scope, "access-token-value", 90*time.Minute))
	accessToken, found, err = cache.GetAccessToken(ctx, scope)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "access-token-value", accessToken)
	accessTTL, err := rdb.TTL(ctx, weComOAuthAccessTokenKeyPrefix+scope).Result()
	require.NoError(t, err)
	require.Equal(t, 90*time.Minute, accessTTL)

	require.NoError(t, cache.SetJSAPITicket(ctx, scope, "jsapi-ticket-value", 80*time.Minute))
	ticket, found, err := cache.GetJSAPITicket(ctx, scope)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "jsapi-ticket-value", ticket)
	ticketTTL, err := rdb.TTL(ctx, weComOAuthJSAPITicketKeyPrefix+scope).Result()
	require.NoError(t, err)
	require.Equal(t, 80*time.Minute, ticketTTL)

	require.NoError(t, cache.DeleteAccessToken(ctx, scope))
	_, found, err = cache.GetAccessToken(ctx, scope)
	require.NoError(t, err)
	require.False(t, found)

	require.NoError(t, cache.DeleteJSAPITicket(ctx, scope))
	_, found, err = cache.GetJSAPITicket(ctx, scope)
	require.NoError(t, err)
	require.False(t, found)
}

func TestWeComOAuthCache_UsesIndependentPrefixesAndCapsTTL(t *testing.T) {
	cache, rdb, mr := newWeComOAuthCacheTestRepository(t)
	ctx := context.Background()
	scope := "instance-99:" + strings.Repeat("b", 64)
	secretValue := "must-never-appear-in-key"

	require.NoError(t, cache.SetAccessToken(ctx, scope, secretValue, 24*time.Hour))
	require.NoError(t, cache.SetJSAPITicket(ctx, scope, "ticket-secret-value", 24*time.Hour))

	accessKey := weComOAuthAccessTokenKeyPrefix + scope
	ticketKey := weComOAuthJSAPITicketKeyPrefix + scope
	accessTTL, err := rdb.TTL(ctx, accessKey).Result()
	require.NoError(t, err)
	require.Equal(t, weComOAuthRedisMaxTTL, accessTTL)
	ticketTTL, err := rdb.TTL(ctx, ticketKey).Result()
	require.NoError(t, err)
	require.Equal(t, weComOAuthRedisMaxTTL, ticketTTL)

	keys := mr.Keys()
	require.ElementsMatch(t, []string{accessKey, ticketKey}, keys)
	for _, key := range keys {
		require.NotContains(t, key, secretValue)
		require.NotContains(t, key, "ticket-secret-value")
		require.NotContains(t, key, oauthTokenKeyPrefix)
	}
}

func TestWeComOAuthCache_RejectsInvalidWrites(t *testing.T) {
	cache, _, _ := newWeComOAuthCacheTestRepository(t)
	ctx := context.Background()

	require.Error(t, cache.SetAccessToken(ctx, "", "token", time.Minute))
	require.Error(t, cache.SetAccessToken(ctx, "scope", "", time.Minute))
	require.Error(t, cache.SetJSAPITicket(ctx, "scope", "ticket", 0))
}

func newWeComOAuthCacheTestRepository(t *testing.T) (*weComOAuthRedisCache, *redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache, ok := NewWeComOAuthCache(rdb).(*weComOAuthRedisCache)
	require.True(t, ok)
	return cache, rdb, mr
}
