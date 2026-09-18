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

func TestMFAEnrollmentRevokesPreMFAFamily(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	principal := seedSessionAccount(t, ctx, pool, "mfa-enrollment-session")
	sessions := newTestSessionService(pool)
	pair, err := sessions.Create(ctx, principal)
	if err != nil {
		t.Fatalf("create pre-MFA session: %v", err)
	}

	mfa := NewMFAService(pool, nil)
	enrollment, err := mfa.Enroll(ctx, principal.UserID, principal.Username)
	if err != nil {
		t.Fatalf("enroll MFA: %v", err)
	}
	code := totpCode(enrollment.Secret, uint64(time.Now().Unix()/30))
	if err := mfa.ConfirmEnrollment(ctx, principal.UserID, code); err != nil {
		t.Fatalf("confirm MFA: %v", err)
	}

	var credentialVersion int64
	var enabled bool
	if err := pool.QueryRow(ctx, `
		SELECT credential_version, mfa_enabled
		FROM iam_accounts
		WHERE user_id = $1
	`, principal.UserID).Scan(&credentialVersion, &enabled); err != nil {
		t.Fatal(err)
	}
	if !enabled || credentialVersion != principal.CredentialVersion+1 {
		t.Fatalf("MFA state/version = enabled:%v version:%d, want true/%d", enabled, credentialVersion, principal.CredentialVersion+1)
	}
	if _, err := sessions.jwt.ParseToken(ctx, pair.AccessToken); err != ErrSessionInactive {
		t.Fatalf("pre-MFA access token after enrollment = %v, want ErrSessionInactive", err)
	}
	if _, _, err := sessions.Rotate(ctx, pair.RefreshToken); err != ErrSessionInactive {
		t.Fatalf("pre-MFA refresh token after enrollment = %v, want ErrSessionInactive", err)
	}
}

func TestMFAConcurrentEnrollKeepsOnePendingSecret(t *testing.T) {
	ctx := context.Background()
	const schemaSQL = `
		CREATE TABLE iam_accounts (
			user_id TEXT PRIMARY KEY,
			username TEXT NOT NULL,
			password_hash TEXT NOT NULL,
			user_type INTEGER NOT NULL,
			status TEXT NOT NULL,
			mfa_secret_encrypted TEXT,
			mfa_enabled BOOLEAN NOT NULL DEFAULT FALSE,
			mfa_enrolled_at TIMESTAMPTZ,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
	`
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2, SchemaSQL: schemaSQL})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	if err := clientsecret.Configure("0123456789abcdef"); err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, username, password_hash, user_type, status)
		VALUES ('mfa-enroll-race', 'mfa-enroll-admin', 'unused', 2, 'active');

		CREATE FUNCTION test_mfa_enroll_delay() RETURNS trigger
		LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.mfa_secret_encrypted IS DISTINCT FROM OLD.mfa_secret_encrypted THEN
				PERFORM pg_sleep(0.5);
			END IF;
			RETURN NEW;
		END
		$$;

		CREATE TRIGGER test_mfa_enroll_delay
		BEFORE UPDATE ON iam_accounts
		FOR EACH ROW
		EXECUTE FUNCTION test_mfa_enroll_delay()
	`); err != nil {
		t.Fatal(err)
	}

	service := NewMFAService(pool, nil)
	start := make(chan struct{})
	results := make(chan MFAEnrollment, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			enrollment, enrollErr := service.Enroll(ctx, "mfa-enroll-race", "mfa-enroll-admin")
			results <- enrollment
			errs <- enrollErr
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)

	for enrollErr := range errs {
		if enrollErr != nil {
			t.Fatalf("concurrent enroll failed: %v", enrollErr)
		}
	}
	var enrollments []MFAEnrollment
	for enrollment := range results {
		enrollments = append(enrollments, enrollment)
	}
	if len(enrollments) != 2 || enrollments[0].Secret == "" || enrollments[0].Secret != enrollments[1].Secret {
		t.Fatalf("concurrent enrollments = %#v, want the same non-empty pending secret", enrollments)
	}

	code := totpCode(enrollments[0].Secret, uint64(time.Now().Unix()/30))
	if err := service.ConfirmEnrollment(ctx, "mfa-enroll-race", code); err != nil {
		t.Fatalf("confirm concurrent enrollment: %v", err)
	}
	enabled, err := service.Enabled(ctx, "mfa-enroll-race")
	if err != nil || !enabled {
		t.Fatalf("MFA enabled after confirmation = %v, err=%v", enabled, err)
	}
	if _, err := service.Enroll(ctx, "mfa-enroll-race", "mfa-enroll-admin"); err != ErrMFAAlreadyEnabled {
		t.Fatalf("enroll after confirmation error = %v, want ErrMFAAlreadyEnabled", err)
	}
}
