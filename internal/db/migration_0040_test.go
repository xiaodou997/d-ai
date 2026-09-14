package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0040ExposesUpstreamAccountDescriptionInResourceView(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2, SchemaSQL: legacySchema40(t)})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		DROP VIEW ai_upstream_resources;
		CREATE VIEW ai_upstream_resources AS
		  SELECT id, 'direct_upstream'::TEXT AS resource_kind, name, price_book_id,
		         tenant_multiplier, status, created_at, updated_at,
		         tenant_display_name, tenant_access_mode
		  FROM ai_upstream_accounts
		  UNION ALL
		  SELECT id, 'oauth_pool'::TEXT AS resource_kind, name, price_book_id,
		         tenant_multiplier, status, created_at, updated_at,
		         tenant_display_name, tenant_access_mode
		  FROM ai_credential_pools;
		UPDATE dai_schema_metadata SET version = 39 WHERE singleton = TRUE
	`); err != nil {
		t.Fatalf("prepare schema 39 fixture: %v", err)
	}
	migration, err := os.ReadFile("changes/0040_20260911_upstream_resource_description_view.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0040: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 40 {
		t.Fatalf("migration version = %d, want 40", version)
	}
	var description string
	if err := pool.QueryRow(ctx, `
		INSERT INTO ai_upstream_accounts (name, description, tenant_display_name, api_key_ciphertext)
		VALUES ('migration-description', 'visible description', 'Migration', 'cipher')
		RETURNING description
	`).Scan(&description); err != nil {
		t.Fatalf("seed described account: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT description FROM ai_upstream_resources WHERE resource_kind = 'direct_upstream' AND name = 'migration-description'
	`).Scan(&description); err != nil {
		t.Fatalf("read resource description: %v", err)
	}
	if description != "visible description" {
		t.Fatalf("resource description = %q, want %q", description, "visible description")
	}
}
