package asynctask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/testsupport"
)

func testSubject() identity.Subject {
	return identity.Subject{
		AuthMethod:    identity.AuthMethodAPIKey,
		RequestSource: identity.RequestSourceAPIKey,
		Scope:         identity.ScopeTenant,
		TenantID:      "tenant-a",
		APIKeyID:      "11111111-1111-1111-1111-111111111111",
	}
}

// newEngine wires an engine against a real schema with a passthrough subject
// resolver. cfg is applied on top of fast test defaults.
func newEngine(t *testing.T, cfg Config) (*Engine, *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 8})
	if err != nil {
		t.Skipf("async task test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if cfg.PollInterval == 0 {
		cfg.PollInterval = 50 * time.Millisecond
	}
	if cfg.ReapInterval == 0 {
		cfg.ReapInterval = 50 * time.Millisecond
	}
	e, err := New(cfg, Deps{
		Pool:   pool,
		Logger: zap.NewNop(),
		Subjects: SubjectResolverFunc(func(_ context.Context, ref SubjectRef) (identity.Subject, error) {
			return identity.Subject{
				AuthMethod: ref.AuthMethod,
				Scope:      identity.ScopeTenant,
				TenantID:   ref.TenantID,
				UserID:     ref.UserID,
				APIKeyID:   ref.APIKeyID,
			}, nil
		}),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return e, pool
}

// assertJSONEqual compares JSON semantically. Payloads round-trip through jsonb,
// which normalizes whitespace and key order, so byte comparison would assert
// Postgres's formatting rather than our behaviour.
func assertJSONEqual(t *testing.T, got, want string, what string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal([]byte(got), &g); err != nil {
		t.Fatalf("%s: got invalid JSON %q: %v", what, got, err)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("%s: want invalid JSON %q: %v", what, want, err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Fatalf("%s = %s, want %s", what, got, want)
	}
}

func waitForStatus(t *testing.T, e *Engine, id string, want domain.TaskStatus) TaskView {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last TaskView
	for time.Now().Before(deadline) {
		view, err := e.Get(context.Background(), testSubject(), id)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		last = view
		if view.Status == want {
			return view
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("task %s status = %s, want %s", id, last.Status, want)
	return last
}

// TestEngineRunsTaskEndToEnd covers the whole path: submit, wake, claim,
// execute, terminal write, query.
func TestEngineRunsTaskEndToEnd(t *testing.T) {
	e, _ := newEngine(t, Config{Workers: 2})

	e.Register(probeType, stubHandler{
		prepare: func(_ context.Context, sub Submission) (Prepared, error) {
			return Prepared{Input: sub.Body, ModelCode: "gpt-image-1"}, nil
		},
		execute: func(_ context.Context, task Task) (Result, error) {
			// The engine must hand the handler exactly what was persisted.
			return Result{
				Status:       domain.TaskCompleted,
				Output:       json.RawMessage(`{"echo":` + string(task.Input) + `}`),
				CallerCharge: 4200,
			}, nil
		},
	}, Options{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.Start(ctx)

	res, err := e.Submit(ctx, SubmitRequest{
		Subject:  testSubject(),
		Type:     probeType,
		Body:     []byte(`{"prompt":"a cat"}`),
		Metadata: json.RawMessage(`{"order_id":"A-1"}`),
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	view := waitForStatus(t, e, res.ID, domain.TaskCompleted)
	assertJSONEqual(t, string(view.Output), `{"echo":{"prompt":"a cat"}}`, "output")
	if view.CallerCharge != 4200 {
		t.Fatalf("caller_charge = %d, want 4200", view.CallerCharge)
	}
	// Metadata is echoed back so a caller can tie the task to its own operation.
	assertJSONEqual(t, string(view.Metadata), `{"order_id":"A-1"}`, "echoed metadata")
	if view.RequestID == "" {
		t.Fatal("request_id was not recorded; the usage log could not be reconciled")
	}
}

// TestEngineExecutesFromPersistedInputOnly is the module's central invariant.
// Prepare deliberately computes something extra and the handler must not be able
// to see it in Execute — only the row survives.
func TestEngineExecutesFromPersistedInputOnly(t *testing.T) {
	e, pool := newEngine(t, Config{Workers: 1})

	var gotInput atomic.Value
	e.Register(probeType, stubHandler{
		prepare: func(_ context.Context, sub Submission) (Prepared, error) {
			// Only Input is durable. Anything else Prepare knows is discarded.
			return Prepared{Input: []byte(`{"redacted":true}`), ModelCode: "m"}, nil
		},
		execute: func(_ context.Context, task Task) (Result, error) {
			gotInput.Store(string(task.Input))
			return Result{Status: domain.TaskCompleted, Output: []byte(`{}`)}, nil
		},
	}, Options{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.Start(ctx)

	res, err := e.Submit(ctx, SubmitRequest{
		Subject: testSubject(),
		Type:    probeType,
		Body:    []byte(`{"secret":"should not reach execute"}`),
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	waitForStatus(t, e, res.ID, domain.TaskCompleted)

	assertJSONEqual(t, gotInput.Load().(string), `{"redacted":true}`,
		"what Execute saw (it must only ever see what Prepare persisted)")

	// And what is stored is the redacted input, not the raw submission.
	var stored string
	if err := pool.QueryRow(ctx,
		`SELECT input_payload::text FROM ai_async_tasks WHERE id = $1::uuid`, res.ID).Scan(&stored); err != nil {
		t.Fatalf("read input_payload: %v", err)
	}
	assertJSONEqual(t, stored, `{"redacted":true}`, "stored input_payload")
}

// TestEngineRejectsSubmissionWhenPrepareRejects: a bad request must be refused
// while the caller still holds the connection, and must not leave a row behind.
func TestEngineRejectsSubmissionWhenPrepareRejects(t *testing.T) {
	e, pool := newEngine(t, Config{Workers: 1})
	e.Register(probeType, stubHandler{
		prepare: func(context.Context, Submission) (Prepared, error) {
			return Prepared{}, Errorf(http.StatusPaymentRequired, "insufficient_balance", "no balance")
		},
	}, Options{})

	ctx := context.Background()
	_, err := e.Submit(ctx, SubmitRequest{Subject: testSubject(), Type: probeType, Body: []byte(`{}`)})
	if err == nil {
		t.Fatal("Submit accepted a request the admission gate rejected")
	}
	apiErr := AsError(err)
	if apiErr.Status != http.StatusPaymentRequired || apiErr.Code != "insufficient_balance" {
		t.Fatalf("error = %+v, want the gate's 402", apiErr)
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ai_async_tasks`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("%d rows written for a rejected submission, want 0", n)
	}
}

func TestEngineRejectsUnsafeWebhookURLBeforePersisting(t *testing.T) {
	e, pool := newEngine(t, Config{Workers: 1})
	e.Register(probeType, stubHandler{}, Options{})

	_, err := e.Submit(context.Background(), SubmitRequest{
		Subject: testSubject(), Type: probeType, Body: []byte(`{}`),
		WebhookURL: "http://127.0.0.1/hooks",
	})
	if err == nil || AsError(err).Code != "invalid_webhook_url" {
		t.Fatalf("error = %v, want invalid_webhook_url", err)
	}
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM ai_async_tasks`).Scan(&count); err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if count != 0 {
		t.Fatalf("persisted %d tasks for an unsafe webhook URL", count)
	}
}

func TestEngineEnforcesInFlightCap(t *testing.T) {
	e, _ := newEngine(t, Config{Workers: 1, MaxInFlightPerTenant: 2})
	// Block execution so submitted tasks stay in flight.
	release := make(chan struct{})
	e.Register(probeType, stubHandler{
		execute: func(ctx context.Context, _ Task) (Result, error) {
			select {
			case <-release:
			case <-ctx.Done():
			}
			return Result{Status: domain.TaskCompleted, Output: []byte(`{}`)}, nil
		},
	}, Options{})
	defer close(release)

	ctx := context.Background()
	for i := range 2 {
		if _, err := e.Submit(ctx, SubmitRequest{Subject: testSubject(), Type: probeType, Body: []byte(`{}`)}); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}
	_, err := e.Submit(ctx, SubmitRequest{Subject: testSubject(), Type: probeType, Body: []byte(`{}`)})
	if err == nil {
		t.Fatal("submit past the in-flight cap was accepted")
	}
	if got := AsError(err); got.Status != http.StatusTooManyRequests || got.Code != "too_many_tasks_in_flight" {
		t.Fatalf("error = %+v, want 429 too_many_tasks_in_flight", got)
	}
}

func TestEngineIdempotentSubmitReturnsSameTask(t *testing.T) {
	e, pool := newEngine(t, Config{Workers: 1})
	release := make(chan struct{})
	e.Register(probeType, stubHandler{
		prepare: func(_ context.Context, sub Submission) (Prepared, error) {
			return Prepared{Input: sub.Body, ModelCode: "m"}, nil
		},
		execute: func(ctx context.Context, _ Task) (Result, error) {
			<-release
			return Result{Status: domain.TaskCompleted, Output: []byte(`{}`)}, nil
		},
	}, Options{})
	defer close(release)

	ctx := context.Background()
	req := SubmitRequest{
		Subject:        testSubject(),
		Type:           probeType,
		Body:           []byte(`{"prompt":"a cat"}`),
		IdempotencyKey: "order-1",
	}
	first, err := e.Submit(ctx, req)
	if err != nil {
		t.Fatalf("first submit: %v", err)
	}
	if first.Duplicate {
		t.Fatal("first submit reported a duplicate")
	}

	second, err := e.Submit(ctx, req)
	if err != nil {
		t.Fatalf("retried submit: %v", err)
	}
	if !second.Duplicate || second.ID != first.ID {
		t.Fatalf("retry returned %+v, want the original task %s", second, first.ID)
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ai_async_tasks`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("%d tasks exist after an idempotent retry, want 1", n)
	}

	// The same key with different input is a client bug worth surfacing, not a
	// silent hand-back of an unrelated task.
	conflicting := req
	conflicting.Body = []byte(`{"prompt":"a dog"}`)
	if _, err := e.Submit(ctx, conflicting); err == nil {
		t.Fatal("a reused key with different input was accepted")
	} else if got := AsError(err); got.Code != "idempotency_key_reuse" {
		t.Fatalf("error = %+v, want idempotency_key_reuse", got)
	}
}

func TestEngineListPaginatesVisibleTasks(t *testing.T) {
	e, _ := newEngine(t, Config{Workers: 1})
	e.Register(probeType, stubHandler{
		prepare: func(_ context.Context, sub Submission) (Prepared, error) {
			return Prepared{Input: sub.Body, ModelCode: "m"}, nil
		},
	}, Options{})

	ctx := context.Background()
	created := make([]string, 0, 3)
	for i := range 3 {
		result, err := e.Submit(ctx, SubmitRequest{
			Subject: testSubject(), Type: probeType,
			Body: []byte(fmt.Sprintf(`{"sequence":%d}`, i)),
		})
		if err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
		created = append(created, result.ID)
	}

	first, err := e.List(ctx, testSubject(), ListFilter{Types: []string{probeType}, Limit: 2})
	if err != nil {
		t.Fatalf("List first page: %v", err)
	}
	if len(first.Data) != 2 || !first.HasMore {
		t.Fatalf("first page = %+v, want 2 rows with has_more", first)
	}
	if first.Data[0].ID != created[2] || first.Data[1].ID != created[1] {
		t.Fatalf("first page ids = %s, %s; want newest %s, %s", first.Data[0].ID, first.Data[1].ID, created[2], created[1])
	}

	second, err := e.List(ctx, testSubject(), ListFilter{
		Types: []string{probeType}, Limit: 2, StartingAfter: first.Data[1].ID,
	})
	if err != nil {
		t.Fatalf("List second page: %v", err)
	}
	if len(second.Data) != 1 || second.HasMore || second.Data[0].ID != created[0] {
		t.Fatalf("second page = %+v, want oldest task only", second)
	}

	intruder := testSubject()
	intruder.TenantID = "tenant-b"
	hidden, err := e.List(ctx, intruder, ListFilter{Types: []string{probeType}, Limit: 10})
	if err != nil {
		t.Fatalf("List other tenant: %v", err)
	}
	if len(hidden.Data) != 0 {
		t.Fatalf("other tenant saw tasks: %+v", hidden.Data)
	}

	if _, err := e.List(ctx, testSubject(), ListFilter{
		Types: []string{"other.surface"}, Limit: 2, StartingAfter: created[1],
	}); err == nil || AsError(err).Code != "invalid_cursor" {
		t.Fatalf("cursor outside filtered task types returned %v, want invalid_cursor", err)
	}
}

func TestEngineListFiltersTaskOwnership(t *testing.T) {
	e, _ := newEngine(t, Config{Workers: 1})
	e.Register(probeType, stubHandler{}, Options{})

	ctx := context.Background()
	tenantSubject := testSubject()
	userA := testSubject()
	userA.Scope = identity.ScopeUser
	userA.UserID = "user-a"
	userB := testSubject()
	userB.Scope = identity.ScopeUser
	userB.UserID = "user-b"

	for name, subject := range map[string]identity.Subject{
		"tenant": tenantSubject,
		"user-a": userA,
		"user-b": userB,
	} {
		if _, err := e.Submit(ctx, SubmitRequest{
			Subject: subject, Type: probeType, Body: []byte(fmt.Sprintf(`{"owner":%q}`, name)),
		}); err != nil {
			t.Fatalf("submit %s task: %v", name, err)
		}
	}

	tenantTasks, err := e.List(ctx, tenantSubject, ListFilter{
		Types: []string{probeType}, OwnerScope: identity.ScopeTenant, Limit: 10,
	})
	if err != nil {
		t.Fatalf("list tenant tasks: %v", err)
	}
	if len(tenantTasks.Data) != 1 || tenantTasks.Data[0].Subject.UserID != "" {
		t.Fatalf("tenant tasks = %+v, want only tenant-owned task", tenantTasks.Data)
	}
	if tenantTasks.Data[0].Subject.AuthMethod != identity.AuthMethodAPIKey {
		t.Fatalf("task auth method = %q, want api_key", tenantTasks.Data[0].Subject.AuthMethod)
	}

	allUserTasks, err := e.List(ctx, tenantSubject, ListFilter{
		Types: []string{probeType}, OwnerScope: identity.ScopeUser, Limit: 10,
	})
	if err != nil {
		t.Fatalf("list user tasks: %v", err)
	}
	if len(allUserTasks.Data) != 2 {
		t.Fatalf("user tasks = %+v, want both user-owned tasks", allUserTasks.Data)
	}

	userATasks, err := e.List(ctx, tenantSubject, ListFilter{
		Types: []string{probeType}, OwnerScope: identity.ScopeUser, OwnerUserID: "user-a", Limit: 10,
	})
	if err != nil {
		t.Fatalf("list user-a tasks: %v", err)
	}
	if len(userATasks.Data) != 1 || userATasks.Data[0].Subject.UserID != "user-a" {
		t.Fatalf("user-a tasks = %+v, want only user-a task", userATasks.Data)
	}

	selfTasks, err := e.List(ctx, userA, ListFilter{
		Types: []string{probeType}, OwnerScope: identity.ScopeTenant, OwnerUserID: "user-b", Limit: 10,
	})
	if err != nil {
		t.Fatalf("list as user-a: %v", err)
	}
	if len(selfTasks.Data) != 1 || selfTasks.Data[0].Subject.UserID != "user-a" {
		t.Fatalf("user-a visible tasks = %+v, want only own task", selfTasks.Data)
	}

	emptyUser := testSubject()
	emptyUser.Scope = identity.ScopeUser
	emptyUser.UserID = ""
	emptyUserTasks, err := e.List(ctx, emptyUser, ListFilter{Types: []string{probeType}, Limit: 10})
	if err != nil {
		t.Fatalf("list as user without user id: %v", err)
	}
	if len(emptyUserTasks.Data) != 0 {
		t.Fatalf("user without user id saw tasks: %+v", emptyUserTasks.Data)
	}
}

// TestEngineFailsTaskWhenSubjectIsGone: a revoked credential must not have its
// queued work run unauthorized.
func TestEngineFailsTaskWhenSubjectIsGone(t *testing.T) {
	e, _ := newEngine(t, Config{Workers: 1})
	e.deps.Subjects = SubjectResolverFunc(func(context.Context, SubjectRef) (identity.Subject, error) {
		return identity.Subject{}, errors.New("api key was revoked")
	})

	var executed atomic.Bool
	e.Register(probeType, stubHandler{
		execute: func(context.Context, Task) (Result, error) {
			executed.Store(true)
			return Result{Status: domain.TaskCompleted, Output: []byte(`{}`)}, nil
		},
	}, Options{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.Start(ctx)

	res, err := e.Submit(ctx, SubmitRequest{Subject: testSubject(), Type: probeType, Body: []byte(`{}`)})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	view := waitForStatus(t, e, res.ID, domain.TaskFailed)
	if view.ErrorCode != "subject_unavailable" {
		t.Fatalf("error_code = %q, want subject_unavailable", view.ErrorCode)
	}
	if executed.Load() {
		t.Fatal("the handler ran for a credential that no longer resolves")
	}
}

func TestEngineFailedTaskPreservesSettledCharge(t *testing.T) {
	e, _ := newEngine(t, Config{Workers: 1})
	e.Register(probeType, stubHandler{
		execute: func(context.Context, Task) (Result, error) {
			return Result{
				Status:       domain.TaskFailed,
				CallerCharge: 9999,
				Failure:      &Failure{Code: "upstream_rejected", Message: "bad prompt", Step: "execute"},
			}, nil
		},
	}, Options{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.Start(ctx)

	res, err := e.Submit(ctx, SubmitRequest{Subject: testSubject(), Type: probeType, Body: []byte(`{}`)})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	view := waitForStatus(t, e, res.ID, domain.TaskFailed)
	if view.ErrorCode != "upstream_rejected" {
		t.Fatalf("error_code = %q", view.ErrorCode)
	}
	if view.CallerCharge != 9999 {
		t.Fatalf("caller_charge = %d on a failed task, want 9999", view.CallerCharge)
	}
}

// TestEngineRedactsInternalDetail proves the injected scrubber is actually
// applied — the engine takes it as a dependency rather than importing serving.
func TestEngineRedactsInternalDetail(t *testing.T) {
	e, pool := newEngine(t, Config{Workers: 1})
	e.deps.RedactDetail = func(string) string { return "[SCRUBBED]" }
	e.Register(probeType, stubHandler{
		execute: func(context.Context, Task) (Result, error) {
			return Result{
				Status: domain.TaskFailed,
				Failure: &Failure{
					Code:           "upstream_rejected",
					Message:        "bad prompt",
					InternalDetail: "Authorization: Bearer sk-super-secret-value",
				},
			}, nil
		},
	}, Options{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.Start(ctx)

	res, err := e.Submit(ctx, SubmitRequest{Subject: testSubject(), Type: probeType, Body: []byte(`{}`)})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	waitForStatus(t, e, res.ID, domain.TaskFailed)

	var detail string
	if err := pool.QueryRow(ctx,
		`SELECT internal_error_detail FROM ai_async_tasks WHERE id = $1::uuid`, res.ID).Scan(&detail); err != nil {
		t.Fatalf("read detail: %v", err)
	}
	if detail != "[SCRUBBED]" {
		t.Fatalf("internal_error_detail = %q; the injected scrubber was not applied", detail)
	}
}

// TestEngineGetHidesOtherTenants: a probe for someone else's id is
// indistinguishable from a missing task.
func TestEngineGetHidesOtherTenants(t *testing.T) {
	e, _ := newEngine(t, Config{Workers: 1})
	e.Register(probeType, stubHandler{}, Options{})

	ctx := context.Background()
	res, err := e.Submit(ctx, SubmitRequest{Subject: testSubject(), Type: probeType, Body: []byte(`{}`)})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	intruder := testSubject()
	intruder.TenantID = "tenant-b"
	if _, err := e.Get(ctx, intruder, res.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant Get returned %v, want ErrNotFound", err)
	}

	// A user-scoped caller sees only their own tasks.
	userScoped := testSubject()
	userScoped.Scope = identity.ScopeUser
	userScoped.UserID = "someone-else"
	if _, err := e.Get(ctx, userScoped, res.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user Get returned %v, want ErrNotFound", err)
	}
}

func TestEngineCancelStopsRunningTask(t *testing.T) {
	e, _ := newEngine(t, Config{Workers: 1, LeaseTTL: 3 * time.Second})

	started := make(chan struct{})
	var interrupted atomic.Bool
	var once atomic.Bool
	e.Register(probeType, stubHandler{
		execute: func(ctx context.Context, _ Task) (Result, error) {
			if once.CompareAndSwap(false, true) {
				close(started)
			}
			<-ctx.Done() // cancelled locally, in-memory, without waiting for the lease
			interrupted.Store(true)
			return Result{Status: domain.TaskFailed, Failure: &Failure{Code: "cancelled"}}, nil
		},
	}, Options{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.Start(ctx)

	res, err := e.Submit(ctx, SubmitRequest{Subject: testSubject(), Type: probeType, Body: []byte(`{}`)})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("task never started")
	}

	view, err := e.Cancel(ctx, testSubject(), res.ID)
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if view.Status != domain.TaskCancelled {
		t.Fatalf("status = %s, want cancelled", view.Status)
	}

	deadline := time.Now().Add(2 * time.Second)
	for !interrupted.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !interrupted.Load() {
		t.Fatal("the running handler was not interrupted by cancellation")
	}

	// The worker's late result must not overwrite the cancelled row.
	final, err := e.Get(ctx, testSubject(), res.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if final.Status != domain.TaskCancelled {
		t.Fatalf("status = %s after the worker returned, want cancelled", final.Status)
	}
}

func TestEngineDeleteTerminalCleansUpAndRemovesTask(t *testing.T) {
	e, pool := newEngine(t, Config{})
	handler := &expiringTestHandler{pool: pool, expired: make(chan expiryObservation, 1)}
	e.Register("test.deletable", handler, Options{})

	ctx := context.Background()
	created, err := e.Submit(ctx, SubmitRequest{
		Subject: testSubject(), Type: "test.deletable", Body: []byte(`{"side_file":true}`),
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := e.DeleteTerminal(ctx, testSubject(), created.ID); err == nil {
		t.Fatal("DeleteTerminal accepted a pending task")
	} else {
		var taskErr *Error
		if !errors.As(err, &taskErr) || taskErr.Code != "task_not_deletable" {
			t.Fatalf("DeleteTerminal pending error = %v, want task_not_deletable", err)
		}
	}
	if _, err := pool.Exec(ctx, `
		UPDATE ai_async_tasks
		SET status = 'completed', completed_at = now()
		WHERE id = $1::uuid`, created.ID); err != nil {
		t.Fatalf("complete task: %v", err)
	}
	if err := e.DeleteTerminal(ctx, testSubject(), created.ID); err != nil {
		t.Fatalf("DeleteTerminal: %v", err)
	}

	select {
	case observed := <-handler.expired:
		if observed.err != nil {
			t.Fatalf("cleanup error: %v", observed.err)
		}
		if observed.task.ID != created.ID || !observed.rowExisted {
			t.Fatalf("cleanup observation = %#v", observed)
		}
	case <-time.After(time.Second):
		t.Fatal("terminal task cleanup was not called")
	}
	if _, err := e.Get(ctx, testSubject(), created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after DeleteTerminal = %v, want ErrNotFound", err)
	}
}

// TestEngineReaperRecoversOrphan simulates a process death: the row stays
// running with a lease nobody renews, and the reaper must resolve it.
func TestEngineReaperRecoversOrphan(t *testing.T) {
	e, pool := newEngine(t, Config{Workers: 1})
	e.Register(probeType, stubHandler{}, Options{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Submit without starting workers, then hand-place the row into the state a
	// crashed worker leaves behind.
	res, err := e.Submit(ctx, SubmitRequest{Subject: testSubject(), Type: probeType, Body: []byte(`{}`)})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE ai_async_tasks
		SET status = 'running', worker_id = 'dead-worker',
		    lease_expires_at = now() - interval '1 second',
		    attempt_count = 1, request_id = 'atsk_dead_1'
		WHERE id = $1::uuid`, res.ID); err != nil {
		t.Fatalf("simulate crash: %v", err)
	}

	e.Start(ctx)

	// MaxAttempts is 1, so the orphan is failed rather than retried.
	view := waitForStatus(t, e, res.ID, domain.TaskFailed)
	if view.ErrorCode != "worker_lost" {
		t.Fatalf("error_code = %q, want worker_lost", view.ErrorCode)
	}
}

type expiringTestHandler struct {
	pool    *pgxpool.Pool
	expired chan expiryObservation
}

type expiryObservation struct {
	task       Task
	rowExisted bool
	err        error
}

func (h *expiringTestHandler) Prepare(_ context.Context, sub Submission) (Prepared, error) {
	return Prepared{Input: sub.Body, ModelCode: "expiry-model"}, nil
}

func (*expiringTestHandler) Execute(context.Context, Task) (Result, error) {
	panic("not used")
}

func (h *expiringTestHandler) OnExpire(ctx context.Context, task Task) error {
	var rowExisted bool
	err := h.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM ai_async_tasks WHERE id = $1::uuid)`, task.ID).Scan(&rowExisted)
	h.expired <- expiryObservation{task: task, rowExisted: rowExisted, err: err}
	return err
}

func TestEngineExpiryCallsTypeHookBeforeDeletingTask(t *testing.T) {
	e, pool := newEngine(t, Config{})
	handler := &expiringTestHandler{pool: pool, expired: make(chan expiryObservation, 1)}
	e.Register("test.expiring", handler, Options{TTL: 10 * time.Millisecond})

	ctx := context.Background()
	created, err := e.Submit(ctx, SubmitRequest{
		Subject: testSubject(), Type: "test.expiring", Body: []byte(`{"side_file":true}`),
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE ai_async_tasks SET expires_at = now() - interval '1 second' WHERE id = $1::uuid`, created.ID); err != nil {
		t.Fatalf("expire task: %v", err)
	}
	e.sweepExpired(ctx)

	select {
	case observation := <-handler.expired:
		if observation.err != nil {
			t.Fatalf("expiry hook database observation: %v", observation.err)
		}
		if !observation.rowExisted {
			t.Fatal("task row was deleted before the expiry hook ran")
		}
		if observation.task.ID != created.ID || observation.task.Type != "test.expiring" {
			t.Fatalf("expired task = %+v", observation.task)
		}
	case <-time.After(time.Second):
		t.Fatal("expiry hook was not called")
	}
	if _, err := e.Get(ctx, testSubject(), created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after expiry = %v, want ErrNotFound", err)
	}
}
