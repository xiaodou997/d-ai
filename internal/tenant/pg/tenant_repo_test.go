package pg

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"xiaodou/dai/internal/dbtest"
)

func TestTenantRepoOwnsTenantDetailsAndEndUserOwnershipQueries(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	createdAt := time.Date(2026, time.August, 23, 1, 2, 3, 0, time.UTC)
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, contact_person, contact_email, status, created_at, updated_at)
		VALUES ('tenant-repo', 'Repository Tenant', 'Owner', 'owner@example.com', 'active', $1, $1)
	`, createdAt); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, status, created_at, updated_at)
		VALUES ('end-user-repo', 'tenant-repo', 'repo-user', 'hash', 4, 'active', $1, $1)
	`, createdAt); err != nil {
		t.Fatalf("seed end user: %v", err)
	}

	repo := NewTenantRepository(pool)
	details, err := repo.GetTenantDetails(ctx, "tenant-repo")
	if err != nil {
		t.Fatalf("GetTenantDetails: %v", err)
	}
	if details.TenantID != "tenant-repo" || details.TenantName != "Repository Tenant" || details.Status != "active" || details.CreatedTime != createdAt.UnixMilli() {
		t.Fatalf("tenant details = %#v", details)
	}
	if details.ContactPerson == nil || *details.ContactPerson != "Owner" || details.ContactEmail == nil || *details.ContactEmail != "owner@example.com" {
		t.Fatalf("tenant contacts = %#v", details)
	}

	tenantID, err := repo.GetEndUserTenantID(ctx, "end-user-repo")
	if err != nil || tenantID != "tenant-repo" {
		t.Fatalf("end-user tenant = %q, err = %v", tenantID, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE iam_accounts SET status = 'deleted' WHERE user_id = 'end-user-repo'`); err != nil {
		t.Fatalf("soft-delete end user: %v", err)
	}
	if _, err := repo.GetEndUserTenantID(ctx, "end-user-repo"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("deleted end-user lookup error = %v, want pgx.ErrNoRows", err)
	}
}
