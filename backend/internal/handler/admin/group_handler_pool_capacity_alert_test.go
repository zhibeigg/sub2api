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
