package pg

import (
	"context"
	"errors"
	"testing"

	"xiaodou/dai/internal/dbtest"
	inviteports "xiaodou/dai/internal/invite/ports"
)

func TestRegisterEndUserFailsClosedWhenInvitationAuthorityChanges(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES
		  ('invite-disabled-tenant', 'Disabled Invite Tenant', 'disabled'),
		  ('invite-expired-tenant', 'Expired Invite Tenant', 'active');

		INSERT INTO iam_invitation_codes (
			code, tenant_id, created_by, max_uses, used_count, status, expires_at
		) VALUES
		  ('DISA2345', 'invite-disabled-tenant', 'tenant-owner', 0, 0, 'active', NULL),
		  ('EXPR2345', 'invite-expired-tenant', 'tenant-owner', 0, 0, 'active', now() - interval '1 second')
	`); err != nil {
		t.Fatal(err)
	}

	repo := NewInviteRepository(pool)
	cases := []struct {
		name     string
		code     string
		tenantID string
		userID   string
	}{
		{name: "disabled tenant", code: "DISA2345", tenantID: "invite-disabled-tenant", userID: "invite-disabled-user"},
		{name: "expired invitation", code: "EXPR2345", tenantID: "invite-expired-tenant", userID: "invite-expired-user"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := repo.RegisterEndUser(ctx, EndUserRegistration{
				InvitationCode: tc.code,
				TenantID:       tc.tenantID,
				UserID:         tc.userID,
				Username:       tc.userID,
				PasswordHash:   "unused",
			})
			if !errors.Is(err, inviteports.ErrInvitationCodeUnavailable) {
				t.Fatalf("RegisterEndUser() error = %v, want ErrInvitationCodeUnavailable", err)
			}

			var exists bool
			if err := pool.QueryRow(ctx, `
				SELECT EXISTS (SELECT 1 FROM iam_accounts WHERE user_id = $1)
			`, tc.userID).Scan(&exists); err != nil {
				t.Fatal(err)
			}
			if exists {
				t.Fatal("failed invitation registration left a durable user account")
			}
		})
	}
}

func TestGetByCodeHidesInvitationsForInactiveTenants(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES ('invite-hidden-tenant', 'Hidden Invite Tenant', 'suspended');
		INSERT INTO iam_invitation_codes (code, tenant_id, created_by, max_uses, used_count, status)
		VALUES ('HIDE2345', 'invite-hidden-tenant', 'tenant-owner', 0, 0, 'active')
	`); err != nil {
		t.Fatal(err)
	}

	if _, err := NewInviteRepository(pool).GetByCode(ctx, "HIDE2345"); !errors.Is(err, inviteports.ErrInvitationCodeNotFound) {
		t.Fatalf("GetByCode() for inactive tenant = %v, want ErrInvitationCodeNotFound", err)
	}
}
