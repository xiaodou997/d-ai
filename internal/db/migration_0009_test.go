package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0009AddsProxyAndNotificationRuntimeTables(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		DROP TABLE sys_notification_deliveries;
		DROP TABLE ai_proxy_nodes;
		UPDATE dai_schema_metadata SET version = 8 WHERE singleton = TRUE;
	`); err != nil {
		t.Fatalf("prepare schema 8 fixture: %v", err)
	}

	migration, err := os.ReadFile("changes/0009_20260818_system_modules.sql")
	if err != nil {
		t.Fatalf("read migration 0009: %v", err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0009: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != 9 {
		t.Fatalf("schema version = %d, want 9", version)
	}

	for _, table := range []string{"ai_proxy_nodes", "sys_notification_deliveries"} {
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT to_regclass(current_schema() || $1) IS NOT NULL`, "."+table).Scan(&exists); err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if !exists {
			t.Fatalf("migration did not create %s", table)
		}
	}
	for _, index := range []string{"idx_ai_proxy_nodes_status", "idx_sys_notification_user", "idx_sys_notification_status"} {
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT to_regclass(current_schema() || $1) IS NOT NULL`, "."+index).Scan(&exists); err != nil {
			t.Fatalf("check index %s: %v", index, err)
		}
		if !exists {
			t.Fatalf("migration did not create %s", index)
		}
	}

	var proxyStatus, healthStatus string
	if err := pool.QueryRow(ctx, `
		INSERT INTO ai_proxy_nodes (name, proxy_type, endpoint)
		VALUES ('migration proxy', 'http', 'http://proxy.internal')
		RETURNING status, health_status
	`).Scan(&proxyStatus, &healthStatus); err != nil {
		t.Fatalf("insert migrated proxy: %v", err)
	}
	if proxyStatus != "disabled" || healthStatus != "unknown" {
		t.Fatalf("proxy defaults = %q/%q, want disabled/unknown", proxyStatus, healthStatus)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO sys_notification_deliveries
			(event_key, channel, title, body, idempotency_key)
		VALUES ('migration-0009', 'in_app', 'Migration', 'Notification', 'migration-0009-key')
	`); err != nil {
		t.Fatalf("insert migrated notification: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO sys_notification_deliveries
			(event_key, channel, title, body, idempotency_key)
		VALUES ('migration-0009-duplicate', 'webhook', 'Duplicate', 'Notification', 'migration-0009-key')
	`); err == nil {
		t.Fatal("duplicate notification idempotency key was accepted")
	}
}
