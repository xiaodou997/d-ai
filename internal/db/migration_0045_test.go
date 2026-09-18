package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0045AddsAsyncJWTAuthReferencesWithoutBackfill(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		ALTER TABLE ai_async_tasks
		    DROP COLUMN auth_user_id,
		    DROP COLUMN auth_user_type,
		    DROP COLUMN auth_session_id,
		    DROP COLUMN auth_credential_version;
		UPDATE dai_schema_metadata SET version = 44 WHERE singleton = TRUE;
		INSERT INTO ai_async_tasks (task_type, auth_method, tenant_id, model_code, input_payload)
		VALUES ('migration-0045-history', 'jwt', 'migration-0045-tenant', 'model', '{}'::jsonb)
	`); err != nil {
		t.Fatalf("prepare schema 44 fixture: %v", err)
	}

	migration, err := os.ReadFile("changes/0045_20260919_async_task_auth_reference.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0045: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 45 {
		t.Fatalf("migration version = %d, want 45", version)
	}

	var nullRefs bool
	if err := pool.QueryRow(ctx, `
		SELECT auth_user_id IS NULL
		   AND auth_user_type IS NULL
		   AND auth_session_id IS NULL
		   AND auth_credential_version IS NULL
		FROM ai_async_tasks
		WHERE task_type = 'migration-0045-history'
	`).Scan(&nullRefs); err != nil {
		t.Fatal(err)
	}
	if !nullRefs {
		t.Fatal("migration 0045 fabricated authentication references for a historical JWT task")
	}
}
