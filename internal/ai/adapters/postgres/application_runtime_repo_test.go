package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/ai/application"
	"xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/testsupport"
)

func TestApplicationRuntimeRepoResolveAgentInvocation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool, fixture, cleanup, err := testsupport.OpenAppLayerTestPool(ctx)
	if err != nil {
		t.Skipf("open app layer test pool: %v", err)
	}
	defer func() { _ = cleanup(ctx) }()

	mustExecAppRuntime(t, ctx, pool, appKeysTestSchema)
	mustExecAppRuntime(t, ctx, pool, `
		INSERT INTO ai_app_keys (
		  id, owner_type, tenant_id, user_id, name, key_hash, key_ciphertext, last_four, status, app_id, expires_at
		) VALUES (
		  '30000000-0000-0000-0000-000000000002', 'user', 'tenant-a', 'user-a', 'ak-agent', 'hash-agent', 'ciphertext-agent', '5678', 'active', $1::uuid, $2
		)
	`, fixture.TenantAAgentID, time.Now().Add(time.Hour))

	repo := NewApplicationRuntimeRepo(pool)
	invocation, err := repo.ResolveRuntimeInvocationByKeyHash(ctx, "hash-agent")
	if err != nil {
		t.Fatalf("ResolveRuntimeInvocationByKeyHash: %v", err)
	}
	if invocation.App == nil {
		t.Fatal("expected runtime app")
	}
	if invocation.BoundModelID != "gpt-tenant-a" {
		t.Fatalf("bound model = %q", invocation.BoundModelID)
	}
	if invocation.App.App.AppType != application.AppTypeChatAgent {
		t.Fatalf("app type = %q", invocation.App.App.AppType)
	}
	if invocation.App.App.GroupID != "30000000-0000-0000-0000-000000000003" {
		t.Fatalf("app group id = %q", invocation.App.App.GroupID)
	}
	if len(invocation.App.PromptBindings) != 1 || invocation.App.PromptBindings[0].TemplateText == "" {
		t.Fatalf("prompt bindings = %#v", invocation.App.PromptBindings)
	}

	byID, err := repo.ResolveRuntimeInvocationByID(
		ctx, identity.ScopeUser, "tenant-a", "user-a", "30000000-0000-0000-0000-000000000002",
	)
	if err != nil {
		t.Fatalf("ResolveRuntimeInvocationByID: %v", err)
	}
	if byID.BoundModelID != invocation.BoundModelID || byID.App == nil || byID.App.App.AppType != invocation.App.App.AppType {
		t.Fatalf("ID expansion = %#v, want same app as hash expansion %#v", byID, invocation)
	}
}

func TestApplicationRuntimeRepoRejectsInvisibleAgent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool, fixture, cleanup, err := testsupport.OpenAppLayerTestPool(ctx)
	if err != nil {
		t.Skipf("open app layer test pool: %v", err)
	}
	defer func() { _ = cleanup(ctx) }()

	mustExecAppRuntime(t, ctx, pool, appKeysTestSchema)
	mustExecAppRuntime(t, ctx, pool, `
		INSERT INTO ai_app_keys (
		  id, owner_type, tenant_id, user_id, name, key_hash, key_ciphertext, last_four, status, app_id
		) VALUES (
		  '30000000-0000-0000-0000-000000000003', 'tenant', 'tenant-a', '', 'ak-invisible', 'hash-invisible', 'ciphertext-invisible', '9999', 'active', $1::uuid
		)
	`, fixture.TenantBAgentID)

	repo := NewApplicationRuntimeRepo(pool)
	_, err = repo.ResolveRuntimeInvocationByKeyHash(ctx, "hash-invisible")
	if !errors.Is(err, application.ErrRuntimeAppNotVisible) {
		t.Fatalf("expected ErrRuntimeAppNotVisible, got %v", err)
	}
}

func mustExecAppRuntime(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("exec sql failed: %v", err)
	}
}
