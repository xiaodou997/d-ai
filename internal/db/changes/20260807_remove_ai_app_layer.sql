-- Remove the retired AI application, managed-prompt, and invoke-key feature.
-- App-targeted workspace threads and invoke-key tasks have no model-only
-- equivalent, so this destructive migration removes them before shrinking the
-- shared tables.

BEGIN;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM dai_schema_metadata WHERE singleton = TRUE AND version = 8
    ) THEN
        RAISE EXCEPTION 'cannot remove AI app layer: expected schema version 8';
    END IF;
END
$$;

DELETE FROM ai_workspace_messages
WHERE thread_id IN (
    SELECT id
    FROM ai_workspace_threads
    WHERE target_kind = 'app' OR app_id IS NOT NULL
);

DELETE FROM ai_workspace_threads
WHERE target_kind = 'app' OR app_id IS NOT NULL;

DELETE FROM ai_async_tasks
WHERE invoke_key_id IS NOT NULL;

DROP TABLE ai_app_prompt_bindings;
DROP TABLE ai_app_prompt_versions;
DROP TABLE ai_app_publications;
DROP TABLE ai_app_keys;
DROP TABLE ai_app_prompts;
DROP TABLE ai_apps;

ALTER TABLE ai_workspace_threads
    DROP COLUMN target_kind,
    DROP COLUMN app_id,
    DROP COLUMN variables_json;

ALTER TABLE ai_usage_logs
    DROP COLUMN app_id,
    DROP COLUMN app_name_snapshot,
    DROP COLUMN app_owner_type_snapshot,
    DROP COLUMN app_owner_tenant_id_snapshot,
    DROP COLUMN app_owner_user_id_snapshot;

ALTER TABLE ai_async_tasks
    DROP COLUMN invoke_key_id;

UPDATE dai_schema_metadata
SET version = 9
WHERE singleton = TRUE AND version = 8;

COMMIT;
