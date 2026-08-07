package asynctask

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
)

// TaskView is a task as a caller sees it. The transport maps this to its own
// wire shape; the engine does not know about HTTP bodies.
type TaskView struct {
	ID        string
	Type      string
	Status    domain.TaskStatus
	ModelCode string

	// Subject is available to trusted in-process adapters for ownership and
	// permission decisions. Public transports must not expose credential IDs.
	Subject SubjectRef

	// Input is the handler-owned persisted input. It is available to trusted
	// in-process consumers such as the legacy console DTO mapper; public
	// transports must not expose it directly.
	Input  json.RawMessage
	Output json.RawMessage
	// Metadata and IdempotencyKey are echoed back so a caller can tie a task to
	// whichever of its own operations triggered it — the reason they are stored
	// at all.
	Metadata       json.RawMessage
	IdempotencyKey string
	WebhookURL     string

	RequestID    string
	Attempt      int
	CallerCharge int64

	ErrorCode    string
	ErrorMessage string

	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
}

// ListFilter selects one newest-first page. StartingAfter is the task id at the
// end of the previous page; the engine resolves its (created_at,id) cursor.
type ListFilter struct {
	Types         []string
	Status        domain.TaskStatus
	OwnerScope    identity.Scope
	OwnerUserID   string
	Limit         int
	StartingAfter string
}

type ListResult struct {
	Data    []TaskView
	HasMore bool
}

func viewFromRow(row taskRow) TaskView {
	return TaskView{
		ID:        row.ID,
		Type:      row.Type,
		Status:    row.Status,
		ModelCode: row.ModelCode,
		Subject: SubjectRef{
			AuthMethod: row.AuthMethod,
			TenantID:   row.TenantID,
			UserID:     row.UserID,
			APIKeyID:   row.APIKeyID,
		},
		Input:          row.Input,
		Output:         row.Output,
		Metadata:       row.Metadata,
		IdempotencyKey: row.IdempotencyKey,
		WebhookURL:     row.WebhookURL,
		RequestID:      row.RequestID,
		Attempt:        row.Attempt,
		CallerCharge:   row.CallerCharge,
		ErrorCode:      row.ErrorCode,
		ErrorMessage:   row.ErrorMessage,
		CreatedAt:      row.CreatedAt,
		StartedAt:      row.StartedAt,
		CompletedAt:    row.CompletedAt,
	}
}

// Get returns a task the caller is allowed to see.
//
// Visibility is tenant + user, not per credential: rotating an API key must not
// orphan the tasks it submitted. This matches how the console already scopes
// image task reads.
func (e *Engine) Get(ctx context.Context, viewer identity.Subject, taskID string) (TaskView, error) {
	row, err := e.store.get(ctx, taskID)
	if err != nil {
		return TaskView{}, err
	}
	if row.ExpiresAt != nil && !row.ExpiresAt.After(time.Now()) {
		return TaskView{}, ErrNotFound
	}
	if !visible(viewer, row) {
		// Deliberately indistinguishable from a missing task: a tenant probing
		// ids must not learn which ones exist.
		return TaskView{}, ErrNotFound
	}
	return viewFromRow(row), nil
}

// Inspect returns a current task without applying caller visibility. It is for
// trusted in-process consumers that enforce a different access capability,
// such as the console image asset URL whose unguessable key lives in Output.
// HTTP transports must use Get instead.
func (e *Engine) Inspect(ctx context.Context, taskID string) (TaskView, error) {
	row, err := e.store.get(ctx, taskID)
	if err != nil {
		return TaskView{}, err
	}
	if row.ExpiresAt != nil && !row.ExpiresAt.After(time.Now()) {
		return TaskView{}, ErrNotFound
	}
	return viewFromRow(row), nil
}

// List returns visible tasks ordered newest first.
func (e *Engine) List(ctx context.Context, viewer identity.Subject, filter ListFilter) (ListResult, error) {
	if viewer.TenantID == "" {
		return ListResult{}, Errorf(http.StatusUnauthorized, "unauthenticated", "the caller has no tenant")
	}
	if viewer.Scope == identity.ScopeUser && viewer.UserID == "" {
		return ListResult{Data: []TaskView{}}, nil
	}
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}
	if len(filter.Types) == 0 {
		return ListResult{Data: []TaskView{}}, nil
	}

	ownerScope, ownerUserID, err := normalizeOwnerFilter(viewer, filter)
	if err != nil {
		return ListResult{}, err
	}

	var cursor *listCursor
	if filter.StartingAfter != "" {
		row, err := e.store.get(ctx, filter.StartingAfter)
		if err != nil || !visible(viewer, row) || !containsTaskType(filter.Types, row.Type) ||
			!matchesOwnerFilter(row, ownerScope, ownerUserID) ||
			(filter.Status != "" && row.Status != filter.Status) ||
			(row.ExpiresAt != nil && !row.ExpiresAt.After(time.Now())) {
			return ListResult{}, Errorf(http.StatusBadRequest, "invalid_cursor", "starting_after is not a visible task")
		}
		cursor = &listCursor{CreatedAt: row.CreatedAt, ID: row.ID}
	}

	rows, err := e.store.list(ctx, listRecord{
		TenantID:   viewer.TenantID,
		OwnerScope: ownerScope,
		UserID:     ownerUserID,
		Types:      filter.Types,
		Status:     filter.Status,
		Limit:      filter.Limit + 1,
		Cursor:     cursor,
	})
	if err != nil {
		return ListResult{}, err
	}
	hasMore := len(rows) > filter.Limit
	if hasMore {
		rows = rows[:filter.Limit]
	}
	data := make([]TaskView, 0, len(rows))
	for _, row := range rows {
		data = append(data, viewFromRow(row))
	}
	return ListResult{Data: data, HasMore: hasMore}, nil
}

