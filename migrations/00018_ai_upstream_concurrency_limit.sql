-- Replace the upstream account per-minute request cap with a concurrency cap.
--
-- Apply this forward update before deploying an AI Service build that reads
-- concurrency_limit. The old service reads rpm_limit and the new one reads
-- concurrency_limit, so old and new builds cannot run against the same database
-- at once — cut over in one step.
--
-- The old rpm_limit values are intentionally NOT carried over: 60 requests per
-- minute and 60 concurrent requests are unrelated quantities, so copying the
-- number across would produce a precise but wrong limit. Every account starts
-- at NULL (unlimited) and must be set from the upstream's actual capacity.

BEGIN;

SELECT pg_advisory_xact_lock(hashtext('unihub-ai-upstream-account-concurrency-limit'));
SET LOCAL lock_timeout = '10s';
SET LOCAL statement_timeout = '5min';

ALTER TABLE ai_upstream_accounts
  ADD COLUMN IF NOT EXISTS concurrency_limit INTEGER;

DO $migration$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conrelid = 'ai_upstream_accounts'::regclass
      AND conname = 'ai_upstream_accounts_concurrency_limit_check'
  ) THEN
    ALTER TABLE ai_upstream_accounts
      ADD CONSTRAINT ai_upstream_accounts_concurrency_limit_check
      CHECK (concurrency_limit IS NULL OR concurrency_limit > 0);
  END IF;
END
$migration$;

ALTER TABLE ai_upstream_accounts
  DROP COLUMN IF EXISTS rpm_limit;

COMMIT;
