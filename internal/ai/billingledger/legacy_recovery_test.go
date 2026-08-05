package billingledger

import (
	"context"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestEnsureLegacyUsageRecoveryDrained(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()

	if err := EnsureLegacyUsageRecoveryDrained(ctx, client); err != nil {
		t.Fatalf("empty legacy stream: %v", err)
	}
	if _, err := client.XAdd(ctx, &redis.XAddArgs{
		Stream: LegacyUsageRecoveryStream,
		Values: map[string]any{"payload": `{"version":1}`},
	}).Result(); err != nil {
		t.Fatalf("seed legacy stream: %v", err)
	}
	count, err := LegacyUsageRecoveryEntries(ctx, client)
	if err != nil {
		t.Fatalf("inspect legacy stream: %v", err)
	}
	if count != 1 {
		t.Fatalf("legacy stream count = %d, want 1", count)
	}
	err = EnsureLegacyUsageRecoveryDrained(ctx, client)
	if err == nil || !strings.Contains(err.Error(), "V2 AI recovery worker") {
		t.Fatalf("expected actionable cutover error, got %v", err)
	}
}
