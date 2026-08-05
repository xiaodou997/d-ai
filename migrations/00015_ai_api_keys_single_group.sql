-- Migrate API keys from ordered multi-group bindings to one required group.
--
-- Apply this forward update while AI Service instances are stopped. For keys
-- with multiple bindings, the group that the old runtime tried first is kept:
-- lowest sort_order, then earliest binding, then group_id as a stable tie-break.
-- Keys without a valid binding abort the transaction and must be repaired in
-- ai_api_key_groups before this script is run again.

BEGIN;

SELECT pg_advisory_xact_lock(hashtext('unihub-ai-api-keys-single-group'));
SET LOCAL lock_timeout = '10s';
SET LOCAL statement_timeout = '5min';

ALTER TABLE ai_api_keys
  ADD COLUMN IF NOT EXISTS group_id UUID;

-- Prevent the old application from changing bindings between backup and
-- cutover. The table is absent when this idempotent script is run again.
DO $migration$
BEGIN
  IF to_regclass('ai_api_key_groups') IS NOT NULL THEN
    LOCK TABLE ai_api_key_groups IN ACCESS EXCLUSIVE MODE;
  END IF;
END
$migration$;

CREATE SCHEMA IF NOT EXISTS migration_history;

CREATE TABLE IF NOT EXISTS migration_history.ai_api_key_groups_single_group_backup (
  tenant_id  TEXT        NOT NULL,
  api_key_id UUID        NOT NULL,
  group_id   UUID        NOT NULL,
  sort_order INTEGER     NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  captured_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (api_key_id, group_id)
);

DO $migration$
BEGIN
  IF to_regclass('ai_api_key_groups') IS NOT NULL THEN
    INSERT INTO migration_history.ai_api_key_groups_single_group_backup (
      tenant_id,
      api_key_id,
      group_id,
      sort_order,
      created_at
    )
    SELECT
      tenant_id,
      api_key_id,
      group_id,
      sort_order,
      created_at
    FROM ai_api_key_groups
    ON CONFLICT (api_key_id, group_id) DO NOTHING;
  END IF;
END
$migration$;

WITH selected_binding AS (
  SELECT DISTINCT ON (binding.api_key_id)
    binding.api_key_id,
    binding.tenant_id,
    binding.group_id
  FROM migration_history.ai_api_key_groups_single_group_backup AS binding
  JOIN ai_api_keys AS api_key
    ON api_key.id = binding.api_key_id
   AND api_key.tenant_id = binding.tenant_id
  JOIN ai_groups AS ai_group
    ON ai_group.id = binding.group_id
   AND ai_group.tenant_id = binding.tenant_id
  ORDER BY
    binding.api_key_id,
    binding.sort_order ASC,
    binding.created_at ASC,
    binding.group_id ASC
)
UPDATE ai_api_keys AS api_key
SET group_id = selected_binding.group_id
FROM selected_binding
WHERE api_key.id = selected_binding.api_key_id
  AND api_key.tenant_id = selected_binding.tenant_id
  AND api_key.group_id IS NULL;

DO $migration$
DECLARE
  missing_count BIGINT;
  missing_sample TEXT;
  invalid_count BIGINT;
  invalid_sample TEXT;
BEGIN
  SELECT count(*)
  INTO missing_count
  FROM ai_api_keys
  WHERE group_id IS NULL;

  SELECT string_agg(id::TEXT, ', ' ORDER BY id::TEXT)
  INTO missing_sample
  FROM (
    SELECT id
    FROM ai_api_keys
    WHERE group_id IS NULL
    ORDER BY id
    LIMIT 10
  ) AS missing;

  IF missing_count > 0 THEN
    RAISE EXCEPTION USING
      MESSAGE = format(
        'cannot migrate API keys: %s key(s) have no valid group binding; sample ids: %s',
        missing_count,
        missing_sample
      ),
      HINT = 'Bind every listed key in ai_api_key_groups, then run this script again.';
  END IF;

  SELECT count(*)
  INTO invalid_count
  FROM ai_api_keys AS api_key
  WHERE NOT EXISTS (
    SELECT 1
    FROM ai_groups AS ai_group
    WHERE ai_group.id = api_key.group_id
      AND ai_group.tenant_id = api_key.tenant_id
  );

  SELECT string_agg(id::TEXT, ', ' ORDER BY id::TEXT)
  INTO invalid_sample
  FROM (
    SELECT api_key.id
    FROM ai_api_keys AS api_key
    WHERE NOT EXISTS (
      SELECT 1
      FROM ai_groups AS ai_group
      WHERE ai_group.id = api_key.group_id
        AND ai_group.tenant_id = api_key.tenant_id
    )
    ORDER BY api_key.id
    LIMIT 10
  ) AS invalid;

  IF invalid_count > 0 THEN
    RAISE EXCEPTION USING
      MESSAGE = format(
        'cannot migrate API keys: %s key(s) reference a missing or cross-tenant group; sample ids: %s',
        invalid_count,
        invalid_sample
      ),
      HINT = 'Repair ai_api_keys.group_id so it references an ai_groups row owned by the same tenant.';
  END IF;
END
$migration$;

ALTER TABLE ai_api_keys
  ALTER COLUMN group_id SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_ai_api_keys_group
  ON ai_api_keys (group_id);

DO $migration$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conrelid = 'ai_api_keys'::regclass
      AND conname = 'ai_api_keys_tenant_group_fk'
  ) THEN
    ALTER TABLE ai_api_keys
      ADD CONSTRAINT ai_api_keys_tenant_group_fk
      FOREIGN KEY (tenant_id, group_id)
      REFERENCES ai_groups (tenant_id, id)
      NOT VALID;
  END IF;
END
$migration$;

ALTER TABLE ai_api_keys
  VALIDATE CONSTRAINT ai_api_keys_tenant_group_fk;

DROP TABLE IF EXISTS ai_api_key_groups;

COMMIT;
