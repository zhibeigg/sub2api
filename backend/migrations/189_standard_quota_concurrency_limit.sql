-- 标准共享额度套餐并发限制：套餐配置与用户订阅实例快照。
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE subscription_plans
    ADD COLUMN IF NOT EXISTS concurrency_limit INTEGER;

ALTER TABLE user_subscriptions
    ADD COLUMN IF NOT EXISTS concurrency_limit INTEGER;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'subscription_plans_concurrency_limit_check'
          AND conrelid = 'subscription_plans'::regclass
    ) THEN
        ALTER TABLE subscription_plans
            ADD CONSTRAINT subscription_plans_concurrency_limit_check
            CHECK (concurrency_limit IS NULL OR concurrency_limit > 0) NOT VALID;
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'user_subscriptions_concurrency_limit_check'
          AND conrelid = 'user_subscriptions'::regclass
    ) THEN
        ALTER TABLE user_subscriptions
            ADD CONSTRAINT user_subscriptions_concurrency_limit_check
            CHECK (concurrency_limit IS NULL OR concurrency_limit > 0) NOT VALID;
    END IF;
END $$;

COMMENT ON COLUMN subscription_plans.concurrency_limit IS 'standard_quota 套餐并发上限；NULL 表示不额外限制';
COMMENT ON COLUMN user_subscriptions.concurrency_limit IS 'standard_quota 用户订阅实例并发上限快照；NULL 表示不额外限制';
