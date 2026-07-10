-- 额度订阅：套餐多分组白名单、订阅实例额度快照与共享额度授权。
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE subscription_plans
    ADD COLUMN IF NOT EXISTS daily_limit_usd DECIMAL(20,8),
    ADD COLUMN IF NOT EXISTS weekly_limit_usd DECIMAL(20,8),
    ADD COLUMN IF NOT EXISTS monthly_limit_usd DECIMAL(20,8);

-- 存量套餐复制当前主分组额度；升级后的新订阅按套餐值生成快照。
UPDATE subscription_plans AS sp
SET daily_limit_usd = g.daily_limit_usd,
    weekly_limit_usd = g.weekly_limit_usd,
    monthly_limit_usd = g.monthly_limit_usd
FROM groups AS g
WHERE g.id = sp.group_id
  AND sp.daily_limit_usd IS NULL
  AND sp.weekly_limit_usd IS NULL
  AND sp.monthly_limit_usd IS NULL;

CREATE TABLE IF NOT EXISTS subscription_plan_groups (
    priority   INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    plan_id    BIGINT NOT NULL,
    group_id   BIGINT NOT NULL,
    PRIMARY KEY (plan_id, group_id),
    CONSTRAINT subscription_plan_groups_subscription_plans_plan
        FOREIGN KEY (plan_id) REFERENCES subscription_plans (id) ON DELETE CASCADE,
    CONSTRAINT subscription_plan_groups_groups_group
        FOREIGN KEY (group_id) REFERENCES groups (id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS subscriptionplangroup_group_id
    ON subscription_plan_groups (group_id);
CREATE INDEX IF NOT EXISTS subscriptionplangroup_plan_id_priority
    ON subscription_plan_groups (plan_id, priority);

INSERT INTO subscription_plan_groups (plan_id, group_id, priority)
SELECT id, group_id, 0
FROM subscription_plans
ON CONFLICT (plan_id, group_id) DO NOTHING;

ALTER TABLE user_subscriptions
    ADD COLUMN IF NOT EXISTS source_plan_id BIGINT,
    ADD COLUMN IF NOT EXISTS quota_snapshotted BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS daily_limit_usd DECIMAL(20,8),
    ADD COLUMN IF NOT EXISTS weekly_limit_usd DECIMAL(20,8),
    ADD COLUMN IF NOT EXISTS monthly_limit_usd DECIMAL(20,8);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'user_subscriptions_subscription_plans_source_plan'
    ) THEN
        ALTER TABLE user_subscriptions
            ADD CONSTRAINT user_subscriptions_subscription_plans_source_plan
            FOREIGN KEY (source_plan_id) REFERENCES subscription_plans (id) ON DELETE SET NULL;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS usersubscription_source_plan_id
    ON user_subscriptions (source_plan_id);

CREATE TABLE IF NOT EXISTS user_subscription_groups (
    subscription_id BIGINT NOT NULL,
    user_id          BIGINT NOT NULL,
    group_id         BIGINT NOT NULL,
    enabled          BOOLEAN NOT NULL DEFAULT TRUE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (subscription_id, group_id),
    CONSTRAINT user_subscription_groups_user_subscriptions_subscription
        FOREIGN KEY (subscription_id) REFERENCES user_subscriptions (id) ON DELETE CASCADE,
    CONSTRAINT user_subscription_groups_users_user
        FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT user_subscription_groups_groups_group
        FOREIGN KEY (group_id) REFERENCES groups (id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS usersubscriptiongroup_group_id
    ON user_subscription_groups (group_id);
CREATE INDEX IF NOT EXISTS usersubscriptiongroup_subscription_id_enabled
    ON user_subscription_groups (subscription_id, enabled);
CREATE UNIQUE INDEX IF NOT EXISTS usersubscriptiongroup_user_id_group_id
    ON user_subscription_groups (user_id, group_id)
    WHERE enabled = TRUE;

INSERT INTO user_subscription_groups (subscription_id, user_id, group_id, enabled)
SELECT id, user_id, group_id, deleted_at IS NULL
FROM user_subscriptions
ON CONFLICT (subscription_id, group_id) DO UPDATE
SET user_id = EXCLUDED.user_id,
    enabled = EXCLUDED.enabled,
    updated_at = NOW();

ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS subscription_snapshot JSONB;

COMMENT ON COLUMN subscription_plans.daily_limit_usd IS '套餐日共享额度；NULL 表示不限额';
COMMENT ON COLUMN subscription_plans.weekly_limit_usd IS '套餐周共享额度；NULL 表示不限额';
COMMENT ON COLUMN subscription_plans.monthly_limit_usd IS '套餐月共享额度；NULL 表示不限额';
COMMENT ON COLUMN user_subscriptions.quota_snapshotted IS 'TRUE 时使用订阅实例额度快照；FALSE 时兼容读取主分组额度';
COMMENT ON COLUMN payment_orders.subscription_snapshot IS '订阅订单履约快照：套餐、白名单分组、共享额度和有效期';
