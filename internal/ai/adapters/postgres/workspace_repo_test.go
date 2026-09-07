package postgres

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"

	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/testsupport"
	"xiaodou/dai/internal/ai/workspace"
)

func TestSanitizeWorkspaceImageAssetsDropsInlinePayloads(t *testing.T) {
	t.Parallel()

	items := []workspace.ImageAsset{
		{
			PreviewURL:  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO2lmw0AAAAASUVORK5CYII=",
			DisplayURL:  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO2lmw0AAAAASUVORK5CYII=",
			OriginalURL: "/runtime/v1/images/tasks/task-1/assets/0/original?key=test",
		},
	}

	got := sanitizeWorkspaceImageAssets(items)
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].PreviewURL != got[0].OriginalURL || got[0].DisplayURL != got[0].OriginalURL {
		t.Fatalf("unexpected sanitized asset: %+v", got[0])
	}
}

func TestWorkspaceChatCatalogQueryMatchesCanonicalSchema(t *testing.T) {
	pool, ctx := openWorkspaceRepoTestPool(t)
	repo := NewWorkspaceRepo(pool, nil, nil)

	_, err := repo.listWorkspaceAuthorizedChatModels(ctx, workspace.Owner{
		Scope:    identity.ScopeTenant,
		TenantID: "tenant-1",
	}, []string{"11111111-1111-1111-1111-111111111111"})
	if err != nil {
		t.Fatalf("list workspace chat models against canonical schema: %v", err)
	}
}

