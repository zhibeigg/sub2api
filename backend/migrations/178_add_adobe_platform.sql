-- Add Adobe as an independent quota platform and widen media request IDs.

ALTER TABLE user_platform_quotas
    DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check;

ALTER TABLE user_platform_quotas
    ADD CONSTRAINT user_platform_quotas_platform_check
    CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'adobe'));

ALTER TABLE usage_logs
    ALTER COLUMN request_id TYPE VARCHAR(255);

COMMENT ON COLUMN groups.image_price_1k IS '1K 图片生成单价 (USD/image)，适用于支持图片生成的平台';
COMMENT ON COLUMN groups.image_price_2k IS '2K 图片生成单价 (USD/image)，适用于支持图片生成的平台';
COMMENT ON COLUMN groups.image_price_4k IS '4K 图片生成单价 (USD/image)，适用于支持图片生成的平台';
COMMENT ON COLUMN groups.video_price_480p IS '480p 视频生成每秒单价 (USD/s)，适用于支持该档位的平台';
COMMENT ON COLUMN groups.video_price_720p IS '720p 视频生成每秒单价 (USD/s)，适用于支持该档位的平台';
COMMENT ON COLUMN groups.video_price_1080p IS '1080p 视频生成每秒单价 (USD/s)，适用于支持该档位的平台';
