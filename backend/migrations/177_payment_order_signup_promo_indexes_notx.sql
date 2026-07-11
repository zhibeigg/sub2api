-- migrate:notx
-- 订单注册优惠码归因报表索引；并发创建避免阻塞线上写入。
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_payment_orders_signup_promo_created
    ON payment_orders (signup_promo_code_id, created_at DESC, id DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_payment_orders_signup_promo_paid
    ON payment_orders (signup_promo_code_id, paid_at DESC, id DESC)
    WHERE paid_at IS NOT NULL;
