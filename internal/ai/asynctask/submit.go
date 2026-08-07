package asynctask

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"time"

	"xiaodou/dai/internal/ai/core/identity"
)

// SubmitRequest is one inbound task submission, after the transport has
// authenticated the caller and resolved the task type.
type SubmitRequest struct {
	Subject     identity.Subject
	Type        string
	Body        []byte
	ContentType string

	// Metadata is the client's own business annotation, echoed back verbatim in
	// the submit response and query response. It is not
	// execution input and is never shown to the handler: keeping it out of
	// Prepared.Input also keeps it out of the idempotency fingerprint and out of
	// the redaction surface.
	Metadata json.RawMessage

	// WebhookURL, when set, receives the terminal state.
	WebhookURL string

	// IdempotencyKey is the client's Idempotency-Key header, if any.
	IdempotencyKey string
}

// Submission returns the handler's view of this request.
func (r SubmitRequest) submission() Submission {
	return Submission{
		Subject:     r.Subject,
		Type:        r.Type,
		Body:        r.Body,
		ContentType: r.ContentType,
	}
}

// SubmitResult is what the caller gets back from Submit.
type SubmitResult struct {
	ID string
	// Duplicate reports that an existing task was returned for a reused
	// Idempotency-Key rather than a new one being created.
	Duplicate bool
}

// Submit validates, gates, persists and enqueues a task.
//
// The order matters. The cheap local cap runs before the handler's admission
// gate, and both run before anything is written, so a tenant with no balance or
// a full queue is turned away without ever occupying a row.
func (e *Engine) Submit(ctx context.Context, req SubmitRequest) (SubmitResult, error) {
	reg, ok := e.registry.lookup(req.Type)
	if !ok {
		return SubmitResult{}, Errorf(http.StatusBadRequest, "unsupported_task_type",
			"task type %q is not supported", req.Type)
	}

	ref := SubjectRefFrom(req.Subject)
	if ref.TenantID == "" {
		return SubmitResult{}, Errorf(http.StatusUnauthorized, "unauthenticated",
			"the caller has no tenant")
	}
	webhookURL := ""
	if req.WebhookURL != "" {
		var err error
		webhookURL, err = normalizeWebhookURL(req.WebhookURL)
		if err != nil {
			return SubmitResult{}, Errorf(http.StatusBadRequest, "invalid_webhook_url", "%s", err.Error())
		}
	}

	// Checked before Prepare so a tenant that is already at its cap does not get
	// to spend the gate's database work, and cannot flood the queue while
	// waiting on upstream admission.
	inFlight, err := e.store.countInFlight(ctx, ref.TenantID)
	if err != nil {
		return SubmitResult{}, err
	}
	if inFlight >= e.cfg.MaxInFlightPerTenant {
		return SubmitResult{}, Errorf(http.StatusTooManyRequests, "too_many_tasks_in_flight",
			"at most %d tasks may be pending or running at once", e.cfg.MaxInFlightPerTenant)
	}

	// Prepare decodes, validates and runs the capability's admission gate. It
	// returns the durable input; whatever else it computed is deliberately
	// discarded, so nothing can reach the worker except through the database.
	prepared, err := reg.handler.Prepare(ctx, req.submission())
	if err != nil {
		return SubmitResult{}, err
	}
	if len(prepared.Input) == 0 {
		return SubmitResult{}, Errorf(http.StatusInternalServerError, "internal_error",
			"handler for %q produced no task input", req.Type)
	}

	ttl := prepared.TTL
	if ttl <= 0 {
		ttl = reg.opts.TTL
	}
	if ttl <= 0 {
		ttl = e.cfg.Retention
	}

	rec := insertRecord{
		Type:        req.Type,
		SubjectRef:  ref,
		ModelCode:   prepared.ModelCode,
		Input:       prepared.Input,
		Metadata:    req.Metadata,
		WebhookURL:  webhookURL,
		MaxAttempts: reg.opts.MaxAttempts,
		ExpiresAt:   time.Now().Add(ttl),
	}
	if req.IdempotencyKey != "" {
		rec.IdempotencyKey = req.IdempotencyKey
		rec.IdempotencyScope = idempotencyScope(ref)
		rec.IdempotencyFingerprint = idempotencyFingerprint(req.Type, prepared.Input)
	}

	id, inserted, err := e.store.insert(ctx, rec)
	if err != nil {
		return SubmitResult{}, err
	}
	if !inserted {
		return e.resolveIdempotentDuplicate(ctx, rec)
	}

	e.signal()
	return SubmitResult{ID: id}, nil
}

// resolveIdempotentDuplicate handles a reused Idempotency-Key: same request
// returns the original task, a different one is a client bug worth surfacing.
func (e *Engine) resolveIdempotentDuplicate(ctx context.Context, rec insertRecord) (SubmitResult, error) {
	hit, err := e.store.findByIdempotencyKey(ctx, rec.IdempotencyScope, rec.IdempotencyKey)
	if err != nil {
		return SubmitResult{}, err
	}
	if !hit.Found {
		// The conflicting row disappeared between INSERT and SELECT (expired and
		// swept). Retrying is the caller's cheapest path.
		return SubmitResult{}, Errorf(http.StatusConflict, "idempotency_key_conflict",
			"the idempotency key was concurrently released; retry the request")
	}
	if hit.Type != rec.Type || !bytes.Equal(hit.Fingerprint, rec.IdempotencyFingerprint) {
		return SubmitResult{}, Errorf(http.StatusConflict, "idempotency_key_reuse",
			"this idempotency key was already used for a different request")
	}
	return SubmitResult{ID: hit.ID, Duplicate: true}, nil
}

// idempotencyScope isolates keys per credential rather than per tenant: two API
// keys in one tenant are two integrations, and their independent "retry-1" keys
// must not collide.
func idempotencyScope(ref SubjectRef) string {
	switch {
	case ref.APIKeyID != "":
		return "key:" + ref.APIKeyID
	default:
		return "user:" + ref.TenantID + ":" + ref.UserID
	}
}

// idempotencyFingerprint identifies the request a key was used for, so a reused
// key carrying different input is rejected rather than silently returning the
// unrelated original. Input is already normalized by Prepare, so it is stable
// across retries of the same logical request.
func idempotencyFingerprint(taskType string, input json.RawMessage) []byte {
	sum := sha256.New()
	sum.Write([]byte(taskType))
	sum.Write([]byte{0})
	sum.Write(input)
	return sum.Sum(nil)
}
