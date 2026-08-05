-- +goose Up

ALTER TABLE gov_clients ADD COLUMN last_seen_at TIMESTAMPTZ;

UPDATE gov_clients c
SET last_seen_at = latest.last_seen
FROM (
    SELECT service_id, MAX(last_seen) AS last_seen
    FROM gov_service_instances
    GROUP BY service_id
) latest
WHERE latest.service_id = c.client_id;

CREATE INDEX idx_gov_clients_last_seen_at ON gov_clients (last_seen_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_gov_clients_last_seen_at;
ALTER TABLE gov_clients DROP COLUMN IF EXISTS last_seen_at;
