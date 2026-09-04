package db_test

import (
	"context"
	"os"
	"testing"

	"xiaodou/dai/internal/dbtest"
)

func TestMigration0032SplitsAccountsEndpointsAndUsageAttribution(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		DROP INDEX idx_ai_usage_logs_upstream_account;
		DROP INDEX idx_ai_usage_logs_endpoint;
		ALTER TABLE ai_usage_logs DROP COLUMN upstream_account_id;
		DROP TABLE ai_upstream_account_endpoints;
		ALTER TABLE ai_upstream_accounts
			ADD COLUMN base_url TEXT NOT NULL DEFAULT '',
			ADD COLUMN extra_headers JSONB NOT NULL DEFAULT '{}',
			ADD COLUMN default_protocol TEXT NOT NULL DEFAULT 'openai_compatible';
		ALTER TABLE ai_upstream_models
			ADD COLUMN api_format TEXT NOT NULL DEFAULT 'openai_chat';
		UPDATE dai_schema_metadata SET version = 31 WHERE singleton = TRUE;
	`); err != nil {
		t.Fatalf("prepare schema 31 fixture: %v", err)
	}

	const (
		multiAccountID    = "11111111-1111-1111-1111-111111111111"
		fallbackAccountID = "22222222-2222-2222-2222-222222222222"
	)
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_upstream_accounts
			(id, name, tenant_display_name, api_key_ciphertext, base_url, extra_headers, default_protocol, status)
		VALUES
			($1::uuid, 'multi', 'Multi', 'cipher-1', 'https://multi.example', '{"X-Test":"one"}', 'openai_compatible', 'active'),
			($2::uuid, 'fallback', 'Fallback', 'cipher-2', 'https://anthropic.example', '{}', 'anthropic', 'active')
	`, multiAccountID, fallbackAccountID); err != nil {
		t.Fatalf("seed schema 31 accounts: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_upstream_models
			(upstream_kind, upstream_id, model_code, capability_type, upstream_model_name, api_format, status)
		VALUES
			('direct_upstream', $1::uuid, 'chat-model', 'chat', 'chat-model', 'openai_chat', 'active'),
			('direct_upstream', $1::uuid, 'responses-model', 'chat', 'responses-model', 'openai_responses', 'active')
	`, multiAccountID); err != nil {
		t.Fatalf("seed schema 31 models: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_usage_logs
			(request_id, key_owner_type, tenant_id, model_code, endpoint_id, provider_format, billing_status, request_status)
		VALUES
			('migration-0032-usage', 'tenant', 'tenant-1', 'responses-model', $1::uuid, 'openai_responses', 'free', 'success')
	`, multiAccountID); err != nil {
		t.Fatalf("seed schema 31 usage: %v", err)
	}

	migration, err := os.ReadFile("changes/0032_20260904_upstream_account_endpoints.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply migration 0032: %v", err)
	}

	var version, endpointCount, oldColumnCount int
	if err := pool.QueryRow(ctx, `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 32 {
		t.Fatalf("migration version = %d, want 32", version)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ai_upstream_account_endpoints`).Scan(&endpointCount); err != nil {
		t.Fatal(err)
	}
	if endpointCount != 3 {
		t.Fatalf("endpoint count = %d, want 3", endpointCount)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND ((table_name = 'ai_upstream_accounts' AND column_name IN ('base_url', 'extra_headers', 'default_protocol'))
		    OR (table_name = 'ai_upstream_models' AND column_name = 'api_format'))
	`).Scan(&oldColumnCount); err != nil {
		t.Fatal(err)
	}
	if oldColumnCount != 0 {
		t.Fatalf("old upstream columns remaining = %d", oldColumnCount)
	}

	var formats []string
	if err := pool.QueryRow(ctx, `
		SELECT array_agg(api_format ORDER BY api_format)
		FROM ai_upstream_account_endpoints WHERE account_id = $1::uuid
	`, multiAccountID).Scan(&formats); err != nil {
		t.Fatal(err)
	}
	if len(formats) != 2 || formats[0] != "openai_chat" || formats[1] != "openai_responses" {
		t.Fatalf("migrated formats = %v", formats)
	}
	if err := pool.QueryRow(ctx, `
		SELECT array_agg(api_format ORDER BY api_format)
		FROM ai_upstream_account_endpoints WHERE account_id = $1::uuid
	`, fallbackAccountID).Scan(&formats); err != nil {
		t.Fatal(err)
	}
	if len(formats) != 1 || formats[0] != "anthropic_messages" {
		t.Fatalf("fallback formats = %v", formats)
	}

	var usageAccountID, usageEndpointFormat string
	if err := pool.QueryRow(ctx, `
		SELECT l.upstream_account_id::text, ae.api_format
		FROM ai_usage_logs l
		JOIN ai_upstream_account_endpoints ae ON ae.id = l.endpoint_id
		WHERE l.request_id = 'migration-0032-usage'
	`).Scan(&usageAccountID, &usageEndpointFormat); err != nil {
		t.Fatal(err)
	}
	if usageAccountID != multiAccountID || usageEndpointFormat != "openai_responses" {
		t.Fatalf("usage attribution = account %q format %q", usageAccountID, usageEndpointFormat)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_upstream_account_endpoints (account_id, api_format, base_url)
		VALUES ($1::uuid, 'openai_responses', 'https://duplicate.example')
	`, multiAccountID); err == nil {
		t.Fatal("duplicate account api_format was accepted")
	}
}
