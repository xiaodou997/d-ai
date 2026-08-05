package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	commercial "xiaodou/dai/internal/ai/commercial"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
)

func openCommercialGroupTestPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()

	dsn := os.Getenv("AI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set AI_TEST_DATABASE_URL to run commercial group target DB tests")
	}
	ctx := context.Background()
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse test database URL: %v", err)
	}
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping test database: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		CREATE TEMP TABLE ai_groups (
			id UUID PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'tenant-1'
		);
		CREATE TEMP TABLE ai_group_client_surfaces (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			group_id UUID NOT NULL,
			surface TEXT NOT NULL,
			bridge_enabled BOOLEAN NOT NULL DEFAULT false,
			status TEXT NOT NULL DEFAULT 'active',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE (group_id, surface)
		);
		CREATE TEMP TABLE ai_group_targets (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			group_id UUID NOT NULL,
			target_kind TEXT NOT NULL,
			target_id UUID NOT NULL,
			priority INTEGER NOT NULL DEFAULT 100,
			status TEXT NOT NULL DEFAULT 'active',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TEMP TABLE ai_group_model_dispatch_rules (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			group_id UUID NOT NULL,
			client_surface TEXT NOT NULL,
			match_type TEXT NOT NULL,
			match_value TEXT NOT NULL,
			target_model_code TEXT NOT NULL,
			priority INTEGER NOT NULL DEFAULT 100,
			status TEXT NOT NULL DEFAULT 'active',
			notes TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TEMP TABLE ai_upstream_accounts (
			id UUID PRIMARY KEY,
			name TEXT NOT NULL,
			tenant_display_name TEXT NOT NULL DEFAULT '',
			tenant_access_mode TEXT NOT NULL DEFAULT 'public',
			base_url TEXT NOT NULL DEFAULT '',
			default_protocol TEXT NOT NULL DEFAULT '',
			tenant_multiplier NUMERIC,
			status TEXT NOT NULL DEFAULT 'active'
		);
		CREATE TEMP TABLE ai_credential_pools (
			id UUID PRIMARY KEY,
			name TEXT NOT NULL,
			tenant_display_name TEXT NOT NULL DEFAULT '',
			tenant_access_mode TEXT NOT NULL DEFAULT 'public',
			fixed_provider_type TEXT NOT NULL DEFAULT '',
			tenant_multiplier NUMERIC,
			status TEXT NOT NULL DEFAULT 'active'
		);
		CREATE TEMP VIEW ai_upstream_resources AS
		  SELECT id, 'direct_upstream'::text AS resource_kind, name, tenant_display_name, tenant_access_mode, tenant_multiplier, status
		  FROM ai_upstream_accounts
		  UNION ALL
		  SELECT id, 'oauth_pool'::text AS resource_kind, name, tenant_display_name, tenant_access_mode, tenant_multiplier, status
		  FROM ai_credential_pools;
		CREATE TEMP TABLE ai_upstream_resource_tenant_policies (
			resource_kind TEXT NOT NULL,
			resource_id UUID NOT NULL,
			tenant_id TEXT NOT NULL,
			access_granted BOOLEAN NOT NULL DEFAULT false,
			tenant_multiplier_override NUMERIC,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (resource_kind, resource_id, tenant_id)
		);
		CREATE TEMP TABLE ai_route_score_weights (
			scope TEXT PRIMARY KEY,
			weights JSONB NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
	`); err != nil {
		t.Fatalf("create group target fixtures: %v", err)
	}
	return pool, ctx
}

func TestCommercialRepoLoadDispatchDataUsesOneBatchSnapshot(t *testing.T) {
	pool, ctx := openCommercialGroupTestPool(t)
	repo := NewCommercialRepo(dbgen.New(pool), pool)

	const (
		groupID  = "55555555-5555-5555-5555-555555555555"
		targetID = "66666666-6666-6666-6666-666666666666"
		routeID  = "77777777-7777-7777-7777-777777777777"
	)
	fixtures := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO ai_groups (id) VALUES ($1::uuid)`, []any{groupID}},
		{`INSERT INTO ai_group_client_surfaces (group_id, surface, bridge_enabled, status)
		  VALUES ($1::uuid, 'openai_chat', true, 'active')`, []any{groupID}},
		{`INSERT INTO ai_group_model_dispatch_rules
		    (group_id, client_surface, match_type, match_value, target_model_code, priority, status)
		  VALUES ($1::uuid, 'openai_chat', 'exact', 'public-model', 'upstream-model', 3, 'active')`, []any{groupID}},
		{`INSERT INTO ai_upstream_accounts (id, name, tenant_display_name) VALUES ($1::uuid, 'internal', 'Public Upstream')`, []any{targetID}},
		{`INSERT INTO ai_group_targets (id, group_id, target_kind, target_id, priority, status)
		  VALUES ($1::uuid, $2::uuid, 'direct_upstream', $3::uuid, 5, 'active')`, []any{routeID, groupID, targetID}},
	}
	for _, fixture := range fixtures {
		if _, err := pool.Exec(ctx, fixture.query, fixture.args...); err != nil {
			t.Fatalf("seed dispatch snapshot: %v", err)
		}
	}

	data, err := repo.LoadDispatchData(ctx, "tenant-1", []string{groupID})
	if err != nil {
		t.Fatalf("LoadDispatchData() error = %v", err)
	}
	if got := data.ClientSurfaces[groupID]; len(got) != 1 || got[0].Surface != "openai_chat" || !got[0].BridgeEnabled {
		t.Fatalf("client surfaces = %#v", got)
	}
	if got := data.Rules[groupID]; len(got) != 1 || got[0].TargetModelID != "upstream-model" || got[0].Priority != 3 {
		t.Fatalf("dispatch rules = %#v", got)
	}
	if got := data.Targets[groupID]; len(got) != 1 || got[0].ID != routeID || got[0].TargetID != targetID || got[0].Priority != 5 {
		t.Fatalf("group targets = %#v", got)
	}
}

