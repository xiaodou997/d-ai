package postgres

import (
	"context"
	"testing"

	"xiaodou/dai/internal/ai/testsupport"
)

func TestCanonicalSchemaRejectsCrossTenantGroupAuthorizations(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("canonical schema test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	const (
		tenantA = "tenant-a"
		tenantB = "tenant-b"
		keyA    = "81000000-0000-0000-0000-000000000001"
		groupA  = "82000000-0000-0000-0000-000000000001"
		groupB  = "82000000-0000-0000-0000-000000000002"
		bookID  = "83000000-0000-0000-0000-000000000001"
	)
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_groups (id, tenant_id, name, retail_price_book_id)
		VALUES
		  ($1::uuid, $2, 'group-a', $4::uuid),
		  ($3::uuid, $5, 'group-b', $4::uuid)
	`, groupA, tenantA, groupB, bookID, tenantB); err != nil {
		t.Fatalf("seed group ownership fixtures: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_api_keys (id, owner_type, tenant_id, group_id, key_hash, key_ciphertext, name)
		VALUES ($1::uuid, 'tenant', $2, $3::uuid, 'ownership-key-a', '', 'key-a')
	`, keyA, tenantA, groupA); err != nil {
		t.Fatalf("same-tenant API key group should succeed: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE ai_api_keys SET group_id = $1::uuid WHERE id = $2::uuid
	`, groupB, keyA); err == nil {
		t.Fatal("cross-tenant API key group should violate tenant/group ownership")
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_user_groups (tenant_id, user_id, group_id)
		VALUES ($1, 'user-a', $2::uuid)
	`, tenantA, groupB); err == nil {
		t.Fatal("cross-tenant user binding should violate tenant/group ownership")
	}
}
