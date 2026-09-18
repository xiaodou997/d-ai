package transport

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"xiaodou/dai/internal/auth"
	"xiaodou/dai/internal/clientsecret"
	"xiaodou/dai/internal/config"
	"xiaodou/dai/internal/dbtest"
	"xiaodou/dai/libs/go/httpx"
)

func TestMFASettingsRequireRecentAuthentication(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	if err := clientsecret.Configure("0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatal(err)
	}

	mini, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mini.Close)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	principal := auth.Principal{
		UserID: "mfa-settings-admin", Username: "mfa-settings-admin",
		UserType: 2, UserTypeDisplay: "平台管理员", CredentialVersion: 1,
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, username, password_hash, user_type, status)
		VALUES ($1, $2, 'unused', 2, 'active')
	`, principal.UserID, principal.Username); err != nil {
		t.Fatal(err)
	}
	jwt := auth.NewJWTService(config.JWTConfig{Expiration: 15 * time.Minute, RefreshExpiration: time.Hour, Issuer: "dai-mfa-settings-test"}, pool)
	sessions := auth.NewSessionService(pool, jwt, time.Hour)
	pair, err := sessions.Create(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	recent := auth.NewRecentAuthService(redisClient)
	mfa := auth.NewMFAService(pool, redisClient)
	d := authModule{
		platformAuthDeps: platformAuthDeps{JWT: jwt, RecentAuth: recent},
		Sessions:         sessions, MFA: mfa, RecentAuth: recent,
		AuthRateLimiters: auth.NewRateLimiters(redisClient),
		Logger:           zap.NewNop(),
	}

	_, api := humatest.New(t)
	registerAuthProtected(api, d, huma.Middlewares{userAuth(api, jwt, nil)})
	authorization := "Authorization: Bearer " + pair.AccessToken

	response := api.Post("/api/auth/mfa/enroll", authorization)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("MFA enroll without recent auth status = %d, want 401; body=%s", response.Code, response.Body.String())
	}
	var encrypted string
	if err := pool.QueryRow(ctx, `
		SELECT COALESCE(mfa_secret_encrypted, '') FROM iam_accounts WHERE user_id = $1
	`, principal.UserID).Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if encrypted != "" {
		t.Fatal("MFA enroll mutated the account before recent authentication")
	}

	if err := recent.Mark(ctx, principal.UserID, pair.SessionID, "password"); err != nil {
		t.Fatal(err)
	}
	response = api.Post("/api/auth/mfa/enroll", authorization)
	if response.Code != http.StatusOK {
		t.Fatalf("MFA enroll with recent auth status = %d, want 200; body=%s", response.Code, response.Body.String())
	}

	if err := redisClient.FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	response = api.Post("/api/auth/mfa/confirm", authorization, map[string]any{"code": "abcdef"})
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("MFA confirm without recent auth status = %d, want 401; body=%s", response.Code, response.Body.String())
	}
	for _, key := range mini.Keys() {
		if strings.HasPrefix(key, "dai:auth:mfa-confirm:") {
			t.Fatalf("MFA confirm handler ran before recent-auth middleware: key=%q", key)
		}
	}
}

func TestMFAVerifyRateLimitAggregatesAcrossChallengesAndIPs(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	if err := clientsecret.Configure("0123456789abcdef0123456789abcdef"); err != nil {
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
	principal := auth.Principal{UserID: "mfa-rate-admin", Username: "mfa-rate-admin", UserType: 2, CredentialVersion: 1}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, username, password_hash, user_type, status, mfa_secret_encrypted, mfa_enabled)
		VALUES ($1, $2, 'unused', 2, 'active', $3, TRUE)
	`, principal.UserID, principal.Username, encrypted); err != nil {
		t.Fatal(err)
	}
	mfa := auth.NewMFAService(pool, redisClient)
	first, err := mfa.CreateChallenge(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	second, err := mfa.CreateChallenge(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	handler := &authHandlers{mfa: mfa, mfaLimiter: auth.NewRateLimiters(redisClient).MFA}

	tokens := []string{first, first, first, first, second}
	for i, token := range tokens {
		attemptCtx := context.WithValue(ctx, requestClientIPCtxKey, fmt.Sprintf("192.0.2.%d", i+1))
		input := &mfaVerifyInput{}
		input.Body.ChallengeToken = token
		input.Body.Code = "abcdef"
		_, err := handler.verifyMFA(attemptCtx, input)
		var appErr *httpx.AppError
		if !errors.As(err, &appErr) {
			t.Fatalf("MFA attempt %d error = %v, want *httpx.AppError", i+1, err)
		}
		wantStatus := http.StatusUnauthorized
		if i == len(tokens)-1 {
			wantStatus = http.StatusTooManyRequests
		}
		if appErr.Status != wantStatus {
			t.Fatalf("MFA attempt %d status = %d, want %d", i+1, appErr.Status, wantStatus)
		}
	}
}
