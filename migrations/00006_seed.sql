-- +goose Up

INSERT INTO iam_admins (user_id, username, password_hash, email, user_type, status)
VALUES ('PLATFORM_ADMIN', 'urm_admin', '$2b$10$Y0sgdds4O5HqpPwIkJVVA.PdIfzEaxJ2laGpP8SwPeXgDI8LyhNMe', NULL, 1, 'active')
ON CONFLICT (user_id) DO NOTHING;

INSERT INTO sys_settings (key, value)
VALUES (
    'payment',
    '{
        "creditsPerCny": 100,
        "tenantCustomTopupFeeBp": 160,
        "tenantWithdrawFeeBp": 160,
        "tenantTopupPackages": [
            {"id": "p10", "name": "10 元体验包", "amount": 1000, "credits": 1000, "enabled": true, "sortOrder": 10},
            {"id": "p20", "name": "20 元基础包", "amount": 2000, "credits": 2000, "enabled": true, "sortOrder": 20},
            {"id": "p50", "name": "50 元常用包", "amount": 5000, "credits": 5000, "enabled": true, "sortOrder": 30},
            {"id": "p100", "name": "100 元进阶包", "amount": 10000, "credits": 10000, "enabled": true, "sortOrder": 40}
        ]
    }'::jsonb
)
ON CONFLICT (key) DO NOTHING;

INSERT INTO pay_wechat_config (id) VALUES (1) ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM pay_wechat_config WHERE id = 1;
DELETE FROM sys_settings WHERE key = 'payment';
DELETE FROM iam_admins WHERE user_id = 'PLATFORM_ADMIN';
