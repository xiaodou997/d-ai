package postgres

import (
	"context"
	"testing"

	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/testsupport"
)

// TestUsageStatsReportsAllThreeLedgerLines pins the reconciliation contract for
// the usage list header against the canonical schema.
//
// The header used to sum user_charged alone. That column is zero by construction
// for tenant-owned API keys (Calculate never fills the user retail line for an
// OwnerTenant subject) and for subscription-covered user traffic (the gate
// zeroes the debit while the tenant is still charged). A single-column header
// therefore reported real, billable traffic as costing nothing, and showed the
// platform administrator tenant->end-user revenue instead of platform receivable.
func TestUsageStatsReportsAllThreeLedgerLines(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("usage stats test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	const tenantID = "tenant-stats"

	// Row 1: a tenant-owned API key. user_charged is 0, tenant_payable is real.
	// Row 2: a subscription-covered end user. user_charged is 0, tenant_payable is real.
	// Row 3: ordinary pay-as-you-go end user traffic. every line is non-zero.
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_usage_logs (
		  request_id, key_owner_type, auth_method, request_source, tenant_id, user_id,
		  model_code, total_tokens,
		  catalog_base, tenant_payable, retail_base, user_payable, user_charged,
		  billing_source, billing_status, request_status, subscription_id
		) VALUES
		  ('req-tenant-key', 'tenant', 'api_key', 'api_key', $1, NULL,
		   'gpt-4o-mini', 100, 1000, 1500, 0, 0, 0, 'payg', 'settled', 'success', NULL),
		  ('req-subscription', 'user', 'api_key', 'api_key', $1, 'user-1',
		   'gpt-4o-mini', 200, 2000, 3000, 5000, 5000, 0, 'subscription', 'settled', 'success',
		   '44444444-4444-4444-4444-444444444444'),
		  ('req-payg-user', 'user', 'api_key', 'api_key', $1, 'user-2',
		   'gpt-4o-mini', 300, 4000, 6000, 9000, 9000, 9000, 'payg', 'settled', 'success', NULL)
	`, tenantID); err != nil {
		t.Fatalf("seed usage rows: %v", err)
	}

	repo := NewUsageRepo(dbgen.New(pool), pool)
	stats, err := repo.StatsFor(ctx, domain.UsageFilter{TenantID: tenantID})
	if err != nil {
		t.Fatalf("StatsFor() error = %v", err)
	}

	if stats.TotalRequests != 3 {
		t.Fatalf("TotalRequests = %d, want 3", stats.TotalRequests)
	}
	// Platform receivable spans every row, including the two whose user debit is zero.
	if got, want := stats.TotalTenantPayableMicro, int64(1500+3000+6000); got != want {
		t.Errorf("TotalTenantPayableMicro = %d, want %d", got, want)
	}
	if got, want := stats.TotalCatalogBaseMicro, int64(1000+2000+4000); got != want {
		t.Errorf("TotalCatalogBaseMicro = %d, want %d", got, want)
	}
	// Tenant revenue from end users counts only the pay-as-you-go row.
	if got, want := stats.TotalUserChargedMicro, int64(9000); got != want {
		t.Errorf("TotalUserChargedMicro = %d, want %d", got, want)
	}
	// The regression this test exists for: a header driven by user debit alone
	// would have reported the first two rows as free traffic.
	if stats.TotalTenantPayableMicro <= stats.TotalUserChargedMicro {
		t.Errorf(
			"tenant charge %d must exceed user debit %d when tenant-key and subscription traffic is present",
			stats.TotalTenantPayableMicro, stats.TotalUserChargedMicro,
		)
	}
}

// TestTenantUpstreamCatalogHidesConnectionDetails pins the disclosure boundary.
// The safe view is the only thing standing between the base table and the
// tenant catalog, so a future "SELECT *"-style edit must fail here. Platform
// purchase cost is no longer a concern because the system does not store it at
// all, but credentials and endpoints still are.
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
func TestUsageChargeSemanticsAreEnforcedInDatabase(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("charge semantics test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	insert := func(requestID, billingSource string, payable, charged int64, subscriptionID any) error {
		_, err := pool.Exec(ctx, `
			INSERT INTO ai_usage_logs (
			  request_id, key_owner_type, auth_method, request_source, tenant_id, user_id,
			  model_code, user_payable, user_charged,
			  billing_source, billing_status, request_status, subscription_id
			) VALUES ($1, 'user', 'api_key', 'api_key', 'tenant-x', 'user-x',
			  'gpt-4o-mini', $2, $3, $4, 'settled', 'success', $5)
		`, requestID, payable, charged, billingSource, subscriptionID)
		return err
	}

	const subID = "55555555-5555-5555-5555-555555555555"

	for _, tc := range []struct {
		name           string
		requestID      string
		billingSource  string
		payable        int64
		charged        int64
		subscriptionID any
		wantErr        bool
	}{
		{name: "payg charges in full", requestID: "ok-payg", billingSource: "payg", payable: 900, charged: 900},
		{name: "tenant key zeroes both sides", requestID: "ok-tenant", billingSource: "payg", payable: 0, charged: 0},
		{
			name: "subscription waives the debit", requestID: "ok-sub", billingSource: "subscription",
			payable: 900, charged: 0, subscriptionID: subID,
		},
		{
			// Silently under-charging without declaring why is the shape that made
			// zeroed debits indistinguishable from genuinely free traffic.
			name: "payg cannot silently discount", requestID: "bad-partial", billingSource: "payg",
			payable: 900, charged: 100, wantErr: true,
		},
		{
			name: "charged may not exceed payable", requestID: "bad-over", billingSource: "payg",
			payable: 100, charged: 900, wantErr: true,
		},
		{
			name: "subscription must name its subscription", requestID: "bad-sub-null", billingSource: "subscription",
			payable: 900, charged: 0, subscriptionID: nil, wantErr: true,
		},
		{
			name: "subscription may not also charge", requestID: "bad-sub-charged", billingSource: "subscription",
			payable: 900, charged: 900, subscriptionID: subID, wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := insert(tc.requestID, tc.billingSource, tc.payable, tc.charged, tc.subscriptionID)
			if tc.wantErr && err == nil {
				t.Fatal("expected ai_usage_logs_charge_semantics to reject this row")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("valid row rejected: %v", err)
			}
		})
	}
}

// TestUpstreamSummaryReportsOutputPerResource pins the report that replaced
// platform-margin reporting. Since the system no longer stores what the platform
// paid upstream, this summary is the denominator an operator divides their own
// top-up ledger by — so it has to be right about two things a naive query gets
// wrong: it must not add image counts to token counts, and it must not lose a
// resource's history when the resource itself is deleted.
func TestUpstreamSummaryReportsOutputPerResource(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("upstream summary test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	const tenantID = "tenant-upstream-summary"
	var accountID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO ai_upstream_accounts (name, tenant_display_name, api_key_ciphertext)
		VALUES ('acct-live', 'acct-live', 'cipher')
		RETURNING id::text
	`).Scan(&accountID); err != nil {
		t.Fatalf("seed account: %v", err)
	}

	// A chat row and an image row on the live account, plus one row whose
	// account no longer exists (upstream_account_id has no FK, preserving history).
	const goneID = "99999999-9999-9999-9999-999999999999"
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_usage_logs (
		  request_id, key_owner_type, auth_method, request_source, tenant_id, user_id,
		  model_code, upstream_account_id, provider_code,
		  prompt_tokens, completion_tokens, total_tokens,
		  billable_unit_type, billable_units,
		  catalog_base, tenant_payable, retail_base, user_payable, user_charged,
		  billing_source, billing_status, request_status
		) VALUES
		  ('req-chat', 'user', 'api_key', 'api_key', $1, 'user-1',
		   'gpt-4o-mini', $2::uuid, 'openai', 100, 50, 150, 'token', 150,
		   1000, 1500, 0, 0, 0, 'payg', 'settled', 'success'),
		  ('req-image', 'user', 'api_key', 'api_key', $1, 'user-1',
		   'gpt-image-1', $2::uuid, 'openai', 0, 0, 0, 'image', 3,
		   2000, 3000, 0, 0, 0, 'payg', 'settled', 'failed'),
		  ('req-orphan', 'user', 'api_key', 'api_key', $1, 'user-1',
		   'gpt-4o-mini', $3::uuid, 'legacy-provider', 10, 5, 15, 'token', 15,
		   100, 200, 0, 0, 0, 'payg', 'settled', 'success')
	`, tenantID, accountID, goneID); err != nil {
		t.Fatalf("seed usage rows: %v", err)
	}

	repo := NewUsageRepo(dbgen.New(pool), pool)
	rows, err := repo.UpstreamSummary(ctx, domain.UsageSummaryFilter{TenantID: tenantID})
	if err != nil {
		t.Fatalf("UpstreamSummary() error = %v", err)
	}

	byID := map[string]domain.UsageUpstreamSummaryRow{}
	for _, row := range rows {
		byID[row.TargetID] = row
	}

	live, ok := byID[accountID]
	if !ok {
		t.Fatalf("live account missing from summary; got %+v", rows)
	}
	if live.TargetName != "acct-live" {
		t.Errorf("TargetName = %q, want acct-live", live.TargetName)
	}
	if live.RequestCount != 2 || live.SuccessCount != 1 || live.FailedCount != 1 {
		t.Errorf("counts = %d/%d/%d, want 2/1/1", live.RequestCount, live.SuccessCount, live.FailedCount)
	}
	if live.TotalPromptTokens != 100 || live.TotalCompletionTokens != 50 || live.TotalTokens != 150 {
		t.Errorf("tokens = %d/%d/%d, want 100/50/150",
			live.TotalPromptTokens, live.TotalCompletionTokens, live.TotalTokens)
	}
	// The split this test exists for: 3 images must not land in the token column.
	if live.TokenUnits != 150 {
		t.Errorf("TokenUnits = %d, want 150", live.TokenUnits)
	}
	if live.ImageUnits != 3 {
		t.Errorf("ImageUnits = %d, want 3", live.ImageUnits)
	}

	orphan, ok := byID[goneID]
	if !ok {
		t.Fatalf("deleted account's history vanished from the summary; got %+v", rows)
	}
	if orphan.TargetName != "" {
		t.Errorf("TargetName = %q, want empty so the caller falls back to provider_code", orphan.TargetName)
	}
	if orphan.ProviderCode != "legacy-provider" {
		t.Errorf("ProviderCode = %q, want legacy-provider as the fallback label", orphan.ProviderCode)
	}
}