func normalizeOwnerFilter(viewer identity.Subject, filter ListFilter) (identity.Scope, string, error) {
	switch viewer.Scope {
	case identity.ScopeUser:
		if viewer.UserID == "" {
			return identity.ScopeUser, "", nil
		}
		return identity.ScopeUser, viewer.UserID, nil
	case identity.ScopeTenant:
		scope := filter.OwnerScope
		userID := filter.OwnerUserID
		if scope == "" && userID != "" {
			scope = identity.ScopeUser
		}
		if scope != "" && scope != identity.ScopeTenant && scope != identity.ScopeUser {
			return "", "", Errorf(http.StatusBadRequest, "invalid_owner_scope", "owner scope is not supported")
		}
		if scope == identity.ScopeTenant && userID != "" {
			return "", "", Errorf(http.StatusBadRequest, "invalid_owner_filter", "tenant tasks cannot have a user filter")
		}
		return scope, userID, nil
	default:
		return "", "", Errorf(http.StatusUnauthorized, "unauthenticated", "the caller has no task scope")
	}
}

func matchesOwnerFilter(row taskRow, scope identity.Scope, userID string) bool {
	switch scope {
	case identity.ScopeTenant:
		return row.UserID == ""
	case identity.ScopeUser:
		if userID == "" {
			return row.UserID != ""
		}
		return row.UserID == userID
	default:
		return true
	}
}

func containsTaskType(types []string, want string) bool {
	for _, taskType := range types {
		if taskType == want {
			return true
		}
	}
	return false
}

// Cancel stops a pending or running task.
func (e *Engine) Cancel(ctx context.Context, viewer identity.Subject, taskID string) (TaskView, error) {
	row, err := e.store.get(ctx, taskID)
	if err != nil {
		return TaskView{}, err
	}
	if row.ExpiresAt != nil && !row.ExpiresAt.After(time.Now()) {
		return TaskView{}, ErrNotFound
	}
	if !visible(viewer, row) {
		return TaskView{}, ErrNotFound
	}
	if row.Status != domain.TaskPending && row.Status != domain.TaskRunning {
		return TaskView{}, Errorf(http.StatusConflict, "task_not_cancellable",
			"a %s task cannot be cancelled", row.Status)
	}
	cancelled, err := e.store.cancel(ctx, taskID)
	if err != nil {
		return TaskView{}, err
	}
	if !cancelled {
		latest, err := e.store.get(ctx, taskID)
		if err != nil {
			return TaskView{}, err
		}
		return TaskView{}, Errorf(http.StatusConflict, "task_not_cancellable",
			"a %s task cannot be cancelled", latest.Status)
	}
	// Instant if this instance is running it; otherwise the owning worker finds
	// out at its next heartbeat, which is why no cross-instance signalling is
	// needed here.
	e.cancelLocal(taskID)
	e.signalDelivery()

	updated, err := e.store.get(ctx, taskID)
	if err != nil {
		return TaskView{}, err
	}
	return viewFromRow(updated), nil
}

// DeleteTerminal removes a completed, failed, or cancelled task that the
// caller is allowed to see. The registered lifecycle cleanup runs first so
// task-owned files are removed together with the task record.
func (e *Engine) DeleteTerminal(ctx context.Context, viewer identity.Subject, taskID string) error {
	row, err := e.store.get(ctx, taskID)
	if err != nil {
		return err
	}
	if row.ExpiresAt != nil && !row.ExpiresAt.After(time.Now()) {
		return ErrNotFound
	}
	if !visible(viewer, row) {
		return ErrNotFound
	}
	if !isTerminal(row.Status) {
		return Errorf(http.StatusConflict, "task_not_deletable",
			"a %s task cannot be deleted", row.Status)
	}

	if reg, ok := e.registry.lookup(row.Type); ok {
		if expirer, ok := reg.handler.(Expirer); ok {
			if err := expirer.OnExpire(ctx, taskFromRow(row)); err != nil {
				return Errorf(http.StatusInternalServerError, "task_cleanup_failed", "task cleanup failed")
			}
		}
	}
	if err := e.store.deleteTask(ctx, row.ID); err != nil {
		return err
	}
	return nil
}

func isTerminal(status domain.TaskStatus) bool {
	return status == domain.TaskCompleted || status == domain.TaskFailed || status == domain.TaskCancelled
}

func taskFromRow(row taskRow) Task {
	return Task{
		ID:        row.ID,
		Type:      row.Type,
		ModelCode: row.ModelCode,
		Input:     row.Input,
		RequestID: row.RequestID,
	}
}

// visible reports whether viewer may see this task. A tenant-scoped caller sees
// the whole tenant; a user-scoped caller sees only their own tasks.
func visible(viewer identity.Subject, row taskRow) bool {
	if viewer.TenantID == "" || viewer.TenantID != row.TenantID {
		return false
	}
	if viewer.Scope == identity.ScopeUser {
		return viewer.UserID != "" && viewer.UserID == row.UserID
	}
	return true
}
