package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupPredictedBalanceAlertMigration(t *testing.T) {
	content, err := FS.ReadFile("194_group_predicted_balance_alert.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	for _, table := range []string{"pool_capacity_alert_states", "pool_capacity_alert_events"} {
		require.Contains(t, sql, "ALTER TABLE "+table)
		require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS scope_type VARCHAR(16) NOT NULL DEFAULT 'context'")
		require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS pool_authoritative_balance_usd NUMERIC(30,12)")
		require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS normal_estimated_balance_usd NUMERIC(30,12)")
		require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS pool_account_count INTEGER NOT NULL DEFAULT 0")
		require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS normal_account_count INTEGER NOT NULL DEFAULT 0")
		require.Contains(t, sql, "ALTER COLUMN account_id DROP NOT NULL")
		require.Contains(t, sql, "ALTER COLUMN api_key_id DROP NOT NULL")
		require.Contains(t, sql, "ALTER COLUMN user_id DROP NOT NULL")
		require.Contains(t, sql, "ALTER COLUMN billing_type DROP NOT NULL")
	}

	require.Contains(t, sql, "CREATE UNIQUE INDEX IF NOT EXISTS idx_pool_capacity_alert_states_context_scope")
	require.Contains(t, sql, "WHERE scope_type = 'context'")
	require.Contains(t, sql, "CREATE UNIQUE INDEX IF NOT EXISTS idx_pool_capacity_alert_states_group_scope")
	require.Contains(t, sql, "WHERE scope_type = 'group'")
	require.Contains(t, sql, "scope_type IN ('context', 'group')")
	require.Contains(t, sql, "scope_type = 'group' AND account_id IS NULL AND api_key_id IS NULL AND user_id IS NULL AND billing_type IS NULL")
	require.Contains(t, sql, "pool_capacity_alert_states_scope_metric_check")
	require.Contains(t, sql, "pool_capacity_alert_events_scope_metric_check")
	require.Contains(t, sql, "scope_type = 'context' AND alert_metric = 'predicted_requests'")
	require.Contains(t, sql, "scope_type = 'group' AND alert_metric = 'remaining_balance_usd'")
	require.Contains(t, sql, ") NOT VALID")
	require.Contains(t, sql, "UPDATE groups SET pool_capacity_alert_generation = pool_capacity_alert_generation + 1")
	require.Contains(t, sql, "WHERE pool_capacity_alert_metric = 'remaining_balance_usd'")
}
