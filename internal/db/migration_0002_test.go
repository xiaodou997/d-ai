package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0002UnifiesLoginIdentifierAndCleansDuplicateEmails(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		DROP INDEX ux_iam_accounts_email_normalized;
		ALTER TABLE iam_accounts
			ADD CONSTRAINT iam_accounts_username_namespace_check CHECK (
				(user_type = 4 AND char_length(username) > 2 AND username LIKE 'u\_%' ESCAPE '\')
				OR (user_type <> 4 AND username NOT LIKE 'u\_%' ESCAPE '\')
			);
		UPDATE dai_schema_metadata SET version = 1 WHERE singleton = TRUE;

		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES ('tenant-m2', 'Migration 2 Tenant', 'active');
		INSERT INTO iam_accounts
			(user_id, tenant_id, username, password_hash, user_type, status, email)
		VALUES
			('user-m2-first', 'tenant-m2', 'u_m2_first', 'x', 4, 'active', 'Same@Example.com'),
			('user-m2-second', 'tenant-m2', 'u_m2_second', 'x', 4, 'active', 'same@example.com');
	`); err != nil {
		t.Fatalf("prepare schema 1 fixture: %v", err)
	}

	migration, err := os.ReadFile("changes/0002_20260811_unify_login_identifier.sql")
	if err != nil {
		t.Fatalf("read migration 0002: %v", err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0002: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != 2 {
		t.Fatalf("schema version = %d, want 2", version)
	}

	var firstEmail, secondEmail string
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT COALESCE(email, '') FROM iam_accounts WHERE user_id = 'user-m2-first'),
			(SELECT COALESCE(email, '') FROM iam_accounts WHERE user_id = 'user-m2-second')
	`).Scan(&firstEmail, &secondEmail); err != nil {
		t.Fatalf("read duplicate email cleanup: %v", err)
	}
	if firstEmail != "Same@Example.com" || secondEmail != "" {
		t.Fatalf("duplicate email cleanup = %q/%q, want retained/null", firstEmail, secondEmail)
	}

	var namespaceConstraint bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_constraint
			WHERE conrelid = 'iam_accounts'::regclass
			  AND conname = 'iam_accounts_username_namespace_check'
		)
	`).Scan(&namespaceConstraint); err != nil {
		t.Fatalf("check namespace constraint: %v", err)
	}
	if namespaceConstraint {
		t.Fatal("legacy username namespace constraint still exists")
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts
			(user_id, tenant_id, username, password_hash, user_type, status)
		VALUES ('user-m2-plain', 'tenant-m2', 'plain_username', 'x', 4, 'active')
	`); err != nil {
		t.Fatalf("plain terminal-user username rejected after migration: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts
			(user_id, tenant_id, username, password_hash, user_type, status, email)
			VALUES ('user-m2-duplicate-email', 'tenant-m2', 'u_m2_duplicate', 'x', 4, 'active', 'SAME@example.com')
	`); err == nil {
		t.Fatal("case-insensitive duplicate email was accepted")
	}
}
