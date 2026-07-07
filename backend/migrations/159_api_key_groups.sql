-- 多分组绑定表：一个 API key 可绑定多个分组，按 priority 升序（值越小优先级越高）
-- 依次尝试调度。此表为加法式设计——api_keys.group_id 旧字段与旧逻辑保留不变，
-- 无本表绑定的 key 仍走单分组路径，存量数据零影响。
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

CREATE TABLE IF NOT EXISTS api_key_groups (
    priority   INTEGER      NOT NULL DEFAULT 0,       -- 值越小优先级越高
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    api_key_id BIGINT       NOT NULL,
    group_id   BIGINT       NOT NULL,
    PRIMARY KEY (api_key_id, group_id),
    CONSTRAINT api_key_groups_api_keys_api_key
        FOREIGN KEY (api_key_id) REFERENCES api_keys (id) ON DELETE CASCADE,
    CONSTRAINT api_key_groups_groups_group
        FOREIGN KEY (group_id) REFERENCES groups (id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS apikeygroup_group_id
    ON api_key_groups (group_id);
CREATE INDEX IF NOT EXISTS apikeygroup_api_key_id_priority
    ON api_key_groups (api_key_id, priority);
