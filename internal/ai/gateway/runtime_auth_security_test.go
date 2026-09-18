package gateway

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/apikey"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/testsupport"
)

func TestResolveAPIKeyDoesNotTrustStaleRedisAuthorization(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := apikey.NewCache(client)

	const (
		tenantID = "runtime-cache-tenant"
		groupID  = "71000000-0000-0000-0000-000000000001"
		keyID    = "72000000-0000-0000-0000-000000000001"
		rawKey   = "sk-ai-stale-cache"
	)
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES ($1, 'Runtime Cache Tenant', 'active')
	`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_groups (id, tenant_id, name, retail_price_book_id)
		VALUES ($1::uuid, $2, 'runtime-cache-group', gen_random_uuid())
	`, groupID, tenantID); err != nil {
		t.Fatal(err)
	}
	keyHash := apikey.Hash(rawKey)
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_api_keys (id, owner_type, tenant_id, group_id, key_hash, key_ciphertext, name, status)
		VALUES ($1::uuid, 'tenant', $2, $3::uuid, $4, '', 'runtime-cache-key', 'active')
	`, keyID, tenantID, groupID, keyHash); err != nil {
		t.Fatal(err)
	}

	queries := dbgen.New(pool)
	activeRow, err := queries.GetAPIKeyByHash(ctx, keyHash)
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Set(ctx, keyHash, activeRow); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE ai_api_keys SET status = 'disabled' WHERE id = $1::uuid`, keyID); err != nil {
		t.Fatal(err)
	}

	g := New(Deps{Logger: zap.NewNop(), Postgres: pool, Queries: queries, APIKeyCache: cache})
	row, err := g.resolveAPIKey(ctx, keyHash)
	if err != nil {
		t.Fatalf("resolve disabled key: %v", err)
	}
	if row.Status != "disabled" {
		t.Fatalf("resolved stale cached status = %q, want disabled from PostgreSQL", row.Status)
	}

	newHash := apikey.Hash("sk-ai-rotated")
	if _, err := pool.Exec(ctx, `UPDATE ai_api_keys SET status = 'active', key_hash = $1 WHERE id = $2::uuid`, newHash, keyID); err != nil {
		t.Fatal(err)
	}
	if _, err := g.resolveAPIKey(ctx, keyHash); err == nil {
		t.Fatal("old key hash still resolved after database rotation")
	}
}

func TestResolveAPIKeyRequiresActiveDatabaseOwner(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	const (
		tenantID = "runtime-owner-tenant"
		userID   = "runtime-owner-user"
		groupID  = "73000000-0000-0000-0000-000000000001"
		keyID    = "74000000-0000-0000-0000-000000000001"
	)
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES ($1, 'Runtime Owner Tenant', 'active')
	`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, user_type, status)
		VALUES ($1, $2, 'runtime-owner-user', 'unused', 4, 'active')
	`, userID, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_groups (id, tenant_id, name, retail_price_book_id)
		VALUES ($1::uuid, $2, 'runtime-owner-group', gen_random_uuid())
	`, groupID, tenantID); err != nil {
		t.Fatal(err)
	}
	keyHash := apikey.Hash("sk-ai-owner")
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_api_keys (id, owner_type, tenant_id, user_id, group_id, key_hash, key_ciphertext, name, status)
		VALUES ($1::uuid, 'user', $2, $3, $4::uuid, $5, '', 'runtime-owner-key', 'active')
	`, keyID, tenantID, userID, groupID, keyHash); err != nil {
		t.Fatal(err)
	}

	g := New(Deps{Logger: zap.NewNop(), Postgres: pool, Queries: dbgen.New(pool)})
	if _, err := g.resolveAPIKey(ctx, keyHash); err != nil {
		t.Fatalf("active owner rejected: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE iam_accounts SET status = 'locked' WHERE user_id = $1`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := g.resolveAPIKey(ctx, keyHash); err == nil {
		t.Fatal("user API key resolved after owner account became non-active")
	}
	if _, err := pool.Exec(ctx, `UPDATE iam_accounts SET status = 'active' WHERE user_id = $1`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE iam_tenants SET status = 'suspended' WHERE tenant_id = $1`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := g.resolveAPIKey(ctx, keyHash); err == nil {
		t.Fatal("user API key resolved after owner tenant became non-active")
	}
}
