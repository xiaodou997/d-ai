package gateway

import (
	"context"
	"testing"

	"xiaodou/dai/internal/ai/asynctask"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/testsupport"
)

func TestTaskSubjectResolverReloadsAPIKeyAuthorization(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("async task test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	const (
		keyID    = "44444444-4444-4444-4444-444444444444"
		groupID  = "55555555-5555-5555-5555-555555555555"
		groupID2 = "55555555-5555-5555-5555-555555555556"
	)
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_groups (id, tenant_id, name, retail_price_book_id)
		VALUES
		  ($1::uuid, 'tenant-subject', 'subject group', '66666666-6666-6666-6666-666666666666'::uuid),
		  ($2::uuid, 'tenant-subject', 'subject group 2', '66666666-6666-6666-6666-666666666666'::uuid)
	`, groupID, groupID2); err != nil {
		t.Fatalf("seed group: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_api_keys (
			id, owner_type, tenant_id, group_id, key_hash, key_ciphertext, last_four, name,
			quota_limit, quota_used, status
		) VALUES (
			$1::uuid, 'tenant', 'tenant-subject', $2::uuid, 'subject-key-hash', '', 'hash', 'subject key',
			9000, 1200, 'active'
		)
	`, keyID, groupID); err != nil {
		t.Fatalf("seed API key: %v", err)
	}

	resolver := NewTaskSubjectResolver(dbgen.New(pool))
	ref := asynctask.SubjectRef{
		AuthMethod: coreidentity.AuthMethodAPIKey,
		TenantID:   "tenant-subject",
		APIKeyID:   keyID,
	}
	subject, err := resolver.Resolve(ctx, ref)
	if err != nil {
		t.Fatalf("Resolve active key: %v", err)
	}
	if subject.AuthMethod != coreidentity.AuthMethodAPIKey || subject.RequestSource != coreidentity.RequestSourceAPIKey {
		t.Fatalf("resolved auth = %+v", subject)
	}
	if subject.APIKeyID != keyID || subject.GroupID != groupID {
		t.Fatalf("resolved key/group = %+v", subject)
	}
	if subject.QuotaLimit == nil || *subject.QuotaLimit != 9000 || subject.QuotaUsed != 1200 {
		t.Fatalf("resolved quota = %+v", subject)
	}

	if _, err := pool.Exec(ctx, `UPDATE ai_api_keys SET group_id = $1::uuid WHERE id = $2::uuid`, groupID2, keyID); err != nil {
		t.Fatalf("change key group: %v", err)
	}
	updated, err := resolver.Resolve(ctx, ref)
	if err != nil || updated.GroupID != groupID2 {
		t.Fatalf("reloaded key group = %+v, %v", updated, err)
	}

	if _, err := pool.Exec(ctx, `UPDATE ai_api_keys SET status = 'disabled' WHERE id = $1::uuid`, keyID); err != nil {
		t.Fatalf("disable key: %v", err)
	}
	if _, err := resolver.Resolve(ctx, ref); err == nil {
		t.Fatal("disabled API key still resolved for queued work")
	}
}
