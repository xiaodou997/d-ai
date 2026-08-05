-- D-AI local-development seed
-- Never apply this file in production.

INSERT INTO iam_admins (user_id, username, password_hash, email, user_type, status)
VALUES ('DAI_ADMIN', 'dai_admin', '$2a$10$VI.y0TjcNQQX/5X/ukr7xOMmmRPfAEnrFs9fnJhkEajX6JPl43JXS', NULL, 1, 'active')
ON CONFLICT (user_id) DO NOTHING;
