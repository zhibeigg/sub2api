-- 将 OpenAI 原生订阅套餐迁移至“GPT 稳定分组 无限制”标准额度分组。
-- 保留迁移时的套餐/用户日周月额度、已用额度、窗口和有效期；不复制用户专属倍率，
-- 使 GPT 订阅消费始终按目标稳定分组的默认、模型和高峰倍率计算。
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

DO $$
DECLARE
    target_count INTEGER;
    target_group RECORD;
BEGIN
    SELECT COUNT(*)
    INTO target_count
    FROM groups
    WHERE name = 'GPT 稳定分组 无限制'
      AND deleted_at IS NULL;

    IF target_count <> 1 THEN
        RAISE EXCEPTION 'expected exactly one active GPT standard group named "GPT 稳定分组 无限制", found %', target_count;
    END IF;

    SELECT id, platform, subscription_type, status
    INTO target_group
    FROM groups
    WHERE name = 'GPT 稳定分组 无限制'
      AND deleted_at IS NULL;

    IF target_group.platform <> 'openai'
       OR target_group.subscription_type <> 'standard'
       OR target_group.status <> 'active' THEN
        RAISE EXCEPTION 'GPT standard group must be active OpenAI standard billing group';
    END IF;
END $$;

CREATE TEMP TABLE gpt_subscription_migration_sources ON COMMIT DROP AS
SELECT
    sp.id AS plan_id,
    sp.group_id AS old_group_id,
    target.id AS target_group_id,
    old_group.daily_limit_usd,
    old_group.weekly_limit_usd,
    old_group.monthly_limit_usd
FROM subscription_plans AS sp
JOIN groups AS old_group ON old_group.id = sp.group_id
CROSS JOIN (
    SELECT id
    FROM groups
    WHERE name = 'GPT 稳定分组 无限制'
      AND deleted_at IS NULL
) AS target
WHERE sp.plan_type = 'subscription'
  AND old_group.platform = 'openai'
  AND old_group.subscription_type = 'subscription'
  AND old_group.deleted_at IS NULL;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM gpt_subscription_migration_sources
        WHERE daily_limit_usd IS NULL
          AND weekly_limit_usd IS NULL
          AND monthly_limit_usd IS NULL
    ) THEN
        RAISE EXCEPTION 'cannot convert unlimited native GPT subscription plans to standard_quota without a current quota snapshot';
    END IF;
END $$;

CREATE TEMP TABLE gpt_subscription_migration_users ON COMMIT DROP AS
SELECT DISTINCT
    us.id AS subscription_id,
    us.user_id,
    us.group_id AS old_group_id,
    source.target_group_id,
    COALESCE(us.daily_limit_usd, source.daily_limit_usd) AS daily_limit_usd,
    COALESCE(us.weekly_limit_usd, source.weekly_limit_usd) AS weekly_limit_usd,
    COALESCE(us.monthly_limit_usd, source.monthly_limit_usd) AS monthly_limit_usd
FROM user_subscriptions AS us
JOIN gpt_subscription_migration_sources AS source ON source.old_group_id = us.group_id
WHERE us.deleted_at IS NULL;

-- 一个用户同时持有多个待迁移 GPT 订阅，或已持有另一条目标分组订阅时不能安全合并额度。
-- 直接失败可避免覆盖有效期、已用额度或窗口锚点。
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM gpt_subscription_migration_users
        GROUP BY user_id
        HAVING COUNT(*) > 1
    ) THEN
        RAISE EXCEPTION 'cannot migrate GPT subscriptions: a user has multiple active migration candidates';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM gpt_subscription_migration_users AS source
        JOIN user_subscriptions AS existing
          ON existing.user_id = source.user_id
         AND existing.group_id = source.target_group_id
         AND existing.deleted_at IS NULL
         AND existing.id <> source.subscription_id
    ) THEN
        RAISE EXCEPTION 'cannot migrate GPT subscriptions: a user already has a target GPT stable-group subscription';
    END IF;
END $$;

-- 先将套餐额度从旧原生订阅分组快照到套餐，再替换套餐白名单分组。
UPDATE subscription_plans AS sp
SET plan_type = 'standard_quota',
    group_id = source.target_group_id,
    daily_limit_usd = source.daily_limit_usd,
    weekly_limit_usd = source.weekly_limit_usd,
    monthly_limit_usd = source.monthly_limit_usd,
    updated_at = NOW()
FROM gpt_subscription_migration_sources AS source
WHERE sp.id = source.plan_id;

DELETE FROM subscription_plan_groups AS binding
USING gpt_subscription_migration_sources AS source
WHERE binding.plan_id = source.plan_id;

INSERT INTO subscription_plan_groups (plan_id, group_id, priority)
SELECT plan_id, target_group_id, 0
FROM gpt_subscription_migration_sources
ON CONFLICT (plan_id, group_id) DO UPDATE
SET priority = EXCLUDED.priority;

-- 将有效用户订阅变成额度快照订阅，同时完整保留原有效期、已用额度和滚动窗口。
UPDATE user_subscriptions AS us
SET group_id = source.target_group_id,
    quota_snapshotted = TRUE,
    daily_limit_usd = source.daily_limit_usd,
    weekly_limit_usd = source.weekly_limit_usd,
    monthly_limit_usd = source.monthly_limit_usd,
    updated_at = NOW()
