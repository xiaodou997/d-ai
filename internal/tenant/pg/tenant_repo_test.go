package pg

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"xiaodou/dai/internal/auth"
	"xiaodou/dai/internal/dbtest"
	tenantports "xiaodou/dai/internal/tenant/ports"
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

func TestTenantRepositoryManagesLifecycleAtomically(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	activation := auth.NewActivationService(pool, time.Hour)
	repo := NewTenantRepository(pool, activation)
	credential, err := activation.NewCredential()
	if err != nil {
		t.Fatalf("initial credential: %v", err)
	}
	if err := repo.CreateTenant(ctx, tenantports.TenantCreateCommand{
		TenantID:      "tenant-lifecycle",
		TenantName:    "Lifecycle Tenant",
		ContactPerson: "Owner",
		ContactEmail:  "owner@example.com",
		Status:        "active",
		InitialUser: &tenantports.TenantInitialUserCreate{
			UserID:              "tenant-lifecycle-user",
			Username:            "lifecycle-user",
			Email:               "user@example.com",
			PasswordHash:        credential.PasswordHash,
			ActivationTokenHash: credential.TokenHash,
			ActivationExpiresAt: credential.ExpiresAt,
		},
	}); err != nil {
		t.Fatalf("CreateTenant with initial user: %v", err)
	}
	if err := repo.CreateTenant(ctx, tenantports.TenantCreateCommand{TenantID: "tenant-lifecycle-conflict", TenantName: "Conflict Tenant", Status: "active"}); err != nil {
		t.Fatalf("CreateTenant conflict fixture: %v", err)
	}

	var tenantName, tenantStatus string
	if err := pool.QueryRow(ctx, `SELECT tenant_name, status FROM iam_tenants WHERE tenant_id = 'tenant-lifecycle'`).Scan(&tenantName, &tenantStatus); err != nil {
		t.Fatalf("read created tenant: %v", err)
	}
	if tenantName != "Lifecycle Tenant" || tenantStatus != "active" {
		t.Fatalf("created tenant = name:%q status:%q", tenantName, tenantStatus)
	}
	var userType, tokenCount int
	if err := pool.QueryRow(ctx, `SELECT user_type FROM iam_accounts WHERE user_id = 'tenant-lifecycle-user'`).Scan(&userType); err != nil {
		t.Fatalf("read initial user: %v", err)
	}
	if userType != 3 {
		t.Fatalf("initial user type = %d, want 3", userType)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM auth_activation_tokens WHERE user_id = 'tenant-lifecycle-user'`).Scan(&tokenCount); err != nil {
		t.Fatalf("count initial activation token: %v", err)
	}
	if tokenCount != 1 {
		t.Fatalf("initial activation token count = %d, want 1", tokenCount)
	}
	rollbackErr := repo.CreateTenant(ctx, tenantports.TenantCreateCommand{
		TenantID:   "tenant-lifecycle-rollback",
		TenantName: "Rollback Tenant",
		Status:     "active",
		InitialUser: &tenantports.TenantInitialUserCreate{
			UserID:              "tenant-lifecycle-rollback-user",
			Username:            "rollback-user",
			PasswordHash:        credential.PasswordHash,
			ActivationTokenHash: credential.TokenHash,
			ActivationExpiresAt: credential.ExpiresAt,
		},
	})
	if rollbackErr == nil {
		t.Fatal("expected duplicate activation token to roll back tenant creation")
	}
	var rollbackCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM iam_tenants WHERE tenant_id = 'tenant-lifecycle-rollback'`).Scan(&rollbackCount); err != nil {
		t.Fatalf("count rolled-back tenant: %v", err)
	}
	if rollbackCount != 0 {
		t.Fatalf("rolled-back tenant count = %d", rollbackCount)
	}

	updated, err := repo.UpdateTenant(ctx, tenantports.TenantUpdateCommand{
		TenantID:      "tenant-lifecycle",
		TenantName:    "Lifecycle Tenant Updated",
		ContactPerson: "New Owner",
		ContactEmail:  "new-owner@example.com",
		Status:        "disabled",
	})
	if err != nil || !updated {
		t.Fatalf("UpdateTenant = updated:%v err:%v", updated, err)
	}
	conflictUpdated, err := repo.UpdateTenant(ctx, tenantports.TenantUpdateCommand{TenantID: "tenant-lifecycle", TenantName: "Conflict Tenant", Status: "active"})
	if err == nil || conflictUpdated || !IsTenantNameTaken(err) {
		t.Fatalf("duplicate-name UpdateTenant = updated:%v err:%v", conflictUpdated, err)
	}
	updated, err = repo.UpdateTenant(ctx, tenantports.TenantUpdateCommand{TenantID: "tenant-lifecycle", TenantName: "Lifecycle Tenant Updated", Status: "active"})
	if err != nil || !updated {
		t.Fatalf("same-name UpdateTenant = updated:%v err:%v", updated, err)
	}
	if err := pool.QueryRow(ctx, `SELECT tenant_name, status FROM iam_tenants WHERE tenant_id = 'tenant-lifecycle'`).Scan(&tenantName, &tenantStatus); err != nil {
		t.Fatalf("read updated tenant: %v", err)
	}
	if tenantName != "Lifecycle Tenant Updated" || tenantStatus != "active" {
		t.Fatalf("updated tenant = name:%q status:%q", tenantName, tenantStatus)
	}

	if err := repo.CreateTenant(ctx, tenantports.TenantCreateCommand{TenantID: "tenant-lifecycle-free", TenantName: "Free Lifecycle Tenant", Status: "active"}); err != nil {
		t.Fatalf("CreateTenant without initial user: %v", err)
	}
	deleted, err := repo.DeleteTenant(ctx, "tenant-lifecycle-free")
	if err != nil || !deleted {
		t.Fatalf("DeleteTenant free = deleted:%v err:%v", deleted, err)
	}
	deleted, err = repo.DeleteTenant(ctx, "tenant-lifecycle-conflict")
	if err != nil || !deleted {
		t.Fatalf("DeleteTenant conflict fixture = deleted:%v err:%v", deleted, err)
	}
	deleted, err = repo.DeleteTenant(ctx, "tenant-lifecycle")
	if err == nil || deleted || !IsTenantReferenced(err) {
		t.Fatalf("DeleteTenant referenced = deleted:%v err:%v", deleted, err)
	}
	deleted, err = repo.DeleteTenant(ctx, "missing-lifecycle-tenant")
	if err != nil || deleted {
		t.Fatalf("DeleteTenant missing = deleted:%v err:%v", deleted, err)
	}
}
