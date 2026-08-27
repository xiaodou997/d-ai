package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func newSecurityTestService(t *testing.T) (*AccountSecurityService, *miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	blacklist := NewBlacklistService(client, zap.NewNop())
	return NewAccountSecurityService(blacklist), mini, client
}

func TestAccountSecurityServiceSyncsUserAndTenantBanState(t *testing.T) {
	ctx := context.Background()
	security, mini, client := newSecurityTestService(t)

	if err := security.SyncUserStatus(ctx, "user-1", "disabled"); err != nil {
		t.Fatalf("disable user: %v", err)
	}
	if !mini.Exists(banUserPrefix+"user-1") || !mini.Exists("user:logout:user-1") {
		t.Fatal("disabled user did not receive ban and logout markers")
	}
	if err := security.SyncUserStatus(ctx, "user-1", "active"); err != nil {
		t.Fatalf("enable user: %v", err)
	}
	if mini.Exists(banUserPrefix + "user-1") {
		t.Fatal("enabled user retained ban marker")
	}

	if err := security.RevokeAccessToken(ctx, "token-1", time.Hour); err != nil {
		t.Fatalf("revoke token: %v", err)
	}
	if !mini.Exists(blacklistPrefix + "token-1") {
		t.Fatal("revoked token marker was not written")
	}

	if err := client.Set(ctx, banUserPrefix+"restored-1", "1", 0).Err(); err != nil {
		t.Fatal(err)
	}
	if err := client.Set(ctx, banUserPrefix+"untouched-1", "1", 0).Err(); err != nil {
		t.Fatal(err)
	}
	if err := security.SyncTenantStatus(ctx, "tenant-1", "disabled", nil); err != nil {
		t.Fatalf("disable tenant: %v", err)
	}
	if !mini.Exists(banTenantPrefix + "tenant-1") {
		t.Fatal("disabled tenant did not receive ban marker")
	}
	if err := security.SyncTenantStatus(ctx, "tenant-1", "active", []string{"restored-1"}); err != nil {
		t.Fatalf("enable tenant: %v", err)
	}
	if mini.Exists(banTenantPrefix+"tenant-1") || mini.Exists(banUserPrefix+"restored-1") {
		t.Fatal("tenant enable did not clear tenant/restored user markers")
	}
	if !mini.Exists(banUserPrefix + "untouched-1") {
		t.Fatal("tenant enable cleared an unrelated user marker")
	}
}

func TestAccountSecurityServiceHonorsCanceledContext(t *testing.T) {
	ctx := context.Background()
	security, _, _ := newSecurityTestService(t)
	canceled, cancel := context.WithCancel(ctx)
	cancel()

	if err := security.RevokeAccessToken(canceled, "token-canceled", time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("revoke canceled context error = %v, want context.Canceled", err)
	}
	if err := security.InvalidateUserSessions(canceled, "user-canceled"); !errors.Is(err, context.Canceled) {
		t.Fatalf("logout canceled context error = %v, want context.Canceled", err)
	}
	if err := security.SyncTenantStatus(canceled, "tenant-canceled", "active", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("tenant sync canceled context error = %v, want context.Canceled", err)
	}
}
