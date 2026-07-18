-- Add OpenCode Go as an independent text quota platform.
--
-- Rebuild the platform CHECK without a validation gap: install and validate the
-- new superset constraint first, then remove any legacy platform CHECK and rename
-- the validated constraint to the canonical Ent/PostgreSQL name.
DO $$
DECLARE
    constraint_record RECORD;
BEGIN
    ALTER TABLE user_platform_quotas
        DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check_v185;

    ALTER TABLE user_platform_quotas
        ADD CONSTRAINT user_platform_quotas_platform_check_v185
        CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'adobe', 'cursor', 'opencode'))
        NOT VALID;

    ALTER TABLE user_platform_quotas
        VALIDATE CONSTRAINT user_platform_quotas_platform_check_v185;

    FOR constraint_record IN
        SELECT conname
        FROM pg_constraint
        WHERE conrelid = 'user_platform_quotas'::regclass
          AND contype = 'c'
          AND conname <> 'user_platform_quotas_platform_check_v185'
          AND pg_get_constraintdef(oid) ILIKE '%platform%'
    LOOP
        EXECUTE format(
            'ALTER TABLE user_platform_quotas DROP CONSTRAINT %I',
            constraint_record.conname
        );
    END LOOP;

    ALTER TABLE user_platform_quotas
        RENAME CONSTRAINT user_platform_quotas_platform_check_v185
        TO user_platform_quotas_platform_check;
END
$$;
