package auth

import (
	"context"
	"testing"
	"time"

	"xiaodou/dai/internal/clientsecret"
	"xiaodou/dai/internal/config"
	"xiaodou/dai/internal/dbtest"
)

func TestRetireExpiredGraceKeysReloadsEveryReplica(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("JWT test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	if err := clientsecret.Configure("0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatal(err)
	}

	cfg := config.JWTConfig{Expiration: 15 * time.Minute, RefreshExpiration: time.Hour, Issuer: "dai-jwt-retire-test"}
	first := NewJWTService(cfg, pool)
	if err := first.RotateKey(); err != nil {
		t.Fatalf("rotate signing key: %v", err)
	}
	second := NewJWTService(cfg, pool)
	if got := len(second.GetJWKS().Keys); got != 2 {
		t.Fatalf("replica JWKS keys before retirement = %d, want 2", got)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE auth_signing_keys
		SET grace_until = $1
		WHERE status = 'grace'
	`, time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatalf("expire grace key: %v", err)
	}
	if err := first.RetireExpiredGraceKeys(ctx); err != nil {
		t.Fatalf("retire expired grace key on first replica: %v", err)
	}
	// The second replica updates zero rows, but must still reload and evict its
	// stale in-memory grace key after the first replica changed the database.
	if err := second.RetireExpiredGraceKeys(ctx); err != nil {
		t.Fatalf("refresh second replica after retirement: %v", err)
	}
	if got := len(second.GetJWKS().Keys); got != 1 {
		t.Fatalf("replica JWKS keys after retirement = %d, want 1", got)
	}

	var retired int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM auth_signing_keys WHERE status = 'retired'`).Scan(&retired); err != nil {
		t.Fatal(err)
	}
	if retired != 1 {
		t.Fatalf("retired signing keys = %d, want 1", retired)
	}
}
