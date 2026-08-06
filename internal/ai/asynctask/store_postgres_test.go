package asynctask

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/testsupport"
)

const (
	probeType  = "test.probe"
	otherType  = "test.other"
	claimLease = 60 * time.Second
)

// openStore provisions an isolated canonical schema. concurrency sets the pool
// size — a contention test with too few connections silently serializes and
// proves nothing, which is why this is explicit rather than defaulted.
func openStore(t *testing.T, concurrency int32) (*postgresStore, *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: concurrency})
	if err != nil {
		t.Skipf("async task test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })
	return newPostgresStore(pool), pool
}

func seedTask(t *testing.T, s *postgresStore, tenant string, opts ...func(*insertRecord)) string {
	t.Helper()
	rec := insertRecord{
		Type:        probeType,
		SubjectRef:  SubjectRef{AuthMethod: identity.AuthMethodAPIKey, TenantID: tenant},
		ModelCode:   "gpt-image-1",
		Input:       []byte(`{"prompt":"a cat"}`),
		MaxAttempts: 1,
		ExpiresAt:   time.Now().Add(24 * time.Hour),
	}
	for _, o := range opts {
		o(&rec)
	}
	id, inserted, err := s.insert(context.Background(), rec)
	if err != nil {
		t.Fatalf("insert task: %v", err)
	}
	if !inserted {
		t.Fatal("expected a fresh insert")
	}
	return id
}

func expireLease(t *testing.T, pool *pgxpool.Pool, taskID string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`UPDATE ai_async_tasks SET lease_expires_at = now() - interval '1 second' WHERE id = $1::uuid`, taskID)
	if err != nil {
		t.Fatalf("expire lease: %v", err)
	}
}

func assertWebhookNotification(t *testing.T, pool *pgxpool.Pool, taskID, event string) {
	t.Helper()
	var payload []byte
	if err := pool.QueryRow(context.Background(),
		`SELECT payload FROM ai_async_task_deliveries WHERE task_id = $1::uuid`, taskID,
	).Scan(&payload); err != nil {
		t.Fatalf("load webhook notification: %v", err)
	}
	var notification map[string]any
	if err := json.Unmarshal(payload, &notification); err != nil {
		t.Fatalf("decode webhook notification: %v", err)
	}
	if len(notification) != 3 || notification["source"] != "D-AI" ||
		notification["event"] != event || notification["task_id"] != taskID {
		t.Fatalf("notification = %#v", notification)
	}
}

// TestClaimIsExclusiveUnderContention is the core guarantee: FOR UPDATE SKIP
// LOCKED must hand every task to exactly one worker. Without it, two replicas
// would run the same task twice and bill the customer twice.
func TestClaimIsExclusiveUnderContention(t *testing.T) {
	const (
		tasks   = 50
		workers = 8
	)
	s, _ := openStore(t, workers+2)
	ctx := context.Background()

	// One tenant, and a cap above the task count, so the only thing under test
	// is exclusion — not fairness.
	for range tasks {
		seedTask(t, s, "tenant-a")
	}

	var (
		mu      sync.Mutex
		claimed = map[string]string{} // task id -> worker that got it
		wg      sync.WaitGroup
	)
	for w := range workers {
		workerID := fmt.Sprintf("worker-%d", w)
		wg.Go(func() {
			for {
				got, ok, err := s.claim(ctx, []string{probeType}, tasks+10, workerID, claimLease)
				if err != nil {
					t.Errorf("claim: %v", err)
					return
				}
				if !ok {
					return
				}
				mu.Lock()
				if prev, dup := claimed[got.ID]; dup {
					t.Errorf("task %s claimed twice: by %s and %s", got.ID, prev, workerID)
				}
				claimed[got.ID] = workerID
				mu.Unlock()
			}
		})
	}
	wg.Wait()

	if len(claimed) != tasks {
		t.Fatalf("claimed %d distinct tasks, want %d", len(claimed), tasks)
	}
	for id, w := range claimed {
		if w == "" {
			t.Fatalf("task %s has no worker", id)
		}
	}
}

