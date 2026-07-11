-- 管理员订单报表：快照用户注册优惠码归因，并保护已使用优惠码的历史链路。
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS signup_promo_code_id BIGINT,
    ADD COLUMN IF NOT EXISTS signup_promo_code VARCHAR(64),
    ADD COLUMN IF NOT EXISTS signup_promo_attribution VARCHAR(20) NOT NULL DEFAULT 'legacy_unknown';

COMMENT ON COLUMN payment_orders.signup_promo_code_id IS '创建订单时快照的注册优惠码 ID；不建立外键以保留历史归因';
COMMENT ON COLUMN payment_orders.signup_promo_code IS '创建订单时快照的注册优惠码文本';
COMMENT ON COLUMN payment_orders.signup_promo_attribution IS '注册优惠码归因：attributed、none 或 legacy_unknown';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'payment_orders_signup_promo_attribution_check'
    ) THEN
        ALTER TABLE payment_orders
            ADD CONSTRAINT payment_orders_signup_promo_attribution_check
            CHECK (signup_promo_attribution IN ('attributed', 'none', 'legacy_unknown'));
    END IF;
END $$;

-- 优先使用 users.promo_code_id；对于字段上线前的用户，回退到最早的优惠码使用记录。
WITH earliest_usage AS (
    SELECT DISTINCT ON (user_id)
        user_id,
        promo_code_id
    FROM promo_code_usages
    ORDER BY user_id, used_at ASC, id ASC
),
user_attribution AS (
    SELECT
        u.id AS user_id,
        COALESCE(u.promo_code_id, eu.promo_code_id) AS promo_code_id
    FROM users AS u
    LEFT JOIN earliest_usage AS eu ON eu.user_id = u.id
)
UPDATE payment_orders AS po
SET signup_promo_code_id = ua.promo_code_id,
    signup_promo_code = pc.code,
    signup_promo_attribution = 'attributed'
FROM user_attribution AS ua
LEFT JOIN promo_codes AS pc ON pc.id = ua.promo_code_id
WHERE po.user_id = ua.user_id
  AND ua.promo_code_id IS NOT NULL
  AND po.signup_promo_attribution = 'legacy_unknown';

-- 已使用优惠码只能禁用，禁止删除后清空用户归因或级联删除使用记录。
ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_promo_code_id_fkey;
ALTER TABLE users
    ADD CONSTRAINT users_promo_code_id_fkey
    FOREIGN KEY (promo_code_id) REFERENCES promo_codes(id) ON DELETE RESTRICT;

ALTER TABLE promo_code_usages
    DROP CONSTRAINT IF EXISTS promo_code_usages_promo_code_id_fkey;
ALTER TABLE promo_code_usages
    ADD CONSTRAINT promo_code_usages_promo_code_id_fkey
    FOREIGN KEY (promo_code_id) REFERENCES promo_codes(id) ON DELETE RESTRICT;
