package pg

import (
	"context"
	"testing"
	"time"

	"xiaodou/dai/internal/auth"
	"xiaodou/dai/internal/dbtest"
	userports "xiaodou/dai/internal/user/ports"
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

func TestAdminAccountRepositoryCreatesAndMutatesLifecycle(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	now := time.Date(2026, time.August, 24, 3, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `INSERT INTO iam_tenants (tenant_id, tenant_name) VALUES ('tenant-account-write', 'Account Write Tenant')`); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, username, password_hash, user_type, status, created_at, updated_at)
		VALUES ('root-account-write', 'root-account-write', 'hash', 1, 'active', $1, $1)
	`, now); err != nil {
		t.Fatalf("seed super admin: %v", err)
	}

	activation := auth.NewActivationService(pool, time.Hour)
	repo := NewAdminAccountRepository(pool, activation)
	systemCredential, err := activation.NewCredential()
	if err != nil {
		t.Fatalf("system credential: %v", err)
	}
	if err := repo.CreateSystemAdmin(ctx, userports.AdminAccountCreate{
		UserID:              "system-account-write",
		Username:            "system-account-write",
		Email:               "system-write@example.com",
		PasswordHash:        systemCredential.PasswordHash,
		ActivationTokenHash: systemCredential.TokenHash,
		ActivationExpiresAt: systemCredential.ExpiresAt,
	}); err != nil {
		t.Fatalf("CreateSystemAdmin: %v", err)
	}
	tenantCredential, err := activation.NewCredential()
	if err != nil {
		t.Fatalf("tenant credential: %v", err)
	}
	if err := repo.CreateTenantUser(ctx, userports.AdminAccountCreate{
		UserID:              "tenant-account-write-user",
		TenantID:            "tenant-account-write",
		Username:            "tenant-account-write-user",
		Email:               "tenant-write@example.com",
		PasswordHash:        tenantCredential.PasswordHash,
		ActivationTokenHash: tenantCredential.TokenHash,
		ActivationExpiresAt: tenantCredential.ExpiresAt,
	}); err != nil {
		t.Fatalf("CreateTenantUser: %v", err)
	}

	var userType int
	var tenantID *string
	var credentialState string
	if err := pool.QueryRow(ctx, `
		SELECT user_type, tenant_id, credential_state
		FROM iam_accounts WHERE user_id = 'tenant-account-write-user'
	`).Scan(&userType, &tenantID, &credentialState); err != nil {
		t.Fatalf("read created tenant user: %v", err)
	}
	if userType != 3 || tenantID == nil || *tenantID != "tenant-account-write" || credentialState != "pending_activation" {
		t.Fatalf("created tenant user = type:%d tenant:%v credential:%q", userType, tenantID, credentialState)
	}
	var tokenCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM auth_activation_tokens WHERE user_id IN ('system-account-write', 'tenant-account-write-user')`).Scan(&tokenCount); err != nil {
		t.Fatalf("count activation tokens: %v", err)
	}
	if tokenCount != 2 {
		t.Fatalf("activation token count = %d, want 2", tokenCount)
	}

	resetSystem, err := repo.ResetSystemAdminPassword(ctx, "system-account-write")
	if err != nil || resetSystem.Token == "" || resetSystem.ExpiresIn <= 0 {
		t.Fatalf("ResetSystemAdminPassword = %#v err:%v", resetSystem, err)
	}
	resetTenant, err := repo.ResetTenantUserPassword(ctx, "tenant-account-write-user")
	if err != nil || resetTenant.Token == "" || resetTenant.ExpiresIn <= 0 {
		t.Fatalf("ResetTenantUserPassword = %#v err:%v", resetTenant, err)
	}
	wrongTypeReset, err := repo.ResetSystemAdminPassword(ctx, "tenant-account-write-user")
	if err != nil || wrongTypeReset.Token != "" {
		t.Fatalf("tenant target ResetSystemAdminPassword = %#v err:%v", wrongTypeReset, err)
	}
	missingReset, err := repo.ResetTenantUserPassword(ctx, "missing-account-write-user")
	if err != nil || missingReset.Token != "" {
		t.Fatalf("missing ResetTenantUserPassword = %#v err:%v", missingReset, err)
	}

	systemResult, err := repo.UpdateSystemAdmin(ctx, userports.AdminAccountUpdate{UserID: "system-account-write", Email: "system-updated@example.com", Status: "disabled"})
	if err != nil || !systemResult.Updated || systemResult.Forbidden {
		t.Fatalf("UpdateSystemAdmin = %#v err:%v", systemResult, err)
	}
	rootResult, err := repo.UpdateSystemAdmin(ctx, userports.AdminAccountUpdate{UserID: "root-account-write", Email: "blocked@example.com", Status: "disabled"})
	if err != nil || !rootResult.Forbidden || rootResult.Updated {
		t.Fatalf("super-admin UpdateSystemAdmin = %#v err:%v", rootResult, err)
	}
	wrongTypeResult, err := repo.UpdateSystemAdmin(ctx, userports.AdminAccountUpdate{UserID: "tenant-account-write-user", Email: "wrong@example.com", Status: "disabled"})
	if err != nil || wrongTypeResult.Forbidden || wrongTypeResult.Updated {
		t.Fatalf("tenant target UpdateSystemAdmin = %#v err:%v", wrongTypeResult, err)
	}

	updated, err := repo.UpdateTenantUserStatus(ctx, "tenant-account-write-user", "disabled")
	if err != nil || !updated {
		t.Fatalf("UpdateTenantUserStatus = updated:%v err:%v", updated, err)
	}
	updated, err = repo.UpdateTenantUser(ctx, userports.AdminAccountUpdate{UserID: "tenant-account-write-user", Email: "tenant-updated@example.com", Status: "active"})
	if err != nil || !updated {
		t.Fatalf("UpdateTenantUser = updated:%v err:%v", updated, err)
	}
	var email, status string
	if err := pool.QueryRow(ctx, `SELECT email, status FROM iam_accounts WHERE user_id = 'tenant-account-write-user'`).Scan(&email, &status); err != nil {
		t.Fatalf("read updated tenant user: %v", err)
	}
	if email != "tenant-updated@example.com" || status != "active" {
		t.Fatalf("updated tenant user = email:%q status:%q", email, status)
	}
	updated, err = repo.UpdateTenantUserStatus(ctx, "missing-account-write-user", "disabled")
	if err != nil || updated {
		t.Fatalf("missing tenant status update = updated:%v err:%v", updated, err)
	}
}
