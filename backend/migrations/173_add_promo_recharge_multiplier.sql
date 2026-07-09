-- 优惠链接功能：优惠码新增"充值到账加成倍率"，用户表新增"注册绑定的优惠码"

-- promo_codes: 充值到账加成倍率（用户通过该优惠码注册后，充值到账金额 = 支付金额 × 全局倍率 × 该倍率）
-- 默认 1 表示不额外加成；> 1 表示到账更多。
ALTER TABLE promo_codes
    ADD COLUMN IF NOT EXISTS recharge_bonus_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1;

COMMENT ON COLUMN promo_codes.recharge_bonus_multiplier IS '充值到账加成倍率，1=不加成，>1=到账更多';

-- users: 注册时绑定的优惠码 id（可空，NULL 表示未通过优惠链接注册）
-- 用于充值加成计算与按优惠链接筛选用量统计。SET NULL：优惠码被删除时保留用户，仅解绑。
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS promo_code_id BIGINT DEFAULT NULL REFERENCES promo_codes(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_users_promo_code_id ON users(promo_code_id);

COMMENT ON COLUMN users.promo_code_id IS '注册时绑定的优惠码 id（优惠链接），用于充值加成与用量统计筛选';
