package postgres

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/testsupport"
)

const (
	directBindingScopeID = "10000000-0000-0000-0000-000000000001"
	poolBindingScopeID   = "20000000-0000-0000-0000-000000000002"
)

func TestUpstreamModelBindingStoreCRUDAndScopeIsolation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("open model binding store test pool: %v", err)
	}
	defer func() { _ = cleanup(ctx) }()
	seedBindingTargets(t, ctx, pool)

	store := NewUpstreamModelBindingStore(pool)
	directScope := domain.UpstreamModelBindingScope{Kind: domain.UpstreamKindDirect, ID: directBindingScopeID}
	poolScope := domain.UpstreamModelBindingScope{Kind: domain.UpstreamKindPool, ID: poolBindingScopeID}
	write := domain.UpstreamModelBindingWrite{
		ModelCode:         "gpt-test",
		CapabilityType:    "chat",
		UpstreamModelName: "vendor-gpt-test",
		Status:            "active",
		ConfigJSON:        []byte(`{"image_generation":{"stream_mode":"force_sync"}}`),
	}

	direct, err := store.Create(ctx, directScope, write)
	if err != nil {
		t.Fatalf("Create(direct): %v", err)
	}
	if direct.ID == "" || direct.ModelCode != write.ModelCode || direct.Status != "active" || direct.CreatedAt.IsZero() || direct.UpdatedAt.IsZero() {
		t.Fatalf("direct binding = %+v", direct)
	}
	if _, err := store.Create(ctx, directScope, write); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("Create(duplicate) error = %v, want conflict", err)
	}
	poolBinding, err := store.Create(ctx, poolScope, write)
	if err != nil {
		t.Fatalf("Create(pool): %v", err)
	}
	if poolBinding.ID == direct.ID {
		t.Fatal("scope-isolated bindings received the same ID")
	}

	items, err := store.List(ctx, directScope)
	if err != nil {
		t.Fatalf("List(direct): %v", err)
	}
	if len(items) != 1 || items[0].ID != direct.ID {
		t.Fatalf("direct items = %+v", items)
	}
	if _, err := store.Get(ctx, poolScope, direct.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Get(cross-scope) error = %v, want not found", err)
	}

	write.Status = "disabled"
	write.UpstreamModelName = "vendor-gpt-test-v2"
	updated, err := store.Update(ctx, directScope, direct.ID, write)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Status != "disabled" || updated.UpstreamModelName != "vendor-gpt-test-v2" || updated.UpdatedAt.Before(direct.UpdatedAt) {
		t.Fatalf("updated binding = %+v", updated)
	}

	codes, err := store.ListModelCodes(ctx, directScope)
	if err != nil {
		t.Fatalf("ListModelCodes: %v", err)
	}
	if !slices.Equal(codes, []string{"gpt-test"}) {
		t.Fatalf("codes = %v", codes)
	}
	activeWrite := write
	activeWrite.CapabilityType = "embedding"
	activeWrite.Status = "active"
	active, err := store.Create(ctx, directScope, activeWrite)
	if err != nil {
		t.Fatalf("Create(active alternate capability): %v", err)
	}
	found, err := store.FindByModel(ctx, directScope, "gpt-test")
	if err != nil {
		t.Fatalf("FindByModel: %v", err)
	}
	if found.ID != active.ID || found.Status != "active" {
		t.Fatalf("found binding = %+v, want active binding %q", found, active.ID)
	}
	deleted, err := store.BatchDelete(ctx, directScope, []string{direct.ID, "30000000-0000-0000-0000-000000000003"})
	if err != nil {
		t.Fatalf("BatchDelete: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	if err := store.Delete(ctx, poolScope, poolBinding.ID); err != nil {
		t.Fatalf("Delete(pool): %v", err)
	}
	if err := store.Delete(ctx, poolScope, poolBinding.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second Delete error = %v, want not found", err)
	}
}

