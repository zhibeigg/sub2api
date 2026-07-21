package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModelSquareFeatureToggleMigration(t *testing.T) {
	content, err := FS.ReadFile("187_add_model_square_feature_toggle.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "INSERT INTO settings (key, value, updated_at)")
	require.Contains(t, sql, "'model_square_enabled'")
	require.Contains(t, sql, "key = 'available_channels_enabled' AND value = 'true'")
	require.Contains(t, sql, "THEN 'true' ELSE 'false'")
	require.Contains(t, sql, "ON CONFLICT (key) DO NOTHING")
	require.NotContains(t, sql, "DO UPDATE")
}
