package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration176AddsCyberAbusePolicyMetadataWithoutPersistingRules(t *testing.T) {
	content, err := FS.ReadFile("176_content_moderation_cyber_abuse.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS policy_source")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS policy_rule_id")
	require.Contains(t, sql, "NOT NULL DEFAULT ''")
	require.Contains(t, sql, "不存储完整规则或原始请求正文")
	require.NotContains(t, sql, "CREATE INDEX")
}
