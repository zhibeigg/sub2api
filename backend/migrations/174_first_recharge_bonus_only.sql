-- 将优惠码充值加成限制为用户首笔成功余额充值。
-- 用户级标记用于并发原子占用资格；订单快照用于未获资格时回退到基础到账金额。

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS first_recharge_bonus_used BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN users.first_recharge_bonus_used IS '是否已经完成过首笔余额充值；用于保证优惠码首充加成只生效一次';

ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS recharge_base_amount DECIMAL(20,2) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS recharge_bonus_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS first_recharge_bonus_applied BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN payment_orders.recharge_base_amount IS '未叠加优惠码首充加成时的基础到账金额；0 表示升级前创建的旧版订单';
COMMENT ON COLUMN payment_orders.recharge_bonus_multiplier IS '创建订单时快照的优惠码首充到账加成倍率；1 表示无加成';
COMMENT ON COLUMN payment_orders.first_recharge_bonus_applied IS '该订单是否成功占用并应用了用户的首充优惠';

-- 历史上只要余额订单已被支付过，就视为首充资格已经使用。
-- paid_at 比最终状态更可靠，也覆盖充值后进入退款或失败重试状态的订单。
UPDATE users AS u
SET first_recharge_bonus_used = TRUE
WHERE u.first_recharge_bonus_used = FALSE
  AND EXISTS (
      SELECT 1
      FROM payment_orders AS po
      WHERE po.user_id = u.id
        AND po.order_type = 'balance'
        AND po.paid_at IS NOT NULL
  );
