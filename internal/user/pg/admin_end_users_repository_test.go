package pg

import (
	"context"
	"testing"
	"time"

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
