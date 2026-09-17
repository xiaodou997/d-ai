package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"xiaodou/dai/internal/clientsecret"
	"xiaodou/dai/internal/config"
	"xiaodou/dai/internal/dbtest"

	"github.com/golang-jwt/jwt/v5"
)

func TestParseTokenReloadsSigningKeysAcrossReplicas(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("JWT test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	if err := clientsecret.Configure("0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatal(err)
	}

	cfg := config.JWTConfig{Expiration: 15 * time.Minute, RefreshExpiration: time.Hour, Issuer: "dai-jwt-cross-replica-rotation"}
	rotatingReplica := NewJWTService(cfg, pool)
	staleReplica := NewJWTService(cfg, pool)

	if err := rotatingReplica.RotateKey(ctx); err != nil {
		t.Fatalf("rotate signing key: %v", err)
	}

	rotatingReplica.mu.RLock()
	rotatedKid := rotatingReplica.activeKey.kid
	rotatingReplica.mu.RUnlock()
	if staleReplica.cachedPublicKey(rotatedKid) != nil {
		t.Fatalf("stale replica unexpectedly knows rotated kid %q before parsing", rotatedKid)
	}

	now := time.Now()
	token, err := rotatingReplica.signClaims(Claims{
		PrincipalType: "cross-replica-test",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    cfg.Issuer,
		},
	})
	if err != nil {
		t.Fatalf("sign token with rotated key: %v", err)
	}

	claims, err := staleReplica.ParseToken(ctx, token)
	if err != nil {
		t.Fatalf("stale replica rejected token signed by rotated key: %v", err)
	}
	if claims.PrincipalType != "cross-replica-test" {
		t.Fatalf("parsed principal type = %q, want cross-replica-test", claims.PrincipalType)
	}
	if staleReplica.cachedPublicKey(rotatedKid) == nil {
		t.Fatalf("stale replica did not cache rotated kid %q after parsing", rotatedKid)
	}
}

func TestVerificationKeyRateLimitsUnknownKidReloads(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("JWT test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	if err := clientsecret.Configure("0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatal(err)
	}

	service := NewJWTService(config.JWTConfig{Expiration: 15 * time.Minute, RefreshExpiration: time.Hour, Issuer: "dai-jwt-unknown-kid-throttle"}, pool)
	if _, err := service.verificationKey(ctx, "forged-kid-1"); err == nil {
		t.Fatal("verificationKey(forged-kid-1) unexpectedly succeeded")
	}
	firstRefresh := service.lastUnknownKidRefresh
	if firstRefresh.IsZero() {
		t.Fatal("first unknown kid did not record a refresh attempt")
	}

	if _, err := service.verificationKey(ctx, "forged-kid-2"); err == nil {
		t.Fatal("verificationKey(forged-kid-2) unexpectedly succeeded")
	}
	if !service.lastUnknownKidRefresh.Equal(firstRefresh) {
		t.Fatalf("second unknown kid moved refresh timestamp from %v to %v inside throttle interval", firstRefresh, service.lastUnknownKidRefresh)
	}
}

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
	if err := first.RotateKey(ctx); err != nil {
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

func TestJWTKeyManagementHonorsCanceledContext(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("JWT test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	if err := clientsecret.Configure("0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatal(err)
	}

	service := NewJWTService(config.JWTConfig{Expiration: 15 * time.Minute, RefreshExpiration: time.Hour, Issuer: "dai-jwt-key-context"}, pool)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := service.ListKeys(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("ListKeys() error = %v, want context.Canceled", err)
	}
	if err := service.RotateKey(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("RotateKey() error = %v, want context.Canceled", err)
	}
}
