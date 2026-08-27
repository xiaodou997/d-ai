package auth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"xiaodou/dai/internal/dbtest"
)

func TestActivationLifecycle(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	service := NewActivationService(pool, time.Hour)
	principal, credential := seedPendingActivationAccount(t, ctx, pool, service, "activation-success")
	sessions := newTestSessionService(pool)
	if _, err := sessions.Create(ctx, principal); !errors.Is(err, ErrAccountInactive) {
		t.Fatalf("pending account session error = %v, want ErrAccountInactive", err)
	}

	password := "Correct-Horse-47"
	if err := service.Activate(ctx, credential.Token, password); err != nil {
		t.Fatalf("activate: %v", err)
	}
	var passwordHash, state string
	var version int64
	if err := pool.QueryRow(ctx, `
		SELECT password_hash, credential_state, credential_version
		FROM iam_accounts WHERE user_id = $1
	`, principal.UserID).Scan(&passwordHash, &state, &version); err != nil {
		t.Fatal(err)
	}
	if state != "active" || version != 2 || bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) != nil {
		t.Fatalf("activated account state=%q version=%d password_matches=%t", state, version, bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) == nil)
	}
	principal.CredentialVersion = version
	if _, err := sessions.Create(ctx, principal); err != nil {
		t.Fatalf("activated account session: %v", err)
	}
	if err := service.Activate(ctx, credential.Token, password); !errors.Is(err, ErrUsedActivationToken) {
		t.Fatalf("reused activation error = %v, want ErrUsedActivationToken", err)
	}
}

func TestActivationExpiryAndResetReplacement(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	service := NewActivationService(pool, time.Hour)
	_, expired := seedPendingActivationAccount(t, ctx, pool, service, "activation-expired")
	if _, err := pool.Exec(ctx, `UPDATE auth_activation_tokens SET expires_at = now() - interval '1 second' WHERE token_hash = $1`, expired.TokenHash); err != nil {
		t.Fatal(err)
	}
	if err := service.Activate(ctx, expired.Token, "Correct-Horse-47"); !errors.Is(err, ErrExpiredActivationToken) {
		t.Fatalf("expired activation error = %v, want ErrExpiredActivationToken", err)
	}

	principal := seedSessionAccount(t, ctx, pool, "activation-reset")
	sessions := newTestSessionService(pool)
	pair, err := sessions.Create(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Reset(ctx, principal.UserID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Reset(ctx, principal.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if first.Token == second.Token {
		t.Fatal("consecutive resets returned the same token")
	}
	if _, err := sessions.jwt.ParseToken(ctx, pair.AccessToken); !errors.Is(err, ErrSessionInactive) {
		t.Fatalf("access after reset = %v, want ErrSessionInactive", err)
	}
	if _, _, err := sessions.Rotate(ctx, pair.RefreshToken); !errors.Is(err, ErrSessionInactive) {
		t.Fatalf("refresh after reset = %v, want ErrSessionInactive", err)
	}
	if err := service.Activate(ctx, first.Token, "Correct-Horse-47"); !errors.Is(err, ErrUsedActivationToken) {
		t.Fatalf("superseded reset token error = %v, want ErrUsedActivationToken", err)
	}
	if err := service.Activate(ctx, second.Token, "Correct-Horse-47"); err != nil {
		t.Fatalf("latest reset token: %v", err)
	}
}

func TestDisabledAccountCanSetCredentialButCannotLogin(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	service := NewActivationService(pool, time.Hour)
	principal, credential := seedPendingActivationAccount(t, ctx, pool, service, "activation-disabled")
	if _, err := pool.Exec(ctx, `UPDATE iam_accounts SET status = 'disabled' WHERE user_id = $1`, principal.UserID); err != nil {
		t.Fatal(err)
	}
	if err := service.Activate(ctx, credential.Token, "Correct-Horse-47"); err != nil {
		t.Fatalf("disabled account activation: %v", err)
	}
	principal.CredentialVersion = 2
	if _, err := newTestSessionService(pool).Create(ctx, principal); !errors.Is(err, ErrAccountInactive) {
		t.Fatalf("disabled account session error = %v, want ErrAccountInactive", err)
	}
}

func TestConcurrentActivationHasSingleWinner(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	service := NewActivationService(pool, time.Hour)
	_, credential := seedPendingActivationAccount(t, ctx, pool, service, "activation-concurrent")
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- service.Activate(ctx, credential.Token, "Correct-Horse-47")
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, ErrUsedActivationToken) {
			t.Fatalf("unexpected concurrent activation result: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent activation successes = %d, want 1", successes)
	}
}

func TestConcurrentResetAndActivationDoNotDeadlock(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	service := NewActivationService(pool, time.Hour)
	principal, credential := seedPendingActivationAccount(t, ctx, pool, service, "activation-reset-race")
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		results <- service.Activate(ctx, credential.Token, "Correct-Horse-47")
	}()
	go func() {
		<-start
		_, err := service.Reset(ctx, principal.UserID)
		results <- err
	}()
	close(start)

	for range 2 {
		err := <-results
		if err != nil && !errors.Is(err, ErrUsedActivationToken) {
			t.Fatalf("concurrent reset/activation result: %v", err)
		}
	}
}

func TestActivationCleanupKeepsRecentReuseEvidence(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	service := NewActivationService(pool, time.Hour)
	_, credential := seedPendingActivationAccount(t, ctx, pool, service, "activation-cleanup")
	if err := service.Activate(ctx, credential.Token, "Correct-Horse-47"); err != nil {
		t.Fatal(err)
	}
	if deleted, err := service.DeleteExpired(ctx, 100); err != nil || deleted != 0 {
		t.Fatalf("recent cleanup deleted=%d error=%v", deleted, err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE auth_activation_tokens
		SET consumed_at = now() - interval '25 hours'
		WHERE token_hash = $1
	`, credential.TokenHash); err != nil {
		t.Fatal(err)
	}
	if deleted, err := service.DeleteExpired(ctx, 100); err != nil || deleted != 1 {
		t.Fatalf("old cleanup deleted=%d error=%v", deleted, err)
	}
}

func seedPendingActivationAccount(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	service *ActivationService,
	suffix string,
) (Principal, ActivationCredential) {
	t.Helper()
	credential, err := service.NewCredential()
	if err != nil {
		t.Fatal(err)
	}
	principal := Principal{UserID: "auth-" + suffix, Username: "user-" + suffix, UserType: 2, CredentialVersion: 1}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, username, password_hash, credential_state, user_type, status)
		VALUES ($1, $2, $3, 'pending_activation', 2, 'active')
	`, principal.UserID, principal.Username, credential.PasswordHash); err != nil {
		t.Fatal(err)
	}
	if err := service.Store(ctx, tx, principal.UserID, ActivationPurposeAccount, credential); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return principal, credential
}
