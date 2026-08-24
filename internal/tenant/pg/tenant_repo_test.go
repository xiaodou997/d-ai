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

func TestTenantRepositoryCascadesStatusAndReturnsRestoredUsers(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	now := time.Date(2026, time.August, 24, 7, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `INSERT INTO iam_tenants (tenant_id, tenant_name, status, created_at, updated_at) VALUES ('tenant-cascade', 'Cascade Tenant', 'active', $1, $1), ('tenant-cascade-other', 'Other Tenant', 'active', $1, $1)`, now); err != nil {
		t.Fatalf("seed tenants: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, status, created_at, updated_at)
		VALUES
			('cascade-active-user', 'tenant-cascade', 'cascade-active-user', 'hash', 3, 'active', $1, $1),
			('cascade-active-end', 'tenant-cascade', 'cascade-active-end', 'hash', 4, 'active', $1, $1),
			('cascade-disabled-user', 'tenant-cascade', 'cascade-disabled-user', 'hash', 3, 'disabled', $1, $1),
			('cascade-locked-end', 'tenant-cascade', 'cascade-locked-end', 'hash', 4, 'locked', $1, $1),
			('cascade-inherited-user', 'tenant-cascade', 'cascade-inherited-user', 'hash', 3, 'inherited_disabled', $1, $1),
			('cascade-inherited-end', 'tenant-cascade', 'cascade-inherited-end', 'hash', 4, 'inherited_disabled', $1, $1),
			('cascade-other-user', 'tenant-cascade-other', 'cascade-other-user', 'hash', 3, 'active', $1, $1)
	`, now); err != nil {
		t.Fatalf("seed accounts: %v", err)
	}

	repo := NewTenantRepository(pool)
	disabled, err := repo.UpdateStatus(ctx, "tenant-cascade", "disabled")
	if err != nil || !disabled.Updated || len(disabled.RestoredUserIDs) != 0 {
		t.Fatalf("disable cascade = %#v err:%v", disabled, err)
	}
	var tenantStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM iam_tenants WHERE tenant_id = 'tenant-cascade'`).Scan(&tenantStatus); err != nil {
		t.Fatalf("read disabled tenant: %v", err)
	}
	if tenantStatus != "disabled" {
		t.Fatalf("tenant status = %q, want disabled", tenantStatus)
	}
	statusRows, err := pool.Query(ctx, `SELECT user_id, status FROM iam_accounts WHERE tenant_id = 'tenant-cascade'`)
	if err != nil {
		t.Fatalf("read disabled cascade: %v", err)
	}
	statuses := map[string]string{}
	for statusRows.Next() {
		var userID, status string
		if err := statusRows.Scan(&userID, &status); err != nil {
			statusRows.Close()
			t.Fatalf("scan disabled cascade: %v", err)
		}
		statuses[userID] = status
	}
	statusRows.Close()
	if err := statusRows.Err(); err != nil {
		t.Fatalf("iterate disabled cascade: %v", err)
	}
	wantDisabled := map[string]string{
		"cascade-active-user":    "inherited_disabled",
		"cascade-active-end":     "inherited_disabled",
		"cascade-disabled-user":  "disabled",
		"cascade-locked-end":     "locked",
		"cascade-inherited-user": "inherited_disabled",
		"cascade-inherited-end":  "inherited_disabled",
	}
	for userID, want := range wantDisabled {
		if statuses[userID] != want {
			t.Fatalf("disabled status %s = %q, want %q", userID, statuses[userID], want)
		}
	}
	var otherStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM iam_accounts WHERE user_id = 'cascade-other-user'`).Scan(&otherStatus); err != nil {
		t.Fatalf("read other tenant account: %v", err)
	}
	if otherStatus != "active" {
		t.Fatalf("other tenant account was cascaded: %q", otherStatus)
	}

	active, err := repo.UpdateStatus(ctx, "tenant-cascade", "active")
	if err != nil || !active.Updated {
		t.Fatalf("enable cascade = %#v err:%v", active, err)
	}
	restored := map[string]bool{}
	for _, userID := range active.RestoredUserIDs {
		restored[userID] = true
	}
	if len(restored) != 4 || !restored["cascade-active-user"] || !restored["cascade-active-end"] || !restored["cascade-inherited-user"] || !restored["cascade-inherited-end"] {
		t.Fatalf("restored users = %#v", active.RestoredUserIDs)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM iam_accounts WHERE user_id = 'cascade-disabled-user'`).Scan(&tenantStatus); err != nil {
		t.Fatalf("read preserved disabled account: %v", err)
	}
	if tenantStatus != "disabled" {
		t.Fatalf("preserved disabled account = %q", tenantStatus)
	}
	missing, err := repo.UpdateStatus(ctx, "missing-cascade-tenant", "disabled")
	if err != nil || missing.Updated {
		t.Fatalf("missing cascade = %#v err:%v", missing, err)
	}
}
