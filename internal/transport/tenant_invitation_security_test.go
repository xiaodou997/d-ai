package transport

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/redis/go-redis/v9"

	"xiaodou/dai/internal/auth"
	"xiaodou/dai/internal/clientsecret"
	"xiaodou/dai/internal/config"
	"xiaodou/dai/internal/dbtest"
	tenantports "xiaodou/dai/internal/tenant/ports"
)

func TestTenantInvitationListRequiresRecentAuthentication(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	if err := clientsecret.Configure("0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatal(err)
	}

	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	const (
		tenantID = "invite-secret-tenant"
		userID   = "invite-secret-tenant-user"
	)
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES ($1, 'Invite Secret Tenant', 'active');
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, status)
		VALUES ($2, $1, 'invite-secret-tenant-user', 'unused', 3, 'active')
	`, tenantID, userID); err != nil {
		t.Fatal(err)
	}

	jwt := auth.NewJWTService(config.JWTConfig{
		Expiration: 15 * time.Minute, RefreshExpiration: time.Hour, Issuer: "dai-invite-secret-test",
	}, pool)
	sessions := auth.NewSessionService(pool, jwt, time.Hour)
	pair, err := sessions.Create(ctx, auth.Principal{
		UserID: userID, Username: "invite-secret-tenant-user", TenantID: tenantID,
		UserType: 3, UserTypeDisplay: "租户", CredentialVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	recent := auth.NewRecentAuthService(redisClient)
	service := &tenantSelfServiceStub{
		invites: []tenantports.InviteCodeItem{{
			ID: 7, Code: "ABC23456", TenantID: tenantID, MaxUses: 0, Status: 1,
		}},
	}

	_, api := humatest.New(t)
	registerTenantSelf(api, tenantSelfModule{
		auth:    platformAuthDeps{JWT: jwt, RecentAuth: recent},
		service: service,
	})
	authorization := "Authorization: Bearer " + pair.AccessToken

	response := api.Get("/api/v1/invitations?page=1&size=20", authorization)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("invitation list without recent auth status = %d, want 401; body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "ABC23456") {
		t.Fatal("invitation bearer credential leaked before recent authentication")
	}

	if err := recent.Mark(ctx, userID, pair.SessionID, "password"); err != nil {
		t.Fatal(err)
	}
	response = api.Get("/api/v1/invitations?page=1&size=20", authorization)
	if response.Code != http.StatusOK {
		t.Fatalf("invitation list with recent auth status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "ABC23456") {
		t.Fatalf("recently authenticated invitation list did not contain expected code; body=%s", response.Body.String())
	}
}
