package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupCapacityPredictionModeMigration(t *testing.T) {
	content, err := FS.ReadFile("196_add_group_capacity_prediction_mode.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS predicted_capacity_mode VARCHAR(32) NOT NULL DEFAULT 'historical_requests'")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS predicted_image_unit_cost_usd NUMERIC(30,12)")
	require.Contains(t, sql, "predicted_capacity_mode IN ('historical_requests', 'fixed_image_cost')")
	require.Contains(t, sql, "predicted_image_unit_cost_usd BETWEEN 0.000000000001 AND 1000000000000000")
	require.Contains(t, sql, "predicted_capacity_mode <> 'fixed_image_cost' OR predicted_image_unit_cost_usd IS NOT NULL")
	require.Contains(t, sql, "predicted_capacity_mode = 'fixed_image_cost' AND predicted_image_unit_cost_usd IS NULL")
	require.Contains(t, sql, "conrelid = 'groups'::regclass")

	// 展示预测配置与既有池容量告警策略、代际完全独立。
	require.NotContains(t, sql, "pool_capacity_alert_metric")
	require.NotContains(t, sql, "pool_capacity_alert_generation")
}
