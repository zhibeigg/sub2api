package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPoolCapacityAlertThresholdsMigration(t *testing.T) {
	content, err := FS.ReadFile("193_add_group_pool_capacity_alert_thresholds.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS pool_capacity_alert_metric VARCHAR(32) NOT NULL DEFAULT 'predicted_requests'")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS pool_capacity_alert_threshold_requests BIGINT NOT NULL DEFAULT 50")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS pool_capacity_alert_threshold_usd NUMERIC(30,12)")
	require.Contains(t, sql, "pool_capacity_alert_metric IN ('predicted_requests', 'remaining_balance_usd')")
	require.Contains(t, sql, "pool_capacity_alert_threshold_requests BETWEEN 1 AND 1000000000")
	require.Contains(t, sql, "pool_capacity_alert_threshold_usd >= 0.01")
	require.Contains(t, sql, "pool_capacity_alert_threshold_usd <= 1000000000000000")
	require.Contains(t, sql, "pool_capacity_alert_metric <> 'remaining_balance_usd' OR pool_capacity_alert_threshold_usd IS NOT NULL")

	require.Contains(t, sql, "ALTER TABLE pool_capacity_alert_states")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS alert_metric VARCHAR(32) NOT NULL DEFAULT 'predicted_requests'")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS remaining_balance_usd NUMERIC(30,12)")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS threshold_requests BIGINT NOT NULL DEFAULT 50")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS threshold_usd NUMERIC(30,12)")

	require.Contains(t, sql, "ALTER TABLE pool_capacity_alert_events")
	require.Contains(t, sql, "ALTER COLUMN predicted_requests DROP NOT NULL")
	require.Contains(t, sql, "ALTER COLUMN threshold_requests DROP NOT NULL")
	require.Contains(t, sql, "ALTER COLUMN threshold_requests DROP DEFAULT")
	require.Contains(t, sql, "UPDATE pool_capacity_alert_events SET alert_metric = 'predicted_requests'")
	require.Contains(t, sql, "UPDATE pool_capacity_alert_events SET threshold_requests = 50 WHERE threshold_requests IS NULL AND alert_metric = 'predicted_requests'")

	require.NotContains(t, sql, "pool_capacity_alert_generation =")
	require.NotContains(t, sql, "CREATE INDEX")
}
