package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupModelRateMultipliersAreAdminOnly(t *testing.T) {
	group := &service.Group{
		ID:                   26,
		Name:                 "cursor",
		Platform:             service.PlatformAnthropic,
		Status:               service.StatusActive,
		RateMultiplier:       0.65,
		ModelRateMultipliers: map[string]float64{"grok-4.5": 0.6, "gpt-*": 0.65},
	}

	adminDTO := GroupFromServiceAdmin(group)
	require.Equal(t, group.ModelRateMultipliers, adminDTO.ModelRateMultipliers)
	adminJSON, err := json.Marshal(adminDTO)
	require.NoError(t, err)
	require.Contains(t, string(adminJSON), `"model_rate_multipliers"`)

	publicDTO := GroupFromService(group)
	publicJSON, err := json.Marshal(publicDTO)
	require.NoError(t, err)
	require.NotContains(t, string(publicJSON), "model_rate_multipliers")
}
