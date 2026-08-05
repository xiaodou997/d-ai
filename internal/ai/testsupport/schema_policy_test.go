package testsupport

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCanonicalSchemaConstraintPolicy(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	schemaPath := filepath.Join(filepath.Dir(currentFile), "..", "..", "db", "init.sql")
	raw, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read canonical schema: %v", err)
	}
	schema := string(raw)
	aiStart := strings.Index(schema, "-- AI domain")
	if aiStart < 0 {
		t.Fatal("canonical schema is missing AI domain marker")
	}
	schema = schema[aiStart:]

	for _, forbidden := range []string{
		"CHECK (auth_method IN",
		"CHECK (request_source IN",
		"CHECK (default_protocol IN",
		"CHECK (capability_type IN",
		"CHECK (api_format IN",
		"CHECK (provider_type IN",
		"CHECK (client_surface IN",
		"CHECK (duration_days IN",
		"CHECK (period_type IN",
		"CHECK (status IN ('active', 'disabled'))",
		"audience = 'tenant_users'",
	} {
		if strings.Contains(schema, forbidden) {
			t.Errorf("canonical schema contains forbidden database policy %q", forbidden)
		}
	}

	for _, required := range []string{
		"CHECK (actual_tenant_micro IS NULL OR actual_tenant_micro >= 0)",
		"status IN ('active', 'reconciling', 'completed')",
		"completion_source IN ('runtime', 'manual')",
		"CONSTRAINT ai_usage_logs_nonnegative CHECK",
		"CONSTRAINT ai_sub_plans_inventory_within_limit_check",
		"CHECK ((status = 'paid' AND paid_at IS NOT NULL)",
		"UNIQUE (owner_type, owner_tenant_id, name)",
		"CONSTRAINT ai_user_groups_tenant_group_fk",
		"CONSTRAINT ai_api_keys_tenant_group_fk",
	} {
		if !strings.Contains(schema, required) {
			t.Errorf("canonical schema is missing stable invariant %q", required)
		}
	}
	if strings.Contains(schema, "CREATE TABLE IF NOT EXISTS ai_user_credit_ledger") {
		t.Error("fresh canonical schema must not recreate the retired V2 local credit ledger")
	}
}

func TestCanonicalSchemaDoesNotKeepUpstreamAccountWeight(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	schemaPath := filepath.Join(filepath.Dir(currentFile), "..", "..", "db", "init.sql")
	raw, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read canonical schema: %v", err)
	}
	schema := string(raw)

	const tableStart = "CREATE TABLE IF NOT EXISTS ai_upstream_accounts ("
	start := strings.Index(schema, tableStart)
	if start < 0 {
		t.Fatal("canonical schema is missing ai_upstream_accounts")
	}
	table := schema[start:]
	if end := strings.Index(table, "\n  );"); end >= 0 {
		table = table[:end]
	}
	if strings.Contains(table, "weight") {
		t.Fatal("canonical schema must not retain ai_upstream_accounts.weight")
	}
}
