-- Add per-user standard-group access restrictions without changing the legacy
-- user_allowed_groups exclusive-group grant semantics.

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS group_access_mode VARCHAR(20) NOT NULL DEFAULT 'inherit';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'users_group_access_mode_check'
    ) THEN
        ALTER TABLE users
            ADD CONSTRAINT users_group_access_mode_check
            CHECK (group_access_mode IN ('inherit', 'restricted'));
    END IF;
END;
$$;

CREATE TABLE IF NOT EXISTS user_group_access_groups (
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    group_id   BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, group_id)
);

CREATE INDEX IF NOT EXISTS idx_user_group_access_groups_group_id
    ON user_group_access_groups(group_id);

COMMENT ON COLUMN users.group_access_mode IS
    'Standard-group access mode: inherit keeps legacy authorization; restricted intersects it with user_group_access_groups';
COMMENT ON TABLE user_group_access_groups IS
    'Per-user standard-group allowlist used only when users.group_access_mode is restricted';

-- Reusable helper: restriction changes must invalidate every active API key for
-- the user, including keys whose affected group only appears in api_key_groups.
CREATE OR REPLACE FUNCTION enqueue_user_api_keys_auth_cache_invalidation(target_user_id BIGINT)
RETURNS VOID
LANGUAGE plpgsql
AS $$
BEGIN
    IF target_user_id IS NULL OR target_user_id <= 0 THEN
        RETURN;
    END IF;

    INSERT INTO auth_cache_invalidation_outbox (cache_key)
    SELECT encode(sha256(convert_to(k.key, 'UTF8')), 'hex')
    FROM api_keys AS k
    WHERE k.user_id = target_user_id
      AND k.deleted_at IS NULL
      AND k.key <> '';
END;
$$;

-- Extend the existing user trigger so mode changes are durable across instances.
CREATE OR REPLACE FUNCTION enqueue_user_auth_cache_invalidation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    target_user_id BIGINT;
BEGIN
    target_user_id := OLD.id;
    IF TG_OP = 'UPDATE'
       AND OLD.status IS NOT DISTINCT FROM NEW.status
       AND OLD.role IS NOT DISTINCT FROM NEW.role
       AND OLD.deleted_at IS NOT DISTINCT FROM NEW.deleted_at
       AND OLD.group_access_mode IS NOT DISTINCT FROM NEW.group_access_mode THEN
        RETURN NEW;
    END IF;

    PERFORM enqueue_user_api_keys_auth_cache_invalidation(target_user_id);
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

-- Exclusive grants also affect low-priority multi-group bindings, so invalidate
-- all keys owned by the changed user rather than only api_keys.group_id matches.
CREATE OR REPLACE FUNCTION enqueue_allowed_group_auth_cache_invalidation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'UPDATE' THEN
        IF OLD.user_id IS NOT DISTINCT FROM NEW.user_id
           AND OLD.group_id IS NOT DISTINCT FROM NEW.group_id THEN
            RETURN NEW;
        END IF;
        PERFORM enqueue_user_api_keys_auth_cache_invalidation(OLD.user_id);
        PERFORM enqueue_user_api_keys_auth_cache_invalidation(NEW.user_id);
        RETURN NEW;
    END IF;

    IF TG_OP = 'INSERT' THEN
        PERFORM enqueue_user_api_keys_auth_cache_invalidation(NEW.user_id);
        RETURN NEW;
    END IF;

    PERFORM enqueue_user_api_keys_auth_cache_invalidation(OLD.user_id);
    RETURN OLD;
END;
$$;

DROP TRIGGER IF EXISTS trg_user_allowed_groups_auth_cache_invalidation ON user_allowed_groups;
CREATE TRIGGER trg_user_allowed_groups_auth_cache_invalidation
AFTER INSERT OR UPDATE OR DELETE ON user_allowed_groups
FOR EACH ROW EXECUTE FUNCTION enqueue_allowed_group_auth_cache_invalidation();

CREATE OR REPLACE FUNCTION enqueue_group_access_auth_cache_invalidation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'UPDATE' THEN
        IF OLD.user_id IS NOT DISTINCT FROM NEW.user_id
           AND OLD.group_id IS NOT DISTINCT FROM NEW.group_id THEN
            RETURN NEW;
        END IF;
        PERFORM enqueue_user_api_keys_auth_cache_invalidation(OLD.user_id);
        PERFORM enqueue_user_api_keys_auth_cache_invalidation(NEW.user_id);
        RETURN NEW;
    END IF;

    IF TG_OP = 'INSERT' THEN
        PERFORM enqueue_user_api_keys_auth_cache_invalidation(NEW.user_id);
        RETURN NEW;
    END IF;

    PERFORM enqueue_user_api_keys_auth_cache_invalidation(OLD.user_id);
    RETURN OLD;
END;
$$;

DROP TRIGGER IF EXISTS trg_user_group_access_groups_auth_cache_invalidation ON user_group_access_groups;
CREATE TRIGGER trg_user_group_access_groups_auth_cache_invalidation
AFTER INSERT OR UPDATE OR DELETE ON user_group_access_groups
FOR EACH ROW EXECUTE FUNCTION enqueue_group_access_auth_cache_invalidation();