FROM gpt_subscription_migration_users AS source
WHERE us.id = source.subscription_id;

INSERT INTO user_subscription_groups (subscription_id, user_id, group_id, enabled, created_at, updated_at)
SELECT subscription_id, user_id, target_group_id, TRUE, NOW(), NOW()
FROM gpt_subscription_migration_users
ON CONFLICT (subscription_id, group_id) DO UPDATE
SET user_id = EXCLUDED.user_id,
    enabled = TRUE,
    updated_at = NOW();

DELETE FROM user_subscription_groups AS binding
USING gpt_subscription_migration_users AS source
WHERE binding.subscription_id = source.subscription_id
  AND binding.group_id = source.old_group_id
  AND binding.group_id <> source.target_group_id;

-- 目标分组若为专属分组，必须保留用户的传统专属分组授权。
-- 不删除旧分组授权，避免误撤销该用户由订阅迁移以外来源获得的访问权限。
INSERT INTO user_allowed_groups (user_id, group_id)
SELECT DISTINCT source.user_id, source.target_group_id
FROM gpt_subscription_migration_users AS source
JOIN groups AS target ON target.id = source.target_group_id
WHERE target.is_exclusive = TRUE
ON CONFLICT (user_id, group_id) DO NOTHING;

-- 受限标准分组用户必须显式获得目标分组访问权限；其他用户沿用 inherit 语义。
INSERT INTO user_group_access_groups (user_id, group_id)
SELECT DISTINCT source.user_id, source.target_group_id
FROM gpt_subscription_migration_users AS source
JOIN users AS u ON u.id = source.user_id
WHERE u.group_access_mode = 'restricted'
ON CONFLICT (user_id, group_id) DO NOTHING;

-- 用目标稳定分组替换受影响用户 API Key 的主/次分组；若次级绑定已存在则先去重。
DELETE FROM api_key_groups AS binding
USING api_keys AS key,
      gpt_subscription_migration_users AS source,
      api_key_groups AS target_binding
WHERE binding.api_key_id = key.id
  AND key.user_id = source.user_id
  AND key.deleted_at IS NULL
  AND binding.group_id = source.old_group_id
  AND target_binding.api_key_id = binding.api_key_id
  AND target_binding.group_id = source.target_group_id;

UPDATE api_key_groups AS binding
SET group_id = source.target_group_id
FROM api_keys AS key,
     gpt_subscription_migration_users AS source
WHERE binding.api_key_id = key.id
  AND key.user_id = source.user_id
  AND key.deleted_at IS NULL
  AND binding.group_id = source.old_group_id;

UPDATE api_keys AS key
SET group_id = source.target_group_id,
    updated_at = NOW()
FROM gpt_subscription_migration_users AS source
WHERE key.user_id = source.user_id
  AND key.deleted_at IS NULL
  AND key.group_id = source.old_group_id;

-- 多分组 Key 继续以 priority 最小的绑定作为兼容主 group_id。
UPDATE api_keys AS key
SET group_id = selected.group_id,
    updated_at = NOW()
FROM (
    SELECT DISTINCT ON (key.id)
        key.id AS api_key_id,
        binding.group_id
    FROM api_keys AS key
    JOIN gpt_subscription_migration_users AS source ON source.user_id = key.user_id
    JOIN api_key_groups AS binding ON binding.api_key_id = key.id
    WHERE key.deleted_at IS NULL
    ORDER BY key.id, binding.priority, binding.group_id
) AS selected
WHERE key.id = selected.api_key_id
  AND key.group_id IS DISTINCT FROM selected.group_id;

-- 遗留旧分组或目标分组的用户专属倍率都不能覆盖 GPT 稳定分组倍率。
DELETE FROM user_group_rate_multipliers AS rate
USING gpt_subscription_migration_users AS source
WHERE rate.user_id = source.user_id
  AND rate.group_id IN (source.old_group_id, source.target_group_id);

-- 尚未支付/履约的 GPT 套餐订单也要使用新套餐快照，避免之后重新发放旧订阅分组。
UPDATE payment_orders AS order_record
SET subscription_group_id = source.target_group_id,
    subscription_snapshot = jsonb_build_object(
        'schema_version', 3,
        'plan_id', source.plan_id,
        'plan_type', 'standard_quota',
        'group_ids', jsonb_build_array(source.target_group_id),
        'validity_days', COALESCE(order_record.subscription_days, plan.validity_days),
        'daily_limit_usd', source.daily_limit_usd,
        'weekly_limit_usd', source.weekly_limit_usd,
        'monthly_limit_usd', source.monthly_limit_usd,
        'concurrency_limit', plan.concurrency_limit
    ),
    updated_at = NOW()
FROM gpt_subscription_migration_sources AS source
JOIN subscription_plans AS plan ON plan.id = source.plan_id
WHERE order_record.plan_id = source.plan_id
  AND order_record.order_type = 'subscription'
  AND order_record.status = 'PENDING';
