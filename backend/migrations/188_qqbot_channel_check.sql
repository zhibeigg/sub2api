-- Keep the QQBot /check channel-status image command opt-in during upgrades.
-- Administrators must first configure a shared stable totp.encryption_key and
-- a public root HTTPS PublicBaseURL, then explicitly enable the command.
INSERT INTO settings (key, value, updated_at)
VALUES ('qqbot_channel_check_enabled', 'false', NOW())
ON CONFLICT (key) DO NOTHING;