// TestClaimFairnessCapsPerTenant proves one tenant cannot starve another: a
// tenant flooding the queue is held to its in-flight cap, so a later arrival
// from another tenant is still served.
func TestClaimFairnessCapsPerTenant(t *testing.T) {
	s, _ := openStore(t, 4)
	ctx := context.Background()

	for range 20 {
		seedTask(t, s, "tenant-noisy")
	}
	// Submitted last, so strict FIFO would leave it behind all 20.
	time.Sleep(10 * time.Millisecond)
	quietID := seedTask(t, s, "tenant-quiet")

	const cap = 2
	seen := map[string]bool{}
	for i := range 3 {
		got, ok, err := s.claim(ctx, []string{probeType}, cap, "w1", claimLease)
		if err != nil {
			t.Fatalf("claim %d: %v", i, err)
		}
		if !ok {
			t.Fatalf("claim %d returned nothing; the quiet tenant was starved", i)
		}
		seen[got.ID] = true
	}
	if !seen[quietID] {
		t.Fatal("the quiet tenant's task was not served within cap+1 claims")
	}

	// The noisy tenant is now at its cap with 2 running, and the quiet tenant is
	// empty, so nothing more may be claimed even though 18 rows are pending.
	if _, ok, err := s.claim(ctx, []string{probeType}, cap, "w1", claimLease); err != nil {
		t.Fatalf("claim: %v", err)
	} else if ok {
		t.Fatal("claimed past the per-tenant in-flight cap")
	}
}

// TestClaimOnlyTakesRegisteredTypes keeps an instance from taking work it has no
// handler for — which is what lets a future worker run as its own process.
func TestClaimOnlyTakesRegisteredTypes(t *testing.T) {
	s, _ := openStore(t, 2)
	ctx := context.Background()
	seedTask(t, s, "tenant-a")

	if _, ok, err := s.claim(ctx, []string{otherType}, 10, "w1", claimLease); err != nil {
		t.Fatalf("claim: %v", err)
	} else if ok {
		t.Fatal("claimed a task whose type is not registered on this instance")
	}
	if _, ok, err := s.claim(ctx, []string{probeType}, 10, "w1", claimLease); err != nil {
		t.Fatalf("claim: %v", err)
	} else if !ok {
		t.Fatal("did not claim a task of a registered type")
	}
}

// TestClaimPreallocatesRequestID: request_id must be settled before the upstream
// call so the usage log joins back, and must differ per attempt because
// ai_usage_logs.request_id is unique.
func TestClaimPreallocatesRequestID(t *testing.T) {
	s, pool := openStore(t, 2)
	ctx := context.Background()
	id := seedTask(t, s, "tenant-a", func(r *insertRecord) { r.MaxAttempts = 3 })

	first, ok, err := s.claim(ctx, []string{probeType}, 10, "w1", claimLease)
	if err != nil || !ok {
		t.Fatalf("first claim: %v ok=%v", err, ok)
	}
	if first.Attempt != 1 {
		t.Fatalf("attempt = %d, want 1", first.Attempt)
	}
	want := fmt.Sprintf("atsk_%s_1", id)
	if first.RequestID != want {
		t.Fatalf("request_id = %q, want %q", first.RequestID, want)
	}

	expireLease(t, pool, id)
	if _, err := s.reapRetryable(ctx, 10); err != nil {
		t.Fatalf("reap: %v", err)
	}
	// Backoff pushed available_at out; pull it back so the retry is claimable.
	if _, err := pool.Exec(ctx, `UPDATE ai_async_tasks SET available_at = now() WHERE id = $1::uuid`, id); err != nil {
		t.Fatalf("clear backoff: %v", err)
	}

	second, ok, err := s.claim(ctx, []string{probeType}, 10, "w1", claimLease)
	if err != nil || !ok {
		t.Fatalf("second claim: %v ok=%v", err, ok)
	}
	if second.RequestID == first.RequestID {
		t.Fatalf("attempt 2 reused request_id %q; the usage log unique index would reject it", second.RequestID)
	}
}

