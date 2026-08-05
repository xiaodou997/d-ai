package postgres

import (
	"errors"
	"testing"

	"xiaodou/dai/internal/ai/commercial"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
)

const (
	dispatchTestPlatformBook = "10000000-0000-0000-0000-000000000001"
	dispatchTestTenantBook   = "10000000-0000-0000-0000-000000000002"
	dispatchTestOtherBook    = "10000000-0000-0000-0000-000000000003"
	dispatchTestEmptyBook    = "10000000-0000-0000-0000-000000000004"
	dispatchTestGroup        = "20000000-0000-0000-0000-000000000001"
)

func TestDispatchRequiredCapability(t *testing.T) {
	tests := map[string]string{
		"openai_chat":        "chat",
		"openai_responses":   "chat",
		"anthropic_messages": "chat",
		"gemini_text":        "chat",
		"openai_embeddings":  "embedding",
		"gemini_embeddings":  "embedding",
		"openai_images":      "image",
		"gemini_images":      "image",
	}
	for surface, want := range tests {
		if got := dispatchRequiredCapability(surface); got != want {
			t.Errorf("dispatchRequiredCapability(%q) = %q, want %q", surface, got, want)
		}
	}
}

func TestCommercialRepoRejectsInvisibleOrDisabledRetailPriceBooks(t *testing.T) {
	pool, ctx := openGroupTransferTestPool(t)
	repo := NewCommercialRepo(dbgen.New(pool), pool)
	for _, seed := range []struct{ id, ownerType, ownerTenant, status string }{
		{dispatchTestPlatformBook, "platform", "", "active"},
		{dispatchTestTenantBook, "tenant", "tenant-1", "active"},
		{dispatchTestOtherBook, "tenant", "tenant-2", "active"},
		{dispatchTestEmptyBook, "platform", "", "disabled"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO ai_price_books (id, owner_type, owner_tenant_id, status) VALUES ($1::uuid, $2, $3, $4)`, seed.id, seed.ownerType, seed.ownerTenant, seed.status); err != nil {
			t.Fatalf("seed price book: %v", err)
		}
	}
	for _, tc := range []struct {
		name    string
		bookID  string
		wantErr bool
	}{
		{name: "platform", bookID: dispatchTestPlatformBook},
		{name: "own tenant", bookID: dispatchTestTenantBook},
		{name: "other tenant", bookID: dispatchTestOtherBook, wantErr: true},
		{name: "disabled", bookID: dispatchTestEmptyBook, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := repo.CreateGroup(ctx, "tenant-1", commercial.GroupWrite{
				Name: tc.name, RetailPriceBookID: tc.bookID,
				DefaultUserMultiplier: 1, Status: commercial.StatusActive,
			})
			var conflict *domain.DispatchRulePriceConflictError
			if tc.wantErr && !errors.As(err, &conflict) {
				t.Fatalf("CreateGroup() error = %v, want price conflict", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("CreateGroup() error = %v", err)
			}
		})
	}
}

func TestCommercialRepoEnforcesDispatchRulePrices(t *testing.T) {
	pool, ctx := openGroupTransferTestPool(t)
	repo := NewCommercialRepo(dbgen.New(pool), pool)
	seeds := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO ai_price_books (id, owner_type, status) VALUES ($1::uuid, 'platform', 'active')`, []any{dispatchTestPlatformBook}},
		{`INSERT INTO ai_price_book_entries (price_book_id, model_code, capability_type) VALUES ($1::uuid, 'priced-model', 'chat')`, []any{dispatchTestPlatformBook}},
		{`INSERT INTO ai_groups (id, tenant_id, name, retail_price_book_id, default_user_multiplier, allow_protocol_conversion, sort_order, status)
		 VALUES ($1::uuid, 'tenant-1', 'Main', $2::uuid, 1, false, 0, 'active')`, []any{dispatchTestGroup, dispatchTestPlatformBook}},
	}
	for _, seed := range seeds {
		if _, err := pool.Exec(ctx, seed.query, seed.args...); err != nil {
			t.Fatalf("seed dispatch fixtures: %v", err)
		}
	}
	scope := commercial.TenantGroupScope{TenantID: "tenant-1", GroupID: dispatchTestGroup}
	created, err := repo.AddDispatchRule(ctx, scope, commercial.DispatchRuleWrite{
		ClientSurface: "openai_chat",
		MatchType:     commercial.DispatchMatchExact, MatchValue: "public-chat", TargetModelID: "priced-model",
		Priority: 10,
	})
	if err != nil {
		t.Fatalf("AddDispatchRule(priced chat) error = %v", err)
	}
	_, err = repo.AddDispatchRule(ctx, scope, commercial.DispatchRuleWrite{
		ClientSurface: "openai_images",
		MatchType:     commercial.DispatchMatchExact, MatchValue: "public-image", TargetModelID: "priced-model",
		Priority: 20,
	})
	var conflict *domain.DispatchRulePriceConflictError
	if !errors.As(err, &conflict) || len(conflict.Conflicts) != 1 || conflict.Conflicts[0].RequiredCapability != "image" {
		t.Fatalf("AddDispatchRule(image) error = %#v, want image price conflict", err)
	}
	if _, err := repo.UpdateDispatchRuleStatus(ctx, scope, created.ID, commercial.StatusDisabled); err != nil {
		t.Fatalf("UpdateDispatchRuleStatus(disable) error = %v", err)
	}
	if _, err := repo.UpdateDispatchRule(ctx, scope, created.ID, commercial.DispatchRuleWrite{
		ClientSurface: "openai_images", MatchType: commercial.DispatchMatchExact,
		MatchValue: "historical", TargetModelID: "missing-model", Priority: 10,
	}); err != nil {
		t.Fatalf("UpdateDispatchRule(disable invalid history) error = %v", err)
	}
	_, err = repo.UpdateDispatchRuleStatus(ctx, scope, created.ID, commercial.StatusActive)
	if !errors.As(err, &conflict) {
		t.Fatalf("UpdateDispatchRule(re-enable) error = %v, want price conflict", err)
	}
}

