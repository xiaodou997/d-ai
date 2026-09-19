package postgres

import (
	"context"
	"testing"
	"time"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/dbtest"
)

func TestAPIKeyMetadataUpdateDoesNotReactivateDisabledKey(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	const (
		tenantID    = "api-key-preserve-tenant"
		priceBookID = "47000000-0000-0000-0000-000000000001"
		groupID     = "47000000-0000-0000-0000-000000000002"
		keyID        = "47000000-0000-0000-0000-000000000003"
		expiredKeyID = "47000000-0000-0000-0000-000000000004"
	)
	if _, err := pool.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, status)
		VALUES ($1, 'API Key Preserve Tenant', 'active')
	`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_price_books (id, owner_type, name, status)
		VALUES ($1::uuid, 'platform', 'API Key Preserve Book', 'active')
	`, priceBookID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_groups (id, tenant_id, name, retail_price_book_id, status)
		VALUES ($1::uuid, $2, 'API Key Preserve Group', $3::uuid, 'active')
	`, groupID, tenantID, priceBookID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_api_keys (
			id, owner_type, tenant_id, group_id, key_hash, key_ciphertext,
			last_four, name, status
		) VALUES (
			$1::uuid, 'tenant', $2, $3::uuid, 'preserve-disabled-hash',
			'preserve-disabled-ciphertext', '0003', 'Disabled Key', 'disabled'
		)
	`, keyID, tenantID, groupID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_api_keys (
			id, owner_type, tenant_id, group_id, key_hash, key_ciphertext,
			last_four, name, quota_limit, status, expires_at
		) VALUES (
			$1::uuid, 'tenant', $2, $3::uuid, 'preserve-expired-hash',
			'preserve-expired-ciphertext', '0004', 'Expired Key', 123, 'active',
			now() - interval '1 hour'
		)
	`, expiredKeyID, tenantID, groupID); err != nil {
		t.Fatal(err)
	}

	repo := NewAPIKeyRepo(dbgen.New(pool))
	updated, _, err := repo.Update(ctx, coreidentity.APIKeyUpdate{
		ID: keyID, TenantID: tenantID, GroupID: groupID, Name: "Renamed Disabled Key", Status: "",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "disabled" {
		t.Fatalf("metadata update status = %q, want disabled", updated.Status)
	}

	var persisted string
	if err := pool.QueryRow(ctx, `SELECT status FROM ai_api_keys WHERE id = $1::uuid`, keyID).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if persisted != "disabled" {
		t.Fatalf("persisted status = %q, want disabled", persisted)
	}

	expired, _, err := repo.Update(ctx, coreidentity.APIKeyUpdate{
		ID: expiredKeyID, TenantID: tenantID, GroupID: groupID, Name: "Renamed Expired Key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if expired.QuotaLimitMicro == nil || *expired.QuotaLimitMicro != 123 {
		t.Fatalf("omitted quota update = %v, want 123", expired.QuotaLimitMicro)
	}
	if expired.ExpiresAt == nil || !expired.ExpiresAt.Before(time.Now()) {
		t.Fatalf("omitted expiry update = %v, want preserved past timestamp", expired.ExpiresAt)
	}

	cleared, _, err := repo.Update(ctx, coreidentity.APIKeyUpdate{
		ID: expiredKeyID, TenantID: tenantID, GroupID: groupID, Name: "Cleared Expiry Key",
		QuotaLimitSet: true, ExpiresAtSet: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.QuotaLimitMicro != nil || cleared.ExpiresAt != nil {
		t.Fatalf("explicit null clear = quota:%v expires:%v, want both nil", cleared.QuotaLimitMicro, cleared.ExpiresAt)
	}
}
