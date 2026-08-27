package pg

import (
	"context"
	"errors"
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

	updated, err = repo.UpdateEndUserStatus(ctx, userports.AdminEndUserStatusUpdate{UserID: "end-write-a", TenantID: "tenant-write-a", Status: "disabled"})
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
	updated, err = repo.UpdateEndUserStatus(ctx, userports.AdminEndUserStatusUpdate{UserID: "end-write-deleted", TenantID: "tenant-write-a", Status: "active"})
	if err != nil || updated {
		t.Fatalf("deleted UpdateEndUserStatus = updated:%v err:%v", updated, err)
	}
	updated, err = repo.UpdateEndUserStatus(ctx, userports.AdminEndUserStatusUpdate{UserID: "end-write-a", TenantID: "tenant-write-b", Status: "active"})
	if err != nil || updated {
		t.Fatalf("cross-tenant UpdateEndUserStatus = updated:%v err:%v", updated, err)
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

func TestAdminEndUserRepositoryResetsOnlyActiveEndUsers(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	now := time.Date(2026, time.August, 24, 5, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `INSERT INTO iam_tenants (tenant_id, tenant_name) VALUES ('tenant-reset-a', 'Reset Tenant')`); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, status, created_at, updated_at)
		VALUES
			('end-reset-a', 'tenant-reset-a', 'end-reset-a', 'hash', 4, 'active', $1, $1),
			('end-reset-deleted', 'tenant-reset-a', 'end-reset-deleted', 'hash', 4, 'deleted', $1, $1),
			('tenant-reset-user', 'tenant-reset-a', 'tenant-reset-user', 'hash', 3, 'active', $1, $1)
	`, now); err != nil {
		t.Fatalf("seed reset accounts: %v", err)
	}

	activation := auth.NewActivationService(pool, time.Hour)
	repo := NewAdminEndUserRepository(pool, activation)
	result, err := repo.ResetEndUserPassword(ctx, userports.AdminEndUserPasswordReset{UserID: "end-reset-a", TenantID: "tenant-reset-a"})
	if err != nil || result.Token == "" || result.ExpiresIn <= 0 {
		t.Fatalf("ResetEndUserPassword = %#v err:%v", result, err)
	}
	var credentialState string
	if err := pool.QueryRow(ctx, `SELECT credential_state FROM iam_accounts WHERE user_id = 'end-reset-a'`).Scan(&credentialState); err != nil {
		t.Fatalf("read reset state: %v", err)
	}
	if credentialState != "pending_activation" {
		t.Fatalf("credential state = %q, want pending_activation", credentialState)
	}
	deleted, err := repo.ResetEndUserPassword(ctx, userports.AdminEndUserPasswordReset{UserID: "end-reset-deleted", TenantID: "tenant-reset-a"})
	if err != nil || deleted.Token != "" {
		t.Fatalf("deleted ResetEndUserPassword = %#v err:%v", deleted, err)
	}
	wrongType, err := repo.ResetEndUserPassword(ctx, userports.AdminEndUserPasswordReset{UserID: "tenant-reset-user", TenantID: "tenant-reset-a"})
	if err != nil || wrongType.Token != "" {
		t.Fatalf("tenant target ResetEndUserPassword = %#v err:%v", wrongType, err)
	}
	crossTenant, err := repo.ResetEndUserPassword(ctx, userports.AdminEndUserPasswordReset{UserID: "end-reset-a", TenantID: "other-tenant"})
	if err != nil || crossTenant.Token != "" {
		t.Fatalf("cross-tenant ResetEndUserPassword = %#v err:%v", crossTenant, err)
	}
}

func TestAdminEndUserRepositoryDeletesWithBalanceAndGuardInvariants(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	now := time.Date(2026, time.August, 24, 6, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `INSERT INTO iam_tenants (tenant_id, tenant_name) VALUES ('tenant-delete-a', 'Delete Tenant')`); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, status, created_at, updated_at)
		VALUES
			('end-delete-zero', 'tenant-delete-a', 'end-delete-zero', 'hash', 4, 'active', $1, $1),
			('end-delete-positive', 'tenant-delete-a', 'end-delete-positive', 'hash', 4, 'active', $1, $1),
			('end-delete-negative', 'tenant-delete-a', 'end-delete-negative', 'hash', 4, 'active', $1, $1),
			('end-delete-guard', 'tenant-delete-a', 'end-delete-guard', 'hash', 4, 'active', $1, $1),
			('end-delete-done', 'tenant-delete-a', 'end-delete-done', 'hash', 4, 'deleted', $1, $1)
	`, now); err != nil {
		t.Fatalf("seed end users: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE bill_accounts SET balance_micro = CASE account_id
		WHEN 'end-delete-positive' THEN 100
		WHEN 'end-delete-negative' THEN -100
		ELSE 0 END
		WHERE account_id IN ('end-delete-zero', 'end-delete-positive', 'end-delete-negative', 'end-delete-guard', 'end-delete-done')`); err != nil {
		t.Fatalf("seed balances: %v", err)
	}

	repo := NewAdminEndUserRepository(pool)
	guardCalls := 0
	guard := func(_ context.Context, _ string) error {
		guardCalls++
		return nil
	}
	deleted, err := repo.DeleteEndUser(ctx, userports.AdminEndUserDeleteCommand{UserID: "end-delete-zero", TenantID: "tenant-delete-a", BeforeCommit: guard})
	if err != nil || !deleted.Found || !deleted.Deleted || deleted.BalanceMicroUSD != 0 || guardCalls != 1 {
		t.Fatalf("zero-balance delete = %#v guardCalls:%d err:%v", deleted, guardCalls, err)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM iam_accounts WHERE user_id = 'end-delete-zero'`).Scan(&status); err != nil {
		t.Fatalf("read deleted status: %v", err)
	}
	if status != "deleted" {
		t.Fatalf("deleted status = %q", status)
	}

	positive, err := repo.DeleteEndUser(ctx, userports.AdminEndUserDeleteCommand{UserID: "end-delete-positive", TenantID: "tenant-delete-a", BeforeCommit: guard})
	if err != nil || !positive.Found || positive.Deleted || positive.BalanceMicroUSD != 100 || guardCalls != 1 {
		t.Fatalf("positive-balance delete = %#v guardCalls:%d err:%v", positive, guardCalls, err)
	}
	negative, err := repo.DeleteEndUser(ctx, userports.AdminEndUserDeleteCommand{UserID: "end-delete-negative", TenantID: "tenant-delete-a", BeforeCommit: guard})
	if err != nil || !negative.Found || negative.Deleted || negative.BalanceMicroUSD != -100 || guardCalls != 1 {
		t.Fatalf("negative-balance delete = %#v guardCalls:%d err:%v", negative, guardCalls, err)
	}

	guardFailure := errors.New("blacklist unavailable")
	guardErr, err := repo.DeleteEndUser(ctx, userports.AdminEndUserDeleteCommand{UserID: "end-delete-guard", TenantID: "tenant-delete-a", BeforeCommit: func(context.Context, string) error { return guardFailure }})
	var classified *userports.AdminEndUserDeleteGuardError
	if err == nil || !errors.As(err, &classified) || !errors.Is(err, guardFailure) || guardErr.Deleted {
		t.Fatalf("guard failure = result:%#v err:%v classified:%v", guardErr, err, classified)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM iam_accounts WHERE user_id = 'end-delete-guard'`).Scan(&status); err != nil {
		t.Fatalf("read guard rollback status: %v", err)
	}
	if status != "active" {
		t.Fatalf("guard rollback status = %q", status)
	}

	missing, err := repo.DeleteEndUser(ctx, userports.AdminEndUserDeleteCommand{UserID: "missing-end-delete", TenantID: "tenant-delete-a", BeforeCommit: guard})
	if err != nil || missing.Found || missing.Deleted {
		t.Fatalf("missing delete = %#v err:%v", missing, err)
	}
	alreadyDeleted, err := repo.DeleteEndUser(ctx, userports.AdminEndUserDeleteCommand{UserID: "end-delete-done", TenantID: "tenant-delete-a", BeforeCommit: guard})
	if err != nil || alreadyDeleted.Found || alreadyDeleted.Deleted {
		t.Fatalf("already-deleted delete = %#v err:%v", alreadyDeleted, err)
	}
	crossTenant, err := repo.DeleteEndUser(ctx, userports.AdminEndUserDeleteCommand{UserID: "end-delete-guard", TenantID: "other-tenant", BeforeCommit: guard})
	if err != nil || crossTenant.Found || crossTenant.Deleted {
		t.Fatalf("cross-tenant delete = %#v err:%v", crossTenant, err)
	}
}
