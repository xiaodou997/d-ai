// Package asynctask owns the generic async task queue: submission, durable
// scheduling, leases, retries and expiry.
//
// The engine knows nothing about images, chat or apps. A capability joins by
// registering a Handler under a task type; everything the engine does for one
// capability it does for all of them. A capability keeps its synchronous
// endpoint untouched — async is an additional surface over the same work, not a
// second implementation of it.
//
// The one rule a capability must satisfy is in the Handler docs: Execute must
// be able to run from the persisted input alone.
package asynctask

import (
	"context"
	"encoding/json"
	"time"

	"xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
)

// Handler is the entire contract for putting a capability behind the queue.
//
// There is exactly one hard rule: Execute must run from its arguments alone.
// Everything Prepare learned from the inbound HTTP request that Execute still
// needs must land in Prepared.Input, because Execute may run in a different
// process, minutes later, after a restart.
//
// The engine enforces this structurally rather than by convention: Prepare's
// return value never reaches Execute — only the row written to ai_async_tasks
// does. The in-memory fast path carries a wake-up signal, not a payload. So the
// crash-recovery path is not dead code that only runs after a crash; it is the
// path every task takes.
type Handler interface {
	// Prepare decodes and validates an inbound submission on the caller's
	// goroutine. The caller is still holding the connection, so bad input must
	// be rejected here (return an *Error to choose the status code).
	//
	// Admission gating (balance, quota, authorization, subscription) belongs
	// here too: Prepare knows its capability and protocol, the engine does not.
	// The engine only enforces the per-tenant in-flight cap.
	//
	// Redaction is a Prepare responsibility: Input is visible to anyone who can
	// read the task row.
	Prepare(ctx context.Context, sub Submission) (Prepared, error)

	// Execute rebuilds from task.Input and runs to a terminal state.
	//
	// A business failure (upstream 4xx, moderation block) is
	// Result{Status: domain.TaskFailed}, not an error — the work happened, it
	// may have cost money, and retrying will not help.
	//
	// Returning an error means the handler never got going (rebuild failed, a
	// dependency is down). Wrap it with Retryable to ask the engine for another
	// attempt; the engine still refuses if the attempt already reached billing.
	Execute(ctx context.Context, task Task) (Result, error)
}

// Expirer is an optional Handler capability. The engine calls OnExpire before
// deleting an expired task row or a user-requested terminal task, letting a type
// clean up side files it owns (the console image directory is the motivating
// case). Failures are logged and the row is deleted regardless during expiry;
// manual deletion instead reports the failure so the user can retry.
type Expirer interface {
	OnExpire(ctx context.Context, task Task) error
}

// Submission is an inbound request as the handler sees it: the transport has
// already unwrapped the envelope and authenticated the caller.
type Submission struct {
	// Subject is the authenticated caller from an API key or console JWT.
	Subject identity.Subject

	// Type is the resolved registry key, e.g. "api.images.generation".
	Type string

	// Body is what the handler decodes:
	//   JSON envelope → the raw bytes of the envelope's `input` object
	//   multipart     → the entire multipart body, so imageedit.Decode works as-is
	Body        []byte
	ContentType string
}

// Prepared is everything Prepare produces. It is deliberately narrow: every
// extra field here is another way for the hot path to smuggle state past the
// database and quietly break crash recovery.
type Prepared struct {
	// Input is the durable, redacted, self-contained execution input — the only
	// thing Execute will get.
	//
	// It is stored as jsonb, so Execute receives it semantically equal but not
	// byte-identical: whitespace is normalized, object keys are reordered, and
	// duplicate keys collapse. Handlers must decode it rather than depend on its
	// bytes — do not persist a signature or hash over Input here and expect to
	// re-derive it in Execute.
	Input json.RawMessage
	// ModelCode is written to the task row and anchors listing and reconciliation.
	ModelCode string
	// TTL overrides the engine's default retention. Zero means use the default.
	TTL time.Duration
}

// Task is the persisted row handed to Execute.
type Task struct {
	ID        string
	Type      string
	ModelCode string
	Input     json.RawMessage
	// Attempt is 1-based.
	Attempt int

	// Subject is re-resolved by the engine before every attempt rather than
	// snapshotted at submit time, so a revoked key, an exhausted quota or a
	// changed group grant all take effect on tasks still in the queue.
	Subject identity.Subject

	// RequestID is pre-allocated by the engine and stable within this attempt.
	// A handler that replays the runtime pipeline must pass it through as
	// X-Request-Id, so the ai_usage_logs row the attempt produces joins back to
	// this task. The reaper relies on that join to refuse retrying an attempt
	// that already reached billing.
	RequestID string
}

// Result is a terminal outcome. Status must be TaskCompleted or TaskFailed;
// cancellation is the engine's to decide, not the handler's.
type Result struct {
	Status domain.TaskStatus
	// Output is written to result_payload.
	Output json.RawMessage
	// CallerCharge is the settled caller charge in micro-credits, copied from the
	// runtime settlement. Failed or cancelled work can still have a charge when
	// the upstream produced billable usage before the terminal state.
	CallerCharge int64
	// Failure is required when Status is TaskFailed.
	Failure *Failure
}

// Failure splits what the client may see from what only an admin may see.
type Failure struct {
	// Code is a stable machine-readable code, visible to the client.
	Code string
	// Message is client-safe prose.
	Message string
	// InternalDetail is admin-only; the engine redacts it before writing.
	InternalDetail string
	// Step names the stage that failed, written to failed_step.
	Step string
}
