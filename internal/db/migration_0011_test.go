package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0011AddsStatefulAuthSessions(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		DROP VIEW user_admin_end_user_projection;
		DROP TABLE auth_activation_tokens;
		ALTER TABLE iam_accounts DROP COLUMN credential_state;
		ALTER TABLE iam_accounts DROP COLUMN mfa_secret_encrypted, DROP COLUMN mfa_enabled, DROP COLUMN mfa_enrolled_at;
		DROP TRIGGER trg_auth_revoke_sessions_on_account_change ON iam_accounts;
		DROP FUNCTION auth_revoke_sessions_on_account_change();
		DROP TABLE auth_refresh_tokens;
		DROP TABLE auth_sessions;
		ALTER TABLE iam_accounts DROP COLUMN credential_version;
		UPDATE dai_schema_metadata SET version = 10 WHERE singleton = TRUE;
	`); err != nil {
		t.Fatalf("prepare schema 10 fixture: %v", err)
	}

	migration, err := os.ReadFile("changes/0011_20260820_auth_sessions.sql")
	if err != nil {
		t.Fatalf("read migration 0011: %v", err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0011: %v", err)
	}

	var version int
	var sessionTable, refreshTable, credentialColumn bool
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT to_regclass(current_schema() || '.auth_sessions') IS NOT NULL`).Scan(&sessionTable); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT to_regclass(current_schema() || '.auth_refresh_tokens') IS NOT NULL`).Scan(&refreshTable); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = 'iam_accounts' AND column_name = 'credential_version'
		)
	`).Scan(&credentialColumn); err != nil {
		t.Fatal(err)
	}
	if version != 11 || !sessionTable || !refreshTable || !credentialColumn {
		t.Fatalf("migration result = version:%d sessions:%t refresh:%t credential:%t", version, sessionTable, refreshTable, credentialColumn)
	}
}

func TestMigration0011RejectsMissingSchemaMetadata(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		DROP VIEW user_admin_end_user_projection;
		DROP TABLE auth_activation_tokens;
		ALTER TABLE iam_accounts DROP COLUMN credential_state;
		ALTER TABLE iam_accounts DROP COLUMN mfa_secret_encrypted, DROP COLUMN mfa_enabled, DROP COLUMN mfa_enrolled_at;
		DROP TRIGGER trg_auth_revoke_sessions_on_account_change ON iam_accounts;
		DROP FUNCTION auth_revoke_sessions_on_account_change();
		DROP TABLE auth_refresh_tokens;
		DROP TABLE auth_sessions;
		ALTER TABLE iam_accounts DROP COLUMN credential_version;
		DELETE FROM dai_schema_metadata;
	`); err != nil {
		t.Fatalf("prepare missing metadata fixture: %v", err)
	}
	migration, err := os.ReadFile("changes/0011_20260820_auth_sessions.sql")
	if err != nil {
		t.Fatalf("read migration 0011: %v", err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err == nil {
		t.Fatal("migration 0011 accepted a database without schema metadata")
	}
}
