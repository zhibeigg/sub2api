-- Add administrator-configurable group list prediction modes.
-- This display-only configuration is intentionally independent from pool capacity alerts.
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS predicted_capacity_mode VARCHAR(32) NOT NULL DEFAULT 'historical_requests',
    ADD COLUMN IF NOT EXISTS predicted_image_unit_cost_usd NUMERIC(30,12);

UPDATE groups
SET predicted_image_unit_cost_usd = NULL
WHERE predicted_image_unit_cost_usd IS NOT NULL
  AND predicted_image_unit_cost_usd NOT BETWEEN 0.000000000001 AND 1000000000000000;

UPDATE groups
SET predicted_capacity_mode = 'historical_requests'
WHERE predicted_capacity_mode IS NULL
   OR predicted_capacity_mode NOT IN ('historical_requests', 'fixed_image_cost')
   OR (predicted_capacity_mode = 'fixed_image_cost' AND predicted_image_unit_cost_usd IS NULL);

-- Re-assert defaults and nullability so rerunning after a partially applied migration is safe.
ALTER TABLE groups
    ALTER COLUMN predicted_capacity_mode SET DEFAULT 'historical_requests',
    ALTER COLUMN predicted_capacity_mode SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'groups_predicted_capacity_mode_check'
          AND conrelid = 'groups'::regclass
    ) THEN
        ALTER TABLE groups ADD CONSTRAINT groups_predicted_capacity_mode_check
            CHECK (predicted_capacity_mode IN ('historical_requests', 'fixed_image_cost'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'groups_predicted_image_unit_cost_usd_range_check'
          AND conrelid = 'groups'::regclass
    ) THEN
        ALTER TABLE groups ADD CONSTRAINT groups_predicted_image_unit_cost_usd_range_check
            CHECK (
                predicted_image_unit_cost_usd IS NULL
                OR predicted_image_unit_cost_usd BETWEEN 0.000000000001 AND 1000000000000000
            );
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'groups_predicted_fixed_image_cost_required_check'
          AND conrelid = 'groups'::regclass
    ) THEN
        ALTER TABLE groups ADD CONSTRAINT groups_predicted_fixed_image_cost_required_check
            CHECK (
                predicted_capacity_mode <> 'fixed_image_cost'
                OR predicted_image_unit_cost_usd IS NOT NULL
            );
    END IF;
END $$;

COMMENT ON COLUMN groups.predicted_capacity_mode IS '管理员分组列表容量预测模式：historical_requests 或 fixed_image_cost';
COMMENT ON COLUMN groups.predicted_image_unit_cost_usd IS 'fixed_image_cost 模式下每个预测图片单位消耗的账号容量 USD';
