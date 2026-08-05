package asynctask

import (
	"context"
	"encoding/json"
	"time"

	"xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
)

// store is the engine's persistence seam. The Postgres implementation lives in
// this package rather than adapters/postgres because the claim and lease SQL is
// the module's semantics, not an incidental detail of storing it: reading
// "how does a claim work" should not require jumping packages.
type store interface {
	// insert writes a new pending task. When rec.IdempotencyKey is set and the
	// (scope, key) pair already exists, it returns the existing row's id and
	// inserted=false instead of creating a second task.
	insert(ctx context.Context, rec insertRecord) (id string, inserted bool, err error)

	// findByIdempotencyKey returns the existing row for a (scope, key) pair.
	findByIdempotencyKey(ctx context.Context, scope, key string) (idempotencyHit, error)

	// countInFlight counts a tenant's pending+running tasks.
	countInFlight(ctx context.Context, tenantID string) (int, error)

	// claim atomically takes the next eligible task for this worker, bounded by
	// the per-tenant in-flight cap. Returns ok=false when nothing is claimable.
	claim(ctx context.Context, types []string, cap int, workerID string, lease time.Duration) (claimed claimedTask, ok bool, err error)

	// heartbeat extends the lease. RowsAffected==0 means either the lease was
	// taken or the task was cancelled — both mean stop working.
	heartbeat(ctx context.Context, taskID, workerID string, lease time.Duration) (held bool, err error)

	// complete writes a terminal state, but only if this worker still holds the
	// lease and the task is still running.
	complete(ctx context.Context, taskID, workerID string, res Result) (written bool, err error)

	// reapRetryable returns expired-lease tasks to pending with backoff, but
	// only those that have attempts left and have not already reached billing.
	reapRetryable(ctx context.Context, limit int) (int64, error)

	// reapDead fails the remaining expired-lease tasks.
	reapDead(ctx context.Context, limit int) (int64, error)

	// releaseWorker returns this worker's running tasks to pending on graceful
	// shutdown, refunding the attempt it did not really use.
	releaseWorker(ctx context.Context, workerID string) (int64, error)

	// get loads a task for the query API.
	get(ctx context.Context, taskID string) (taskRow, error)

	// list loads one newest-first page within an already-normalized owner scope.
	list(ctx context.Context, filter listRecord) ([]taskRow, error)

	// cancel marks a task cancelled. Running tasks stop at the next heartbeat.
	cancel(ctx context.Context, taskID string) (cancelled bool, err error)

	// listExpired returns rows past expires_at, for the expiry sweep.
	listExpired(ctx context.Context, limit int) ([]taskRow, error)

	// deleteTask removes a row after its Expirer hook ran.
	deleteTask(ctx context.Context, taskID string) error

	// claimDelivery leases one pending or abandoned webhook delivery.
	claimDelivery(ctx context.Context, workerID string, lease time.Duration) (claimedDelivery, bool, error)
	finishDelivery(ctx context.Context, deliveryID, workerID string, outcome deliveryOutcome) (bool, error)
	reapDeadDeliveries(ctx context.Context, limit int) (int64, error)
	releaseDeliveries(ctx context.Context, workerID string) (int64, error)
}

// insertRecord is a submission ready to persist.
type insertRecord struct {
	Type        string
	SubjectRef  SubjectRef
	ModelCode   string
	Input       json.RawMessage
	Metadata    json.RawMessage
	WebhookURL  string
	MaxAttempts int
	ExpiresAt   time.Time

	IdempotencyKey         string
	IdempotencyScope       string
	IdempotencyFingerprint []byte
}

// idempotencyHit is the existing row behind a reused Idempotency-Key.
type idempotencyHit struct {
	ID          string
	Type        string
	Fingerprint []byte
	Found       bool
}

// claimedTask is a row the worker now holds the lease on.
type claimedTask struct {
	ID         string
	Type       string
	ModelCode  string
	Input      json.RawMessage
	Attempt    int
	RequestID  string
	SubjectRef SubjectRef
}

// taskRow is a task as the query API and expiry sweep see it.
type taskRow struct {
	ID          string
	Type        string
	Status      domain.TaskStatus
	ModelCode   string
	AuthMethod  identity.AuthMethod
	TenantID    string
	UserID      string
	APIKeyID    string
	InvokeKeyID string
	Input       json.RawMessage
	Output      json.RawMessage
	Metadata    json.RawMessage
	WebhookURL  string

	IdempotencyKey string
	RequestID      string
	Attempt        int
	CallerCharge   int64

	ErrorCode    string
	ErrorMessage string

	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	ExpiresAt   *time.Time
}

type listRecord struct {
	TenantID   string
	OwnerScope identity.Scope
	UserID     string
	Types      []string
	Status     domain.TaskStatus
	Limit      int
	Cursor     *listCursor
}

type listCursor struct {
	CreatedAt time.Time
	ID        string
}

type claimedDelivery struct {
	ID          string
	TaskID      string
	URL         string
	Payload     json.RawMessage
	Attempt     int
	MaxAttempts int
}

type deliveryOutcome struct {
	Status     string
	StatusCode int
	LastError  string
	RetryAfter time.Duration
}
