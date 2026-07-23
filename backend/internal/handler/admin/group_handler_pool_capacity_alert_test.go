//go:build unit

package admin

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUpdateGroupRequestPoolCapacityAlertUSDUsesNullablePatchSemantics(t *testing.T) {
	t.Run("omitted", func(t *testing.T) {
		var req UpdateGroupRequest
		require.NoError(t, json.Unmarshal([]byte(`{"pool_capacity_alert_enabled":true}`), &req))
		require.Nil(t, req.PoolCapacityAlertThresholdUSD.ToServicePatch())
	})

	t.Run("explicit null", func(t *testing.T) {
		var req UpdateGroupRequest
		require.NoError(t, json.Unmarshal([]byte(`{"pool_capacity_alert_threshold_usd":null}`), &req))
		patch := req.PoolCapacityAlertThresholdUSD.ToServicePatch()
		require.NotNil(t, patch)
		require.Nil(t, *patch)
	})

	t.Run("numeric value", func(t *testing.T) {
		var req UpdateGroupRequest
		require.NoError(t, json.Unmarshal([]byte(`{"pool_capacity_alert_threshold_usd":12.34}`), &req))
		patch := req.PoolCapacityAlertThresholdUSD.ToServicePatch()
		require.NotNil(t, patch)
		require.NotNil(t, *patch)
		require.Equal(t, 12.34, **patch)
	})
}

func TestUpdateGroupRequestPredictedImageCostUsesNullablePatchSemantics(t *testing.T) {
	t.Run("omitted", func(t *testing.T) {
		var req UpdateGroupRequest
		require.NoError(t, json.Unmarshal([]byte(`{"predicted_capacity_mode":"historical_requests"}`), &req))
		require.Nil(t, req.PredictedImageUnitCostUSD.ToServicePatch())
	})

	t.Run("explicit null", func(t *testing.T) {
		var req UpdateGroupRequest
		require.NoError(t, json.Unmarshal([]byte(`{"predicted_image_unit_cost_usd":null}`), &req))
		patch := req.PredictedImageUnitCostUSD.ToServicePatch()
		require.NotNil(t, patch)
		require.Nil(t, *patch)
	})

	t.Run("numeric value", func(t *testing.T) {
		var req UpdateGroupRequest
		require.NoError(t, json.Unmarshal([]byte(`{"predicted_image_unit_cost_usd":0.025}`), &req))
		patch := req.PredictedImageUnitCostUSD.ToServicePatch()
		require.NotNil(t, patch)
		require.NotNil(t, *patch)
		require.Equal(t, 0.025, **patch)
	})
}

func TestCreateGroupRequestAcceptsPredictionConfigAndExplicitNull(t *testing.T) {
	var fixed CreateGroupRequest
	require.NoError(t, json.Unmarshal([]byte(`{
		"name":"fixed-image",
		"predicted_capacity_mode":"fixed_image_cost",
		"predicted_image_unit_cost_usd":0.125
	}`), &fixed))
	require.Equal(t, service.PredictedCapacityModeFixedImageCost, fixed.PredictedCapacityMode)
	require.Equal(t, 0.125, *fixed.PredictedImageUnitCostUSD.Value())

	var historical CreateGroupRequest
	require.NoError(t, json.Unmarshal([]byte(`{
		"name":"historical",
		"predicted_capacity_mode":"historical_requests",
		"predicted_image_unit_cost_usd":null
	}`), &historical))
	require.Equal(t, service.PredictedCapacityModeHistoricalRequests, historical.PredictedCapacityMode)
	require.Nil(t, historical.PredictedImageUnitCostUSD.Value())
}

func TestCreateGroupRequestAcceptsPoolCapacityAlertPolicyFields(t *testing.T) {
	var req CreateGroupRequest
	err := json.Unmarshal([]byte(`{
		"name":"pool-alert",
		"pool_capacity_alert_enabled":true,
		"pool_capacity_alert_metric":"remaining_balance_usd",
		"pool_capacity_alert_threshold_requests":125,
		"pool_capacity_alert_threshold_usd":42.5
	}`), &req)
	require.NoError(t, err)
	require.True(t, req.PoolCapacityAlertEnabled)
	require.Equal(t, service.PoolCapacityAlertMetricRemainingBalanceUSD, *req.PoolCapacityAlertMetric)
	require.Equal(t, int64(125), *req.PoolCapacityAlertThresholdRequests)
	require.Equal(t, 42.5, *req.PoolCapacityAlertThresholdUSD)
}
