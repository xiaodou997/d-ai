package auth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/clientsecret"
	"xiaodou/dai/internal/config"
	"xiaodou/dai/internal/dbtest"
)

func TestSessionRefreshRotationAndReplayRevocation(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	principal := seedSessionAccount(t, ctx, pool, "session-replay")
	service := newTestSessionService(pool)

	initial, err := service.Create(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	rotated, _, err := service.Rotate(ctx, initial.RefreshToken)
	if err != nil {
		t.Fatalf("first rotation: %v", err)
	}
	if rotated.RefreshToken == initial.RefreshToken || rotated.AccessToken == "" {
		t.Fatal("rotation did not issue a new token pair")
	}
	if _, _, err := service.Rotate(ctx, initial.RefreshToken); !errors.Is(err, ErrRefreshTokenReused) {
		t.Fatalf("old token replay error = %v, want ErrRefreshTokenReused", err)
	}
	if _, _, err := service.Rotate(ctx, rotated.RefreshToken); !errors.Is(err, ErrSessionInactive) {
		t.Fatalf("replacement after replay error = %v, want ErrSessionInactive", err)
	}
}

func TestSessionConcurrentRefreshHasSingleWinner(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 6})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	principal := seedSessionAccount(t, ctx, pool, "session-concurrent")
	service := newTestSessionService(pool)
	initial, err := service.Create(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, _, err := service.Rotate(ctx, initial.RefreshToken)
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, ErrRefreshTokenReused) {
			t.Fatalf("unexpected concurrent result: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent refresh successes = %d, want 1", successes)
	}
}

