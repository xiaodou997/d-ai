package pg

import (
	"context"
	"testing"
	"time"

	"xiaodou/dai/internal/dbtest"
)

func TestAdminAccountRepositoryListsSystemAndTenantUsers(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	older := time.Date(2026, time.August, 23, 1, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	if _, err := pool.Exec(ctx, `INSERT INTO iam_tenants (tenant_id, tenant_name) VALUES ('tenant-list-a', 'Tenant A'), ('tenant-list-b', 'Tenant B')`); err != nil {
		t.Fatalf("seed tenants: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, username, password_hash, email, user_type, status, credential_state, created_at, updated_at)
		VALUES
			('admin-list-a', 'alice-admin', 'hash', 'alice@example.com', 2, 'active', 'active', $1, $1),
			('admin-list-b', 'bob-admin', 'hash', 'bob@example.com', 2, 'disabled', 'pending_activation', $2, $2)
	`, older, newer); err != nil {
		t.Fatalf("seed admin accounts: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, email, user_type, status, created_at, updated_at)
		VALUES
			('tenant-user-a1', 'tenant-list-a', 'tenant-a1', 'hash', 'a1@example.com', 3, 'active', $1, $1),
			('tenant-user-a2', 'tenant-list-a', 'tenant-a2', 'hash', 'a2@example.com', 3, 'inherited_disabled', $2, $2),
			('tenant-user-b1', 'tenant-list-b', 'tenant-b1', 'hash', 'b1@example.com', 3, 'active', $2, $2)
	`, older, newer); err != nil {
		t.Fatalf("seed admin accounts: %v", err)
	}

	repo := NewAdminAccountRepository(pool)
	admins, err := repo.ListSystemAdmins(ctx, "alice", 1, 20)
	if err != nil {
		t.Fatalf("ListSystemAdmins: %v", err)
	}
	if admins.Total != 1 || len(admins.Records) != 1 || admins.Records[0].UserID != "admin-list-a" {
		t.Fatalf("system admins = %#v", admins)
	}
	if admins.Records[0].Email == nil || *admins.Records[0].Email != "alice@example.com" {
		t.Fatalf("system admin email = %#v", admins.Records[0].Email)
	}

	users, err := repo.ListTenantUsers(ctx, "tenant-list-a", "", 1, 1)
	if err != nil {
		t.Fatalf("ListTenantUsers: %v", err)
	}
	if users.Total != 2 || len(users.Records) != 1 || users.Records[0].UserID != "tenant-user-a2" {
		t.Fatalf("tenant users page = %#v", users)
	}
	if users.Records[0].Status != "inherited_disabled" || users.Page != 1 || users.Size != 1 {
		t.Fatalf("tenant user projection = %#v", users.Records[0])
	}
}