func TestPriceAndGroupWritesProtectActiveRules(t *testing.T) {
	pool, ctx := openGroupTransferTestPool(t)
	if _, err := pool.Exec(ctx, `
		ALTER TABLE ai_price_books
			ADD COLUMN name TEXT NOT NULL DEFAULT '', ADD COLUMN description TEXT NOT NULL DEFAULT '',
			ADD COLUMN revision BIGINT NOT NULL DEFAULT 1,
			ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT now(), ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
	`); err != nil {
		t.Fatalf("seed protected price fixtures: %v", err)
	}
	seeds := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO ai_price_books (id, owner_type, status, name) VALUES ($1::uuid, 'platform', 'active', 'Priced'), ($2::uuid, 'platform', 'active', 'Empty')`, []any{dispatchTestPlatformBook, dispatchTestEmptyBook}},
		{`INSERT INTO ai_price_book_entries (price_book_id, model_code, capability_type) VALUES ($1::uuid, 'priced-model', 'chat')`, []any{dispatchTestPlatformBook}},
		{`INSERT INTO ai_groups (id, tenant_id, name, retail_price_book_id, default_user_multiplier, allow_protocol_conversion, sort_order, status)
		 VALUES ($1::uuid, 'tenant-1', 'Main', $2::uuid, 1, false, 0, 'active')`, []any{dispatchTestGroup, dispatchTestPlatformBook}},
		{`INSERT INTO ai_group_model_dispatch_rules (group_id, client_surface, match_type, match_value, target_model_code, priority, status, notes)
		 VALUES ($1::uuid, 'openai_chat', 'exact', 'public', 'priced-model', 10, 'active', '')`, []any{dispatchTestGroup}},
	}
	for _, seed := range seeds {
		if _, err := pool.Exec(ctx, seed.query, seed.args...); err != nil {
			t.Fatalf("seed protected price fixtures: %v", err)
		}
	}
	q := dbgen.New(pool)
	commercialRepo := NewCommercialRepo(q, pool)
	priceRepo := NewPriceBookRepo(q, pool)
	_, err := commercialRepo.UpdateGroup(ctx, commercial.TenantGroupScope{TenantID: "tenant-1", GroupID: dispatchTestGroup}, commercial.GroupWrite{
		Name: "Main", RetailPriceBookID: dispatchTestEmptyBook,
		DefaultUserMultiplier: 1, Status: commercial.StatusActive,
	})
	var conflict *domain.DispatchRulePriceConflictError
	if !errors.As(err, &conflict) || len(conflict.Conflicts) != 1 {
		t.Fatalf("UpdateGroup(price switch) error = %#v, want one price conflict", err)
	}
	if err := priceRepo.DeleteEntry(ctx, dispatchTestPlatformBook, "priced-model", "chat"); !errors.As(err, &conflict) {
		t.Fatalf("DeleteEntry() error = %v, want price conflict", err)
	}
	if _, err := priceRepo.UpdatePriceBook(ctx, dispatchTestPlatformBook, "Priced", "", "disabled"); !errors.As(err, &conflict) {
		t.Fatalf("UpdatePriceBook(disabled) error = %v, want price conflict", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE ai_groups SET status = 'disabled' WHERE id = $1::uuid`, dispatchTestGroup); err != nil {
		t.Fatalf("seed historical invalid group: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM ai_price_book_entries WHERE price_book_id = $1::uuid`, dispatchTestPlatformBook); err != nil {
		t.Fatalf("remove historical price entry: %v", err)
	}
	if _, err := commercialRepo.UpdateGroupStatus(ctx, commercial.TenantGroupScope{TenantID: "tenant-1", GroupID: dispatchTestGroup}, commercial.StatusActive); !errors.As(err, &conflict) {
		t.Fatalf("UpdateGroupStatus(active) error = %v, want price conflict", err)
	}
}

