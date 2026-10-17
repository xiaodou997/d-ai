package db_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0047PreservesSingleConnectionsAndRejectsLegacyCardinality(t *testing.T) {
	migration, err := os.ReadFile("changes/0047_20261017_single_upstream_account_endpoint.sql")
	if err != nil {
		t.Fatal(err)
	}
	rollback, err := os.ReadFile("rollback/0047_rollback.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, count := range []int{0, 1, 2} {
		t.Run([]string{"missing", "single", "multiple"}[count], func(t *testing.T) {
			ctx := t.Context()
			pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
			if err != nil {
				t.Skipf("database unavailable: %v", err)
			}
			t.Cleanup(func() { _ = cleanup(context.Background()) })
			conn, err := pool.Acquire(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Release()
			if _, err := conn.Exec(ctx, string(rollback)); err != nil {
				t.Fatal(err)
			}
			var accountID string
			if err := conn.QueryRow(ctx, `INSERT INTO ai_upstream_accounts(name,tenant_display_name,api_key_ciphertext) VALUES('single-account','Single','untouched-secret') RETURNING id::text`).Scan(&accountID); err != nil {
				t.Fatal(err)
			}
			for _, format := range []string{"openai_responses", "openai_chat"}[:count] {
				if _, err := conn.Exec(ctx, `INSERT INTO ai_upstream_account_endpoints(account_id,api_format,base_url) VALUES($1,$2,'https://example.test')`, accountID, format); err != nil {
					t.Fatal(err)
				}
			}
			var before string
			if err := conn.QueryRow(ctx, `SELECT coalesce(jsonb_agg(to_jsonb(e) ORDER BY e.id),'[]')::text FROM ai_upstream_account_endpoints e`).Scan(&before); err != nil {
				t.Fatal(err)
			}
			_, applyErr := conn.Exec(ctx, string(migration))
			if count != 1 {
				if applyErr == nil || !strings.Contains(applyErr.Error(), "exactly one endpoint") {
					t.Fatalf("invalid cardinality migration error = %v", applyErr)
				}
				if _, err := conn.Exec(ctx, "ROLLBACK"); err != nil {
					t.Fatal(err)
				}
			} else if applyErr != nil {
				t.Fatal(applyErr)
			}
			var version int
			var after, secret string
			if err := conn.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton`).Scan(&version); err != nil {
				t.Fatal(err)
			}
			wantVersion := 46
			if count == 1 {
				wantVersion = 47
			}
			if version != wantVersion {
				t.Fatalf("schema version = %d, want %d", version, wantVersion)
			}
			if err := conn.QueryRow(ctx, `SELECT coalesce(jsonb_agg(to_jsonb(e) ORDER BY e.id),'[]')::text FROM ai_upstream_account_endpoints e`).Scan(&after); err != nil {
				t.Fatal(err)
			}
			if err := conn.QueryRow(ctx, `SELECT api_key_ciphertext FROM ai_upstream_accounts WHERE id=$1`, accountID).Scan(&secret); err != nil {
				t.Fatal(err)
			}
			if before != after || secret != "untouched-secret" {
				t.Fatal("migration changed endpoint data or credentials")
			}
			if count == 1 {
				if _, err := conn.Exec(ctx, `INSERT INTO ai_upstream_account_endpoints(account_id,api_format,base_url) VALUES($1,'openai_chat','https://second.test')`, accountID); err == nil {
					t.Fatal("database accepted a second endpoint of a different format")
				}
				if _, err := conn.Exec(ctx, string(rollback)); err != nil {
					t.Fatal(err)
				}
				if _, err := conn.Exec(ctx, `INSERT INTO ai_upstream_account_endpoints(account_id,api_format,base_url) VALUES($1,'openai_chat','https://second.test')`, accountID); err != nil {
					t.Fatalf("rollback did not restore schema 46: %v", err)
				}
			}
		})
	}
}
