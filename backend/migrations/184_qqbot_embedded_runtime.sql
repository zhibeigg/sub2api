-- Embedded QQBot runtime configuration.
-- Credentials are intentionally empty and the runtime starts disabled.

INSERT INTO settings (key, value)
VALUES (
    'qqbot_runtime_config',
    '{"enabled":false,"app_id":"","sandbox":false,"public_base_url":"","worker_count":4,"queue_capacity":4096,"api_timeout_ms":10000,"config_version":1,"updated_at":"0001-01-01T00:00:00Z","updated_by":0,"change_summary":"{\"bootstrap\":false,\"enabled\":false}"}'
)
ON CONFLICT (key) DO NOTHING;

UPDATE settings
SET value = $qqbot$欢迎使用 PokeAPI 账户助手。

绑定账户：请私聊发送 /bind 你的邮箱
查看帮助：发送 /help

验证链接只会发送到 Sub2API 账户邮箱。数字 QQ 仅作为展示信息，实际身份以机器人 OpenID 为准。$qqbot$,
    updated_at = NOW()
WHERE key = 'qqbot_help_message'
  AND BTRIM(value) = '';
