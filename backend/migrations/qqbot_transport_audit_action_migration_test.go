package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration200AllowsTransportSettingsAuditActionIdempotently(t *testing.T) {
	content, err := FS.ReadFile("200_qqbot_audit_transport_settings.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "DROP CONSTRAINT IF EXISTS qqbot_binding_audit_logs_action_check")
	require.Contains(t, sql, "ADD CONSTRAINT qqbot_binding_audit_logs_action_check")
	for _, action := range []string{"settings", "onebot_settings", "onebot_request_approval", "transport_settings"} {
		require.Contains(t, sql, "'"+action+"'")
	}
}
