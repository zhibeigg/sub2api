-- Expand group endpoint semantics without changing the legacy platform column.
-- Defaults keep pre-195 binaries able to insert rows after a rollback; the new
-- application always writes explicit values, while empty protocol arrays remain
-- a safe legacy/unknown value for old writers.
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS endpoint_protocols JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS quota_platform VARCHAR(50) NOT NULL DEFAULT 'anthropic';

ALTER TABLE account_groups
    ADD COLUMN IF NOT EXISTS endpoint_compatibility_enabled BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE account_groups
SET endpoint_compatibility_enabled = FALSE
WHERE endpoint_compatibility_enabled IS NULL;

-- Preserve the historical quota bucket exactly. Endpoint compatibility changes
-- routing only and must not silently move existing usage to another platform.
UPDATE groups
SET quota_platform = platform;

-- Backfill the public endpoint families that the legacy platform/flags exposed.
-- Arrays are deliberately emitted in the registry's stable protocol order and
-- mirror service.LegacyEndpointProtocols.
UPDATE groups
SET endpoint_protocols =
    (CASE
        WHEN platform IN ('anthropic', 'gemini', 'antigravity', 'grok', 'cursor', 'opencode', 'kiro')
             OR (platform = 'openai' AND allow_messages_dispatch)
            THEN jsonb_build_array('anthropic_messages')
        ELSE '[]'::jsonb
    END)
    || (CASE
        WHEN platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'cursor', 'opencode', 'kiro')
            THEN jsonb_build_array('openai_chat_completions')
        ELSE '[]'::jsonb
    END)
    || (CASE
        WHEN platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'cursor', 'opencode', 'kiro')
            THEN jsonb_build_array('openai_responses')
        ELSE '[]'::jsonb
    END)
    || (CASE
        WHEN platform IN ('gemini', 'antigravity')
            THEN jsonb_build_array('gemini_generate_content')
        ELSE '[]'::jsonb
    END)
    || (CASE
        WHEN platform = 'openai'
            THEN jsonb_build_array('openai_embeddings')
        ELSE '[]'::jsonb
    END)
    || (CASE
        WHEN platform = 'openai'
            THEN jsonb_build_array('openai_alpha_search')
        ELSE '[]'::jsonb
    END)
    || (CASE
        WHEN platform = 'adobe'
             OR (platform IN ('openai', 'gemini', 'antigravity', 'grok')
                 AND (allow_image_generation OR allow_batch_image_generation))
            THEN jsonb_build_array('openai_images')
        ELSE '[]'::jsonb
    END)
    || (CASE
        WHEN platform = 'adobe'
             OR (platform IN ('openai', 'grok')
                 AND (allow_image_generation OR allow_batch_image_generation))
            THEN jsonb_build_array('openai_videos')
        ELSE '[]'::jsonb
    END);

-- Only preserve cross-platform bindings that were unambiguously active under
-- the old mixed_scheduling rules. Unknown/ambiguous bindings remain disabled.
UPDATE account_groups AS ag
SET endpoint_compatibility_enabled = TRUE
FROM accounts AS a, groups AS g
WHERE ag.account_id = a.id
  AND ag.group_id = g.id
  AND a.deleted_at IS NULL
  AND g.deleted_at IS NULL
  AND a.platform <> g.platform
  AND lower(COALESCE(a.extra->>'mixed_scheduling', 'false')) = 'true'
  AND (
        (a.platform IN ('antigravity', 'kiro') AND g.platform IN ('anthropic', 'gemini', 'openai'))
     OR (a.platform = 'cursor' AND g.platform IN ('anthropic', 'gemini', 'openai', 'grok'))
     OR (a.platform = 'opencode' AND g.platform IN ('anthropic', 'openai'))
  );

-- Re-assert constraints so rerunning after a partially applied expand remains safe.
ALTER TABLE groups
    ALTER COLUMN endpoint_protocols SET DEFAULT '[]'::jsonb,
    ALTER COLUMN endpoint_protocols SET NOT NULL,
    ALTER COLUMN quota_platform SET DEFAULT 'anthropic',
    ALTER COLUMN quota_platform SET NOT NULL;

ALTER TABLE account_groups
    ALTER COLUMN endpoint_compatibility_enabled SET DEFAULT FALSE,
    ALTER COLUMN endpoint_compatibility_enabled SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'groups_endpoint_protocols_array_check') THEN
        ALTER TABLE groups ADD CONSTRAINT groups_endpoint_protocols_array_check
            CHECK (jsonb_typeof(endpoint_protocols) = 'array');
    END IF;
END $$;

COMMENT ON COLUMN groups.endpoint_protocols IS '分组公开入站协议集合；与账号供应平台解耦，按注册表稳定顺序存储';
COMMENT ON COLUMN groups.quota_platform IS '分组用量归属平台；协议兼容不得改变该配额桶';
COMMENT ON COLUMN account_groups.endpoint_compatibility_enabled IS '是否允许该账号通过协议兼容参与此分组；默认关闭';
