-- Add an optimistic-concurrency generation for OAuth token rotation.
--
-- Apply this forward update before deploying an AI Service build that writes
-- token_version. It is online-safe: existing rows receive generation 1 and
-- normal credential selection can continue while the column is added.

BEGIN;

SELECT pg_advisory_xact_lock(hashtext('unihub-ai-oauth-token-version'));
SET LOCAL lock_timeout = '10s';
SET LOCAL statement_timeout = '5min';

ALTER TABLE ai_provider_oauth_credentials
  ADD COLUMN IF NOT EXISTS token_version BIGINT NOT NULL DEFAULT 1;

DO $migration$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conrelid = 'ai_provider_oauth_credentials'::regclass
      AND conname = 'ai_provider_oauth_credentials_token_version_check'
  ) THEN
    ALTER TABLE ai_provider_oauth_credentials
      ADD CONSTRAINT ai_provider_oauth_credentials_token_version_check
      CHECK (token_version >= 1);
  END IF;
END
$migration$;

COMMIT;
