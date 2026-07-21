package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration189AddsNullablePositiveConcurrencySnapshotsIdempotently(t *testing.T) {
	content, err := FS.ReadFile("189_standard_quota_concurrency_limit.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ALTER TABLE subscription_plans ADD COLUMN IF NOT EXISTS concurrency_limit INTEGER")
	require.Contains(t, sql, "ALTER TABLE user_subscriptions ADD COLUMN IF NOT EXISTS concurrency_limit INTEGER")
	require.Contains(t, sql, "subscription_plans_concurrency_limit_check")
	require.Contains(t, sql, "conrelid = 'subscription_plans'::regclass")
	require.Contains(t, sql, "user_subscriptions_concurrency_limit_check")
	require.Contains(t, sql, "conrelid = 'user_subscriptions'::regclass")
	require.Equal(t, 2, strings.Count(sql, "CHECK (concurrency_limit IS NULL OR concurrency_limit > 0) NOT VALID"))
	require.NotContains(t, sql, "UPDATE subscription_plans")
	require.NotContains(t, sql, "UPDATE user_subscriptions")
	require.Contains(t, sql, "COMMENT ON COLUMN subscription_plans.concurrency_limit")
	require.Contains(t, sql, "COMMENT ON COLUMN user_subscriptions.concurrency_limit")
}
