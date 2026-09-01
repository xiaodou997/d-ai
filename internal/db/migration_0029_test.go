package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0029SimplifiesGroupRoutingPolicy(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		ALTER TABLE ai_groups DROP COLUMN route_policy;
		ALTER TABLE ai_groups
			ADD COLUMN route_strategy TEXT NOT NULL DEFAULT 'adaptive',
			ADD COLUMN route_objective TEXT NOT NULL DEFAULT 'balanced';
		ALTER TABLE ai_group_targets
			ADD COLUMN priority INTEGER NOT NULL DEFAULT 100,
			ADD COLUMN routing_weight NUMERIC(10,4) NOT NULL DEFAULT 1;
		UPDATE dai_schema_metadata SET version = 28 WHERE singleton = TRUE;
	`); err != nil {
		t.Fatalf("prepare schema 28 fixture: %v", err)
	}

	migration, err := os.ReadFile("changes/0029_20260901_simplify_group_routing_policy.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0029: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 29 {
		t.Fatalf("migration version = %d, want 29", version)
	}

	var oldGroupFields, oldTargetFields, newPolicy int
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'ai_groups' AND column_name IN ('route_strategy', 'route_objective')),
			(SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'ai_group_targets' AND column_name IN ('priority', 'routing_weight')),
			(SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'ai_groups' AND column_name = 'route_policy')
	`).Scan(&oldGroupFields, &oldTargetFields, &newPolicy); err != nil {
		t.Fatal(err)
	}
	if oldGroupFields != 0 || oldTargetFields != 0 || newPolicy != 1 {
		t.Fatalf("migration result = old group fields %d, old target fields %d, new policy %d; want 0/0/1", oldGroupFields, oldTargetFields, newPolicy)
	}
}