func TestHeartbeatHoldsAndDetectsLostLease(t *testing.T) {
	s, pool := openStore(t, 2)
	ctx := context.Background()
	id := seedTask(t, s, "tenant-a")

	if _, ok, err := s.claim(ctx, []string{probeType}, 10, "w1", claimLease); err != nil || !ok {
		t.Fatalf("claim: %v ok=%v", err, ok)
	}

	held, err := s.heartbeat(ctx, id, "w1", claimLease)
	if err != nil || !held {
		t.Fatalf("heartbeat by lease holder: held=%v err=%v", held, err)
	}

	// Another instance takes over.
	if _, err := pool.Exec(ctx, `UPDATE ai_async_tasks SET worker_id = 'w2' WHERE id = $1::uuid`, id); err != nil {
		t.Fatalf("reassign: %v", err)
	}
	held, err = s.heartbeat(ctx, id, "w1", claimLease)
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if held {
		t.Fatal("heartbeat reported the lease held after another worker took it")
	}
}

// TestCancelStopsWorkerAndSurvivesCompletion covers the cancellation path that
// needs no cross-instance signalling: the heartbeat returning zero rows is the
// signal, and a late completion must not resurrect the task.
func TestCancelStopsWorkerAndSurvivesCompletion(t *testing.T) {
	s, _ := openStore(t, 2)
	ctx := context.Background()
	id := seedTask(t, s, "tenant-a")

	if _, ok, err := s.claim(ctx, []string{probeType}, 10, "w1", claimLease); err != nil || !ok {
		t.Fatalf("claim: %v ok=%v", err, ok)
	}
	cancelled, err := s.cancel(ctx, id)
	if err != nil || !cancelled {
		t.Fatalf("cancel: cancelled=%v err=%v", cancelled, err)
	}

	held, err := s.heartbeat(ctx, id, "w1", claimLease)
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if held {
		t.Fatal("heartbeat reported the lease held on a cancelled task")
	}

	written, err := s.complete(ctx, id, "w1", Result{Status: domain.TaskCompleted, Output: []byte(`{"ok":true}`)})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if written {
		t.Fatal("a cancelled task was overwritten by its worker's result")
	}
	row, err := s.get(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if row.Status != domain.TaskCancelled {
		t.Fatalf("status = %s, want cancelled", row.Status)
	}
}

func TestCompletingTaskAtomicallyCreatesWebhookDelivery(t *testing.T) {
	s, pool := openStore(t, 2)
	ctx := context.Background()
	id := seedTask(t, s, "tenant-a", func(r *insertRecord) {
		r.WebhookURL = "https://hooks.example.com/task-events"
	})

	claimed, ok, err := s.claim(ctx, []string{probeType}, 10, "w1", claimLease)
	if err != nil || !ok {
		t.Fatalf("claim: %v ok=%v", err, ok)
	}
	written, err := s.complete(ctx, claimed.ID, "w1", Result{
		Status: domain.TaskCompleted, Output: []byte(`{"ok":true}`),
	})
	if err != nil || !written {
		t.Fatalf("complete: written=%v err=%v", written, err)
	}

	var taskID, target, status string
	err = pool.QueryRow(ctx, `
				SELECT task_id::text, url, status
				FROM ai_async_task_deliveries
				WHERE task_id = $1::uuid
			`, id).Scan(&taskID, &target, &status)
	if err != nil {
		t.Fatalf("load delivery: %v", err)
	}
	if taskID != id || target != "https://hooks.example.com/task-events" || status != "pending" {
		t.Fatalf("delivery = task=%q url=%q status=%q", taskID, target, status)
	}
	assertWebhookNotification(t, pool, id, "task.completed")
}

func TestWebhookDeliveryClaimUsesAnExclusiveLease(t *testing.T) {
	s, pool := openStore(t, 3)
	ctx := context.Background()
	id := seedTask(t, s, "tenant-a", func(r *insertRecord) {
		r.WebhookURL = "https://hooks.example.com/task-events"
	})
	claimedTask, ok, err := s.claim(ctx, []string{probeType}, 10, "task-worker", claimLease)
	if err != nil || !ok {
		t.Fatalf("claim task: %v ok=%v", err, ok)
	}
	if written, err := s.complete(ctx, claimedTask.ID, "task-worker", Result{
		Status: domain.TaskCompleted, Output: []byte(`{"ok":true}`),
	}); err != nil || !written {
		t.Fatalf("complete task: written=%v err=%v", written, err)
	}

	delivery, ok, err := s.claimDelivery(ctx, "delivery-worker-1", 30*time.Second)
	if err != nil || !ok {
		t.Fatalf("claim delivery: %v ok=%v", err, ok)
	}
	if delivery.TaskID != id || delivery.Attempt != 1 || delivery.MaxAttempts != 6 {
		t.Fatalf("claimed delivery = %+v", delivery)
	}
	if _, ok, err := s.claimDelivery(ctx, "delivery-worker-2", 30*time.Second); err != nil {
		t.Fatalf("second claim: %v", err)
	} else if ok {
		t.Fatal("second worker claimed a delivery with a live lease")
	}
	if _, err := pool.Exec(ctx, `
		UPDATE ai_async_task_deliveries
		SET lease_expires_at = now() - interval '1 second'
		WHERE id = $1::uuid
	`, delivery.ID); err != nil {
		t.Fatalf("expire delivery lease: %v", err)
	}
	reclaimed, ok, err := s.claimDelivery(ctx, "delivery-worker-2", 30*time.Second)
	if err != nil || !ok {
		t.Fatalf("reclaim delivery: %v ok=%v", err, ok)
	}
	if reclaimed.ID != delivery.ID || reclaimed.Attempt != 2 {
		t.Fatalf("reclaimed delivery = %+v", reclaimed)
	}
}

func TestWebhookDeliveryRetryPersistsBackoff(t *testing.T) {
	s, pool := openStore(t, 2)
	ctx := context.Background()
	id := seedTask(t, s, "tenant-a", func(r *insertRecord) {
		r.WebhookURL = "https://hooks.example.com/task-events"
	})
	task, ok, err := s.claim(ctx, []string{probeType}, 10, "task-worker", claimLease)
	if err != nil || !ok {
		t.Fatalf("claim task: %v ok=%v", err, ok)
	}
	if written, err := s.complete(ctx, task.ID, "task-worker", Result{
		Status: domain.TaskCompleted, Output: []byte(`{}`),
	}); err != nil || !written {
		t.Fatalf("complete task: written=%v err=%v", written, err)
	}
	delivery, ok, err := s.claimDelivery(ctx, "delivery-worker", 30*time.Second)
	if err != nil || !ok {
		t.Fatalf("claim delivery: %v ok=%v", err, ok)
	}
	written, err := s.finishDelivery(ctx, delivery.ID, "delivery-worker", deliveryOutcome{
		Status: "pending", StatusCode: 500, LastError: "webhook returned HTTP 500", RetryAfter: 10 * time.Second,
	})
	if err != nil || !written {
		t.Fatalf("finish delivery: written=%v err=%v", written, err)
	}
	var (
		status, lastError string
		statusCode        int
		availableAt       time.Time
	)
	if err := pool.QueryRow(ctx, `
		SELECT status, last_status_code, last_error, available_at
		FROM ai_async_task_deliveries WHERE task_id = $1::uuid
	`, id).Scan(&status, &statusCode, &lastError, &availableAt); err != nil {
		t.Fatalf("load delivery: %v", err)
	}
	if status != "pending" || statusCode != 500 || lastError != "webhook returned HTTP 500" {
		t.Fatalf("retry state = status %q code %d error %q", status, statusCode, lastError)
	}
	if availableAt.Before(time.Now().Add(8 * time.Second)) {
		t.Fatalf("available_at = %s, want about 10s in the future", availableAt)
	}
}

func TestCancellingTaskAtomicallyCreatesOneWebhookDelivery(t *testing.T) {
	s, pool := openStore(t, 2)
	ctx := context.Background()
	id := seedTask(t, s, "tenant-a", func(r *insertRecord) {
		r.WebhookURL = "https://hooks.example.com/task-events"
	})

	cancelled, err := s.cancel(ctx, id)
	if err != nil || !cancelled {
		t.Fatalf("cancel: cancelled=%v err=%v", cancelled, err)
	}
	if cancelled, err := s.cancel(ctx, id); err != nil || cancelled {
		t.Fatalf("second cancel: cancelled=%v err=%v", cancelled, err)
	}
	var count int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM ai_async_task_deliveries WHERE task_id = $1::uuid`, id).Scan(&count); err != nil {
		t.Fatalf("count deliveries: %v", err)
	}
	if count != 1 {
		t.Fatalf("delivery count = %d, want 1", count)
	}
	assertWebhookNotification(t, pool, id, "task.cancelled")
}

func TestDeleteTaskExplicitlyRemovesWebhookDelivery(t *testing.T) {
	s, pool := openStore(t, 2)
	ctx := context.Background()
	id := seedTask(t, s, "tenant-a", func(r *insertRecord) {
		r.WebhookURL = "https://hooks.example.com/task-events"
	})

	if cancelled, err := s.cancel(ctx, id); err != nil || !cancelled {
		t.Fatalf("cancel: cancelled=%v err=%v", cancelled, err)
	}
	var deliveriesBefore int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM ai_async_task_deliveries WHERE task_id = $1::uuid`, id,
	).Scan(&deliveriesBefore); err != nil {
		t.Fatalf("count delivery before delete: %v", err)
	}
	if deliveriesBefore != 1 {
		t.Fatalf("deliveries before delete = %d, want 1", deliveriesBefore)
	}

	if err := s.deleteTask(ctx, id); err != nil {
		t.Fatalf("delete task: %v", err)
	}
	var tasksAfter, deliveriesAfter int
	if err := pool.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM ai_async_tasks WHERE id = $1::uuid),
		  (SELECT count(*) FROM ai_async_task_deliveries WHERE task_id = $1::uuid)
	`, id).Scan(&tasksAfter, &deliveriesAfter); err != nil {
		t.Fatalf("count rows after delete: %v", err)
	}
	if tasksAfter != 0 || deliveriesAfter != 0 {
		t.Fatalf("rows after delete = tasks %d deliveries %d, want both zero", tasksAfter, deliveriesAfter)
	}
}

func TestCompleteRejectsNonLeaseHolder(t *testing.T) {
	s, _ := openStore(t, 2)
	ctx := context.Background()
	id := seedTask(t, s, "tenant-a")
	if _, ok, err := s.claim(ctx, []string{probeType}, 10, "w1", claimLease); err != nil || !ok {
		t.Fatalf("claim: %v ok=%v", err, ok)
	}

	// A zombie worker whose lease was reaped and reassigned must not write.
	written, err := s.complete(ctx, id, "w-zombie", Result{Status: domain.TaskCompleted, Output: []byte(`{}`)})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if written {
		t.Fatal("a non-lease-holder wrote a terminal state")
	}
}

func TestReapRequeuesRetryableOrphan(t *testing.T) {
	s, pool := openStore(t, 2)
	ctx := context.Background()
	id := seedTask(t, s, "tenant-a", func(r *insertRecord) { r.MaxAttempts = 2 })

	if _, ok, err := s.claim(ctx, []string{probeType}, 10, "w1", claimLease); err != nil || !ok {
		t.Fatalf("claim: %v ok=%v", err, ok)
	}
	expireLease(t, pool, id)

	n, err := s.reapRetryable(ctx, 10)
	if err != nil {
		t.Fatalf("reapRetryable: %v", err)
	}
	if n != 1 {
		t.Fatalf("reapRetryable requeued %d, want 1", n)
	}

	var (
		status      string
		availableAt time.Time
		attempts    int
	)
	if err := pool.QueryRow(ctx,
		`SELECT status, available_at, attempt_count FROM ai_async_tasks WHERE id = $1::uuid`, id,
	).Scan(&status, &availableAt, &attempts); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if status != "pending" {
		t.Fatalf("status = %s, want pending", status)
	}
	if !availableAt.After(time.Now()) {
		t.Fatal("requeued without backoff; a failing task would spin")
	}
	if attempts != 1 {
		t.Fatalf("attempt_count = %d, want 1 (the used attempt is kept)", attempts)
	}
}

func TestReapFailsOrphanWithNoAttemptsLeft(t *testing.T) {
	s, pool := openStore(t, 2)
	ctx := context.Background()
	id := seedTask(t, s, "tenant-a", func(r *insertRecord) {
		r.WebhookURL = "https://hooks.example.com/task-events"
	}) // MaxAttempts: 1

	if _, ok, err := s.claim(ctx, []string{probeType}, 10, "w1", claimLease); err != nil || !ok {
		t.Fatalf("claim: %v ok=%v", err, ok)
	}
	expireLease(t, pool, id)

	if n, err := s.reapRetryable(ctx, 10); err != nil {
		t.Fatalf("reapRetryable: %v", err)
	} else if n != 0 {
		t.Fatalf("reapRetryable took %d tasks with no attempts left, want 0", n)
	}
	if n, err := s.reapDead(ctx, 10); err != nil {
		t.Fatalf("reapDead: %v", err)
	} else if n != 1 {
		t.Fatalf("reapDead failed %d tasks, want 1", n)
	}

	row, err := s.get(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if row.Status != domain.TaskFailed {
		t.Fatalf("status = %s, want failed", row.Status)
	}
	if row.ErrorCode != "worker_lost" {
		t.Fatalf("error_code = %q, want worker_lost", row.ErrorCode)
	}
	var deliveries int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM ai_async_task_deliveries WHERE task_id = $1::uuid`, id).Scan(&deliveries); err != nil {
		t.Fatalf("count deliveries: %v", err)
	}
	if deliveries != 1 {
		t.Fatalf("delivery count = %d, want 1", deliveries)
	}
	assertWebhookNotification(t, pool, id, "task.failed")
}