func TestCommercialRepoDeleteGroupReportsBusinessReferencesAndCascadesConfiguration(t *testing.T) {
	pool, ctx := openGroupTransferTestPool(t)
	if _, err := pool.Exec(ctx, `
		ALTER TABLE ai_sub_plan_groups ADD COLUMN plan_id UUID NOT NULL DEFAULT gen_random_uuid();
		CREATE TEMP TABLE ai_apps (group_id UUID)
	`); err != nil {
		t.Fatalf("seed group delete fixtures: %v", err)
	}
	seeds := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO ai_price_books (id, owner_type, status) VALUES ($1::uuid, 'platform', 'active')`, []any{dispatchTestPlatformBook}},
		{`INSERT INTO ai_groups (id, tenant_id, name, retail_price_book_id, default_user_multiplier, allow_protocol_conversion, sort_order, status)
		 VALUES ($1::uuid, 'tenant-1', 'Main', $2::uuid, 1, false, 0, 'active')`, []any{dispatchTestGroup, dispatchTestPlatformBook}},
		{`INSERT INTO ai_group_client_surfaces (group_id, surface, bridge_enabled, status) VALUES ($1::uuid, 'openai_chat', false, 'active')`, []any{dispatchTestGroup}},
		{`INSERT INTO ai_group_model_dispatch_rules (group_id, client_surface, match_type, match_value, target_model_code, priority, status, notes)
		 VALUES ($1::uuid, 'openai_chat', 'exact', 'public', 'model', 10, 'disabled', '')`, []any{dispatchTestGroup}},
		{`INSERT INTO ai_group_targets (group_id, target_kind, target_id, priority, status) VALUES ($1::uuid, 'direct_upstream', gen_random_uuid(), 10, 'active')`, []any{dispatchTestGroup}},
		{`INSERT INTO ai_user_groups (group_id) VALUES ($1::uuid)`, []any{dispatchTestGroup}},
		{`INSERT INTO ai_api_keys (group_id) VALUES ($1::uuid)`, []any{dispatchTestGroup}},
		{`INSERT INTO ai_sub_plan_groups (group_id) VALUES ($1::uuid)`, []any{dispatchTestGroup}},
		{`INSERT INTO ai_apps (group_id) VALUES ($1::uuid)`, []any{dispatchTestGroup}},
	}
	for _, seed := range seeds {
		if _, err := pool.Exec(ctx, seed.query, seed.args...); err != nil {
			t.Fatalf("seed group delete fixtures: %v", err)
		}
	}
	repo := NewCommercialRepo(dbgen.New(pool), pool)
	err := repo.DeleteGroup(ctx, commercial.TenantGroupScope{TenantID: "tenant-1", GroupID: dispatchTestGroup})
	var inUse *domain.GroupInUseError
	if !errors.As(err, &inUse) {
		t.Fatalf("DeleteGroup(referenced) error = %v, want group in use", err)
	}
	if inUse.Dependencies != (domain.GroupDependencyCounts{UserBindings: 1, APIKeyBindings: 1, SubscriptionPlans: 1, Applications: 1}) {
		t.Fatalf("DeleteGroup dependencies = %+v", inUse.Dependencies)
	}
	if _, err := pool.Exec(ctx, `
		DELETE FROM ai_user_groups; DELETE FROM ai_api_keys; DELETE FROM ai_sub_plan_groups; DELETE FROM ai_apps;
	`); err != nil {
		t.Fatalf("clear business references: %v", err)
	}
	if err := repo.DeleteGroup(ctx, commercial.TenantGroupScope{TenantID: "tenant-1", GroupID: dispatchTestGroup}); err != nil {
		t.Fatalf("DeleteGroup(unreferenced) error = %v", err)
	}
	for _, table := range []string{"ai_groups", "ai_group_client_surfaces", "ai_group_model_dispatch_rules", "ai_group_targets"} {
		var count int
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("%s count = %d, want 0", table, count)
		}
	}
}
