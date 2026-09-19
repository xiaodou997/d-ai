package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	"xiaodou/dai/internal/clientsecret"
	"xiaodou/dai/internal/config"
	"xiaodou/dai/internal/dbtest"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSignDetachedUsesActiveRS256Key(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	service := &JWTService{activeKey: &keyEntry{
		kid: "webhook-test-kid", privateKey: privateKey, publicKey: &privateKey.PublicKey,
	}}
	message := []byte("D-AI-WEBHOOK-V1\n1700000000\ndelivery-1\n1\n{}")
	kid, signature, err := service.SignDetached(message)
	if err != nil {
		t.Fatalf("SignDetached: %v", err)
	}
	if kid != "webhook-test-kid" {
		t.Fatalf("kid = %q", kid)
	}
	digest := sha256.Sum256(message)
	if err := rsa.VerifyPKCS1v15(&privateKey.PublicKey, crypto.SHA256, digest[:], signature); err != nil {
		t.Fatalf("verify detached signature: %v", err)
	}
}

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

func TestConcurrentRotateKeyPreservesSingleActiveKeyAcrossReplicas(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("JWT test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	replicaPool := openJWTReplicaPool(t, ctx, pool)
	t.Cleanup(replicaPool.Close)

	if err := clientsecret.Configure("0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatal(err)
	}

	cfg := config.JWTConfig{Expiration: 15 * time.Minute, RefreshExpiration: time.Hour, Issuer: "dai-jwt-concurrent-rotation"}
	first := NewJWTService(cfg, pool)
	second := NewJWTService(cfg, replicaPool)

	// Keep the first rotation transaction open after it inserts its replacement
	// key. Without PostgreSQL-level rotation serialization this gives the second
	// replica a deterministic window to overlap the first transaction.
	if _, err := pool.Exec(ctx, `
		CREATE FUNCTION jwt_rotation_overlap_delay() RETURNS trigger
		LANGUAGE plpgsql AS $$
		BEGIN
			PERFORM pg_sleep(1);
			RETURN NEW;
		END
		$$;
		CREATE TRIGGER jwt_rotation_overlap_delay
		AFTER INSERT ON auth_signing_keys
		FOR EACH ROW
		WHEN (NEW.status = 'active')
		EXECUTE FUNCTION jwt_rotation_overlap_delay()
	`); err != nil {
		t.Fatalf("install rotation overlap trigger: %v", err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	for _, service := range []*JWTService{first, second} {
		go func(service *JWTService) {
			<-start
			results <- service.RotateKey(ctx)
		}(service)
	}
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("concurrent RotateKey failed: %v", err)
		}
	}

	var active, grace, retired int
	if err := pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status = 'active'),
			COUNT(*) FILTER (WHERE status = 'grace'),
			COUNT(*) FILTER (WHERE status = 'retired')
		FROM auth_signing_keys
	`).Scan(&active, &grace, &retired); err != nil {
		t.Fatal(err)
	}
	if active != 1 || grace != 2 || retired != 0 {
		t.Fatalf("signing key states active/grace/retired = %d/%d/%d, want 1/2/0", active, grace, retired)
	}

	if err := first.reloadKeys(ctx); err != nil {
		t.Fatalf("reload first replica: %v", err)
	}
	if err := second.reloadKeys(ctx); err != nil {
		t.Fatalf("reload second replica: %v", err)
	}
	first.mu.RLock()
	firstKid := first.activeKey.kid
	first.mu.RUnlock()
	second.mu.RLock()
	secondKid := second.activeKey.kid
	second.mu.RUnlock()
	if firstKid != secondKid {
		t.Fatalf("replica active kids differ after reload: %q != %q", firstKid, secondKid)
	}

	for _, pair := range []struct {
		name     string
		signer   *JWTService
		verifier *JWTService
	}{
		{name: "first-to-second", signer: first, verifier: second},
		{name: "second-to-first", signer: second, verifier: first},
	} {
		t.Run(pair.name, func(t *testing.T) {
			now := time.Now()
			token, err := pair.signer.signClaims(Claims{
				PrincipalType: "rotation-invariant-test",
				RegisteredClaims: jwt.RegisteredClaims{
					ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
					IssuedAt:  jwt.NewNumericDate(now),
					Issuer:    cfg.Issuer,
				},
			})
			if err != nil {
				t.Fatalf("sign token: %v", err)
			}
			claims, err := pair.verifier.ParseToken(ctx, token)
			if err != nil {
				t.Fatalf("cross-replica verify: %v", err)
			}
			if claims.PrincipalType != "rotation-invariant-test" {
				t.Fatalf("principal type = %q, want rotation-invariant-test", claims.PrincipalType)
			}
		})
	}
}

func TestReloadKeysRejectsInvalidActiveKeyCount(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("JWT test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	if err := clientsecret.Configure("0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatal(err)
	}

	service := NewJWTService(config.JWTConfig{Expiration: 15 * time.Minute, RefreshExpiration: time.Hour, Issuer: "dai-jwt-active-invariant"}, pool)
	service.mu.RLock()
	originalKid := service.activeKey.kid
	service.mu.RUnlock()

	if _, err := pool.Exec(ctx, `
		UPDATE auth_signing_keys
		SET status = 'grace', grace_until = now() + interval '1 hour'
		WHERE status = 'active'
	`); err != nil {
		t.Fatal(err)
	}
	if err := service.reloadKeys(ctx); err == nil || !strings.Contains(err.Error(), "found 0") {
		t.Fatalf("reloadKeys() with zero active keys error = %v, want invariant violation", err)
	}
	if _, err := service.signClaims(Claims{}); err == nil {
		t.Fatal("service kept signing after zero-active invariant violation")
	}

	if _, err := pool.Exec(ctx, `
		UPDATE auth_signing_keys
		SET status = 'active', grace_until = NULL
		WHERE kid = $1
	`, originalKid); err != nil {
		t.Fatal(err)
	}
	if err := service.reloadKeys(ctx); err != nil {
		t.Fatalf("restore valid active key: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		DROP INDEX ux_auth_signing_keys_single_active;
		INSERT INTO auth_signing_keys (kid, private_key, public_key, status, created_at)
		SELECT kid || '-duplicate', private_key, public_key, 'active', created_at + interval '1 microsecond'
		FROM auth_signing_keys
		WHERE status = 'active'
	`); err != nil {
		t.Fatalf("prepare duplicate active keys: %v", err)
	}
	if err := service.reloadKeys(ctx); err == nil || !strings.Contains(err.Error(), "found 2") {
		t.Fatalf("reloadKeys() with two active keys error = %v, want invariant violation", err)
	}
	if _, err := service.signClaims(Claims{}); err == nil {
		t.Fatal("service kept signing after duplicate-active invariant violation")
	}
}

func openJWTReplicaPool(t *testing.T, ctx context.Context, source *pgxpool.Pool) *pgxpool.Pool {
	t.Helper()
	cfg := source.Config().Copy()
	cfg.MaxConns = 4
	replica, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("open independent JWT replica pool: %v", err)
	}
	if err := replica.Ping(ctx); err != nil {
		replica.Close()
		t.Fatalf("ping independent JWT replica pool: %v", err)
	}
	return replica
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

	// Prove that the second forged kid does not touch PostgreSQL. Once the
	// first lookup has populated the throttle timestamp, close the pool: a
	// second DB reload would now fail with a pool-closed error instead of the
	// normal unknown-kid result.
	pool.Close()
	_, err = service.verificationKey(ctx, "forged-kid-2")
	if err == nil {
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