-- from_version: 36
-- to_version: 37
-- description: store redacted request and response headers for request logging
BEGIN;
SELECT pg_advisory_xact_lock(82624001);
DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM dai_schema_metadata WHERE singleton=TRUE AND version=36) THEN
  RAISE EXCEPTION 'expected D-AI schema version 36';
 END IF;
END $$;
ALTER TABLE ai_request_payloads ADD COLUMN request_headers JSONB, ADD COLUMN response_headers JSONB;
INSERT INTO ai_settings (key, value) VALUES ('request_recording', '{
  "level": "basic",
  "sensitive_headers": ["authorization", "proxy-authorization", "x-api-key", "api-key", "cookie", "set-cookie"]
}'::jsonb) ON CONFLICT (key) DO NOTHING;
UPDATE dai_schema_metadata SET version=37, updated_at=now() WHERE singleton=TRUE AND version=36;
COMMIT;
