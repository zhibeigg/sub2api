-- 订阅套餐双类型：单订阅分组使用原生额度，标准分组使用套餐共享额度。
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE subscription_plans
    ADD COLUMN IF NOT EXISTS plan_type VARCHAR(40);

-- subscription_plan_groups 在迁移 175 中已回填主分组；这里仍使用 group_id 兜底，
-- 兼容手工建表或部分迁移环境。
WITH effective_plan_groups AS (
    SELECT sp.id AS plan_id, COALESCE(spg.group_id, sp.group_id) AS group_id
    FROM subscription_plans AS sp
    LEFT JOIN subscription_plan_groups AS spg ON spg.plan_id = sp.id
), plan_group_stats AS (
    SELECT
        epg.plan_id,
        COUNT(*) FILTER (WHERE g.id IS NOT NULL) AS group_count,
        BOOL_AND(g.subscription_type = 'subscription') FILTER (WHERE g.id IS NOT NULL) AS all_subscription,
        BOOL_AND(g.subscription_type = 'standard') FILTER (WHERE g.id IS NOT NULL) AS all_standard
    FROM effective_plan_groups AS epg
    LEFT JOIN groups AS g ON g.id = epg.group_id
    GROUP BY epg.plan_id
)
UPDATE subscription_plans AS sp
SET plan_type = CASE
    WHEN stats.group_count = 1 AND COALESCE(stats.all_subscription, FALSE) THEN 'subscription'
    WHEN stats.group_count >= 1 AND COALESCE(stats.all_standard, FALSE) THEN 'standard_quota'
    ELSE 'legacy_shared_subscription'
END
FROM plan_group_stats AS stats
WHERE stats.plan_id = sp.id
  AND (sp.plan_type IS NULL OR BTRIM(sp.plan_type) = '');

UPDATE subscription_plans
SET plan_type = 'legacy_shared_subscription'
WHERE plan_type IS NULL OR BTRIM(plan_type) = '';

-- 原生订阅套餐重新读取分组限额，不再保留套餐级额度副本。
UPDATE subscription_plans
SET daily_limit_usd = NULL,
    weekly_limit_usd = NULL,
    monthly_limit_usd = NULL
WHERE plan_type = 'subscription';

-- 多订阅分组旧套餐无法无损归入新模型：保留已有订阅/订单快照，但禁止继续售卖。
UPDATE subscription_plans
SET for_sale = FALSE
WHERE plan_type = 'legacy_shared_subscription';

ALTER TABLE subscription_plans
    ALTER COLUMN plan_type SET DEFAULT 'subscription',
    ALTER COLUMN plan_type SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'subscription_plans_plan_type_check'
    ) THEN
        ALTER TABLE subscription_plans
            ADD CONSTRAINT subscription_plans_plan_type_check
            CHECK (plan_type IN ('subscription', 'standard_quota', 'legacy_shared_subscription'));
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS subscriptionplan_plan_type
    ON subscription_plans (plan_type);

COMMENT ON COLUMN subscription_plans.plan_type IS
    'subscription=单订阅分组并读取分组额度；standard_quota=标准分组共享套餐额度；legacy_shared_subscription=只读兼容旧套餐';
