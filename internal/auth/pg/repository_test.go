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
