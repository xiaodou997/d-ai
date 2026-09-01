package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	commercial "xiaodou/dai/internal/ai/commercial"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/testsupport"
)

func openCommercialGroupTestPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()

	ctx := context.Background()
	// MaxConns 必须为 1：下面用的是 TEMP 表，只在建表的那个会话里可见。
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 1})
	if err != nil {
		t.Skipf("commercial group target test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	if _, err := pool.Exec(ctx, `
		CREATE TEMP TABLE ai_groups (
			id UUID PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'tenant-1',
			route_policy_version BIGINT NOT NULL DEFAULT 1,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
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
		{`INSERT INTO ai_group_targets (id, group_id, target_kind, target_id, status)
		  VALUES ($1::uuid, $2::uuid, 'direct_upstream', $3::uuid, 'active')`, []any{routeID, groupID, targetID}},
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
	if got := data.Targets[groupID]; len(got) != 1 || got[0].ID != routeID || got[0].TargetID != targetID {
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
				TargetKind: tc.kind, TargetID: tc.id, Status: commercial.StatusActive,
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
		Status: commercial.StatusActive,
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

func TestCommercialRepoReplaceGroupTargetsIsAtomicAndVersioned(t *testing.T) {
	pool, ctx := openCommercialGroupTestPool(t)
	repo := NewCommercialRepo(dbgen.New(pool), pool)

	const (
		groupID   = "99999999-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		accountID = "99999999-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
		poolID    = "99999999-cccc-cccc-cccc-cccccccccccc"
	)
	if _, err := pool.Exec(ctx, `INSERT INTO ai_groups (id, tenant_id, route_policy_version) VALUES ($1::uuid, 'tenant-1', 3)`, groupID); err != nil {
		t.Fatalf("seed group: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO ai_upstream_accounts (id, name, tenant_display_name, tenant_access_mode, status) VALUES ($1::uuid, 'account', 'Account', 'public', 'active')`, accountID); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO ai_credential_pools (id, name, tenant_display_name, tenant_access_mode, status) VALUES ($1::uuid, 'pool', 'Pool', 'public', 'active')`, poolID); err != nil {
		t.Fatalf("seed pool: %v", err)
	}
	scope := commercial.TenantGroupScope{TenantID: "tenant-1", GroupID: groupID}
	first, err := repo.ReplaceGroupTargets(ctx, scope, commercial.GroupTargetBatchWrite{
		ExpectedVersion: 3,
		Targets: []commercial.GroupTargetWrite{
			{TargetKind: commercial.TargetKindDirectUpstream, TargetID: accountID, Status: commercial.StatusActive},
			{TargetKind: commercial.TargetKindOAuthPool, TargetID: poolID, Status: commercial.StatusDisabled},
		},
	})
	if err != nil {
		t.Fatalf("first replacement: %v", err)
	}
	if first.RoutePolicyVersion != 4 || len(first.Targets) != 2 {
		t.Fatalf("first replacement = version %d targets %d, want 4/2", first.RoutePolicyVersion, len(first.Targets))
	}

	_, err = repo.ReplaceGroupTargets(ctx, scope, commercial.GroupTargetBatchWrite{
		ExpectedVersion: 3,
		Targets:         []commercial.GroupTargetWrite{{TargetKind: commercial.TargetKindDirectUpstream, TargetID: accountID, Status: commercial.StatusActive}},
	})
	var conflict *domain.GroupRoutePolicyConflictError
	if !errors.As(err, &conflict) || conflict.ActualVersion != 4 {
		t.Fatalf("stale replacement error = %v, conflict = %#v", err, conflict)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM ai_group_targets WHERE group_id = $1::uuid`, groupID).Scan(&count); err != nil {
		t.Fatalf("count after stale replacement: %v", err)
	}
	if count != 2 {
		t.Fatalf("stale replacement changed targets: count = %d, want 2", count)
	}

	second, err := repo.ReplaceGroupTargets(ctx, scope, commercial.GroupTargetBatchWrite{
		ExpectedVersion: 4,
		Targets:         []commercial.GroupTargetWrite{{TargetKind: commercial.TargetKindOAuthPool, TargetID: poolID, Status: commercial.StatusActive}},
	})
	if err != nil {
		t.Fatalf("second replacement: %v", err)
	}
	if second.RoutePolicyVersion != 5 || len(second.Targets) != 1 || second.Targets[0].TargetID != poolID {
		t.Fatalf("second replacement = %#v, want version 5 and pool target", second)
	}
	unchanged, err := repo.ReplaceGroupTargets(ctx, scope, commercial.GroupTargetBatchWrite{
		ExpectedVersion: 5,
		Targets:         []commercial.GroupTargetWrite{{TargetKind: commercial.TargetKindOAuthPool, TargetID: poolID, Status: commercial.StatusActive}},
	})
	if err != nil {
		t.Fatalf("unchanged replacement: %v", err)
	}
	if unchanged.RoutePolicyVersion != 5 || len(unchanged.Targets) != 1 {
		t.Fatalf("unchanged replacement = version %d targets %d, want 5/1", unchanged.RoutePolicyVersion, len(unchanged.Targets))
	}
}

func TestCommercialRepoUpdateGroupRoutePolicyIsVersioned(t *testing.T) {
	pool, ctx := openCommercialGroupTestPool(t)
	// The target-focused fixture keeps its group table minimal. Add the columns
	// returned by the generated group query for this narrow-policy contract test.
	if _, err := pool.Exec(ctx, `
		ALTER TABLE ai_groups
			ADD COLUMN name TEXT NOT NULL DEFAULT 'group',
			ADD COLUMN description TEXT NOT NULL DEFAULT '',
			ADD COLUMN retail_price_book_id UUID,
			ADD COLUMN default_user_multiplier NUMERIC NOT NULL DEFAULT 1,
			ADD COLUMN user_default_visible BOOLEAN NOT NULL DEFAULT false,
			ADD COLUMN allow_protocol_conversion BOOLEAN NOT NULL DEFAULT false,
			ADD COLUMN route_policy TEXT NOT NULL DEFAULT 'balanced',
			ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0,
			ADD COLUMN status TEXT NOT NULL DEFAULT 'active',
			ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	`); err != nil {
		t.Fatalf("extend group fixture: %v", err)
	}
	repo := NewCommercialRepo(dbgen.New(pool), pool)
	const groupID = "99999999-dddd-dddd-dddd-dddddddddddd"
	if _, err := pool.Exec(ctx, `INSERT INTO ai_groups (id, tenant_id, route_policy_version) VALUES ($1::uuid, 'tenant-1', 7)`, groupID); err != nil {
		t.Fatalf("seed route policy group: %v", err)
	}
	scope := commercial.TenantGroupScope{TenantID: "tenant-1", GroupID: groupID}
	updated, err := repo.UpdateGroupRoutePolicy(ctx, scope, commercial.GroupRoutePolicyWrite{
		ExpectedVersion: 7,
		RoutePolicy:     commercial.RoutePolicyCost,
	})
	if err != nil {
		t.Fatalf("route policy update: %v", err)
	}
	if updated.RoutePolicyVersion != 8 || updated.RoutePolicy != commercial.RoutePolicyCost {
		t.Fatalf("updated group = version %d policy %q, want 8/cost", updated.RoutePolicyVersion, updated.RoutePolicy)
	}

	_, err = repo.UpdateGroupRoutePolicy(ctx, scope, commercial.GroupRoutePolicyWrite{
		ExpectedVersion: 7,
		RoutePolicy:     commercial.RoutePolicyLatency,
	})
	var conflict *domain.GroupRoutePolicyConflictError
	if !errors.As(err, &conflict) || conflict.ExpectedVersion != 7 || conflict.ActualVersion != 8 {
		t.Fatalf("stale route policy update = %v, conflict = %#v", err, conflict)
	}
	var policy string
	var version int64
	if err := pool.QueryRow(ctx, `SELECT route_policy, route_policy_version FROM ai_groups WHERE id = $1::uuid`, groupID).Scan(&policy, &version); err != nil {
		t.Fatalf("read route policy group: %v", err)
	}
	if policy != string(commercial.RoutePolicyCost) || version != 8 {
		t.Fatalf("stale route policy changed group = %q/%d, want cost/8", policy, version)
	}
}

func TestGroupTargetBindingToCommercial(t *testing.T) {
	now := time.Unix(1710000000, 0)
	item := domain.GroupTargetBinding{
		ID:         "target-1",
		GroupID:    "group-1",
		TargetKind: string(commercial.TargetKindOAuthPool),
		TargetID:   "pool-1",
		Status:     "disabled",
		CreatedAt:  now,
		UpdatedAt:  now.Add(time.Minute),
	}

	got := groupTargetBindingToCommercial(item)
	if got.TargetKind != commercial.TargetKindOAuthPool || got.TargetID != "pool-1" {
		t.Fatalf("target identity mismatch: %+v", got)
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
		INSERT INTO ai_group_targets (id, group_id, target_kind, target_id, status)
		VALUES ($1::uuid, $2::uuid, 'direct_upstream', $3::uuid, 'active')
	`, bindingID, groupID, accountID); err != nil {
		t.Fatalf("seed group target: %v", err)
	}

	got, err := repo.UpdateGroupTarget(ctx, commercial.TenantGroupScope{TenantID: "tenant-1", GroupID: groupID}, bindingID, commercial.GroupTargetWrite{
		Status: commercial.StatusActive,
	})
	if err != nil {
		t.Fatalf("UpdateGroupTarget() error = %v", err)
	}
	if got.ID != bindingID || got.GroupID != groupID || got.TargetID != accountID {
		t.Fatalf("updated target identity = %+v", got)
	}
	if got.Status != commercial.StatusActive {
		t.Fatalf("updated target = %+v", got)
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