func TestCommercialRepoAddGroupTargetRejectsDisabledTargets(t *testing.T) {
	pool, ctx := openCommercialGroupTestPool(t)
	repo := NewCommercialRepo(dbgen.New(pool), pool)

	const (
		groupID   = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		accountID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
		poolID    = "cccccccc-cccc-cccc-cccc-cccccccccccc"
	)
	if _, err := pool.Exec(ctx, `INSERT INTO ai_groups (id) VALUES ($1::uuid)`, groupID); err != nil {
		t.Fatalf("seed group: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO ai_upstream_accounts (id, name, status) VALUES ($1::uuid, 'disabled account', 'disabled')`, accountID); err != nil {
		t.Fatalf("seed disabled account: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO ai_credential_pools (id, name, status) VALUES ($1::uuid, 'disabled pool', 'disabled')`, poolID); err != nil {
		t.Fatalf("seed disabled pool: %v", err)
	}

	for _, tc := range []struct {
		name string
		kind commercial.TargetKind
		id   string
	}{
		{name: "direct upstream", kind: commercial.TargetKindDirectUpstream, id: accountID},
		{name: "oauth pool", kind: commercial.TargetKindOAuthPool, id: poolID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := repo.AddGroupTarget(ctx, commercial.TenantGroupScope{TenantID: "tenant-1", GroupID: groupID}, commercial.GroupTargetWrite{
				TargetKind: tc.kind, TargetID: tc.id,
				Priority: 100, Status: commercial.StatusActive,
			})
			if !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("AddGroupTarget() error = %v, want validation error", err)
			}
		})
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM ai_group_targets WHERE group_id = $1::uuid`, groupID).Scan(&count); err != nil {
		t.Fatalf("count group targets: %v", err)
	}
	if count != 0 {
		t.Fatalf("group target count = %d, want 0", count)
	}
}

func TestCommercialRepoAddGroupTargetRequiresRestrictedGrant(t *testing.T) {
	pool, ctx := openCommercialGroupTestPool(t)
	repo := NewCommercialRepo(dbgen.New(pool), pool)

	const (
		groupID   = "11111111-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		accountID = "22222222-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	)
	if _, err := pool.Exec(ctx, `INSERT INTO ai_groups (id, tenant_id) VALUES ($1::uuid, 'tenant-1')`, groupID); err != nil {
		t.Fatalf("seed group: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_upstream_accounts (id, name, tenant_display_name, tenant_access_mode)
		VALUES ($1::uuid, 'restricted internal', 'Restricted', 'restricted')
	`, accountID); err != nil {
		t.Fatalf("seed restricted account: %v", err)
	}

	write := commercial.GroupTargetWrite{
		TargetKind: commercial.TargetKindDirectUpstream, TargetID: accountID,
		Priority: 100, Status: commercial.StatusActive,
	}
	if _, err := repo.AddGroupTarget(ctx, commercial.TenantGroupScope{TenantID: "tenant-1", GroupID: groupID}, write); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("AddGroupTarget() without grant error = %v, want validation error", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_upstream_resource_tenant_policies (resource_kind, resource_id, tenant_id, access_granted)
		VALUES ('direct_upstream', $1::uuid, 'tenant-1', true)
	`, accountID); err != nil {
		t.Fatalf("seed tenant grant: %v", err)
	}
	if _, err := repo.AddGroupTarget(ctx, commercial.TenantGroupScope{TenantID: "tenant-1", GroupID: groupID}, write); err != nil {
		t.Fatalf("AddGroupTarget() with grant: %v", err)
	}
}

func TestGroupTargetBindingToCommercial(t *testing.T) {
	now := time.Unix(1710000000, 0)
	item := domain.GroupTargetBinding{
		ID:         "target-1",
		GroupID:    "group-1",
		TargetKind: string(commercial.TargetKindOAuthPool),
		TargetID:   "pool-1",
		Priority:   7,
		Status:     "disabled",
		CreatedAt:  now,
		UpdatedAt:  now.Add(time.Minute),
	}

	got := groupTargetBindingToCommercial(item)
	if got.TargetKind != commercial.TargetKindOAuthPool || got.TargetID != "pool-1" {
		t.Fatalf("target identity mismatch: %+v", got)
	}
	if got.Priority != 7 {
		t.Fatalf("target routing fields mismatch: %+v", got)
	}
	if got.Status != commercial.StatusDisabled || !got.CreatedAt.Equal(now) || !got.UpdatedAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("target metadata mismatch: %+v", got)
	}
}

func TestGroupTargetDetailToCommercial(t *testing.T) {
	item := domain.GroupTargetDetail{
		GroupTargetBinding: domain.GroupTargetBinding{
			ID:         "target-2",
			GroupID:    "group-2",
			TargetKind: string(commercial.TargetKindDirectUpstream),
			TargetID:   "account-2",
			Priority:   3,
			Status:     "active",
		},
		AccountName:       "OpenAI Primary",
		DefaultProtocol:   "openai_compatible",
		PoolName:          "",
		FixedProviderType: "",
	}

	got := groupTargetDetailToCommercial(item)
	if got.TargetKind != commercial.TargetKindDirectUpstream || got.TargetID != "account-2" {
		t.Fatalf("group target mismatch: %+v", got.GroupTarget)
	}
	if got.AccountName != "OpenAI Primary" || got.DefaultProtocol != "openai_compatible" {
		t.Fatalf("display fields mismatch: %+v", got)
	}
}

func TestCommercialRepoUpdateGroupTargetUsesCurrentSchema(t *testing.T) {
	pool, ctx := openCommercialGroupTestPool(t)
	repo := NewCommercialRepo(dbgen.New(pool), pool)

	const (
		groupID   = "11111111-1111-1111-1111-111111111111"
		bindingID = "22222222-2222-2222-2222-222222222222"
		accountID = "33333333-3333-3333-3333-333333333333"
	)
	if _, err := pool.Exec(ctx, `INSERT INTO ai_groups (id) VALUES ($1::uuid)`, groupID); err != nil {
		t.Fatalf("seed group: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_upstream_accounts (id, name)
		VALUES ($1::uuid, 'primary account')
	`, accountID); err != nil {
		t.Fatalf("seed upstream account: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_group_targets (id, group_id, target_kind, target_id, priority, status)
		VALUES ($1::uuid, $2::uuid, 'direct_upstream', $3::uuid, 100, 'active')
	`, bindingID, groupID, accountID); err != nil {
		t.Fatalf("seed group target: %v", err)
	}

	got, err := repo.UpdateGroupTarget(ctx, commercial.TenantGroupScope{TenantID: "tenant-1", GroupID: groupID}, bindingID, commercial.GroupTargetWrite{
		Priority: 7,
		Status:   commercial.StatusActive,
	})
	if err != nil {
		t.Fatalf("UpdateGroupTarget() error = %v", err)
	}
	if got.ID != bindingID || got.GroupID != groupID || got.TargetID != accountID {
		t.Fatalf("updated target identity = %+v", got)
	}
	if got.Priority != 7 || got.Status != commercial.StatusActive {
		t.Fatalf("updated routing fields = %+v", got)
	}
}

func TestCommercialRepoReplaceGroupClientSurfaces(t *testing.T) {
	pool, ctx := openCommercialGroupTestPool(t)
	repo := NewCommercialRepo(dbgen.New(pool), pool)

	const groupID = "44444444-4444-4444-4444-444444444444"
	if _, err := pool.Exec(ctx, `INSERT INTO ai_groups (id) VALUES ($1::uuid)`, groupID); err != nil {
		t.Fatalf("seed group: %v", err)
	}
	if err := repo.ReplaceGroupClientSurfaces(ctx, commercial.TenantGroupScope{TenantID: "tenant-1", GroupID: groupID}, []commercial.GroupClientSurfaceWrite{
		{Surface: "openai_images", BridgeEnabled: true, Status: commercial.StatusActive},
		{Surface: "openai_embeddings", BridgeEnabled: true, Status: commercial.StatusActive},
	}); err != nil {
		t.Fatalf("ReplaceGroupClientSurfaces(restricted) error = %v", err)
	}
	items, err := repo.ListGroupClientSurfaces(ctx, commercial.TenantGroupScope{TenantID: "tenant-1", GroupID: groupID})
	if err != nil {
		t.Fatalf("ListGroupClientSurfaces() error = %v", err)
	}
	if len(items) != 2 || items[0].Surface != "openai_embeddings" || items[1].Surface != "openai_images" {
		t.Fatalf("restricted surfaces = %#v", items)
	}

	if err := repo.ReplaceGroupClientSurfaces(ctx, commercial.TenantGroupScope{TenantID: "tenant-1", GroupID: groupID}, nil); err != nil {
		t.Fatalf("ReplaceGroupClientSurfaces(all) error = %v", err)
	}
	items, err = repo.ListGroupClientSurfaces(ctx, commercial.TenantGroupScope{TenantID: "tenant-1", GroupID: groupID})
	if err != nil {
		t.Fatalf("ListGroupClientSurfaces() after reset error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("all-mode surfaces = %#v, want empty", items)
	}
}
