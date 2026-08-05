BEGIN;

SELECT pg_advisory_xact_lock(hashtext('unihub-ai-oauth-credential-cooldown'));

ALTER TABLE ai_provider_oauth_credentials
  ADD COLUMN IF NOT EXISTS cooldown_until TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_oauth_cred_pool_cooldown
  ON ai_provider_oauth_credentials (pool_id, cooldown_until)
  WHERE status = 'active' AND cooldown_until IS NOT NULL;

COMMIT;
