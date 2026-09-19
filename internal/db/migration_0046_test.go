package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0046RevokesSessionsOnAuthorizationContextChanges(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		DROP TRIGGER trg_auth_revoke_sessions_on_account_change ON iam_accounts;
		CREATE OR REPLACE FUNCTION auth_revoke_sessions_on_account_change() RETURNS TRIGGER AS $$
		BEGIN
		    IF NEW.status <> OLD.status OR NEW.credential_version <> OLD.credential_version THEN
		        UPDATE auth_sessions
		        SET status = 'revoked',
		            revoked_at = COALESCE(revoked_at, now()),
		            revoke_reason = CASE
		                WHEN NEW.status <> OLD.status THEN 'account_status_changed'
		                ELSE 'credential_changed'
		            END,
		            updated_at = now()
		        WHERE user_id = NEW.user_id AND status = 'active';
		    END IF;
		    RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER trg_auth_revoke_sessions_on_account_change
		    AFTER UPDATE OF status, credential_version ON iam_accounts
		    FOR EACH ROW EXECUTE FUNCTION auth_revoke_sessions_on_account_change();
		UPDATE dai_schema_metadata SET version = 45 WHERE singleton = TRUE;

		INSERT INTO iam_tenants (tenant_id, tenant_name)
		VALUES ('migration-0046-a', 'Migration 0046 A'),
		       ('migration-0046-b', 'Migration 0046 B');
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, status)
		VALUES ('migration-0046-role', NULL, 'migration-0046-role', 'unused', 2, 'active'),
		       ('migration-0046-scope', 'migration-0046-a', 'migration-0046-scope', 'unused', 3, 'active');
		INSERT INTO auth_sessions (session_id, user_id, credential_version, expires_at)
		VALUES ('46000000-0000-0000-0000-000000000001'::uuid, 'migration-0046-role', 1, now() + interval '1 hour'),
		       ('46000000-0000-0000-0000-000000000002'::uuid, 'migration-0046-scope', 1, now() + interval '1 hour');
	`); err != nil {
		t.Fatalf("prepare schema 45 fixture: %v", err)
	}

	migration, err := os.ReadFile("changes/0046_20260919_auth_session_context_invariant.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0046: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 46 {
		t.Fatalf("migration version = %d, want 46", version)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE iam_accounts
		SET user_type = 3, tenant_id = 'migration-0046-a'
		WHERE user_id = 'migration-0046-role';
		UPDATE iam_accounts
		SET tenant_id = 'migration-0046-b'
		WHERE user_id = 'migration-0046-scope';
	`); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		sessionID string
		reason    string
	}{
		{sessionID: "46000000-0000-0000-0000-000000000001", reason: "account_role_changed"},
		{sessionID: "46000000-0000-0000-0000-000000000002", reason: "tenant_scope_changed"},
	} {
		var status, reason string
		if err := pool.QueryRow(ctx, `
			SELECT status, COALESCE(revoke_reason, '')
			FROM auth_sessions
			WHERE session_id = $1::uuid
		`, tc.sessionID).Scan(&status, &reason); err != nil {
			t.Fatal(err)
		}
		if status != "revoked" || reason != tc.reason {
			t.Fatalf("session %s = %s/%s, want revoked/%s", tc.sessionID, status, reason, tc.reason)
		}
	}
}
