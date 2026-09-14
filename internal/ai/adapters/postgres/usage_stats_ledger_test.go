package postgres

import (
	"context"
	"testing"

	"xiaodou/dai/internal/ai/testsupport"
)

// Tenant catalog must continue hiding upstream connection secrets.
func TestTenantUpstreamCatalogHidesConnectionDetails(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("upstream catalog test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	rows, err := pool.Query(ctx, `
		SELECT column_name
		FROM information_schema.columns
		WHERE table_name = 'ai_upstream_resources'
	`)
	if err != nil {
		t.Fatalf("read view columns: %v", err)
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate columns: %v", err)
	}

	for _, forbidden := range []string{
		"base_url", "api_key_ciphertext", "extra_headers", "upstream_model",
		// Cost columns were removed outright; assert they never come back.
		"upstream_multiplier", "monthly_cost_usd",
	} {
		if columns[forbidden] {
			t.Errorf("ai_upstream_resources exposes %q to tenants", forbidden)
		}
	}
	// Guard against the opposite mistake: the view still has to carry the
	// tenant's own rate, which tenants are entitled to see.
	if !columns["tenant_multiplier"] {
		t.Error("ai_upstream_resources lost tenant_multiplier, which tenants need")
	}
}

// TestUsageChargeSemanticsAreEnforcedInDatabase pins the subscription contract
// at the storage layer rather than trusting every writer to uphold it.
//
// The relationship between what a user owes and what they were actually charged
// is discriminated solely by billing_source. Keeping the discount implicit in
// the amounts is what let earlier reporting code treat a zeroed debit as free
// traffic, so the shapes that would allow it are rejected outright.
