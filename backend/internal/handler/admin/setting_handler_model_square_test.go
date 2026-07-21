//go:build unit

package admin

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestModelSquareUpdateRequestPreservesAndOverridesValue(t *testing.T) {
	var omitted UpdateSettingsRequest
	require.NoError(t, json.Unmarshal([]byte(`{}`), &omitted))
	require.Nil(t, omitted.ModelSquareEnabled)
	require.True(t, boolValueOrDefault(omitted.ModelSquareEnabled, true))

	var disabled UpdateSettingsRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model_square_enabled":false}`), &disabled))
	require.NotNil(t, disabled.ModelSquareEnabled)
	require.False(t, boolValueOrDefault(disabled.ModelSquareEnabled, true))
}

func TestModelSquareAdminResponseAndAuditKey(t *testing.T) {
	raw, err := json.Marshal(dto.SystemSettings{ModelSquareEnabled: true})
	require.NoError(t, err)
	var response map[string]any
	require.NoError(t, json.Unmarshal(raw, &response))
	require.Equal(t, true, response[service.SettingKeyModelSquareEnabled])

	changed := diffSettings(
		&service.SystemSettings{ModelSquareEnabled: false},
		&service.SystemSettings{ModelSquareEnabled: true},
		nil,
		nil,
		UpdateSettingsRequest{},
	)
	require.Contains(t, changed, service.SettingKeyModelSquareEnabled)
}
