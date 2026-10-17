package routing

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"xiaodou/dai/internal/ai/domain"
)

func TestCredentialQuotaObservationsAreOrderedAndScoped(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	runtime := NewRedisAvailability(client)
	ctx := context.Background()
	now := time.Now()
	used := 100.0
	quota := domain.CredentialQuota{Model: "model-a", ObservedAt: now.UnixMilli(), Source: "response_headers", Windows: []domain.CredentialQuotaWindow{{Key: "requests", UsedPercent: &used, ResetAt: now.Add(time.Hour).UnixMilli()}}}
	if err := runtime.ObserveCredentialQuota(ctx, "credential-a", quota); err != nil {
		t.Fatal(err)
	}
	if allowed, _, err := runtime.CredentialQuotaEligibility(ctx, "credential-a", "model-a"); err != nil || allowed {
		t.Fatalf("exhausted model allowed=%v err=%v", allowed, err)
	}
	for _, key := range [][2]string{{"credential-a", "model-b"}, {"credential-b", "model-a"}} {
		if allowed, _, err := runtime.CredentialQuotaEligibility(ctx, key[0], key[1]); err != nil || !allowed {
			t.Fatalf("unrelated account/model blocked: %v", err)
		}
	}
	// Late observations cannot undo a newer exhaustion snapshot.
	quota.ObservedAt--
	quota.Windows = nil
	if err := runtime.ObserveCredentialQuota(ctx, "credential-a", quota); err != nil {
		t.Fatal(err)
	}
	if allowed, _, _ := runtime.CredentialQuotaEligibility(ctx, "credential-a", "model-a"); allowed {
		t.Fatal("late observation replaced current quota")
	}
	quota.ObservedAt += 2
	quota.Windows = []domain.CredentialQuotaWindow{{Key: "requests", UsedPercent: &used, ResetAt: now.Add(-time.Second).UnixMilli()}}
	if err := runtime.ObserveCredentialQuota(ctx, "credential-a", quota); err != nil {
		t.Fatal(err)
	}
	if allowed, _, _ := runtime.CredentialQuotaEligibility(ctx, "credential-a", "model-a"); !allowed {
		t.Fatal("reset quota did not recover")
	}
}
