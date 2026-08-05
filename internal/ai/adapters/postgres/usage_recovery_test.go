package postgres

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/billingledger"
	dbgen "xiaodou/dai/internal/ai/db/gen"
)

func TestUsageRecoveryPayloadJSONRoundTrip(t *testing.T) {
	numeric := floatToNumeric(1.25)
	payload := usageRecoveryPayload{
		Version:   2,
		RequestID: "request-1",
		LeaseID:   "lease-1",
		Usage: dbgen.CreateUsageLogParams{
			RequestID:                          "request-1",
			GroupDefaultUserMultiplierSnapshot: numeric,
			BillingBreakdown:                   []byte(`{"cost_micro":125}`),
		},
		Rollup:      dbgen.UpsertUsageRollupHourlyParams{TenantID: "tenant-1"},
		APIKeyID:    pgtype.UUID{Bytes: [16]byte{1}, Valid: true},
		TenantMicro: 125,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	var decoded usageRecoveryPayload
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if decoded.RequestID != payload.RequestID || decoded.TenantMicro != 125 || decoded.Rollup.TenantID != "tenant-1" {
		t.Fatalf("decoded payload mismatch: %#v", decoded)
	}
	if !decoded.Usage.GroupDefaultUserMultiplierSnapshot.Valid || decoded.Usage.GroupDefaultUserMultiplierSnapshot.Int == nil {
		t.Fatalf("numeric snapshot was not preserved: %#v", decoded.Usage.GroupDefaultUserMultiplierSnapshot)
	}
	if string(decoded.Usage.BillingBreakdown) != string(payload.Usage.BillingBreakdown) {
		t.Fatalf("billing breakdown mismatch: %q", decoded.Usage.BillingBreakdown)
	}
}

func TestUsageRecoveryStreamDoesNotEvictPendingEntries(t *testing.T) {
	args := newUsageRecoveryXAddArgs(`{"request_id":"request-1"}`)
	if args.Stream != usageRecoveryStream {
		t.Fatalf("stream = %q", args.Stream)
	}
	if args.MaxLen != 0 || args.Approx {
		t.Fatalf("recovery stream must not trim unresolved entries: %#v", args)
	}
	if args.Stream != billingledger.UsageRecoveryStream {
		t.Fatalf("V3 payloads must use the isolated V3 recovery stream, got %q", args.Stream)
	}
}

func TestQuarantineRecoveryEntryMovesMessageAtomically(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()

	id, err := client.XAdd(ctx, &redis.XAddArgs{
		Stream: usageRecoveryStream,
		Values: map[string]any{"payload": `{"version":999}`},
	}).Result()
	if err != nil {
		t.Fatalf("seed recovery stream: %v", err)
	}
	logger := &UsageLogger{recoveryRedis: client, logger: zap.NewNop()}
	if err := logger.quarantineRecoveryEntry(ctx, redis.XMessage{
		ID: id, Values: map[string]any{"payload": `{"version":999}`},
	}, "unsupported version"); err != nil {
		t.Fatalf("quarantine recovery entry: %v", err)
	}
	if count := client.XLen(ctx, usageRecoveryStream).Val(); count != 0 {
		t.Fatalf("source stream count = %d, want 0", count)
	}
	if count := client.XLen(ctx, usageRecoveryQuarantineStream).Val(); count != 1 {
		t.Fatalf("quarantine stream count = %d, want 1", count)
	}
	entries := client.XRange(ctx, usageRecoveryQuarantineStream, "-", "+").Val()
	if len(entries) != 1 || entries[0].Values["source_id"] != id {
		t.Fatalf("unexpected quarantine envelope: %#v", entries)
	}
}
