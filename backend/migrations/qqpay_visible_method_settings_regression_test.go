package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration182SeedsQQPayVisibleMethodSettingsDisabled(t *testing.T) {
	content, err := FS.ReadFile("182_add_qqpay_visible_method_settings.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "('payment_visible_method_qqpay_source', '')")
	require.Contains(t, sql, "('payment_visible_method_qqpay_enabled', 'false')")
	require.Contains(t, sql, "ON CONFLICT (key) DO NOTHING")
}
