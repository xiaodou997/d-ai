package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0026MovesRoutingPolicyIntoGroups(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		ALTER TABLE ai_groups DROP COLUMN route_policy;
		CREATE TABLE ai_route_score_weights (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			scope TEXT NOT NULL,
			weights JSONB NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE (scope)
		);
		UPDATE dai_schema_metadata SET version = 25 WHERE singleton = TRUE;
	`); err != nil {
		t.Fatalf("prepare schema 25 fixture: %v", err)
	}
	migration, err := os.ReadFile("changes/0026_20260831_group_route_policy_v2.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0026: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 26 {
		t.Fatalf("migration version = %d, want 26", version)
	}

	var groupPolicyColumns, targetWeightColumns, legacyTable int
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'ai_groups' AND column_name IN ('route_strategy', 'route_objective')),
			(SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'ai_group_targets' AND column_name = 'routing_weight'),
			(SELECT COUNT(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = current_schema() AND c.relname = 'ai_route_score_weights')
	`).Scan(&groupPolicyColumns, &targetWeightColumns, &legacyTable); err != nil {
		t.Fatal(err)
	}
	if groupPolicyColumns != 2 || targetWeightColumns != 1 || legacyTable != 0 {
		t.Fatalf("migration result = group policy %d, target weight %d, legacy table %d; want 2/1/0", groupPolicyColumns, targetWeightColumns, legacyTable)
	}
}