// TestReapRefusesToRetryAnAttemptThatReachedBilling is the double-spend guard.
// request_id is written before the upstream call, so a usage log row proves the
// attempt already cost money — retrying it would charge the customer twice.
func TestReapRefusesToRetryAnAttemptThatReachedBilling(t *testing.T) {
	s, pool := openStore(t, 2)
	ctx := context.Background()
	id := seedTask(t, s, "tenant-a", func(r *insertRecord) { r.MaxAttempts = 5 })

	claimed, ok, err := s.claim(ctx, []string{probeType}, 10, "w1", claimLease)
	if err != nil || !ok {
		t.Fatalf("claim: %v ok=%v", err, ok)
	}
	expireLease(t, pool, id)

	// The attempt reached billing before the worker died.
	if _, err := pool.Exec(ctx,
		`INSERT INTO ai_usage_logs (request_id, key_owner_type, tenant_id, model_code, billing_status, request_status)
		 VALUES ($1, 'tenant', $2, $3, 'settled', 'success')`,
		claimed.RequestID, "tenant-a", "gpt-image-1"); err != nil {
		t.Fatalf("seed usage log: %v", err)
	}

	if n, err := s.reapRetryable(ctx, 10); err != nil {
		t.Fatalf("reapRetryable: %v", err)
	} else if n != 0 {
		t.Fatal("retried an attempt that had already been billed; the customer would be charged twice")
	}

	// It still gets resolved, just as failed rather than retried.
	if n, err := s.reapDead(ctx, 10); err != nil {
		t.Fatalf("reapDead: %v", err)
	} else if n != 1 {
		t.Fatalf("reapDead failed %d tasks, want 1", n)
	}
}

