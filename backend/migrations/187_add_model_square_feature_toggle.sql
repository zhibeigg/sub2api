-- Add an independent opt-in switch for the model square.
-- Existing installations inherit the available-channels switch only when its
-- stored value is exactly "true"; missing or any other value remains disabled.
INSERT INTO settings (key, value, updated_at)
SELECT
    'model_square_enabled',
    CASE
        WHEN EXISTS (
            SELECT 1
            FROM settings
            WHERE key = 'available_channels_enabled'
              AND value = 'true'
        ) THEN 'true'
        ELSE 'false'
    END,
    NOW()
ON CONFLICT (key) DO NOTHING;
