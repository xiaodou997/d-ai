package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0044EnforcesSingleActiveJWTSigningKey(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		DROP INDEX ux_auth_signing_keys_single_active;
		UPDATE dai_schema_metadata SET version = 43 WHERE singleton = TRUE;
		INSERT INTO auth_signing_keys (kid, private_key, public_key, status)
		VALUES ('migration-0044-active', 'private', 'public', 'active')
	`); err != nil {
		t.Fatalf("prepare schema 43 fixture: %v", err)
	}

	migration, err := os.ReadFile("changes/0044_20260918_jwt_active_key_invariant.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0044: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 44 {
		t.Fatalf("migration version = %d, want 44", version)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO auth_signing_keys (kid, private_key, public_key, status)
		VALUES ('migration-0044-second-active', 'private', 'public', 'active')
	`); err == nil {
		t.Fatal("second active JWT signing key accepted after migration 0044")
	}
}

func TestMigration0044RejectsExistingDuplicateActiveJWTSigningKeys(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		DROP INDEX ux_auth_signing_keys_single_active;
		UPDATE dai_schema_metadata SET version = 43 WHERE singleton = TRUE;
		INSERT INTO auth_signing_keys (kid, private_key, public_key, status)
		VALUES
			('migration-0044-active-a', 'private', 'public', 'active'),
			('migration-0044-active-b', 'private', 'public', 'active')
	`); err != nil {
		t.Fatalf("prepare duplicate-active fixture: %v", err)
	}

	migration, err := os.ReadFile("changes/0044_20260918_jwt_active_key_invariant.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err == nil {
		t.Fatal("migration 0044 accepted duplicate active JWT signing keys")
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 43 {
		t.Fatalf("schema version after rejected migration = %d, want 43", version)
	}
}
