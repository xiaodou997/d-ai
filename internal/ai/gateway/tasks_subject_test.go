package gateway

import (
	"context"
	"testing"

	"xiaodou/dai/internal/ai/application"
	"xiaodou/dai/internal/ai/asynctask"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	coreruntime "xiaodou/dai/internal/ai/core/runtime"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/testsupport"
)

type taskInvokeExpanderStub struct {
	expansion coreruntime.InvokeExpansion
	err       error
	scope     coreidentity.Scope
	tenantID  string
	userID    string
	keyID     string
}

func (s *taskInvokeExpanderStub) ExpandByKeyID(
	_ context.Context,
	scope coreidentity.Scope,
	tenantID, userID, keyID string,
	_ coreruntime.Request,
) (coreruntime.InvokeExpansion, error) {
	s.scope, s.tenantID, s.userID, s.keyID = scope, tenantID, userID, keyID
	return s.expansion, s.err
}

func TestTaskSubjectResolverReloadsAPIKeyAuthorization(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("async task test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	const (
		keyID    = "44444444-4444-4444-4444-444444444444"
		groupID  = "55555555-5555-5555-5555-555555555555"
		groupID2 = "55555555-5555-5555-5555-555555555556"
	)
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_groups (id, tenant_id, name, retail_price_book_id)
		VALUES
		  ($1::uuid, 'tenant-subject', 'subject group', '66666666-6666-6666-6666-666666666666'::uuid),
		  ($2::uuid, 'tenant-subject', 'subject group 2', '66666666-6666-6666-6666-666666666666'::uuid)
	`, groupID, groupID2); err != nil {
		t.Fatalf("seed group: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_api_keys (
			id, owner_type, tenant_id, group_id, key_hash, key_ciphertext, last_four, name,
			quota_limit, quota_used, status
		) VALUES (
			$1::uuid, 'tenant', 'tenant-subject', $2::uuid, 'subject-key-hash', '', 'hash', 'subject key',
			9000, 1200, 'active'
		)
	`, keyID, groupID); err != nil {
		t.Fatalf("seed API key: %v", err)
	}

	resolver := NewTaskSubjectResolver(dbgen.New(pool))
	ref := asynctask.SubjectRef{
		AuthMethod: coreidentity.AuthMethodAPIKey,
		TenantID:   "tenant-subject",
		APIKeyID:   keyID,
	}
	subject, err := resolver.Resolve(ctx, ref)
	if err != nil {
		t.Fatalf("Resolve active key: %v", err)
	}
	if subject.AuthMethod != coreidentity.AuthMethodAPIKey || subject.RequestSource != coreidentity.RequestSourceAPIKey {
		t.Fatalf("resolved auth = %+v", subject)
	}
	if subject.APIKeyID != keyID || subject.GroupID != groupID {
		t.Fatalf("resolved key/group = %+v", subject)
	}
	if subject.QuotaLimit == nil || *subject.QuotaLimit != 9000 || subject.QuotaUsed != 1200 {
		t.Fatalf("resolved quota = %+v", subject)
	}

	if _, err := pool.Exec(ctx, `UPDATE ai_api_keys SET group_id = $1::uuid WHERE id = $2::uuid`, groupID2, keyID); err != nil {
		t.Fatalf("change key group: %v", err)
	}
	updated, err := resolver.Resolve(ctx, ref)
	if err != nil || updated.GroupID != groupID2 {
		t.Fatalf("reloaded key group = %+v, %v", updated, err)
	}

	if _, err := pool.Exec(ctx, `UPDATE ai_api_keys SET status = 'disabled' WHERE id = $1::uuid`, keyID); err != nil {
		t.Fatalf("disable key: %v", err)
	}
	if _, err := resolver.Resolve(ctx, ref); err == nil {
		t.Fatal("disabled API key still resolved for queued work")
	}
}

func TestTaskSubjectResolverReloadsAppKeyAuthorization(t *testing.T) {
	expander := &taskInvokeExpanderStub{expansion: coreruntime.InvokeExpansion{
		Subject: coreidentity.Subject{
			AuthMethod: coreidentity.AuthMethodInvokeKey, Scope: coreidentity.ScopeUser,
			TenantID: "tenant-a", UserID: "user-a", InvokeKeyID: "invoke-1",
			ForcedGroupID: "group-image", AppID: "app-image",
		},
		InvokeKey: application.InvokeKey{ID: "invoke-1", Status: application.StatusActive},
		App: &application.RuntimeApp{App: application.App{
			ID: "app-image", AppType: application.AppTypeImageGenerationAgent,
			Status: application.StatusActive,
		}},
	}}
	resolver := NewTaskSubjectResolver(nil, expander)
	ref := asynctask.SubjectRef{
		AuthMethod: coreidentity.AuthMethodInvokeKey,
		TenantID:   "tenant-a", UserID: "user-a", InvokeKeyID: "invoke-1",
	}

	subject, err := resolver.Resolve(context.Background(), ref)
	if err != nil {
		t.Fatalf("Resolve active app key: %v", err)
	}
	if expander.scope != coreidentity.ScopeUser || expander.keyID != "invoke-1" {
		t.Fatalf("ID lookup = scope %q id %q", expander.scope, expander.keyID)
	}
	if subject.InvokeKeyID != "invoke-1" || subject.AppID != "app-image" || subject.ForcedGroupID != "group-image" {
		t.Fatalf("resolved app subject = %+v", subject)
	}

	expander.expansion.InvokeKey.Status = application.StatusDisabled
	if _, err := resolver.Resolve(context.Background(), ref); err == nil {
		t.Fatal("disabled app key still resolved for queued work")
	}
}
