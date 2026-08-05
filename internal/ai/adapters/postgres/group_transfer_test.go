package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/ai/commercial"
	"xiaodou/dai/internal/ai/core/surface"
	dbgen "xiaodou/dai/internal/ai/db/gen"
)

func openGroupTransferTestPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	dsn := os.Getenv("AI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set AI_TEST_DATABASE_URL to run group transfer DB tests")
	}
	ctx := context.Background()
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse test database URL: %v", err)
	}
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `
		CREATE TEMP TABLE ai_groups (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id TEXT NOT NULL,
				name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '',
				retail_price_book_id UUID NOT NULL, default_user_multiplier NUMERIC NOT NULL,
				user_default_visible BOOLEAN NOT NULL DEFAULT false,
				allow_protocol_conversion BOOLEAN NOT NULL,
			sort_order INTEGER NOT NULL, status TEXT NOT NULL,
			UNIQUE (tenant_id, name),
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TEMP TABLE ai_group_targets (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(), group_id UUID NOT NULL, target_kind TEXT NOT NULL,
			target_id UUID NOT NULL, priority INTEGER NOT NULL, status TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TEMP TABLE ai_group_client_surfaces (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(), group_id UUID NOT NULL, surface TEXT NOT NULL,
			bridge_enabled BOOLEAN NOT NULL, status TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE (group_id, surface)
		);
		CREATE TEMP TABLE ai_group_model_dispatch_rules (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(), group_id UUID NOT NULL, client_surface TEXT NOT NULL,
			match_type TEXT NOT NULL, match_value TEXT NOT NULL, target_model_code TEXT NOT NULL,
			priority INTEGER NOT NULL, status TEXT NOT NULL, notes TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TEMP TABLE ai_price_books (
			id UUID PRIMARY KEY, owner_type TEXT NOT NULL, owner_tenant_id TEXT NOT NULL DEFAULT '', status TEXT NOT NULL
		);
		CREATE TEMP TABLE ai_price_book_entries (price_book_id UUID NOT NULL, model_code TEXT NOT NULL, capability_type TEXT NOT NULL);
		CREATE TEMP TABLE ai_user_groups (group_id UUID NOT NULL);
		CREATE TEMP TABLE ai_api_keys (group_id UUID NOT NULL);
		CREATE TEMP TABLE ai_sub_plan_groups (group_id UUID NOT NULL);
	`); err != nil {
		t.Fatalf("create fixtures: %v", err)
	}
	return pool, ctx
}

