package pg

import (
	"context"
	"testing"
	"time"

	"xiaodou/dai/internal/auth"
	"xiaodou/dai/internal/dbtest"
	userports "xiaodou/dai/internal/user/ports"
)

func TestAdminEndUserRepositoryAppliesScopeFiltersAndBalanceProjection(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	older := time.Date(2026, time.August, 23, 1, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	lastLogin := newer.Add(10 * time.Minute)
	if _, err := pool.Exec(ctx, `INSERT INTO iam_tenants (tenant_id, tenant_name) VALUES ('tenant-end-a', 'Alpha Tenant'), ('tenant-end-b', 'Beta Tenant')`); err != nil {
		t.Fatalf("seed tenants: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, email, phone, internal_note, nickname, avatar, user_type, status, credential_state, last_login_at, created_at, updated_at)
		VALUES
			('end-list-a1', 'tenant-end-a', 'alpha-old', 'hash', 'old@example.com', '111', 'search note', 'Old', 'old.png', 4, 'active', 'active', NULL, $1, $1),
			('end-list-a2', 'tenant-end-a', 'alpha-new', 'hash', 'new@example.com', '222', '', 'New', 'new.png', 4, 'disabled', 'pending_activation', $2, $2, $2),
			('end-list-b1', 'tenant-end-b', 'beta-user', 'hash', 'beta@example.com', '333', '', 'Beta', 'beta.png', 4, 'active', 'active', NULL, $2, $2)
	`, older, lastLogin); err != nil {
		t.Fatalf("seed end users: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE bill_accounts SET balance_micro = 123456 WHERE account_id = 'end-list-a2'`); err != nil {
		t.Fatalf("seed end-user balance: %v", err)
	}

	repo := NewAdminEndUserRepository(pool)
	page, err := repo.ListEndUsers(ctx, userports.AdminEndUserListFilter{TenantID: "tenant-end-a", Page: 1, Size: 1})
	if err != nil {
		t.Fatalf("ListEndUsers scoped page: %v", err)
	}
	if page.Total != 2 || len(page.Records) != 1 || page.Records[0].UserID != "end-list-a2" {
		t.Fatalf("scoped page = %#v", page)
	}
	row := page.Records[0]
	if row.TenantName == nil || *row.TenantName != "Alpha Tenant" || row.BalanceMicroUSD != 123456 || row.Status != "disabled" || row.CredentialState != "pending_activation" {
		t.Fatalf("end-user projection = %#v", row)
	}
	if row.LastLoginAt == nil || !row.LastLoginAt.Equal(lastLogin) {
		t.Fatalf("last login = %v, want %v", row.LastLoginAt, lastLogin)
	}

	filtered, err := repo.ListEndUsers(ctx, userports.AdminEndUserListFilter{TenantName: "Beta", Status: "active", Page: 1, Size: 20})
	if err != nil {
		t.Fatalf("ListEndUsers tenant/status filter: %v", err)
	}
	if filtered.Total != 1 || filtered.Records[0].UserID != "end-list-b1" {
		t.Fatalf("tenant/status filter = %#v", filtered)
	}

	keyword, err := repo.ListEndUsers(ctx, userports.AdminEndUserListFilter{Keyword: "search note", Page: 1, Size: 20})
	if err != nil {
		t.Fatalf("ListEndUsers keyword filter: %v", err)
	}
	if keyword.Total != 1 || keyword.Records[0].UserID != "end-list-a1" {
		t.Fatalf("keyword filter = %#v", keyword)
	}
}

func TestAdminEndUserRepositoryUpdatesScopedProfileAndStatus(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	now := time.Date(2026, time.August, 23, 3, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name) VALUES ('tenant-write-a', 'Write Tenant A'), ('tenant-write-b', 'Write Tenant B')
	`); err != nil {
		t.Fatalf("seed tenants: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, email, phone, internal_note, user_type, status, created_at, updated_at)
		VALUES
			('end-write-a', 'tenant-write-a', 'write-a', 'hash', 'before@example.com', '100', 'before', 4, 'active', $1, $1),
			('end-write-deleted', 'tenant-write-a', 'write-deleted', 'hash', 'deleted@example.com', '101', 'deleted', 4, 'deleted', $1, $1),
			('end-write-b', 'tenant-write-b', 'write-b', 'hash', 'other@example.com', '200', 'other', 4, 'active', $1, $1)
	`, now); err != nil {
		t.Fatalf("seed end users: %v", err)
	}

	repo := NewAdminEndUserRepository(pool)
	updated, err := repo.UpdateEndUser(ctx, userports.AdminEndUserUpdate{
		UserID:          "end-write-a",
		TenantID:        "tenant-write-a",
		EmailSet:        true,
		Email:           "after@example.com",
		PhoneSet:        true,
		Phone:           "",
		InternalNoteSet: true,
		InternalNote:    "after",
	})
	if err != nil || !updated {
		t.Fatalf("UpdateEndUser = updated:%v err:%v", updated, err)
	}
	var email, phone, note string
	if err := pool.QueryRow(ctx, `
		SELECT COALESCE(email, ''), COALESCE(phone, ''), internal_note
		FROM iam_accounts WHERE user_id = 'end-write-a'
	`).Scan(&email, &phone, &note); err != nil {
		t.Fatalf("read updated end user: %v", err)
	}
	if email != "after@example.com" || phone != "" || note != "after" {
		t.Fatalf("updated profile = email:%q phone:%q note:%q", email, phone, note)
	}

	updated, err = repo.UpdateEndUser(ctx, userports.AdminEndUserUpdate{UserID: "end-write-a", TenantID: "tenant-write-b", EmailSet: true, Email: "wrong@example.com"})
	if err != nil || updated {
		t.Fatalf("cross-tenant UpdateEndUser = updated:%v err:%v", updated, err)
	}
	updated, err = repo.UpdateEndUser(ctx, userports.AdminEndUserUpdate{UserID: "end-write-deleted", TenantID: "tenant-write-a", EmailSet: true, Email: "wrong@example.com"})
	if err != nil || updated {
		t.Fatalf("deleted UpdateEndUser = updated:%v err:%v", updated, err)
	}

	updated, err = repo.UpdateEndUserStatus(ctx, "end-write-a", "disabled")
	if err != nil || !updated {
		t.Fatalf("UpdateEndUserStatus = updated:%v err:%v", updated, err)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM iam_accounts WHERE user_id = 'end-write-a'`).Scan(&status); err != nil {
		t.Fatalf("read updated status: %v", err)
	}
	if status != "disabled" {
		t.Fatalf("status = %q, want disabled", status)
	}
	updated, err = repo.UpdateEndUserStatus(ctx, "end-write-deleted", "active")
	if err != nil || updated {
		t.Fatalf("deleted UpdateEndUserStatus = updated:%v err:%v", updated, err)
	}
}

func TestAdminEndUserRepositoryCreatesActivationAtomically(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `INSERT INTO iam_tenants (tenant_id, tenant_name) VALUES ('tenant-create-a', 'Create Tenant')`); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	activation := auth.NewActivationService(pool, time.Hour)
	repo := NewAdminEndUserRepository(pool, activation)
	credential, err := activation.NewCredential()
	if err != nil {
		t.Fatalf("NewCredential: %v", err)
	}
	email := "create@example.com"
	phone := "123"
	input := userports.AdminEndUserCreate{
		UserID:              "end-create-a",
		TenantID:            "tenant-create-a",
		Username:            "create-a",
		Email:               &email,
		Phone:               &phone,
		InternalNote:        "created",
		PasswordHash:        credential.PasswordHash,
		ActivationTokenHash: credential.TokenHash,
		ActivationExpiresAt: credential.ExpiresAt,
	}
	if err := repo.CreateEndUser(ctx, input); err != nil {
		t.Fatalf("CreateEndUser: %v", err)
	}

	var credentialState string
	if err := pool.QueryRow(ctx, `SELECT credential_state FROM iam_accounts WHERE user_id = $1`, input.UserID).Scan(&credentialState); err != nil {
		t.Fatalf("read created account: %v", err)
	}
	if credentialState != "pending_activation" {
		t.Fatalf("credential state = %q, want pending_activation", credentialState)
	}
	var tokenHash []byte
	var purpose string
	if err := pool.QueryRow(ctx, `SELECT token_hash, purpose FROM auth_activation_tokens WHERE user_id = $1`, input.UserID).Scan(&tokenHash, &purpose); err != nil {
		t.Fatalf("read activation token: %v", err)
	}
	if string(tokenHash) != string(credential.TokenHash) || purpose != auth.ActivationPurposeAccount {
		t.Fatalf("activation record = hash:%x purpose:%q", tokenHash, purpose)
	}

	// Reusing a token hash makes the activation insert fail after the account
	// insert. The repository must roll the account back with the token write.
	input.UserID = "end-create-b"
	input.Username = "create-b"
	input.Email = nil
	input.Phone = nil
	if err := repo.CreateEndUser(ctx, input); err == nil {
		t.Fatal("expected duplicate activation token error")
	}
	var accountCount, tokenCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM iam_accounts WHERE user_id = $1`, input.UserID).Scan(&accountCount); err != nil {
		t.Fatalf("count rolled-back account: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM auth_activation_tokens WHERE user_id = $1`, input.UserID).Scan(&tokenCount); err != nil {
		t.Fatalf("count rolled-back token: %v", err)
	}
	if accountCount != 0 || tokenCount != 0 {
		t.Fatalf("rollback counts = account:%d token:%d", accountCount, tokenCount)
	}
}
