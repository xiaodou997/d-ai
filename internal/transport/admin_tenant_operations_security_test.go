package transport

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/redis/go-redis/v9"

	"xiaodou/dai/internal/auth"
	"xiaodou/dai/internal/clientsecret"
	"xiaodou/dai/internal/config"
	"xiaodou/dai/internal/dbtest"
	tenantpg "xiaodou/dai/internal/tenant/pg"
)

func TestTenantOperationsTokenRequiresRecentAuthentication(t *testing.T) {
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

	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES ('operations-recent-tenant', 'Operations Recent Tenant', 'active');
		INSERT INTO iam_accounts (user_id, username, password_hash, user_type, status)
		VALUES ('operations-recent-admin', 'operations-recent-admin', 'unused', 2, 'active')
	`); err != nil {
		t.Fatal(err)
	}
	jwt := auth.NewJWTService(config.JWTConfig{
		Expiration: 15 * time.Minute, RefreshExpiration: time.Hour, Issuer: "dai-operations-recent-test",
	}, pool)
	sessions := auth.NewSessionService(pool, jwt, time.Hour)
	pair, err := sessions.Create(ctx, auth.Principal{
		UserID: "operations-recent-admin", Username: "operations-recent-admin",
		UserType: 2, UserTypeDisplay: "平台管理员", CredentialVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	recent := auth.NewRecentAuthService(redisClient)
	_, api := humatest.New(t)
	Register(api, NewPlatformAdminModule(PlatformAdminModuleDeps{
		Tenants: AdminTenantModuleDeps{
			AdminRouteAuthDeps: AdminRouteAuthDeps{
				PlatformAuthDeps: PlatformAuthDeps{JWT: jwt, RecentAuth: recent},
				RecentAuth:       recent,
			},
			TenantReader: tenantpg.NewTenantRepository(pool),
		},
	}))
	authorization := "Authorization: Bearer " + pair.AccessToken
	path := "/api/v1/tenants/operations-recent-tenant/operations-token"

	if response := api.Post(path, authorization); response.Code != http.StatusUnauthorized {
		t.Fatalf("operations token without recent auth status = %d, want 401; body=%s", response.Code, response.Body.String())
	}
	if err := recent.Mark(ctx, "operations-recent-admin", pair.SessionID, "password"); err != nil {
		t.Fatal(err)
	}
	if response := api.Post(path, authorization); response.Code != http.StatusOK {
		t.Fatalf("operations token with recent auth status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
}
