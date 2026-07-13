-- 分组级模型倍率配置。
-- 该字段仅覆盖最终计费倍率，不修改渠道或平台模型价格。
-- key 为模型匹配模式（支持 * 通配符），value 为正数倍率。

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS model_rate_multipliers JSONB NOT NULL DEFAULT '{}'::jsonb;
