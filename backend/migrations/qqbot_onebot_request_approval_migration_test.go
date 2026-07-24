package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration198AllowsOneBotRequestApprovalAuditActionIdempotently(t *testing.T) {
	content, err := FS.ReadFile("198_qqbot_audit_onebot_request_approval.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "DROP CONSTRAINT IF EXISTS qqbot_binding_audit_logs_action_check")
	require.Contains(t, sql, "ADD CONSTRAINT qqbot_binding_audit_logs_action_check")
	require.Contains(t, sql, "'onebot_settings'")
	require.Contains(t, sql, "'onebot_request_approval'")
}