func TestUpstreamModelBindingStoreImportIsOrderedAndIdempotent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("open model binding import test pool: %v", err)
	}
	defer func() { _ = cleanup(ctx) }()
	seedBindingTargets(t, ctx, pool)

	store := NewUpstreamModelBindingStore(pool)
	scope := domain.UpstreamModelBindingScope{Kind: domain.UpstreamKindPool, ID: poolBindingScopeID}
	if _, err := store.Create(ctx, scope, importBindingWrite("existing-model")); err != nil {
		t.Fatalf("seed binding: %v", err)
	}

	result, err := store.Import(ctx, scope, []domain.UpstreamModelBindingWrite{
		importBindingWrite("new-model"),
		importBindingWrite("existing-model"),
		importBindingWrite("new-model"),
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if !slices.Equal(result.Created, []string{"new-model"}) || !slices.Equal(result.Skipped, []string{"existing-model", "new-model"}) {
		t.Fatalf("result = %+v", result)
	}
}

func TestUpstreamModelBindingStoreImportRollsBackOnFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("open model binding rollback test pool: %v", err)
	}
	defer func() { _ = cleanup(ctx) }()
	seedBindingTargets(t, ctx, pool)

	store := NewUpstreamModelBindingStore(pool)
	bad := importBindingWrite("bad\x00model")
	scope := domain.UpstreamModelBindingScope{Kind: domain.UpstreamKindDirect, ID: directBindingScopeID}
	if _, err := store.Import(ctx, scope, []domain.UpstreamModelBindingWrite{
		importBindingWrite("must-roll-back"),
		bad,
	}); err == nil {
		t.Fatal("Import error = nil, want failure")
	}
	codes, err := store.ListModelCodes(ctx, domain.UpstreamModelBindingScope{Kind: domain.UpstreamKindDirect, ID: directBindingScopeID})
	if err != nil {
		t.Fatalf("ListModelCodes: %v", err)
	}
	if len(codes) != 0 {
		t.Fatalf("codes after rollback = %v, want empty", codes)
	}
}

func TestUpstreamModelBindingStoreRejectsCapabilitiesUnsupportedByActiveTarget(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("open model binding validation test pool: %v", err)
	}
	defer func() { _ = cleanup(ctx) }()
	seedBindingTargets(t, ctx, pool)

	store := NewUpstreamModelBindingStore(pool)
	imageWrite := domain.UpstreamModelBindingWrite{
		ModelCode:         "image-model",
		CapabilityType:    string(domain.CapabilityImage),
		UpstreamModelName: "image-model",
		Status:            "active",
	}
	if _, err := store.Create(ctx, domain.UpstreamModelBindingScope{Kind: domain.UpstreamKindDirect, ID: directBindingScopeID}, imageWrite); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("Create(direct incompatible capability) error = %v, want validation", err)
	}
	if _, err := store.Create(ctx, domain.UpstreamModelBindingScope{Kind: domain.UpstreamKindPool, ID: poolBindingScopeID}, imageWrite); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("Create(pool incompatible capability) error = %v, want validation", err)
	}
}

func importBindingWrite(modelCode string) domain.UpstreamModelBindingWrite {
	return domain.UpstreamModelBindingWrite{
		ModelCode:         modelCode,
		CapabilityType:    "chat",
		UpstreamModelName: modelCode,
		Status:            "active",
	}
}

func seedBindingTargets(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_upstream_accounts (id, name, tenant_display_name, api_key_ciphertext, status)
		VALUES ($1::uuid, 'binding-direct', 'Binding Direct', 'cipher', 'active')
	`, directBindingScopeID); err != nil {
		t.Fatalf("seed direct model binding target: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_upstream_account_endpoints (account_id, api_format, base_url, status)
		VALUES
			($1::uuid, 'openai_responses', 'https://example.test', 'active'),
			($1::uuid, 'openai_embeddings', 'https://example.test', 'active')
	`, directBindingScopeID); err != nil {
		t.Fatalf("seed direct model binding endpoints: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_credential_pools (id, name, tenant_display_name, fixed_provider_type, status)
		VALUES ($1::uuid, 'binding-pool', 'Binding Pool', 'codex', 'active')
	`, poolBindingScopeID); err != nil {
		t.Fatalf("seed pool model binding target: %v", err)
	}
}
