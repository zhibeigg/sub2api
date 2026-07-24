package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration199MigratesGPTSubscriptionsToStableStandardGroup(t *testing.T) {
	content, err := FS.ReadFile("199_migrate_gpt_subscription_to_standard_group.sql")
	require.NoError(t, err)
	sql := string(content)

	require.Contains(t, sql, "GPT 稳定分组 无限制")
	require.Contains(t, sql, "target_count <> 1")
	require.Contains(t, sql, "target_group.platform <> 'openai'")
	require.Contains(t, sql, "target_group.subscription_type <> 'standard'")
	require.Contains(t, sql, "target_group.status <> 'active'")

	require.Contains(t, sql, "sp.plan_type = 'subscription'")
	require.Contains(t, sql, "old_group.platform = 'openai'")
	require.Contains(t, sql, "old_group.subscription_type = 'subscription'")
	require.Contains(t, sql, "SET plan_type = 'standard_quota'")
	require.Contains(t, sql, "INSERT INTO subscription_plan_groups")

	require.Contains(t, sql, "quota_snapshotted = TRUE")
	require.Contains(t, sql, "COALESCE(us.daily_limit_usd, source.daily_limit_usd)")
	require.Contains(t, sql, "COALESCE(us.weekly_limit_usd, source.weekly_limit_usd)")
	require.Contains(t, sql, "COALESCE(us.monthly_limit_usd, source.monthly_limit_usd)")
	require.Contains(t, sql, "gpt_subscription_migration_user_merges")
	require.Contains(t, sql, "SUM(GREATEST(daily_limit_usd - daily_usage_usd, 0))")
	require.Contains(t, sql, "SUM(GREATEST(weekly_limit_usd - weekly_usage_usd, 0))")
	require.Contains(t, sql, "SUM(GREATEST(monthly_limit_usd - monthly_usage_usd, 0))")
	require.Contains(t, sql, "source.subscription_id <> merged.primary_subscription_id")
	require.Contains(t, sql, "source_subscription_count > 1 THEN 0 ELSE us.daily_usage_usd")
	require.Contains(t, sql, "a user already has a target GPT stable-group subscription")
	require.Contains(t, sql, "INSERT INTO user_subscription_groups")
	require.Contains(t, sql, "INSERT INTO user_allowed_groups")
	require.Contains(t, sql, "target.is_exclusive = TRUE")
	require.Contains(t, sql, "INSERT INTO user_group_access_groups")

	require.Contains(t, sql, "UPDATE api_key_groups AS binding")
	require.Contains(t, sql, "PARTITION BY binding.api_key_id")
	require.Contains(t, sql, "WHERE merge_rank > 1")
	require.Contains(t, sql, "UPDATE api_keys AS key")
	require.Contains(t, sql, "DELETE FROM user_group_rate_multipliers")
	require.Contains(t, sql, "order_record.status = 'PENDING'")
	require.Contains(t, sql, "'plan_type', 'standard_quota'")
}
