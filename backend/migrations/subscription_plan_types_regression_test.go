package migrations

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration177AddsSubscriptionPlanTypesAndBackfillsSafely(t *testing.T) {
	entries, err := FS.ReadDir(".")
	require.NoError(t, err)

	previousIndex := -1
	currentIndex := -1
	for i, entry := range entries {
		switch entry.Name() {
		case "175_shared_quota_subscriptions.sql":
			previousIndex = i
		case "177_subscription_plan_types.sql":
			currentIndex = i
		}
	}
	require.NotEqual(t, -1, previousIndex)
	require.NotEqual(t, -1, currentIndex)
	require.Less(t, previousIndex, currentIndex)

	content, err := FS.ReadFile("177_subscription_plan_types.sql")
	require.NoError(t, err)
	sql := string(content)

	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS plan_type VARCHAR(40)")
	require.Contains(t, sql, "WHEN stats.group_count = 1 AND COALESCE(stats.all_subscription, FALSE) THEN 'subscription'")
	require.Contains(t, sql, "WHEN stats.group_count >= 1 AND COALESCE(stats.all_standard, FALSE) THEN 'standard_quota'")
	require.Contains(t, sql, "ELSE 'legacy_shared_subscription'")
	require.Contains(t, sql, "AND (sp.plan_type IS NULL OR BTRIM(sp.plan_type) = '')")

	require.Contains(t, sql, "WHERE plan_type = 'subscription'")
	require.Contains(t, sql, "SET daily_limit_usd = NULL")
	require.Contains(t, sql, "weekly_limit_usd = NULL")
	require.Contains(t, sql, "monthly_limit_usd = NULL")
	require.Contains(t, sql, "SET for_sale = FALSE")
	require.Contains(t, sql, "WHERE plan_type = 'legacy_shared_subscription'")

	require.Contains(t, sql, "ALTER COLUMN plan_type SET DEFAULT 'subscription'")
	require.Contains(t, sql, "ALTER COLUMN plan_type SET NOT NULL")
	require.Contains(t, sql, "subscription_plans_plan_type_check")
	require.Contains(t, sql, "CHECK (plan_type IN ('subscription', 'standard_quota', 'legacy_shared_subscription'))")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS subscriptionplan_plan_type")
}

func TestMigration177DoesNotRetrofitPlanTypeIntoMigration175(t *testing.T) {
	content, err := FS.ReadFile("175_shared_quota_subscriptions.sql")
	require.NoError(t, err)

	const expectedSHA256 = "1e99d1438f50daf88f59f5966ccbb3e4dfce8d8edc4f529e9011cab26887a240"
	actualSHA256 := fmt.Sprintf("%x", sha256.Sum256(content))
	require.Equal(t, expectedSHA256, actualSHA256, "旧迁移 175 不得被双类型改动回写")
	require.NotContains(t, string(content), "plan_type")
}