func TestCredentialAndStatusChangesRevokeAllSessions(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	principal := seedSessionAccount(t, ctx, pool, "session-account-change")
	service := newTestSessionService(pool)
	first, err := service.Create(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Create(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(ctx, `UPDATE iam_accounts SET credential_version = credential_version + 1 WHERE user_id = $1`, principal.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.jwt.ParseToken(first.AccessToken); !errors.Is(err, ErrSessionInactive) {
		t.Fatalf("access after credential change = %v, want ErrSessionInactive", err)
	}
	for _, raw := range []string{first.RefreshToken, second.RefreshToken} {
		if _, _, err := service.Rotate(ctx, raw); !errors.Is(err, ErrSessionInactive) {
			t.Fatalf("refresh after credential change = %v, want ErrSessionInactive", err)
		}
	}

	principal.CredentialVersion++
	third, err := service.Create(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE iam_accounts SET status = 'disabled' WHERE user_id = $1`, principal.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.jwt.ParseToken(third.AccessToken); !errors.Is(err, ErrSessionInactive) {
		t.Fatalf("access after account disable = %v, want ErrSessionInactive", err)
	}
	if _, _, err := service.Rotate(ctx, third.RefreshToken); !errors.Is(err, ErrSessionInactive) {
		t.Fatalf("refresh after account disable = %v, want ErrSessionInactive", err)
	}
}

func TestLogoutRevokesCurrentSessionOnly(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	principal := seedSessionAccount(t, ctx, pool, "session-logout")
	service := newTestSessionService(pool)
	first, err := service.Create(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Create(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := service.jwt.ParseToken(first.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Revoke(ctx, claims.SessionID, "logout"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.jwt.ParseToken(first.AccessToken); !errors.Is(err, ErrSessionInactive) {
		t.Fatalf("access token after logout = %v, want ErrSessionInactive", err)
	}
	if _, _, err := service.Rotate(ctx, first.RefreshToken); !errors.Is(err, ErrSessionInactive) {
		t.Fatalf("logged-out session refresh = %v, want ErrSessionInactive", err)
	}
	if _, _, err := service.Rotate(ctx, second.RefreshToken); err != nil {
		t.Fatalf("other session should remain active: %v", err)
	}
}

func TestTenantDisableAndAccountDeleteRejectRefresh(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	if _, err := pool.Exec(ctx, `INSERT INTO iam_tenants (tenant_id, tenant_name) VALUES ('auth-tenant', 'Auth Tenant')`); err != nil {
		t.Fatal(err)
	}
	principal := Principal{
		UserID: "auth-tenant-user", Username: "auth-tenant-user", TenantID: "auth-tenant",
		UserType: 3, UserTypeDisplay: "租户", CredentialVersion: 1,
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, status)
		VALUES ($1, $2, $3, 'unused', 3, 'active')
	`, principal.UserID, principal.TenantID, principal.Username); err != nil {
		t.Fatal(err)
	}
	service := newTestSessionService(pool)
	tenantSession, err := service.Create(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE iam_tenants SET status = 'disabled' WHERE tenant_id = $1`, principal.TenantID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Rotate(ctx, tenantSession.RefreshToken); !errors.Is(err, ErrTenantInactive) {
		t.Fatalf("refresh after tenant disable = %v, want ErrTenantInactive", err)
	}

	admin := seedSessionAccount(t, ctx, pool, "session-delete")
	adminSession, err := service.Create(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM iam_accounts WHERE user_id = $1`, admin.UserID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Rotate(ctx, adminSession.RefreshToken); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("refresh after account delete = %v, want ErrInvalidRefreshToken", err)
	}
}

func TestRoleAndTenantScopeChangesInvalidateExistingAccessToken(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name)
		VALUES ('auth-role-change-tenant', 'Role Change Tenant'),
		       ('auth-scope-change-tenant', 'Scope Change Tenant')
	`); err != nil {
		t.Fatal(err)
	}
	principal := seedSessionAccount(t, ctx, pool, "role-change")
	service := newTestSessionService(pool)
	pair, err := service.Create(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}

	// A role change also changes the required tenant scope. The old token must
	// fail closed instead of retaining its former platform-admin capability.
	if _, err := pool.Exec(ctx, `
		UPDATE iam_accounts
		SET user_type = 3, tenant_id = 'auth-role-change-tenant'
		WHERE user_id = $1
	`, principal.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.jwt.ParseToken(pair.AccessToken); !errors.Is(err, ErrSessionInactive) {
		t.Fatalf("access token after role/scope change = %v, want ErrSessionInactive", err)
	}

	rotated, refreshedPrincipal, err := service.Rotate(ctx, pair.RefreshToken)
	if err != nil {
		t.Fatalf("refresh after role/scope change: %v", err)
	}
	if refreshedPrincipal.UserType != 3 || refreshedPrincipal.TenantID != "auth-role-change-tenant" {
		t.Fatalf("refreshed principal = %#v, want tenant-scoped role", refreshedPrincipal)
	}
	claims, err := service.jwt.ParseToken(rotated.AccessToken)
	if err != nil {
		t.Fatalf("refreshed access token should be valid: %v", err)
	}
	if claims.UserType != 3 || claims.TenantID != "auth-role-change-tenant" {
		t.Fatalf("refreshed claims = %#v, want current role/scope", claims)
	}

	// Moving an already tenant-scoped account must invalidate the token even
	// when its role is unchanged, otherwise the old tenant scope remains usable.
	if _, err := pool.Exec(ctx, `
		UPDATE iam_accounts
		SET tenant_id = 'auth-scope-change-tenant'
		WHERE user_id = $1
	`, principal.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.jwt.ParseToken(rotated.AccessToken); !errors.Is(err, ErrSessionInactive) {
		t.Fatalf("access token after tenant scope change = %v, want ErrSessionInactive", err)
	}
	rotatedAgain, refreshedPrincipal, err := service.Rotate(ctx, rotated.RefreshToken)
	if err != nil {
		t.Fatalf("refresh after tenant scope change: %v", err)
	}
	if refreshedPrincipal.UserType != 3 || refreshedPrincipal.TenantID != "auth-scope-change-tenant" {
		t.Fatalf("principal after tenant scope change = %#v, want new tenant", refreshedPrincipal)
	}
	claims, err = service.jwt.ParseToken(rotatedAgain.AccessToken)
	if err != nil {
		t.Fatalf("access token after tenant scope refresh should be valid: %v", err)
	}
	if claims.TenantID != "auth-scope-change-tenant" {
		t.Fatalf("claims after tenant scope refresh = %#v, want new tenant", claims)
	}
}

func newTestSessionService(pool *pgxpool.Pool) *SessionService {
	if err := clientsecret.Configure("0123456789abcdef0123456789abcdef"); err != nil {
		panic(err)
	}
	jwt := NewJWTService(config.JWTConfig{Expiration: 15 * time.Minute, RefreshExpiration: time.Hour, Issuer: "dai-test"}, pool)
	return NewSessionService(pool, jwt, time.Hour)
}

func seedSessionAccount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, suffix string) Principal {
	t.Helper()
	userID := "auth-" + suffix
	username := "auth-" + suffix
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, username, password_hash, user_type, status)
		VALUES ($1, $2, 'unused', 2, 'active')
	`, userID, username); err != nil {
		t.Fatal(err)
	}
	return Principal{UserID: userID, Username: username, UserType: 2, UserTypeDisplay: "平台管理员", CredentialVersion: 1}
}
