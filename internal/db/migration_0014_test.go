package db_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0014AddsDurableAuditInbox(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		UPDATE dai_schema_metadata SET version = 13 WHERE singleton = TRUE;
		DROP TABLE IF EXISTS ai_audit_inbox;
		DROP INDEX IF EXISTS idx_arp_request_id;
		CREATE INDEX idx_arp_request_id ON ai_request_payloads (request_id);
	`); err != nil {
		t.Fatalf("prepare schema 13 fixture: %v", err)
	}
	migration, err := os.ReadFile("changes/0014_20260820_durable_audit_inbox.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0014: %v", err)
	}

	var version int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	var unique bool
	if err := pool.QueryRow(ctx, `
		SELECT ix.indisunique
		FROM pg_index ix
		JOIN pg_class c ON c.oid = ix.indexrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = current_schema() AND c.relname = 'idx_arp_request_id'
	`).Scan(&unique); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := pool.QueryRow(ctx, `
		INSERT INTO ai_audit_inbox (request_id, payload)
		VALUES ('migration-audit', '{"requestId":"migration-audit"}'::jsonb)
		RETURNING status
	`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_audit_inbox (request_id, payload)
		VALUES ('migration-audit', '{}'::jsonb)
	`); err == nil || !isUniqueViolation(err) {
		t.Fatalf("duplicate inbox request was accepted: %v", err)
	}
	if version != 14 || !unique || status != "pending" {
		t.Fatalf("migration result = version:%d unique:%t status:%q", version, unique, status)
	}
}

func isUniqueViolation(err error) bool {
	pgErr, ok := err.(*pgconn.PgError)
	return ok && pgErr.Code == "23505"
}
