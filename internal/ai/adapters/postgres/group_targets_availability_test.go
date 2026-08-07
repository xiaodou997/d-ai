package postgres

import (
	"context"
	"testing"

	commercial "xiaodou/dai/internal/ai/commercial"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/testsupport"
)

// TestListGroupTargetDetailsAvailability exercises the P2-b visibility signal:
// a bound target whose upstream resource is later disabled, turned restricted,
// or has its tenant grant revoked must surface Available=false with a reason so
// the tenant sees the otherwise-silent "bound but every request is rejected"
// failure. Runs against the real db/init.sql schema (isolated schema pool), not
// hand-copied TEMP tables, so it stays honest about the actual columns.
func TestListGroupTargetDetailsAvailability(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{})
	if err != nil {
		t.Skipf("set DAI_TEST_DATABASE_URL to run group target availability DB tests: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(ctx) })

	const (
		tenantID  = "tenant-1"
		groupID   = "55555555-5555-5555-5555-555555555555"
		priceBook = "66666666-6666-6666-6666-666666666666"
		accountID = "77777777-7777-7777-7777-777777777777"
		bindingID = "88888888-8888-8888-8888-888888888888"
	)
	seed := []struct {
		q    string
		args []any
	}{
		{`INSERT INTO ai_groups (id, tenant_id, name, retail_price_book_id) VALUES ($1::uuid, $2, 'g', $3::uuid)`, []any{groupID, tenantID, priceBook}},
		{`INSERT INTO ai_upstream_accounts (id, name, tenant_display_name, tenant_access_mode, base_url, api_key_ciphertext, status)
		  VALUES ($1::uuid, 'acct', 'Acct', 'public', 'https://u', 'x', 'active')`, []any{accountID}},
		{`INSERT INTO ai_group_targets (id, group_id, target_kind, target_id, priority, status)
		  VALUES ($1::uuid, $2::uuid, 'direct_upstream', $3::uuid, 100, 'active')`, []any{bindingID, groupID, accountID}},
	}
	for _, s := range seed {
		if _, err := pool.Exec(ctx, s.q, s.args...); err != nil {
			t.Fatalf("seed %q: %v", s.q, err)
		}
	}

	repo := NewCommercialRepo(dbgen.New(pool), pool)
	scope := commercial.TenantGroupScope{TenantID: tenantID, GroupID: groupID}

	availability := func(t *testing.T) (bool, string) {
		t.Helper()
		items, err := repo.ListGroupTargetDetails(ctx, scope)
		if err != nil {
			t.Fatalf("ListGroupTargetDetails: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("want 1 target, got %d", len(items))
		}
		return items[0].Available, items[0].UnavailableReason
	}

	// Public + active → available.
	if ok, reason := availability(t); !ok || reason != "" {
		t.Fatalf("public active: available=%v reason=%q, want true/\"\"", ok, reason)
	}

	// Disabled resource → inactive (fail-closed at request time).
	if _, err := pool.Exec(ctx, `UPDATE ai_upstream_accounts SET status = 'disabled' WHERE id = $1::uuid`, accountID); err != nil {
		t.Fatalf("disable account: %v", err)
	}
	if ok, reason := availability(t); ok || reason != "inactive" {
		t.Fatalf("disabled: available=%v reason=%q, want false/inactive", ok, reason)
	}

	// Re-activate but turn restricted without a grant → access_revoked.
	if _, err := pool.Exec(ctx, `UPDATE ai_upstream_accounts SET status = 'active', tenant_access_mode = 'restricted' WHERE id = $1::uuid`, accountID); err != nil {
		t.Fatalf("restrict account: %v", err)
	}
	if ok, reason := availability(t); ok || reason != "access_revoked" {
		t.Fatalf("restricted ungranted: available=%v reason=%q, want false/access_revoked", ok, reason)
	}

	// Grant tenant access → available again.
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_upstream_resource_tenant_policies (resource_kind, resource_id, tenant_id, access_granted)
		VALUES ('direct_upstream', $1::uuid, $2, true)
	`, accountID, tenantID); err != nil {
		t.Fatalf("grant access: %v", err)
	}
	if ok, reason := availability(t); !ok || reason != "" {
		t.Fatalf("restricted granted: available=%v reason=%q, want true/\"\"", ok, reason)
	}

	// Revoke the grant → access_revoked again.
	if _, err := pool.Exec(ctx, `UPDATE ai_upstream_resource_tenant_policies SET access_granted = false WHERE resource_id = $1::uuid AND tenant_id = $2`, accountID, tenantID); err != nil {
		t.Fatalf("revoke access: %v", err)
	}
	if ok, reason := availability(t); ok || reason != "access_revoked" {
		t.Fatalf("revoked: available=%v reason=%q, want false/access_revoked", ok, reason)
	}
}