// TestReleaseWorkerIsScopedToOneInstance covers the bug the lease design exists
// to fix: the old console engine reset every running row on boot, with no owner
// filter, so a second instance would seize tasks the first was still running.
func TestReleaseWorkerIsScopedToOneInstance(t *testing.T) {
	s, pool := openStore(t, 3)
	ctx := context.Background()
	// Claims are ordered by created_at, so w1 takes the older task and w2 the
	// newer one — which of them each worker holds is deterministic.
	w1Task := seedTask(t, s, "tenant-a")
	time.Sleep(10 * time.Millisecond)
	w2Task := seedTask(t, s, "tenant-b")

	if got, ok, err := s.claim(ctx, []string{probeType}, 10, "w1", claimLease); err != nil || !ok {
		t.Fatalf("claim w1: %v ok=%v", err, ok)
	} else if got.ID != w1Task {
		t.Fatalf("w1 claimed %s, want the older task %s", got.ID, w1Task)
	}
	if got, ok, err := s.claim(ctx, []string{probeType}, 10, "w2", claimLease); err != nil || !ok {
		t.Fatalf("claim w2: %v ok=%v", err, ok)
	} else if got.ID != w2Task {
		t.Fatalf("w2 claimed %s, want the newer task %s", got.ID, w2Task)
	}

	n, err := s.releaseWorker(ctx, "w1")
	if err != nil {
		t.Fatalf("releaseWorker: %v", err)
	}
	if n != 1 {
		t.Fatalf("released %d tasks, want 1 (only this worker's)", n)
	}

	assertStatus := func(id, want string) {
		t.Helper()
		var status string
		if err := pool.QueryRow(ctx, `SELECT status FROM ai_async_tasks WHERE id = $1::uuid`, id).Scan(&status); err != nil {
			t.Fatalf("read status: %v", err)
		}
		if status != want {
			t.Fatalf("task %s status = %s, want %s", id, status, want)
		}
	}
	assertStatus(w1Task, "pending")
	// The other instance's task must be untouched. This is the exact bug the
	// lease design exists to prevent.
	assertStatus(w2Task, "running")

	// A graceful release did not really use an attempt, so it must not burn one.
	var attempts int
	if err := pool.QueryRow(ctx,
		`SELECT attempt_count FROM ai_async_tasks WHERE id = $1::uuid`, w1Task).Scan(&attempts); err != nil {
		t.Fatalf("read attempts: %v", err)
	}
	if attempts != 0 {
		t.Fatalf("attempt_count = %d after graceful release, want 0", attempts)
	}
}

