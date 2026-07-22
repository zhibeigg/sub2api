-- Add per-group pool-capacity alert metrics and thresholds while preserving the
-- historical predicted-requests / 50 behavior and existing generations.
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS pool_capacity_alert_metric VARCHAR(32) NOT NULL DEFAULT 'predicted_requests',
    ADD COLUMN IF NOT EXISTS pool_capacity_alert_threshold_requests BIGINT NOT NULL DEFAULT 50,
    ADD COLUMN IF NOT EXISTS pool_capacity_alert_threshold_usd NUMERIC(30,12);

UPDATE groups
SET pool_capacity_alert_metric = 'predicted_requests'
WHERE pool_capacity_alert_metric IS NULL OR pool_capacity_alert_metric = '';
UPDATE groups
SET pool_capacity_alert_threshold_requests = 50
WHERE pool_capacity_alert_threshold_requests IS NULL;

ALTER TABLE groups
    ALTER COLUMN pool_capacity_alert_metric SET DEFAULT 'predicted_requests',
    ALTER COLUMN pool_capacity_alert_metric SET NOT NULL,
    ALTER COLUMN pool_capacity_alert_threshold_requests SET DEFAULT 50,
    ALTER COLUMN pool_capacity_alert_threshold_requests SET NOT NULL;

COMMENT ON COLUMN groups.pool_capacity_alert_metric IS '池容量告警指标：predicted_requests 或 remaining_balance_usd';
COMMENT ON COLUMN groups.pool_capacity_alert_threshold_requests IS '预计剩余请求数告警阈值，范围 1..1000000000';
COMMENT ON COLUMN groups.pool_capacity_alert_threshold_usd IS '剩余可用金额告警阈值（USD），范围 0.01..1e15；可空';

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'groups_pool_capacity_alert_metric_check') THEN
        ALTER TABLE groups ADD CONSTRAINT groups_pool_capacity_alert_metric_check
            CHECK (pool_capacity_alert_metric IN ('predicted_requests', 'remaining_balance_usd'));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'groups_pool_capacity_alert_threshold_requests_check') THEN
        ALTER TABLE groups ADD CONSTRAINT groups_pool_capacity_alert_threshold_requests_check
            CHECK (pool_capacity_alert_threshold_requests BETWEEN 1 AND 1000000000);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'groups_pool_capacity_alert_threshold_usd_check') THEN
        ALTER TABLE groups ADD CONSTRAINT groups_pool_capacity_alert_threshold_usd_check
            CHECK (pool_capacity_alert_threshold_usd IS NULL OR
                   (pool_capacity_alert_threshold_usd >= 0.01 AND pool_capacity_alert_threshold_usd <= 1000000000000000));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'groups_pool_capacity_alert_balance_threshold_required_check') THEN
        ALTER TABLE groups ADD CONSTRAINT groups_pool_capacity_alert_balance_threshold_required_check
            CHECK (pool_capacity_alert_metric <> 'remaining_balance_usd' OR pool_capacity_alert_threshold_usd IS NOT NULL);
    END IF;
END $$;

ALTER TABLE pool_capacity_alert_states
    ADD COLUMN IF NOT EXISTS alert_metric VARCHAR(32) NOT NULL DEFAULT 'predicted_requests',
    ADD COLUMN IF NOT EXISTS remaining_balance_usd NUMERIC(30,12),
    ADD COLUMN IF NOT EXISTS threshold_requests BIGINT NOT NULL DEFAULT 50,
    ADD COLUMN IF NOT EXISTS threshold_usd NUMERIC(30,12);

UPDATE pool_capacity_alert_states
SET alert_metric = 'predicted_requests'
WHERE alert_metric IS NULL OR alert_metric = '';
UPDATE pool_capacity_alert_states
SET threshold_requests = 50
WHERE threshold_requests IS NULL;

ALTER TABLE pool_capacity_alert_states
    ALTER COLUMN alert_metric SET DEFAULT 'predicted_requests',
    ALTER COLUMN alert_metric SET NOT NULL,
    ALTER COLUMN threshold_requests SET DEFAULT 50,
    ALTER COLUMN threshold_requests SET NOT NULL;

COMMENT ON COLUMN pool_capacity_alert_states.alert_metric IS '产生当前状态的池容量告警指标';
COMMENT ON COLUMN pool_capacity_alert_states.remaining_balance_usd IS '最近一次可信评估的剩余可用金额（USD）';
COMMENT ON COLUMN pool_capacity_alert_states.threshold_requests IS '当前状态使用的请求数阈值快照';
COMMENT ON COLUMN pool_capacity_alert_states.threshold_usd IS '当前状态使用的 USD 阈值快照';

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'pool_capacity_alert_states_metric_check') THEN
        ALTER TABLE pool_capacity_alert_states ADD CONSTRAINT pool_capacity_alert_states_metric_check
            CHECK (alert_metric IN ('predicted_requests', 'remaining_balance_usd'));
    END IF;
END $$;

ALTER TABLE pool_capacity_alert_events
    ADD COLUMN IF NOT EXISTS alert_metric VARCHAR(32) NOT NULL DEFAULT 'predicted_requests',
    ADD COLUMN IF NOT EXISTS remaining_balance_usd NUMERIC(30,12),
    ADD COLUMN IF NOT EXISTS threshold_usd NUMERIC(30,12);

UPDATE pool_capacity_alert_events
SET alert_metric = 'predicted_requests'
WHERE alert_metric IS NULL OR alert_metric = '';
UPDATE pool_capacity_alert_events
SET threshold_requests = 50
WHERE threshold_requests IS NULL AND alert_metric = 'predicted_requests';

ALTER TABLE pool_capacity_alert_events
    ALTER COLUMN alert_metric SET DEFAULT 'predicted_requests',
    ALTER COLUMN alert_metric SET NOT NULL,
    ALTER COLUMN predicted_requests DROP NOT NULL,
    ALTER COLUMN threshold_requests DROP NOT NULL,
    ALTER COLUMN threshold_requests DROP DEFAULT;

COMMENT ON COLUMN pool_capacity_alert_events.alert_metric IS '告警事件使用的池容量指标快照';
COMMENT ON COLUMN pool_capacity_alert_events.remaining_balance_usd IS '告警事件的剩余可用金额（USD）快照';
COMMENT ON COLUMN pool_capacity_alert_events.threshold_requests IS '告警事件的请求数阈值快照；金额事件可空';
COMMENT ON COLUMN pool_capacity_alert_events.threshold_usd IS '告警事件的 USD 阈值快照';

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'pool_capacity_alert_events_metric_check') THEN
        ALTER TABLE pool_capacity_alert_events ADD CONSTRAINT pool_capacity_alert_events_metric_check
            CHECK (alert_metric IN ('predicted_requests', 'remaining_balance_usd'));
    END IF;
END $$;
