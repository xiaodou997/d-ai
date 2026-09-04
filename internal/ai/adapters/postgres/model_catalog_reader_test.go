package postgres

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/testsupport"
)

const (
	catalogTenantID       = "catalog-tenant"
	catalogPublicGroupID  = "10000000-0000-0000-0000-000000000001"
	catalogPrivateGroupID = "10000000-0000-0000-0000-000000000002"
	catalogRetailBookOne  = "20000000-0000-0000-0000-000000000001"
	catalogRetailBookTwo  = "20000000-0000-0000-0000-000000000002"
	catalogUpstreamBook   = "20000000-0000-0000-0000-000000000003"
	catalogAccountID      = "30000000-0000-0000-0000-000000000001"
)

func TestModelCatalogReaderPreservesVisibilityAndPricing(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("open model catalog test pool: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(ctx) })
	seedModelCatalog(t, ctx, pool)

	reader := NewModelCatalogReader(pool)
	tenantRows, err := reader.ListAvailableModelPrices(ctx, domain.ModelCatalogScope{TenantID: catalogTenantID})
	if err != nil {
		t.Fatalf("ListAvailableModelPrices(tenant): %v", err)
	}
	prices := make([]float64, 0, len(tenantRows))
	for _, row := range tenantRows {
		if row.ModelCode != "chat-model" || len(row.TokenPriceTiers) != 1 {
			t.Fatalf("tenant row = %+v", row)
		}
		prices = append(prices, row.TokenPriceTiers[0].InputPerToken)
	}
	slices.Sort(prices)
	if !slices.Equal(prices, []float64{0.000001, 0.000002}) {
		t.Fatalf("tenant rows = %+v", tenantRows)
	}

	userRows, err := reader.ListAvailableModelPrices(ctx, domain.ModelCatalogScope{TenantID: catalogTenantID, UserID: "user-1"})
	if err != nil {
		t.Fatalf("ListAvailableModelPrices(user default): %v", err)
	}
	if len(userRows) != 1 || userRows[0].TokenPriceTiers[0].InputPerToken != 0.000001 {
		t.Fatalf("default-visible user rows = %+v", userRows)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_user_groups (tenant_id, user_id, group_id)
		VALUES ($1, 'user-1', $2::uuid)
	`, catalogTenantID, catalogPrivateGroupID); err != nil {
		t.Fatalf("grant private group: %v", err)
	}
	userRows, err = reader.ListAvailableModelPrices(ctx, domain.ModelCatalogScope{TenantID: catalogTenantID, UserID: "user-1"})
	if err != nil {
		t.Fatalf("ListAvailableModelPrices(user granted): %v", err)
	}
	if len(userRows) != 2 {
		t.Fatalf("granted user rows = %+v", userRows)
	}

	groupRows, err := reader.ListRoutedGroupPrices(ctx, catalogPrivateGroupID)
	if err != nil {
		t.Fatalf("ListRoutedGroupPrices: %v", err)
	}
	if len(groupRows) != 1 || groupRows[0].TokenPriceTiers[0].InputPerToken != 0.000002 {
		t.Fatalf("group rows = %+v", groupRows)
	}
}

func TestModelCatalogReaderTenantResourceProjection(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("open tenant resource catalog test pool: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(ctx) })
	seedModelCatalog(t, ctx, pool)

	reader := NewModelCatalogReader(pool)
	if _, err := pool.Exec(ctx, `UPDATE ai_upstream_accounts SET tenant_access_mode = 'restricted' WHERE id = $1::uuid`, catalogAccountID); err != nil {
		t.Fatalf("restrict account: %v", err)
	}
	items, err := reader.ListTenantUpstreamResources(ctx, catalogTenantID)
	if err != nil {
		t.Fatalf("ListTenantUpstreamResources(restricted): %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("restricted resources = %+v, want empty", items)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_upstream_resource_tenant_policies (
			resource_kind, resource_id, tenant_id, access_granted, tenant_multiplier_override
		) VALUES ('direct_upstream', $1::uuid, $2, true, 1.75)
	`, catalogAccountID, catalogTenantID); err != nil {
		t.Fatalf("grant account: %v", err)
	}
	items, err = reader.ListTenantUpstreamResources(ctx, catalogTenantID)
	if err != nil {
		t.Fatalf("ListTenantUpstreamResources(granted): %v", err)
	}
	if len(items) != 1 || items[0].Kind != domain.UpstreamKindDirect || items[0].TenantMultiplier != 1.75 || items[0].PriceBookID != catalogUpstreamBook {
		t.Fatalf("resources = %+v", items)
	}
	if got := catalogModelCodes(items[0].Models); !slices.Equal(got, []string{"chat-model", "image-without-price"}) {
		t.Fatalf("model codes = %v", got)
	}
	if items[0].Models[0].Price == nil || len(items[0].Models[0].Price.TokenPriceTiers) != 1 {
		t.Fatalf("priced model = %+v", items[0].Models[0])
	}
	if items[0].Models[1].Price != nil {
		t.Fatalf("unpriced model = %+v, want nil price", items[0].Models[1])
	}
}

func seedModelCatalog(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	tokenTiersOne := `[{"up_to_input_tokens":null,"input_per_token":0.000001,"output_per_token":0.000003,"cache_write_per_token":0,"cache_read_per_token":0}]`
	tokenTiersTwo := `[{"up_to_input_tokens":null,"input_per_token":0.000002,"output_per_token":0.000004,"cache_write_per_token":0,"cache_read_per_token":0}]`
	seeds := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO ai_price_books (id, owner_type, name, status)
		  VALUES ($1::uuid, 'platform', 'retail-one', 'active'),
		         ($2::uuid, 'platform', 'retail-two', 'active'),
		         ($3::uuid, 'platform', 'upstream', 'active')`, []any{catalogRetailBookOne, catalogRetailBookTwo, catalogUpstreamBook}},
		{`INSERT INTO ai_price_book_entries (price_book_id, model_code, capability_type, token_price_tiers)
		  VALUES ($1::uuid, 'chat-model', 'chat', $2::jsonb),
		         ($3::uuid, 'chat-model', 'chat', $4::jsonb),
		         ($5::uuid, 'chat-model', 'chat', $2::jsonb)`, []any{catalogRetailBookOne, tokenTiersOne, catalogRetailBookTwo, tokenTiersTwo, catalogUpstreamBook}},
		{`INSERT INTO ai_upstream_accounts (
			id, name, tenant_display_name, tenant_access_mode,
			api_key_ciphertext, price_book_id, tenant_multiplier, status
		  ) VALUES ($1::uuid, 'catalog-account', 'Catalog Account', 'public',
			'x', $2::uuid, 1.25, 'active')`, []any{catalogAccountID, catalogUpstreamBook}},
		{`INSERT INTO ai_upstream_account_endpoints (account_id, api_format, base_url, status)
		  VALUES ($1::uuid, 'openai_chat', 'https://upstream.example', 'active'),
		         ($1::uuid, 'openai_images', 'https://upstream.example', 'active')`, []any{catalogAccountID}},
		{`INSERT INTO ai_upstream_models (
			upstream_kind, upstream_id, model_code, capability_type, upstream_model_name, status
		  ) VALUES
			('direct_upstream', $1::uuid, 'chat-model', 'chat', 'vendor-chat', 'active'),
			('direct_upstream', $1::uuid, 'image-without-price', 'image', 'vendor-image', 'active')`, []any{catalogAccountID}},
		{`INSERT INTO ai_groups (
			id, tenant_id, name, retail_price_book_id, user_default_visible, status
		  ) VALUES
			($1::uuid, $3, 'public-group', $4::uuid, true, 'active'),
			($2::uuid, $3, 'private-group', $5::uuid, false, 'active')`, []any{catalogPublicGroupID, catalogPrivateGroupID, catalogTenantID, catalogRetailBookOne, catalogRetailBookTwo}},
		{`INSERT INTO ai_group_targets (group_id, target_kind, target_id, status)
		  VALUES ($1::uuid, 'direct_upstream', $3::uuid, 'active'),
		         ($2::uuid, 'direct_upstream', $3::uuid, 'active')`, []any{catalogPublicGroupID, catalogPrivateGroupID, catalogAccountID}},
	}
	for _, seed := range seeds {
		if _, err := pool.Exec(ctx, seed.query, seed.args...); err != nil {
			t.Fatalf("seed model catalog: %v\nSQL: %s", err, seed.query)
		}
	}
}

func catalogModelCodes(items []domain.TenantUpstreamModel) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.ModelCode)
	}
	return out
}
