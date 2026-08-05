-- +goose Up

CREATE TABLE ann_announcements (
    announcement_id TEXT PRIMARY KEY,
    publisher_type TEXT NOT NULL CHECK (publisher_type IN ('platform', 'tenant')),
    publisher_tenant_id TEXT,
    title VARCHAR(200) NOT NULL,
    content_markdown TEXT NOT NULL,
    category TEXT NOT NULL DEFAULT 'general'
        CHECK (category IN ('general', 'maintenance', 'upgrade', 'pricing', 'security')),
    severity TEXT NOT NULL DEFAULT 'info'
        CHECK (severity IN ('info', 'important', 'critical')),
    display_mode TEXT NOT NULL DEFAULT 'inbox'
        CHECK (display_mode IN ('inbox', 'popup')),
    status TEXT NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'published', 'archived')),
    starts_at TIMESTAMPTZ,
    ends_at TIMESTAMPTZ,
    published_at TIMESTAMPTZ,
    archived_at TIMESTAMPTZ,
    audience_size_at_publish BIGINT CHECK (audience_size_at_publish IS NULL OR audience_size_at_publish >= 0),
    created_by TEXT NOT NULL,
    updated_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((publisher_type = 'platform' AND publisher_tenant_id IS NULL)
        OR (publisher_type = 'tenant' AND publisher_tenant_id IS NOT NULL)),
    CHECK (ends_at IS NULL OR starts_at IS NULL OR starts_at < ends_at),
    CHECK (char_length(content_markdown) BETWEEN 1 AND 50000)
);

CREATE INDEX idx_ann_visible_window
    ON ann_announcements (status, starts_at, ends_at, published_at DESC);
CREATE INDEX idx_ann_publisher
    ON ann_announcements (publisher_type, publisher_tenant_id, created_at DESC);

CREATE TABLE ann_audiences (
    id BIGSERIAL PRIMARY KEY,
    announcement_id TEXT NOT NULL REFERENCES ann_announcements(announcement_id) ON DELETE CASCADE,
    audience_kind TEXT NOT NULL CHECK (audience_kind IN ('admin', 'tenant_user', 'end_user')),
    scope_type TEXT NOT NULL CHECK (scope_type IN ('all', 'tenant')),
    tenant_id TEXT,
    CHECK ((scope_type = 'all' AND tenant_id IS NULL)
        OR (scope_type = 'tenant' AND tenant_id IS NOT NULL)),
    CHECK (audience_kind <> 'admin' OR scope_type = 'all')
);

CREATE UNIQUE INDEX uq_ann_audience_rule
    ON ann_audiences (announcement_id, audience_kind, scope_type, COALESCE(tenant_id, ''));
CREATE INDEX idx_ann_audience_match
    ON ann_audiences (audience_kind, scope_type, tenant_id, announcement_id);

CREATE TABLE ann_receipts (
    announcement_id TEXT NOT NULL REFERENCES ann_announcements(announcement_id) ON DELETE CASCADE,
    user_type INTEGER NOT NULL CHECK (user_type IN (1, 2, 3, 4)),
    user_id TEXT NOT NULL,
    tenant_id TEXT,
    read_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (announcement_id, user_type, user_id)
);

CREATE INDEX idx_ann_receipts_user
    ON ann_receipts (user_type, user_id, read_at DESC);
CREATE INDEX idx_ann_receipts_announcement
    ON ann_receipts (announcement_id, read_at DESC);

CREATE TABLE ann_audit_events (
    id BIGSERIAL PRIMARY KEY,
    announcement_id TEXT NOT NULL,
    event_type TEXT NOT NULL
        CHECK (event_type IN ('created', 'updated', 'published', 'archived', 'draft_deleted')),
    actor_user_type INTEGER NOT NULL CHECK (actor_user_type IN (1, 2, 3)),
    actor_user_id TEXT NOT NULL,
    actor_tenant_id TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_ann_audit_announcement
    ON ann_audit_events (announcement_id, created_at DESC);

-- +goose Down

DROP TABLE IF EXISTS ann_receipts;
DROP TABLE IF EXISTS ann_audiences;
DROP TABLE IF EXISTS ann_announcements;
DROP TABLE IF EXISTS ann_audit_events;
