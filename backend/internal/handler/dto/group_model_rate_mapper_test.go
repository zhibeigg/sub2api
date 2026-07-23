package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupModelRateMultipliersAreAdminOnly(t *testing.T) {
	thresholdUSD := 17.25
	predictionUnitCostUSD := 0.125
	group := &service.Group{
		ID:                                 26,
		Name:                               "cursor",
		Platform:                           service.PlatformAnthropic,
		Status:                             service.StatusActive,
		RateMultiplier:                     0.65,
		ModelRateMultipliers:               map[string]float64{"grok-4.5": 0.6, "gpt-*": 0.65},
		PoolCapacityAlertEnabled:           true,
		PoolCapacityAlertMetric:            service.PoolCapacityAlertMetricRemainingBalanceUSD,
		PoolCapacityAlertThresholdRequests: 125,
		PoolCapacityAlertThresholdUSD:      &thresholdUSD,
		PoolCapacityAlertGeneration:        9,
		PredictedCapacityMode:              service.PredictedCapacityModeFixedImageCost,
		PredictedImageUnitCostUSD:          &predictionUnitCostUSD,
	}

	adminDTO := GroupFromServiceAdmin(group)
	require.Equal(t, group.ModelRateMultipliers, adminDTO.ModelRateMultipliers)
	adminJSON, err := json.Marshal(adminDTO)
	require.NoError(t, err)
	require.Contains(t, string(adminJSON), `"model_rate_multipliers"`)
	require.Contains(t, string(adminJSON), `"pool_capacity_alert_enabled":true`)
	require.Contains(t, string(adminJSON), `"pool_capacity_alert_metric":"remaining_balance_usd"`)
	require.Contains(t, string(adminJSON), `"pool_capacity_alert_threshold_requests":125`)
	require.Contains(t, string(adminJSON), `"pool_capacity_alert_threshold_usd":17.25`)
	require.Contains(t, string(adminJSON), `"predicted_capacity_mode":"fixed_image_cost"`)
	require.Contains(t, string(adminJSON), `"predicted_image_unit_cost_usd":0.125`)
	require.NotContains(t, string(adminJSON), "pool_capacity_alert_generation")

	publicDTO := GroupFromService(group)
	publicJSON, err := json.Marshal(publicDTO)
	require.NoError(t, err)
	require.NotContains(t, string(publicJSON), "model_rate_multipliers")
	require.NotContains(t, string(publicJSON), "pool_capacity_alert_enabled")
	require.NotContains(t, string(publicJSON), "pool_capacity_alert_metric")
	require.NotContains(t, string(publicJSON), "pool_capacity_alert_threshold_requests")
	require.NotContains(t, string(publicJSON), "pool_capacity_alert_threshold_usd")
	require.NotContains(t, string(publicJSON), "pool_capacity_alert_generation")
	require.NotContains(t, string(publicJSON), "predicted_capacity_mode")
	require.NotContains(t, string(publicJSON), "predicted_image_unit_cost_usd")
}
