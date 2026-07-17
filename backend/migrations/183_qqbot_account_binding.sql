-- QQBot account binding foundation.
-- Adds the qqbot auth provider, one-time verification challenges, and immutable audit records.

ALTER TABLE auth_identities
    DROP CONSTRAINT IF EXISTS auth_identities_provider_type_check;

ALTER TABLE auth_identities
    ADD CONSTRAINT auth_identities_provider_type_check
    CHECK (provider_type IN ('email', 'linuxdo', 'wechat', 'oidc', 'github', 'google', 'dingtalk', 'qqbot'));

ALTER TABLE auth_identity_channels
    DROP CONSTRAINT IF EXISTS auth_identity_channels_provider_type_check;

ALTER TABLE auth_identity_channels
    ADD CONSTRAINT auth_identity_channels_provider_type_check
    CHECK (provider_type IN ('email', 'linuxdo', 'wechat', 'oidc', 'github', 'google', 'dingtalk', 'qqbot'));

ALTER TABLE pending_auth_sessions
    DROP CONSTRAINT IF EXISTS pending_auth_sessions_provider_type_check;

ALTER TABLE pending_auth_sessions
    ADD CONSTRAINT pending_auth_sessions_provider_type_check
    CHECK (provider_type IN ('email', 'linuxdo', 'wechat', 'oidc', 'github', 'google', 'dingtalk', 'qqbot'));

ALTER TABLE user_provider_default_grants
    DROP CONSTRAINT IF EXISTS user_provider_default_grants_provider_type_check;

ALTER TABLE user_provider_default_grants
    ADD CONSTRAINT user_provider_default_grants_provider_type_check
    CHECK (provider_type IN ('email', 'linuxdo', 'wechat', 'oidc', 'github', 'google', 'dingtalk', 'qqbot'));

CREATE TABLE IF NOT EXISTS qqbot_binding_challenges (
    id BIGSERIAL PRIMARY KEY,
    event_id TEXT NOT NULL,
    message_id TEXT NOT NULL DEFAULT '',
    challenge_token_hash CHAR(64) NOT NULL,
    bot_app_id TEXT NOT NULL,
    scene VARCHAR(20) NOT NULL,
    provider_subject TEXT NOT NULL,
    source_id TEXT NOT NULL DEFAULT '',
    channel_id TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    email_hash CHAR(64) NOT NULL,
    masked_email VARCHAR(255) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    declared_qq_number VARCHAR(20) NOT NULL DEFAULT '',
    bonus_amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    balance_before DECIMAL(20,8) NULL,
    balance_after DECIMAL(20,8) NULL,
    failure_code VARCHAR(80) NOT NULL DEFAULT '',
    email_status VARCHAR(20) NOT NULL DEFAULT 'pending',
    notification_status VARCHAR(20) NOT NULL DEFAULT 'pending',
    expires_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ NULL,
    revoked_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT qqbot_binding_challenges_status_check
        CHECK (status IN ('pending', 'completed', 'expired', 'failed', 'revoked')),
    CONSTRAINT qqbot_binding_challenges_email_status_check
        CHECK (email_status IN ('pending', 'queued', 'sent', 'failed', 'skipped')),
    CONSTRAINT qqbot_binding_challenges_notification_status_check
        CHECK (notification_status IN ('pending', 'queued', 'sent', 'failed', 'skipped')),
    CONSTRAINT qqbot_binding_challenges_scene_check
        CHECK (scene IN ('group', 'c2c', 'guild'))
);

CREATE UNIQUE INDEX IF NOT EXISTS qqbot_binding_challenges_event_id_key
    ON qqbot_binding_challenges (event_id);

CREATE UNIQUE INDEX IF NOT EXISTS qqbot_binding_challenges_token_hash_key
    ON qqbot_binding_challenges (challenge_token_hash);

CREATE INDEX IF NOT EXISTS qqbot_binding_challenges_status_created_idx
    ON qqbot_binding_challenges (status, created_at DESC);

CREATE INDEX IF NOT EXISTS qqbot_binding_challenges_user_id_idx
    ON qqbot_binding_challenges (user_id);

CREATE INDEX IF NOT EXISTS qqbot_binding_challenges_identity_idx
    ON qqbot_binding_challenges (bot_app_id, provider_subject);

CREATE INDEX IF NOT EXISTS qqbot_binding_challenges_expires_at_idx
    ON qqbot_binding_challenges (expires_at);

CREATE TABLE IF NOT EXISTS qqbot_binding_audit_logs (
    id BIGSERIAL PRIMARY KEY,
    challenge_id BIGINT NULL REFERENCES qqbot_binding_challenges(id) ON DELETE SET NULL,
    action VARCHAR(30) NOT NULL,
    status VARCHAR(20) NOT NULL,
    actor_type VARCHAR(20) NOT NULL DEFAULT 'system',
    actor_subject TEXT NOT NULL DEFAULT '',
    user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    bot_app_id TEXT NOT NULL DEFAULT '',
    provider_subject_hash CHAR(64) NOT NULL DEFAULT '',
    masked_email VARCHAR(255) NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT qqbot_binding_audit_logs_action_check
        CHECK (action IN ('prepare', 'complete', 'expire', 'email', 'notify', 'unbind', 'settings')),
    CONSTRAINT qqbot_binding_audit_logs_actor_type_check
        CHECK (actor_type IN ('system', 'qq_user', 'admin'))
);

CREATE INDEX IF NOT EXISTS qqbot_binding_audit_logs_challenge_id_idx
    ON qqbot_binding_audit_logs (challenge_id, created_at DESC);

CREATE INDEX IF NOT EXISTS qqbot_binding_audit_logs_user_id_idx
    ON qqbot_binding_audit_logs (user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS qqbot_binding_audit_logs_created_at_idx
    ON qqbot_binding_audit_logs (created_at DESC);

INSERT INTO settings (key, value)
VALUES
    ('qqbot_binding_enabled', 'true'),
    ('qqbot_first_bind_bonus', '5'),
    ('qqbot_link_ttl_minutes', '15'),
    ('qqbot_welcome_enabled', 'true'),
    ('qqbot_first_interaction_enabled', 'true'),
    ('qqbot_help_message', ''),
    ('qqbot_allowed_group_ids', '[]'),
    ('qqbot_allowed_guild_ids', '[]'),
    ('qqbot_guild_welcome_channels', '{}')
ON CONFLICT (key) DO NOTHING;
