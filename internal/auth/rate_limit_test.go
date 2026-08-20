package auth

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestLoginRateLimiterProgressiveBackoffAndReset(t *testing.T) {
	mini, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mini.Close)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	limiter := NewLoginRateLimiter(client)
	dimensions := LoginRateDimensions{Account: "admin@example.test", IP: "192.0.2.10", Tenant: "tenant-a"}
	for i := 0; i < rateLimitThreshold-1; i++ {
		if retry, err := limiter.RecordFailure(context.Background(), dimensions); err != nil || retry != 0 {
			t.Fatalf("failure %d retry=%s err=%v", i, retry, err)
		}
	}
	if retry, err := limiter.RecordFailure(context.Background(), dimensions); err != nil || retry <= 0 {
		t.Fatalf("threshold failure retry=%s err=%v", retry, err)
	}
	decision, err := limiter.Check(context.Background(), dimensions)
	if err != nil || decision.Allowed || decision.RetryAfter <= 0 {
		t.Fatalf("blocked decision=%+v err=%v", decision, err)
	}
	if err := limiter.Reset(context.Background(), dimensions); err != nil {
		t.Fatal(err)
	}
	decision, err = limiter.Check(context.Background(), dimensions)
	if err != nil || !decision.Allowed {
		t.Fatalf("reset decision=%+v err=%v", decision, err)
	}
}

func TestLoginRateLimiterFailsClosedWithoutRedis(t *testing.T) {
	decision, err := NewLoginRateLimiter(nil).Check(context.Background(), LoginRateDimensions{Account: "a", IP: "b"})
	if err == nil || decision.Allowed {
		t.Fatalf("decision=%+v err=%v, want unavailable", decision, err)
	}
}

func TestLoginRateLimiterSharesStateAcrossInstances(t *testing.T) {
	mini, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mini.Close)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	first := NewLoginRateLimiter(client)
	second := NewLoginRateLimiter(client)
	dimensions := LoginRateDimensions{Account: "admin@example.test", IP: "192.0.2.11"}
	for i := 0; i < rateLimitThreshold; i++ {
		if _, err := first.RecordFailure(context.Background(), dimensions); err != nil {
			t.Fatal(err)
		}
	}
	decision, err := second.Check(context.Background(), dimensions)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || decision.RetryAfter <= 0 {
		t.Fatalf("second limiter decision=%+v, want shared block", decision)
	}
}

func TestVerifyTOTPAcceptsAdjacentTimeWindow(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP"
	now := time.Unix(1_700_000_000, 0).UTC()
	if !VerifyTOTP(secret, totpCode(secret, uint64(now.Unix()/30)), now) {
		t.Fatal("current TOTP code was rejected")
	}
	if !VerifyTOTP(secret, totpCode(secret, uint64(now.Unix()/30+1)), now) {
		t.Fatal("adjacent TOTP code was rejected")
	}
	if VerifyTOTP(secret, "000000", now) {
		t.Fatal("invalid TOTP code was accepted")
	}
}
