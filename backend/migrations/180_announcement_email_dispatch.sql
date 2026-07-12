CREATE TABLE IF NOT EXISTS announcement_email_jobs (
    id BIGSERIAL PRIMARY KEY,
    announcement_id BIGINT NOT NULL REFERENCES announcements(id) ON DELETE CASCADE,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    scheduled_at TIMESTAMPTZ NOT NULL,
    recipient_cutoff_id BIGINT NOT NULL DEFAULT 0,
    preparation_cursor_id BIGINT NOT NULL DEFAULT 0,
    recipient_count BIGINT NOT NULL DEFAULT 0,
    pending_count BIGINT NOT NULL DEFAULT 0,
    sending_count BIGINT NOT NULL DEFAULT 0,
    sent_count BIGINT NOT NULL DEFAULT 0,
    failed_count BIGINT NOT NULL DEFAULT 0,
    ambiguous_count BIGINT NOT NULL DEFAULT 0,
    skipped_count BIGINT NOT NULL DEFAULT 0,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    created_by BIGINT,
    last_error_code VARCHAR(128),
    announcement_title TEXT NOT NULL,
    announcement_content TEXT NOT NULL,
    announcement_starts_at TIMESTAMPTZ,
    lease_owner VARCHAR(128),
    lease_expires_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    CONSTRAINT announcement_email_jobs_announcement_uq UNIQUE (announcement_id)
);
CREATE INDEX IF NOT EXISTS announcement_email_jobs_status_scheduled_idx ON announcement_email_jobs(status, scheduled_at);
CREATE INDEX IF NOT EXISTS announcement_email_jobs_lease_idx ON announcement_email_jobs(lease_expires_at);

CREATE TABLE IF NOT EXISTS announcement_email_deliveries (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL REFERENCES announcement_email_jobs(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recipient_email VARCHAR(255) NOT NULL,
    recipient_name VARCHAR(100) NOT NULL DEFAULT '',
    locale VARCHAR(16) NOT NULL DEFAULT 'en',
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 5,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_owner VARCHAR(128),
    lease_expires_at TIMESTAMPTZ,
    last_error_class VARCHAR(32),
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at TIMESTAMPTZ,
    CONSTRAINT announcement_email_deliveries_job_user_uq UNIQUE(job_id, user_id)
);
CREATE INDEX IF NOT EXISTS announcement_email_deliveries_job_status_next_idx ON announcement_email_deliveries(job_id, status, next_attempt_at);
CREATE INDEX IF NOT EXISTS announcement_email_deliveries_status_next_idx ON announcement_email_deliveries(status, next_attempt_at);
CREATE INDEX IF NOT EXISTS announcement_email_deliveries_lease_idx ON announcement_email_deliveries(lease_expires_at);