func TestCommercialRepoApplyGroupImportReplacesConfigurationAndPreservesAssociations(t *testing.T) {
	pool, ctx := openGroupTransferTestPool(t)
	repo := NewCommercialRepo(dbgen.New(pool), pool)
	const (
		groupID     = "11111111-1111-1111-1111-111111111111"
		priceBookID = "22222222-2222-2222-2222-222222222222"
	)
	seeds := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO ai_price_books (id, owner_type, status) VALUES ($1::uuid, 'platform', 'active')`, []any{priceBookID}},
		{`INSERT INTO ai_price_book_entries (price_book_id, model_code, capability_type) VALUES ($1::uuid, 'gpt-5.5', 'chat')`, []any{priceBookID}},
		{`INSERT INTO ai_groups (id, tenant_id, name, retail_price_book_id, default_user_multiplier,
				user_default_visible, allow_protocol_conversion, sort_order, status)
			  VALUES ($1::uuid, 'tenant-1', '原分组', $2::uuid, 1, false, false, 0, 'active')`, []any{groupID, priceBookID}},
		{`INSERT INTO ai_group_targets (group_id, target_kind, target_id, priority, status)
		  VALUES ($1::uuid, 'direct_upstream', gen_random_uuid(), 100, 'active')`, []any{groupID}},
		{`INSERT INTO ai_group_client_surfaces (group_id, surface, bridge_enabled, status)
		  VALUES ($1::uuid, 'openai_chat', false, 'active')`, []any{groupID}},
		{`INSERT INTO ai_group_model_dispatch_rules (group_id, client_surface, match_type, match_value, target_model_code, priority, status, notes)
		  VALUES ($1::uuid, 'openai_chat', 'exact', 'old', 'gpt-4o', 100, 'active', 'old')`, []any{groupID}},
		{`INSERT INTO ai_user_groups (group_id) VALUES ($1::uuid)`, []any{groupID}},
		{`INSERT INTO ai_api_keys (group_id) VALUES ($1::uuid)`, []any{groupID}},
		{`INSERT INTO ai_sub_plan_groups (group_id) VALUES ($1::uuid)`, []any{groupID}},
	}
	for _, seed := range seeds {
		if _, err := pool.Exec(ctx, seed.sql, seed.args...); err != nil {
			t.Fatalf("seed fixtures: %v", err)
		}
	}
	snapshots, err := repo.SnapshotGroupConfigurations(ctx, "tenant-1", []string{groupID})
	if err != nil || len(snapshots) != 1 {
		t.Fatalf("SnapshotGroupConfigurations() = %#v, %v", snapshots, err)
	}

	source := commercial.GroupTransferGroup{
		Name:                    "更新分组",
		Description:             "imported",
		DefaultUserMultiplier:   1.5,
		UserDefaultVisible:      true,
		AllowProtocolConversion: true,
		SortOrder:               9,
		Status:                  commercial.StatusActive,
		ClientSurfacePolicy: commercial.GroupTransferClientSurfacePolicy{
			Mode:            commercial.GroupClientSurfacePolicyRestricted,
			AllowedSurfaces: []surface.ID{surface.OpenAIResponses},
		},
		DispatchRules: []commercial.GroupTransferDispatchRule{{
			ClientSurface: surface.OpenAIResponses, MatchType: commercial.DispatchMatchExact,
			MatchValue: "latest", TargetModelID: "gpt-5.5", Priority: 10, Status: commercial.StatusActive,
		}},
	}
	preview := commercial.GroupImportPreviewItem{
		SourceName: "更新分组", TargetName: "更新分组", Action: commercial.GroupImportActionUpdate,
		TargetGroupID: groupID, PriceBookID: priceBookID, AppliedStatus: commercial.StatusActive,
	}
	applied, err := repo.ApplyGroupImport(ctx, "tenant-1", commercial.PlannedGroupImport{
		Source:  source,
		Preview: preview,
	})
	if err != nil {
		t.Fatalf("ApplyGroupImport() error = %v", err)
	}
	if applied.GroupID != groupID || applied.Status != commercial.StatusActive {
		t.Fatalf("unexpected applied result: %+v", applied)
	}

	var name string
	var ruleCount, activeSurfaceCount int
	if err := pool.QueryRow(ctx, `SELECT name FROM ai_groups WHERE id = $1::uuid`, groupID).Scan(&name); err != nil {
		t.Fatalf("read group: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM ai_group_model_dispatch_rules WHERE group_id = $1::uuid AND match_value = 'latest'`, groupID).Scan(&ruleCount); err != nil {
		t.Fatalf("read rules: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM ai_group_client_surfaces WHERE group_id = $1::uuid AND status = 'active'`, groupID).Scan(&activeSurfaceCount); err != nil {
		t.Fatalf("read surfaces: %v", err)
	}
	if name != "更新分组" || ruleCount != 1 || activeSurfaceCount != 1 {
		t.Fatalf("configuration not replaced: name=%q rules=%d surfaces=%d", name, ruleCount, activeSurfaceCount)
	}
	for _, table := range []string{"ai_group_targets", "ai_user_groups", "ai_api_keys", "ai_sub_plan_groups"} {
		var count int
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM `+table+` WHERE group_id = $1::uuid`, groupID).Scan(&count); err != nil {
			t.Fatalf("read %s: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("%s association count = %d, want 1", table, count)
		}
	}
}
