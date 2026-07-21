-- Persistent state and per-recipient delivery queue for pool-mode capacity alerts.

CREATE TABLE IF NOT EXISTS pool_capacity_alert_states (
    id BIGSERIAL PRIMARY KEY,
    group_id BIGINT NOT NULL,
    group_generation BIGINT NOT NULL DEFAULT 0,
    account_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    billing_type SMALLINT NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'healthy',
    episode BIGINT NOT NULL DEFAULT 0,
    predicted_requests BIGINT,
    account_requests BIGINT,
    api_key_requests BIGINT,
    wallet_requests BIGINT,
    avg_account_cost NUMERIC(30,12) NOT NULL DEFAULT 0,
    avg_actual_cost NUMERIC(30,12) NOT NULL DEFAULT 0,
    sample_count INTEGER NOT NULL DEFAULT 0,
    bottleneck VARCHAR(32) NOT NULL DEFAULT '',
    last_evaluated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_alerted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT pool_capacity_alert_states_status_check CHECK (status IN ('healthy', 'low'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_pool_capacity_alert_states_scope
    ON pool_capacity_alert_states (group_id, group_generation, account_id, api_key_id, user_id, billing_type);
CREATE INDEX IF NOT EXISTS idx_pool_capacity_alert_states_group_generation
    ON pool_capacity_alert_states (group_id, group_generation);
CREATE INDEX IF NOT EXISTS idx_pool_capacity_alert_states_status_updated
    ON pool_capacity_alert_states (status, updated_at);

CREATE TABLE IF NOT EXISTS pool_capacity_alert_events (
    id BIGSERIAL PRIMARY KEY,
    state_id BIGINT NOT NULL REFERENCES pool_capacity_alert_states(id) ON DELETE CASCADE,
    episode BIGINT NOT NULL,
    group_id BIGINT NOT NULL,
    group_generation BIGINT NOT NULL DEFAULT 0,
    account_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    billing_type SMALLINT NOT NULL,
    group_name VARCHAR(255) NOT NULL DEFAULT '',
    account_name VARCHAR(255) NOT NULL DEFAULT '',
    api_key_name VARCHAR(255) NOT NULL DEFAULT '',
    user_email VARCHAR(255) NOT NULL DEFAULT '',
    predicted_requests BIGINT NOT NULL,
    threshold_requests BIGINT NOT NULL DEFAULT 50,
    account_requests BIGINT,
    api_key_requests BIGINT,
    wallet_requests BIGINT,
    avg_account_cost NUMERIC(30,12) NOT NULL,
    avg_actual_cost NUMERIC(30,12) NOT NULL,
    account_remaining NUMERIC(30,12),
    api_key_remaining NUMERIC(30,12),
    wallet_remaining NUMERIC(30,12),
    sample_count INTEGER NOT NULL DEFAULT 50,
    bottleneck VARCHAR(32) NOT NULL DEFAULT '',
    qqbot_app_id VARCHAR(128) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_pool_capacity_alert_events_state_episode
    ON pool_capacity_alert_events (state_id, episode);
CREATE INDEX IF NOT EXISTS idx_pool_capacity_alert_events_group_created
    ON pool_capacity_alert_events (group_id, created_at DESC);

CREATE TABLE IF NOT EXISTS pool_capacity_alert_deliveries (
    id BIGSERIAL PRIMARY KEY,
    event_id BIGINT NOT NULL REFERENCES pool_capacity_alert_events(id) ON DELETE CASCADE,
    channel VARCHAR(24) NOT NULL,
    recipient_user_id BIGINT NOT NULL,
    identity_channel_id BIGINT NOT NULL DEFAULT 0,
    recipient_email VARCHAR(255) NOT NULL DEFAULT '',
    recipient_name VARCHAR(100) NOT NULL DEFAULT '',
    locale VARCHAR(16) NOT NULL DEFAULT 'en',
    status VARCHAR(24) NOT NULL DEFAULT 'pending',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 6,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_owner VARCHAR(128),
    lease_expires_at TIMESTAMPTZ,
    last_error_class VARCHAR(32),
    last_error TEXT,
    sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT pool_capacity_alert_deliveries_channel_check CHECK (channel IN ('email', 'qqbot')),
    CONSTRAINT pool_capacity_alert_deliveries_status_check CHECK (status IN ('pending', 'sending', 'sent', 'retry', 'dead', 'cancelled'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_pool_capacity_alert_deliveries_recipient
    ON pool_capacity_alert_deliveries (event_id, channel, recipient_user_id, identity_channel_id);
CREATE INDEX IF NOT EXISTS idx_pool_capacity_alert_deliveries_due
    ON pool_capacity_alert_deliveries (status, next_attempt_at, id);
CREATE INDEX IF NOT EXISTS idx_pool_capacity_alert_deliveries_lease
    ON pool_capacity_alert_deliveries (lease_expires_at);
