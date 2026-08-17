package apikey

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"

	dbgen "xiaodou/dai/internal/ai/db/gen"
)

func TestCacheDelByIDInvalidatesQuotaSnapshot(t *testing.T) {
	addr := os.Getenv("DAI_TEST_REDIS_ADDR")
	if addr == "" {
		// Same default as the local compose stack, so the test runs with the
		// dependencies a developer already has up rather than needing a flag
		// nobody remembers to set.
		addr = "127.0.0.1:16379"
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("cannot reach Redis: %v", err)
	}
	id := uuid.New()
	pgID := pgtype.UUID{Bytes: id, Valid: true}
	keyHash := "hash-" + id.String()
	cache := NewCache(client)
	t.Cleanup(func() {
		_ = client.Del(ctx, cacheKeyPrefix+keyHash, cacheIDPrefix+id.String()).Err()
	})
	if err := cache.Set(ctx, keyHash, dbgen.GetAPIKeyByHashRow{ID: pgID, QuotaUsed: 99}); err != nil {
		t.Fatalf("set cache: %v", err)
	}
	if row, ok := cache.Get(ctx, keyHash); !ok || row.QuotaUsed != 99 {
		t.Fatalf("cache read = %#v, %v", row, ok)
	}
	if err := cache.DelByID(ctx, id.String()); err != nil {
		t.Fatalf("delete by id: %v", err)
	}
	if _, ok := cache.Get(ctx, keyHash); ok {
		t.Fatal("quota-bearing cache entry survived DelByID")
	}
}
