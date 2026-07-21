-- 分组池容量告警开关与认证缓存传播代际。
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS pool_capacity_alert_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS pool_capacity_alert_generation BIGINT NOT NULL DEFAULT 0;

COMMENT ON COLUMN groups.pool_capacity_alert_enabled IS '是否启用分组池容量告警';
COMMENT ON COLUMN groups.pool_capacity_alert_generation IS '分组池容量告警配置代际；开关变化时递增，仅供内部缓存一致性使用';
