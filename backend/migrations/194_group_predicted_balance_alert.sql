-- Convert remaining_balance_usd alerts from final billing-context bottlenecks
-- into a durable group-level predicted balance:
--   sum(pool authoritative USD) + sum(normal estimated USD).
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE pool_capacity_alert_states
    ADD COLUMN IF NOT EXISTS scope_type VARCHAR(16) NOT NULL DEFAULT 'context',
    ADD COLUMN IF NOT EXISTS pool_authoritative_balance_usd NUMERIC(30,12),
    ADD COLUMN IF NOT EXISTS normal_estimated_balance_usd NUMERIC(30,12),
    ADD COLUMN IF NOT EXISTS pool_account_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS normal_account_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS skipped_account_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS unknown_account_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS stale_account_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS incompatible_unit_account_count INTEGER NOT NULL DEFAULT 0;

UPDATE pool_capacity_alert_states
SET scope_type = 'context'
WHERE scope_type IS NULL OR scope_type = '';

ALTER TABLE pool_capacity_alert_states
    ALTER COLUMN account_id DROP NOT NULL,
    ALTER COLUMN api_key_id DROP NOT NULL,
    ALTER COLUMN user_id DROP NOT NULL,
    ALTER COLUMN billing_type DROP NOT NULL;

DROP INDEX IF EXISTS idx_pool_capacity_alert_states_scope;
CREATE UNIQUE INDEX IF NOT EXISTS idx_pool_capacity_alert_states_context_scope
    ON pool_capacity_alert_states (group_id, group_generation, account_id, api_key_id, user_id, billing_type)
    WHERE scope_type = 'context';
CREATE UNIQUE INDEX IF NOT EXISTS idx_pool_capacity_alert_states_group_scope
    ON pool_capacity_alert_states (group_id, group_generation)
    WHERE scope_type = 'group';

COMMENT ON COLUMN pool_capacity_alert_states.scope_type IS '告警状态范围：context 为请求数旧模式，group 为分组预测余额模式';
COMMENT ON COLUMN pool_capacity_alert_states.pool_authoritative_balance_usd IS '分组内池模式账号权威 USD 余额小计';
COMMENT ON COLUMN pool_capacity_alert_states.normal_estimated_balance_usd IS '分组内普通账号估算 USD 余额小计';
COMMENT ON COLUMN pool_capacity_alert_states.pool_account_count IS '分组预测余额中纳入的池模式账号数';
COMMENT ON COLUMN pool_capacity_alert_states.normal_account_count IS '分组预测余额中纳入的普通账号数';

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'pool_capacity_alert_states_scope_type_check') THEN
        ALTER TABLE pool_capacity_alert_states ADD CONSTRAINT pool_capacity_alert_states_scope_type_check
            CHECK (scope_type IN ('context', 'group'));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'pool_capacity_alert_states_scope_columns_check') THEN
        ALTER TABLE pool_capacity_alert_states ADD CONSTRAINT pool_capacity_alert_states_scope_columns_check
            CHECK (
                (scope_type = 'context' AND account_id IS NOT NULL AND api_key_id IS NOT NULL AND user_id IS NOT NULL AND billing_type IS NOT NULL)
                OR
                (scope_type = 'group' AND account_id IS NULL AND api_key_id IS NULL AND user_id IS NULL AND billing_type IS NULL)
            );
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'pool_capacity_alert_states_scope_metric_check') THEN
        ALTER TABLE pool_capacity_alert_states ADD CONSTRAINT pool_capacity_alert_states_scope_metric_check
            CHECK (
                (scope_type = 'context' AND alert_metric = 'predicted_requests')
                OR
                (scope_type = 'group' AND alert_metric = 'remaining_balance_usd')
            ) NOT VALID;
    END IF;
END $$;

ALTER TABLE pool_capacity_alert_events
    ADD COLUMN IF NOT EXISTS scope_type VARCHAR(16) NOT NULL DEFAULT 'context',
    ADD COLUMN IF NOT EXISTS pool_authoritative_balance_usd NUMERIC(30,12),
    ADD COLUMN IF NOT EXISTS normal_estimated_balance_usd NUMERIC(30,12),
    ADD COLUMN IF NOT EXISTS pool_account_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS normal_account_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS skipped_account_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS unknown_account_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS stale_account_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS incompatible_unit_account_count INTEGER NOT NULL DEFAULT 0;

UPDATE pool_capacity_alert_events
SET scope_type = 'context'
WHERE scope_type IS NULL OR scope_type = '';

ALTER TABLE pool_capacity_alert_events
    ALTER COLUMN account_id DROP NOT NULL,
    ALTER COLUMN api_key_id DROP NOT NULL,
    ALTER COLUMN user_id DROP NOT NULL,
    ALTER COLUMN billing_type DROP NOT NULL;

COMMENT ON COLUMN pool_capacity_alert_events.scope_type IS '告警事件范围：context 或 group';
COMMENT ON COLUMN pool_capacity_alert_events.pool_authoritative_balance_usd IS '事件发生时分组池模式账号权威 USD 余额小计';
COMMENT ON COLUMN pool_capacity_alert_events.normal_estimated_balance_usd IS '事件发生时分组普通账号估算 USD 余额小计';
COMMENT ON COLUMN pool_capacity_alert_events.pool_account_count IS '事件发生时纳入的池模式账号数';
COMMENT ON COLUMN pool_capacity_alert_events.normal_account_count IS '事件发生时纳入的普通账号数';

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'pool_capacity_alert_events_scope_type_check') THEN
        ALTER TABLE pool_capacity_alert_events ADD CONSTRAINT pool_capacity_alert_events_scope_type_check
            CHECK (scope_type IN ('context', 'group'));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'pool_capacity_alert_events_scope_columns_check') THEN
        ALTER TABLE pool_capacity_alert_events ADD CONSTRAINT pool_capacity_alert_events_scope_columns_check
            CHECK (
                (scope_type = 'context' AND account_id IS NOT NULL AND api_key_id IS NOT NULL AND user_id IS NOT NULL AND billing_type IS NOT NULL)
                OR
                (scope_type = 'group' AND account_id IS NULL AND api_key_id IS NULL AND user_id IS NULL AND billing_type IS NULL)
            );
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'pool_capacity_alert_events_scope_metric_check') THEN
        ALTER TABLE pool_capacity_alert_events ADD CONSTRAINT pool_capacity_alert_events_scope_metric_check
            CHECK (
                (scope_type = 'context' AND alert_metric = 'predicted_requests')
                OR
                (scope_type = 'group' AND alert_metric = 'remaining_balance_usd')
            ) NOT VALID;
    END IF;
END $$;

-- The meaning of remaining_balance_usd changes at this migration. Advance the
-- generation once so old-context deliveries are cancelled before any new
-- group-level episode is created.
UPDATE groups
SET pool_capacity_alert_generation = pool_capacity_alert_generation + 1,
    updated_at = NOW()
WHERE pool_capacity_alert_metric = 'remaining_balance_usd';
