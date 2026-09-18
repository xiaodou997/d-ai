package gateway

import (
	"context"
	"testing"

	"xiaodou/dai/internal/ai/asynctask"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/testsupport"
)

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
		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES ('tenant-subject', 'Subject Tenant', 'active')
	`); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
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

	resolver := NewTaskSubjectResolver(pool, dbgen.New(pool))
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
	if _, err := pool.Exec(ctx, `UPDATE ai_api_keys SET status = 'active' WHERE id = $1::uuid`, keyID); err != nil {
		t.Fatalf("re-enable key: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE iam_tenants SET status = 'suspended' WHERE tenant_id = 'tenant-subject'`); err != nil {
		t.Fatalf("suspend key owner tenant: %v", err)
	}
	if _, err := resolver.Resolve(ctx, ref); err == nil {
		t.Fatal("active API key resolved after its tenant owner became inactive")
	}
}

func TestTaskSubjectResolverRevalidatesJWTSessionAndScope(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("async task test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES
		  ('jwt-task-user-tenant', 'JWT Task User Tenant', 'active'),
		  ('jwt-task-ops-tenant', 'JWT Task Ops Tenant', 'active');
		INSERT INTO iam_accounts (
			user_id, tenant_id, username, password_hash, user_type, status, credential_version
		) VALUES
		  ('jwt-task-user', 'jwt-task-user-tenant', 'jwt-task-user', 'unused', 4, 'active', 1),
		  ('jwt-task-operator', NULL, 'jwt-task-operator', 'unused', 2, 'active', 1);
		INSERT INTO auth_sessions (
			session_id, user_id, credential_version, expires_at
		) VALUES
		  ('81000000-0000-0000-0000-000000000001'::uuid, 'jwt-task-user', 1, now() + interval '1 hour'),
		  ('81000000-0000-0000-0000-000000000002'::uuid, 'jwt-task-operator', 1, now() + interval '1 hour')
	`); err != nil {
		t.Fatal(err)
	}

	resolver := NewTaskSubjectResolver(pool, dbgen.New(pool))
	userRef := asynctask.SubjectRef{
		AuthMethod:           coreidentity.AuthMethodJWT,
		TenantID:             "jwt-task-user-tenant",
		UserID:               "jwt-task-user",
		JWTAuthUserID:        "jwt-task-user",
		JWTAuthUserType:      4,
		JWTSessionID:         "81000000-0000-0000-0000-000000000001",
		JWTCredentialVersion: 1,
	}
	userSubject, err := resolver.Resolve(ctx, userRef)
	if err != nil {
		t.Fatalf("resolve active user JWT task: %v", err)
	}
	if userSubject.Scope != coreidentity.ScopeUser || userSubject.UserID != "jwt-task-user" {
		t.Fatalf("resolved user JWT subject = %#v", userSubject)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE iam_accounts
		SET credential_version = credential_version + 1
		WHERE user_id = 'jwt-task-user'
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Resolve(ctx, userRef); err == nil {
		t.Fatal("queued user JWT task survived a credential change")
	}

	opsRef := asynctask.SubjectRef{
		AuthMethod:           coreidentity.AuthMethodJWT,
		TenantID:             "jwt-task-ops-tenant",
		JWTAuthUserID:        "jwt-task-operator",
		JWTAuthUserType:      2,
		JWTSessionID:         "81000000-0000-0000-0000-000000000002",
		JWTCredentialVersion: 1,
	}
	opsSubject, err := resolver.Resolve(ctx, opsRef)
	if err != nil {
		t.Fatalf("resolve active tenant-operations JWT task: %v", err)
	}
	if opsSubject.Scope != coreidentity.ScopeTenant || opsSubject.UserID != "" ||
		opsSubject.JWTAuthUserID != "jwt-task-operator" {
		t.Fatalf("resolved tenant-operations subject = %#v", opsSubject)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE iam_tenants SET status = 'disabled'
		WHERE tenant_id = 'jwt-task-ops-tenant'
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Resolve(ctx, opsRef); err == nil {
		t.Fatal("queued tenant-operations task survived target tenant disable")
	}

	if _, err := resolver.Resolve(ctx, asynctask.SubjectRef{
		AuthMethod: coreidentity.AuthMethodJWT,
		TenantID:   "jwt-task-ops-tenant",
	}); err == nil {
		t.Fatal("historical JWT task without session reference did not fail closed")
	}
}
