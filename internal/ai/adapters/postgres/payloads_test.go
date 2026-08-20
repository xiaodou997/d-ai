package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"xiaodou/dai/internal/ai/audit"
	"xiaodou/dai/internal/ai/testsupport"
)

func TestAuditStoreDurableInboxIsIdempotentAndRecoverable(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("canonical schema test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	requestID := fmt.Sprintf("audit-inbox-%d", time.Now().UnixNano())
	store := NewAuditStore(pool)
	payload := &audit.Payload{
		RequestID:       requestID,
		ClientProtocol:  "openai_chat",
		RequestModel:    "test-model",
		RequestStatus:   "success",
		RequestMessages: []byte(`[{"role":"user","content":"hello"}]`),
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM ai_request_payloads WHERE request_id = $1`, requestID)
		_, _ = pool.Exec(ctx, `DELETE FROM ai_audit_inbox WHERE request_id = $1`, requestID)
	})

	if err := store.Enqueue(ctx, payload); err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if err := store.Enqueue(ctx, payload); err != nil {
		t.Fatalf("duplicate enqueue: %v", err)
	}
	var inboxCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM ai_audit_inbox WHERE request_id = $1`, requestID).Scan(&inboxCount); err != nil {
		t.Fatal(err)
	}
	if inboxCount != 1 {
		t.Fatalf("inbox rows = %d, want 1", inboxCount)
	}

	deliveries, err := store.Claim(ctx, "test-worker", 10, time.Minute)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("claim = %#v, err=%v", deliveries, err)
	}
	if deliveries[0].Attempts != 1 || deliveries[0].Payload.RequestID != requestID {
		t.Fatalf("claimed delivery = %#v", deliveries[0])
	}
	if _, err := pool.Exec(ctx, `
		UPDATE ai_audit_inbox
		SET locked_at = now() - interval '2 minutes'
		WHERE request_id = $1
	`, requestID); err != nil {
		t.Fatal(err)
	}
	recovered, err := store.Claim(ctx, "recovery-worker", 10, time.Minute)
	if err != nil || len(recovered) != 1 || recovered[0].Attempts != 2 {
		t.Fatalf("recovered claim = %#v, err=%v", recovered, err)
	}
	if err := store.Complete(ctx, deliveries[0]); err != nil {
		t.Fatalf("stale complete: %v", err)
	}
	if err := store.Complete(ctx, recovered[0]); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if err := store.Complete(ctx, recovered[0]); err != nil {
		t.Fatalf("idempotent complete: %v", err)
	}
	var payloadCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM ai_request_payloads WHERE request_id = $1`, requestID).Scan(&payloadCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM ai_audit_inbox WHERE request_id = $1`, requestID).Scan(&inboxCount); err != nil {
		t.Fatal(err)
	}
	if payloadCount != 1 || inboxCount != 0 {
		t.Fatalf("materialized rows = payload:%d inbox:%d", payloadCount, inboxCount)
	}
}

func TestAuditStoreEnqueueTxRollsBackWithCallerTransaction(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("canonical schema test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	requestID := fmt.Sprintf("audit-tx-%d", time.Now().UnixNano())
	store := NewAuditStore(pool)
	payload := &audit.Payload{RequestID: requestID, ClientProtocol: "openai_chat", RequestStatus: "failed"}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM ai_audit_inbox WHERE request_id = $1`, requestID)
	})

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnqueueTx(ctx, tx, payload); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil && err != pgx.ErrTxClosed {
		t.Fatal(err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM ai_audit_inbox WHERE request_id = $1`, requestID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rolled-back inbox rows = %d", count)
	}
}
