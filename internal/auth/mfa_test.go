package auth

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"xiaodou/dai/internal/clientsecret"
	"xiaodou/dai/internal/dbtest"
)

func TestMFAChallengeIsConsumedByOnlyOneConcurrentVerifier(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	if err := clientsecret.Configure("0123456789abcdef"); err != nil {
		t.Fatal(err)
	}

	mini, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mini.Close)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	secret := "JBSWY3DPEHPK3PXP"
	encrypted, err := clientsecret.Encrypt(secret)
	if err != nil {
		t.Fatal(err)
	}
	principal := Principal{UserID: "mfa-concurrent", Username: "mfa-admin", UserType: 2, CredentialVersion: 1}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, username, password_hash, user_type, status, mfa_secret_encrypted, mfa_enabled)
		VALUES ($1, $2, '$2a$10$VI.y0TjcNQQX/5X/ukr7xOMmmRPfAEnrFs9fnJhkEajX6JPl43JXS', 2, 'active', $3, TRUE)
	`, principal.UserID, principal.Username, encrypted); err != nil {
		t.Fatal(err)
	}

	service := NewMFAService(pool, redisClient)
	token, err := service.CreateChallenge(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	code := totpCode(secret, uint64(time.Now().Unix()/30))
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, verifyErr := service.VerifyChallenge(ctx, token, code)
			results <- verifyErr
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	successes := 0
	for verifyErr := range results {
		if verifyErr == nil {
			successes++
		} else if verifyErr != ErrInvalidMFACode {
			t.Fatalf("unexpected concurrent MFA result: %v", verifyErr)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent MFA successes = %d, want 1", successes)
	}
}
