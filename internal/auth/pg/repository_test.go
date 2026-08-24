package pg

import (
	"context"
	"errors"
	"testing"
	"time"

	authports "xiaodou/dai/internal/auth/ports"
	"xiaodou/dai/internal/dbtest"
)

func TestAuthRepositoryOwnsProtectedAccountProjectionAndMutations(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	now := time.Date(2026, time.August, 24, 9, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `INSERT INTO iam_tenants (tenant_id, tenant_name, status, created_at, updated_at) VALUES ('auth-repo-tenant', 'Auth Repository Tenant', 'active', $1, $1)`, now); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, email, user_type, status, mfa_enabled, created_at, updated_at)
		VALUES ('auth-repo-user', 'auth-repo-tenant', 'auth-repo-user', 'old-hash', 'old@example.com', 4, 'active', true, $1, $1)
	`, now); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, email, user_type, status, created_at, updated_at)
		VALUES ('auth-repo-other', 'auth-repo-tenant', 'auth-repo-other', 'hash', 'other@example.com', 4, 'active', $1, $1)
	`, now); err != nil {
		t.Fatalf("seed other account: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, email, user_type, status, created_at, updated_at)
		VALUES ('auth-repo-deleted', 'auth-repo-tenant', 'auth-repo-deleted', 'hash', 'deleted@example.com', 4, 'deleted', $1, $1)
	`, now); err != nil {
		t.Fatalf("seed deleted account: %v", err)
	}

	repo := NewAuthRepository(pool)
	snapshot, err := repo.GetCurrentUserSnapshot(ctx, "auth-repo-user", 4)
	if err != nil {
		t.Fatalf("GetCurrentUserSnapshot: %v", err)
	}
	if snapshot.UserID != "auth-repo-user" || snapshot.Username != "auth-repo-user" || snapshot.TenantID != "auth-repo-tenant" || snapshot.TenantName != "Auth Repository Tenant" || !snapshot.MFAEnabled || snapshot.Status != "active" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	hash, err := repo.GetPasswordHash(ctx, "auth-repo-user", 4)
	if err != nil || hash != "old-hash" {
		t.Fatalf("GetPasswordHash = %q err:%v", hash, err)
	}

	updated, err := repo.UpdateProfile(ctx, authports.ProfileUpdate{UserID: "auth-repo-user", UserType: 4, UsernameSet: true, Username: "auth-repo-user-updated", EmailSet: true, Email: "updated@example.com"})
	if err != nil || !updated {
		t.Fatalf("UpdateProfile = updated:%v err:%v", updated, err)
	}
	updated, err = repo.UpdateProfile(ctx, authports.ProfileUpdate{UserID: "auth-repo-user", UserType: 4, UsernameSet: true, Username: "auth-repo-other"})
	if err == nil || updated || !IsUsernameTaken(err) {
		t.Fatalf("duplicate username UpdateProfile = updated:%v err:%v", updated, err)
	}
	updated, err = repo.UpdateProfile(ctx, authports.ProfileUpdate{UserID: "auth-repo-user", UserType: 4, EmailSet: true, Email: "other@example.com"})
	if err == nil || updated || !IsEmailTaken(err) {
		t.Fatalf("duplicate email UpdateProfile = updated:%v err:%v", updated, err)
	}

	updated, err = repo.UpdatePassword(ctx, "auth-repo-user", 4, "new-hash")
	if err != nil || !updated {
		t.Fatalf("UpdatePassword = updated:%v err:%v", updated, err)
	}
	hash, err = repo.GetPasswordHash(ctx, "auth-repo-user", 4)
	if err != nil || hash != "new-hash" {
		t.Fatalf("updated password hash = %q err:%v", hash, err)
	}
	updated, err = repo.UpdatePassword(ctx, "auth-repo-deleted", 4, "should-not-write")
	if err != nil || updated {
		t.Fatalf("deleted UpdatePassword = updated:%v err:%v", updated, err)
	}
	if _, err := repo.GetPasswordHash(ctx, "auth-repo-deleted", 4); !errors.Is(err, authports.ErrAccountNotFound) {
		t.Fatalf("deleted GetPasswordHash = %v", err)
	}
	active, err := repo.CheckTenantActive(ctx, "auth-repo-tenant")
	if err != nil || !active {
		t.Fatalf("CheckTenantActive active = %v err:%v", active, err)
	}
}

func TestAuthRepositoryListsAuditLogsWithFiltersAndStablePagination(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	first := time.Date(2026, time.August, 24, 10, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `
		INSERT INTO auth_audit_logs
			(event_type, principal_type, user_id, decision, reason_code, reason_message, created_at)
		VALUES
			('user_login', 'user', 'audit-user-a', 'success', NULL, NULL, $1),
			('user_login', 'admin', 'audit-admin', 'deny', 'mfa_required', 'MFA required', $2),
			('token_refresh', 'user', 'audit-user-b', 'error', 'session_missing', 'Session missing', $3),
			('user_login', 'user', 'audit-user-c', 'deny', 'rate_limited', 'Too many attempts', $4)
	`, first, first.Add(time.Minute), first.Add(2*time.Minute), first.Add(3*time.Minute)); err != nil {
		t.Fatalf("seed auth audit logs: %v", err)
	}

	repo := NewAuthRepository(pool)
	page, err := repo.ListAuthAuditLogs(ctx, authports.AuthAuditLogFilter{Page: 1, Size: 2})
	if err != nil {
		t.Fatalf("list auth audit logs: %v", err)
	}
	if page.Total != 4 || page.Page != 1 || page.Size != 2 || len(page.Records) != 2 {
		t.Fatalf("audit page = %#v", page)
	}
	if page.Records[0].EventType != "user_login" || page.Records[0].UserID != "audit-user-c" || page.Records[0].ReasonCode != "rate_limited" {
		t.Fatalf("latest audit record = %#v", page.Records[0])
	}
	page, err = repo.ListAuthAuditLogs(ctx, authports.AuthAuditLogFilter{EventType: "user_login", Decision: "deny", Page: 1, Size: 20})
	if err != nil {
		t.Fatalf("list filtered auth audit logs: %v", err)
	}
	if page.Total != 2 || len(page.Records) != 2 || page.Records[0].UserID != "audit-user-c" || page.Records[1].UserID != "audit-admin" {
		t.Fatalf("filtered audit page = %#v", page)
	}
	page, err = repo.ListAuthAuditLogs(ctx, authports.AuthAuditLogFilter{Page: 0, Size: 0})
	if err != nil {
		t.Fatalf("list normalized auth audit logs: %v", err)
	}
	if page.Page != 1 || page.Size != 20 || len(page.Records) != 4 {
		t.Fatalf("normalized audit page = %#v", page)
	}
}
