-- 风控中心：本地 Cyber Abuse Guard 策略来源与稳定规则标识。
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '5min';

ALTER TABLE content_moderation_logs
    ADD COLUMN IF NOT EXISTS policy_source VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS policy_rule_id VARCHAR(128) NOT NULL DEFAULT '';

COMMENT ON COLUMN content_moderation_logs.policy_source IS '策略来源，如 local_cyber_guard 或 upstream_cyber_policy';
COMMENT ON COLUMN content_moderation_logs.policy_rule_id IS '稳定策略规则 ID；不存储完整规则或原始请求正文';