func TestInsertIsIdempotentPerScopeAndKey(t *testing.T) {
	s, _ := openStore(t, 2)
	ctx := context.Background()

	rec := insertRecord{
		Type:                   probeType,
		SubjectRef:             SubjectRef{AuthMethod: identity.AuthMethodAPIKey, TenantID: "tenant-a", APIKeyID: "11111111-1111-1111-1111-111111111111"},
		ModelCode:              "gpt-image-1",
		Input:                  []byte(`{"prompt":"a cat"}`),
		MaxAttempts:            1,
		ExpiresAt:              time.Now().Add(time.Hour),
		IdempotencyKey:         "order-1",
		IdempotencyScope:       "key:11111111-1111-1111-1111-111111111111",
		IdempotencyFingerprint: idempotencyFingerprint(probeType, []byte(`{"prompt":"a cat"}`)),
	}

	id, inserted, err := s.insert(ctx, rec)
	if err != nil || !inserted {
		t.Fatalf("first insert: id=%s inserted=%v err=%v", id, inserted, err)
	}

	// Same key again: no second task, and the caller can find the original.
	_, inserted, err = s.insert(ctx, rec)
	if err != nil {
		t.Fatalf("second insert: %v", err)
	}
	if inserted {
		t.Fatal("a reused idempotency key created a second task")
	}
	hit, err := s.findByIdempotencyKey(ctx, rec.IdempotencyScope, rec.IdempotencyKey)
	if err != nil || !hit.Found {
		t.Fatalf("lookup: found=%v err=%v", hit.Found, err)
	}
	if hit.ID != id {
		t.Fatalf("lookup returned %s, want the original %s", hit.ID, id)
	}

	// The same key under a different credential is a different integration and
	// must not collide.
	other := rec
	other.SubjectRef.APIKeyID = "22222222-2222-2222-2222-222222222222"
	other.IdempotencyScope = "key:22222222-2222-2222-2222-222222222222"
	if _, inserted, err := s.insert(ctx, other); err != nil || !inserted {
		t.Fatalf("insert under another credential: inserted=%v err=%v", inserted, err)
	}
}

func TestCountInFlightIgnoresTerminalTasks(t *testing.T) {
	s, _ := openStore(t, 2)
	ctx := context.Background()
	a := seedTask(t, s, "tenant-a")
	seedTask(t, s, "tenant-a")
	seedTask(t, s, "tenant-b")

	n, err := s.countInFlight(ctx, "tenant-a")
	if err != nil {
		t.Fatalf("countInFlight: %v", err)
	}
	if n != 2 {
		t.Fatalf("in-flight = %d, want 2", n)
	}

	if _, ok, err := s.claim(ctx, []string{probeType}, 10, "w1", claimLease); err != nil || !ok {
		t.Fatalf("claim: %v ok=%v", err, ok)
	}
	if _, err := s.complete(ctx, a, "w1", Result{Status: domain.TaskCompleted, Output: []byte(`{}`)}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	n, err = s.countInFlight(ctx, "tenant-a")
	if err != nil {
		t.Fatalf("countInFlight: %v", err)
	}
	if n != 1 {
		t.Fatalf("in-flight after completion = %d, want 1", n)
	}
}
