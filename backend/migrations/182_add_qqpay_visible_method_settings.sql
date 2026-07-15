INSERT INTO settings (key, value)
VALUES
    ('payment_visible_method_qqpay_source', ''),
    ('payment_visible_method_qqpay_enabled', 'false')
ON CONFLICT (key) DO NOTHING;