func TestWorkspaceGroupSelectionSnapshotMatchesCanonicalSchema(t *testing.T) {
	pool, ctx := openWorkspaceRepoTestPool(t)
	repo := NewWorkspaceRepo(pool, nil, nil)
	const groupID = "11111111-1111-1111-1111-111111111111"
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_groups (id, tenant_id, name, retail_price_book_id, default_user_multiplier, status)
		VALUES ($1::uuid, 'tenant-1', 'default', '22222222-2222-2222-2222-222222222222'::uuid, 1.25, 'active')
	`, groupID); err != nil {
		t.Fatalf("seed workspace group: %v", err)
	}

	snapshot, err := repo.getWorkspaceGroupSelectionSnapshot(ctx, workspace.Owner{
		Scope:    identity.ScopeTenant,
		TenantID: "tenant-1",
	}, groupID)
	if err != nil {
		t.Fatalf("get workspace group snapshot against canonical schema: %v", err)
	}
	if snapshot.Name != "default" || snapshot.EffectiveUserMultiplier != 1.25 {
		t.Fatalf("unexpected group snapshot: %+v", snapshot)
	}
}

// openWorkspaceRepoTestPool binds the workspace tests to the canonical schema
// loaded from db/init.sql. Hand-copied TEMP tables previously stood in for the
// real ones and silently drifted from them, so repository tests could pass while
// production queries no longer matched the canonical schema.
func openWorkspaceRepoTestPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("workspace repository test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	return pool, ctx
}

func TestWorkspaceRepoDeleteChatSessionPhysicallyDeletesOwnedMessages(t *testing.T) {
	pool, ctx := openWorkspaceRepoTestPool(t)
	repo := NewWorkspaceRepo(pool, nil, nil)
	const (
		ownedSessionID = "11111111-1111-1111-1111-111111111111"
		otherSessionID = "22222222-2222-2222-2222-222222222222"
	)
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_workspace_threads (id, owner_scope, tenant_id, user_id, target_model_code) VALUES
			($1::uuid, 'user', 'tenant-1', 'user-1', 'gpt-4o-mini'),
			($2::uuid, 'user', 'tenant-1', 'user-2', 'gpt-4o-mini')
	`, ownedSessionID, otherSessionID); err != nil {
		t.Fatalf("seed workspace threads: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_workspace_messages (id, thread_id, role) VALUES
			('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa', $1::uuid, 'user'),
			('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb', $2::uuid, 'user')
	`, ownedSessionID, otherSessionID); err != nil {
		t.Fatalf("seed workspace messages: %v", err)
	}

	owner := workspace.Owner{Scope: identity.ScopeUser, TenantID: "tenant-1", UserID: "user-1"}
	if err := repo.DeleteChatSession(ctx, owner, ownedSessionID); err != nil {
		t.Fatalf("DeleteChatSession() error = %v", err)
	}
	assertWorkspaceRowCount(t, ctx, pool, "ai_workspace_threads", ownedSessionID, 0)
	assertWorkspaceRowCount(t, ctx, pool, "ai_workspace_messages", ownedSessionID, 0)
	assertWorkspaceRowCount(t, ctx, pool, "ai_workspace_threads", otherSessionID, 1)
	assertWorkspaceRowCount(t, ctx, pool, "ai_workspace_messages", otherSessionID, 1)

	if err := repo.DeleteChatSession(ctx, owner, ownedSessionID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second DeleteChatSession() error = %v, want not found", err)
	}
}

func assertWorkspaceRowCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table, sessionID string, want int) {
	t.Helper()
	column := "id"
	if table == "ai_workspace_messages" {
		column = "thread_id"
	}
	var got int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM "+table+" WHERE "+column+" = $1::uuid", sessionID).Scan(&got); err != nil {
		t.Fatalf("count %s rows: %v", table, err)
	}
	if got != want {
		t.Fatalf("%s rows for session %s = %d, want %d", table, sessionID, got, want)
	}
}

func TestWorkspaceChatCatalogBatchPreservesGroupProtocolIsolation(t *testing.T) {
	pool, ctx := openWorkspaceRepoTestPool(t)
	book := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO ai_price_book_entries
 (price_book_id, model_code, capability_type, token_price_tiers)
 VALUES ($1::uuid, 'shared-model', 'chat', '[{"input_per_token":0.01,"output_per_token":0.02}]')`, book); err != nil {
		t.Fatal(err)
	}
	expected := map[string]string{}
	protocols := []string{string(domain.ProtocolOpenAIChat), string(domain.ProtocolAnthropicMessages), string(domain.ProtocolGeminiGenerate)}
	for i, protocol := range protocols {
		group, account := uuid.NewString(), uuid.NewString()
		tenant := "tenant-1"
		if i == 2 {
			tenant = "tenant-2"
		} else {
			expected[group] = protocol
		}
		statements := []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO ai_groups (id, tenant_id, name, retail_price_book_id, allow_protocol_conversion) VALUES ($1::uuid, $2, $3, $4::uuid, false)`, []any{group, tenant, fmt.Sprint(i), book}},
			{`INSERT INTO ai_upstream_accounts (id, name, tenant_display_name, api_key_ciphertext, tenant_access_mode, price_book_id, status) VALUES ($1::uuid, $2, $2, 'x', 'public', $3::uuid, 'active')`, []any{account, fmt.Sprint(i), book}},
			{`INSERT INTO ai_upstream_account_endpoints (account_id, api_format, base_url) VALUES ($1::uuid, $2, 'https://example.test')`, []any{account, protocol}},
			{`INSERT INTO ai_group_targets (group_id, target_kind, target_id) VALUES ($1::uuid, 'direct_upstream', $2::uuid)`, []any{group, account}},
			{`INSERT INTO ai_upstream_models (upstream_kind, upstream_id, model_code, capability_type, upstream_model_name) VALUES ('direct_upstream', $1::uuid, 'shared-model', 'chat', 'upstream-model')`, []any{account}},
		}
		for _, statement := range statements {
			if _, err := pool.Exec(ctx, statement.sql, statement.args...); err != nil {
				t.Fatal(err)
			}
		}
	}
	repo := NewWorkspaceRepo(pool, NewGroupAccessReader(pool), NewRouteInspector(pool))
	models, err := NewWorkspaceChatCatalog(repo).ListChatModels(ctx, workspace.Owner{Scope: identity.ScopeTenant, TenantID: "tenant-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != len(expected) {
		t.Fatalf("models = %+v", models)
	}
	for _, model := range models {
		protocol, ok := expected[model.GroupID]
		if !ok || len(model.AvailableProtocols) != 1 || model.AvailableProtocols[0] != protocol || model.DefaultProtocol != protocol {
			t.Fatalf("protocols crossed group boundary: %+v", model)
		}
	}
}
