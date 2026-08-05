package asynctask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
)

// postgresStore implements store against ai_async_tasks.
type postgresStore struct {
	pool *pgxpool.Pool
}

func newPostgresStore(pool *pgxpool.Pool) *postgresStore {
	return &postgresStore{pool: pool}
}

func (s *postgresStore) insert(ctx context.Context, rec insertRecord) (string, bool, error) {
	var (
		id        string
		expiresAt any = nil
	)
	if !rec.ExpiresAt.IsZero() {
		expiresAt = rec.ExpiresAt
	}

	// ON CONFLICT DO NOTHING makes a concurrent duplicate submit lose the race
	// cleanly instead of returning a unique-violation the caller has to decode.
	// The partial unique index only covers non-null idempotency keys, so tasks
	// without one never conflict.
	err := s.pool.QueryRow(ctx, `
			INSERT INTO ai_async_tasks (
			  task_type, auth_method, tenant_id, user_id, api_key_id, invoke_key_id,
			  model_code, input_payload, metadata, webhook_url,
			  idempotency_key, idempotency_scope, idempotency_fingerprint,
			  max_attempts, status, available_at, expires_at
			) VALUES (
			  $1, $2, $3, NULLIF($4,''), NULLIF($5,'')::uuid, NULLIF($6,'')::uuid,
			  $7, $8, $9, NULLIF($10,''),
			  NULLIF($11,''), NULLIF($12,''), $13,
			  $14, 'pending', now(), $15
		)
		ON CONFLICT (idempotency_scope, idempotency_key)
		  WHERE idempotency_key IS NOT NULL
		DO NOTHING
		RETURNING id::text
	`,
		rec.Type, string(rec.SubjectRef.AuthMethod), rec.SubjectRef.TenantID,
		rec.SubjectRef.UserID, rec.SubjectRef.APIKeyID, rec.SubjectRef.InvokeKeyID,
		rec.ModelCode, []byte(rec.Input), nullableJSON(rec.Metadata), rec.WebhookURL,
		rec.IdempotencyKey, rec.IdempotencyScope, rec.IdempotencyFingerprint,
		rec.MaxAttempts, expiresAt,
	).Scan(&id)

	if errors.Is(err, pgx.ErrNoRows) {
		// DO NOTHING fired: an identical key already exists.
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("insert async task: %w", err)
	}
	return id, true, nil
}

func (s *postgresStore) findByIdempotencyKey(ctx context.Context, scope, key string) (idempotencyHit, error) {
	var hit idempotencyHit
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, task_type, COALESCE(idempotency_fingerprint, '')
		FROM ai_async_tasks
		WHERE idempotency_scope = $1 AND idempotency_key = $2
	`, scope, key).Scan(&hit.ID, &hit.Type, &hit.Fingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return idempotencyHit{Found: false}, nil
	}
	if err != nil {
		return idempotencyHit{}, fmt.Errorf("lookup idempotency key: %w", err)
	}
	hit.Found = true
	return hit, nil
}

func (s *postgresStore) countInFlight(ctx context.Context, tenantID string) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM ai_async_tasks
		WHERE tenant_id = $1 AND status IN ('pending', 'running')
	`, tenantID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count in-flight tasks: %w", err)
	}
	return n, nil
}

// claimSQL takes the next eligible task.
//
// Fairness: a tenant's queue position plus what it already has running must not
// exceed the cap, so one tenant queueing a thousand tasks cannot starve another.
//
// FOR UPDATE is not allowed on a query with window functions, hence the split:
// `ranked` computes the eligible set, `candidate` locks a row from it. Repeating
// status='pending' in `candidate` is not redundant — after FOR UPDATE takes the
// lock, Postgres re-evaluates the WHERE clause against the latest row version
// (EvalPlanQual), which rejects a row someone else claimed since `ranked` was
// snapshotted.
//
// request_id is settled here, before the upstream is ever called, so the
// ai_usage_logs row the attempt produces joins back to this task — and so the
// reaper can tell whether an attempt already reached billing.
const claimSQL = `
WITH running AS (
    SELECT tenant_id, count(*)::int AS n
    FROM ai_async_tasks
    WHERE status = 'running' AND lease_expires_at > now()
    GROUP BY tenant_id
),
ranked AS (
    SELECT t.id,
           t.created_at,
           row_number() OVER (PARTITION BY t.tenant_id ORDER BY t.created_at, t.id)
             + COALESCE(r.n, 0) AS tenant_rank
    FROM ai_async_tasks t
    LEFT JOIN running r ON r.tenant_id = t.tenant_id
    WHERE t.status = 'pending'
      AND t.available_at <= now()
      AND t.task_type = ANY($1::text[])
),
candidate AS (
    SELECT id
    FROM ai_async_tasks
    WHERE status = 'pending'
      AND id IN (SELECT id FROM ranked WHERE tenant_rank <= $2)
    ORDER BY created_at, id
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
UPDATE ai_async_tasks t
SET status           = 'running',
    worker_id        = $3,
    lease_expires_at = now() + make_interval(secs => $4),
    attempt_count    = t.attempt_count + 1,
    started_at       = COALESCE(t.started_at, now()),
    request_id       = 'atsk_' || t.id::text || '_' || (t.attempt_count + 1)::text
FROM candidate c
WHERE t.id = c.id AND t.status = 'pending'
RETURNING t.id::text, t.task_type, t.auth_method, t.tenant_id,
          COALESCE(t.user_id, ''), COALESCE(t.api_key_id::text, ''),
          COALESCE(t.invoke_key_id::text, ''), t.model_code,
          t.input_payload, t.attempt_count, COALESCE(t.request_id, '')
`

func (s *postgresStore) claim(ctx context.Context, types []string, cap int, workerID string, lease time.Duration) (claimedTask, bool, error) {
	if len(types) == 0 {
		return claimedTask{}, false, nil
	}
	var (
		t          claimedTask
		authMethod string
	)
	err := s.pool.QueryRow(ctx, claimSQL, types, cap, workerID, lease.Seconds()).Scan(
		&t.ID, &t.Type, &authMethod, &t.SubjectRef.TenantID,
		&t.SubjectRef.UserID, &t.SubjectRef.APIKeyID, &t.SubjectRef.InvokeKeyID,
		&t.ModelCode, &t.Input, &t.Attempt, &t.RequestID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return claimedTask{}, false, nil
	}
	if err != nil {
		return claimedTask{}, false, fmt.Errorf("claim async task: %w", err)
	}
	t.SubjectRef.AuthMethod = identity.AuthMethod(authMethod)
	return t, true, nil
}

func (s *postgresStore) heartbeat(ctx context.Context, taskID, workerID string, lease time.Duration) (bool, error) {
	// Scoping to worker_id and status='running' means zero rows has exactly two
	// causes: the lease was taken, or the task was cancelled. Both mean stop.
	// That is also how cross-instance cancellation reaches a running worker —
	// no pub/sub needed.
	tag, err := s.pool.Exec(ctx, `
		UPDATE ai_async_tasks
		SET lease_expires_at = now() + make_interval(secs => $3)
		WHERE id = $1::uuid AND worker_id = $2 AND status = 'running'
	`, taskID, workerID, lease.Seconds())
	if err != nil {
		return false, fmt.Errorf("heartbeat async task: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (s *postgresStore) complete(ctx context.Context, taskID, workerID string, res Result) (bool, error) {
	var failure Failure
	if res.Failure != nil {
		failure = *res.Failure
	}
	// Requiring worker_id blocks both a cancelled task being overwritten and a
	// zombie worker writing after its lease was reaped and reassigned.
	var written bool
	err := s.pool.QueryRow(ctx, `
		WITH completed AS (
		  UPDATE ai_async_tasks
		  SET status                = $3,
		      result_payload        = $4,
		      error_code            = NULLIF($5, ''),
		      error_message         = NULLIF($6, ''),
		      internal_error_detail = NULLIF($7, ''),
		      failed_step           = NULLIF($8, ''),
		      caller_charge           = GREATEST(caller_charge, $9),
		      completed_at          = now(),
		      worker_id             = NULL,
		      lease_expires_at      = NULL
		  WHERE id = $1::uuid AND worker_id = $2 AND status = 'running'
		  RETURNING id, status, webhook_url
		), enqueued AS (
		  INSERT INTO ai_async_task_deliveries (task_id, url, payload)
		  SELECT id, webhook_url, jsonb_build_object(
		    'source', 'UniHub',
		    'event', 'task.' || status,
		    'task_id', id::text
		  )
		  FROM completed
		  WHERE NULLIF(webhook_url, '') IS NOT NULL
		  ON CONFLICT (task_id) DO NOTHING
		)
		SELECT EXISTS (SELECT 1 FROM completed)
	`, taskID, workerID, string(res.Status), nullableJSON(res.Output),
		failure.Code, failure.Message, failure.InternalDetail, failure.Step,
		res.CallerCharge).Scan(&written)
	if err != nil {
		return false, fmt.Errorf("complete async task: %w", err)
	}
	return written, nil
}

// reapRetryableSQL returns orphaned tasks to pending with exponential backoff.
//
// The NOT EXISTS clause is the double-spend guard. request_id is written before
// the upstream call, so a matching ai_usage_logs row proves the attempt already
// reached billing; retrying it would charge the customer twice. Such tasks fall
// through to reapDead instead.
const reapRetryableSQL = `
UPDATE ai_async_tasks
SET status           = 'pending',
    worker_id        = NULL,
    lease_expires_at = NULL,
    available_at     = now() + make_interval(secs => least(300, 5 * (1 << least(attempt_count, 6))))
WHERE id IN (
    SELECT id FROM ai_async_tasks t
    WHERE t.status = 'running'
      AND t.lease_expires_at < now()
      AND t.attempt_count < t.max_attempts
      AND NOT EXISTS (
          SELECT 1 FROM ai_usage_logs u WHERE u.request_id = t.request_id
      )
    ORDER BY t.lease_expires_at
    FOR UPDATE SKIP LOCKED
    LIMIT $1
)
`

func (s *postgresStore) reapRetryable(ctx context.Context, limit int) (int64, error) {
	tag, err := s.pool.Exec(ctx, reapRetryableSQL, limit)
	if err != nil {
		return 0, fmt.Errorf("reap retryable async tasks: %w", err)
	}
	return tag.RowsAffected(), nil
}

// reapDeadSQL fails the orphans that reapRetryable left behind: attempts
// exhausted, or the attempt already cost money.
const reapDeadSQL = `
WITH dead AS (
    UPDATE ai_async_tasks
    SET status           = 'failed',
        worker_id        = NULL,
        lease_expires_at = NULL,
        error_code       = 'worker_lost',
        error_message    = 'task worker stopped renewing its lease',
        failed_step      = 'lease',
        completed_at     = now()
    WHERE id IN (
        SELECT id FROM ai_async_tasks t
        WHERE t.status = 'running' AND t.lease_expires_at < now()
        ORDER BY t.lease_expires_at
        FOR UPDATE SKIP LOCKED
        LIMIT $1
    )
    RETURNING id, status, webhook_url
), enqueued AS (
    INSERT INTO ai_async_task_deliveries (task_id, url, payload)
    SELECT id, webhook_url, jsonb_build_object(
        'source', 'UniHub',
        'event', 'task.' || status,
        'task_id', id::text
    )
    FROM dead
    WHERE NULLIF(webhook_url, '') IS NOT NULL
    ON CONFLICT (task_id) DO NOTHING
)
SELECT count(*) FROM dead
`

func (s *postgresStore) reapDead(ctx context.Context, limit int) (int64, error) {
	var count int64
	err := s.pool.QueryRow(ctx, reapDeadSQL, limit).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("reap dead async tasks: %w", err)
	}
	return count, nil
}

func (s *postgresStore) releaseWorker(ctx context.Context, workerID string) (int64, error) {
	// Scoped to this worker's id, so a second instance is untouched. This and
	// the reaper are the only paths allowed to move running back to pending —
	// the old console code did it unconditionally for every running row, which
	// is why a second instance could reset tasks the first was still executing.
	//
	// The NOT EXISTS clause is the same double-spend guard reapRetryable uses: an
	// attempt whose request_id already reached ai_usage_logs has billed, so it
	// must not be requeued and run again. Such a row is left running for the
	// lease-expiry path (reapDead) to fail. With MaxAttempts=1 handlers no row is
	// running here at all once workers have drained; the guard is what keeps the
	// fast handback safe if a handler ever opts into retries.
	tag, err := s.pool.Exec(ctx, `
		UPDATE ai_async_tasks
		SET status           = 'pending',
		    worker_id        = NULL,
		    lease_expires_at = NULL,
		    started_at       = NULL,
		    attempt_count    = greatest(attempt_count - 1, 0)
		WHERE worker_id = $1 AND status = 'running'
		  AND NOT EXISTS (
		      SELECT 1 FROM ai_usage_logs u WHERE u.request_id = ai_async_tasks.request_id
		  )
	`, workerID)
	if err != nil {
		return 0, fmt.Errorf("release worker async tasks: %w", err)
	}
	return tag.RowsAffected(), nil
}

const taskSelectColumns = `
	id::text, task_type, status, model_code, auth_method, tenant_id,
	COALESCE(user_id, ''), COALESCE(api_key_id::text, ''), COALESCE(invoke_key_id::text, ''),
	input_payload, result_payload, metadata, COALESCE(webhook_url, ''),
	COALESCE(idempotency_key, ''), COALESCE(request_id, ''), attempt_count, caller_charge,
	COALESCE(error_code, ''), COALESCE(error_message, ''),
	created_at, started_at, completed_at, expires_at
`

func scanTaskRow(row pgx.Row) (taskRow, error) {
	var (
		t      taskRow
		status string
	)
	err := row.Scan(
		&t.ID, &t.Type, &status, &t.ModelCode, &t.AuthMethod, &t.TenantID,
		&t.UserID, &t.APIKeyID, &t.InvokeKeyID,
		&t.Input, &t.Output, &t.Metadata, &t.WebhookURL,
		&t.IdempotencyKey, &t.RequestID, &t.Attempt, &t.CallerCharge,
		&t.ErrorCode, &t.ErrorMessage,
		&t.CreatedAt, &t.StartedAt, &t.CompletedAt, &t.ExpiresAt,
	)
	if err != nil {
		return taskRow{}, err
	}
	t.Status = domain.TaskStatus(status)
	return t, nil
}

func (s *postgresStore) get(ctx context.Context, taskID string) (taskRow, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+taskSelectColumns+` FROM ai_async_tasks WHERE id = $1::uuid`, taskID)
	t, err := scanTaskRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return taskRow{}, ErrNotFound
	}
	if err != nil {
		return taskRow{}, fmt.Errorf("get async task: %w", err)
	}
	return t, nil
}

func (s *postgresStore) list(ctx context.Context, filter listRecord) ([]taskRow, error) {
	if len(filter.Types) == 0 || filter.Limit <= 0 {
		return []taskRow{}, nil
	}
	var cursorTime any
	cursorID := ""
	if filter.Cursor != nil {
		cursorTime = filter.Cursor.CreatedAt
		cursorID = filter.Cursor.ID
	}
	rows, err := s.pool.Query(ctx, `SELECT `+taskSelectColumns+`
		FROM ai_async_tasks
		WHERE tenant_id = $1
		  AND (
		    $2 = ''
		    OR ($2 = 'tenant' AND COALESCE(user_id, '') = '')
		    OR ($2 = 'user' AND ($3 = '' OR COALESCE(user_id, '') = $3) AND COALESCE(user_id, '') <> '')
		  )
		  AND task_type = ANY($4::text[])
		  AND ($5 = '' OR status = $5)
		  AND (expires_at IS NULL OR expires_at > now())
		  AND ($6::timestamptz IS NULL OR (created_at, id) < ($6, NULLIF($7, '')::uuid))
		ORDER BY created_at DESC, id DESC
		LIMIT $8
	`, filter.TenantID, string(filter.OwnerScope), filter.UserID, filter.Types, string(filter.Status), cursorTime, cursorID, filter.Limit)
	if err != nil {
		return nil, fmt.Errorf("list async tasks: %w", err)
	}
	defer rows.Close()
	out := make([]taskRow, 0, filter.Limit)
	for rows.Next() {
		row, err := scanTaskRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan async task list: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list async tasks: %w", err)
	}
	return out, nil
}

func (s *postgresStore) cancel(ctx context.Context, taskID string) (bool, error) {
	// A running task is cancelled in the row here; its worker finds out at the
	// next heartbeat (zero rows) and drops the context. The submitting instance
	// also cancels its in-memory context immediately, so the common case is
	// instant and the cross-instance case costs at most one heartbeat interval.
	var cancelled bool
	err := s.pool.QueryRow(ctx, `
		WITH cancelled AS (
		  UPDATE ai_async_tasks
		  SET status           = 'cancelled',
		      error_code       = 'cancelled',
		      error_message    = 'cancelled by user',
		      completed_at     = now(),
		      worker_id        = NULL,
		      lease_expires_at = NULL
		  WHERE id = $1::uuid AND status IN ('pending', 'running')
		  RETURNING id, status, webhook_url
		), enqueued AS (
		  INSERT INTO ai_async_task_deliveries (task_id, url, payload)
		  SELECT id, webhook_url, jsonb_build_object(
		    'source', 'UniHub',
		    'event', 'task.' || status,
		    'task_id', id::text
		  )
		  FROM cancelled
		  WHERE NULLIF(webhook_url, '') IS NOT NULL
		  ON CONFLICT (task_id) DO NOTHING
		)
		SELECT EXISTS (SELECT 1 FROM cancelled)
	`, taskID).Scan(&cancelled)
	if err != nil {
		return false, fmt.Errorf("cancel async task: %w", err)
	}
	return cancelled, nil
}

func (s *postgresStore) listExpired(ctx context.Context, limit int) ([]taskRow, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+taskSelectColumns+`
		FROM ai_async_tasks
		WHERE expires_at IS NOT NULL AND expires_at < now()
		ORDER BY expires_at
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list expired async tasks: %w", err)
	}
	defer rows.Close()
	out := make([]taskRow, 0, limit)
	for rows.Next() {
		t, err := scanTaskRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan expired async task: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *postgresStore) deleteTask(ctx context.Context, taskID string) error {
	// Deliveries belong to the task module, so cleanup is explicit and atomic
	// instead of relying on a database foreign-key cascade.
	if _, err := s.pool.Exec(ctx, `
		WITH deleted_deliveries AS (
		  DELETE FROM ai_async_task_deliveries
		  WHERE task_id = $1::uuid
		)
		DELETE FROM ai_async_tasks WHERE id = $1::uuid
	`, taskID); err != nil {
		return fmt.Errorf("delete async task: %w", err)
	}
	return nil
}

const claimDeliverySQL = `
WITH candidate AS (
    SELECT id
    FROM ai_async_task_deliveries
    WHERE ((
        status = 'pending' AND available_at <= now()
    ) OR (
        status = 'running' AND lease_expires_at < now()
    ))
      AND attempt_count < max_attempts
    ORDER BY available_at, created_at, id
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
UPDATE ai_async_task_deliveries d
SET status           = 'running',
    worker_id        = $1,
    lease_expires_at = now() + make_interval(secs => $2),
    attempt_count    = d.attempt_count + 1
FROM candidate c
WHERE d.id = c.id
RETURNING d.id::text, d.task_id::text, d.url, d.payload,
          d.attempt_count, d.max_attempts
`

func (s *postgresStore) claimDelivery(ctx context.Context, workerID string, lease time.Duration) (claimedDelivery, bool, error) {
	var delivery claimedDelivery
	err := s.pool.QueryRow(ctx, claimDeliverySQL, workerID, lease.Seconds()).Scan(
		&delivery.ID, &delivery.TaskID, &delivery.URL, &delivery.Payload,
		&delivery.Attempt, &delivery.MaxAttempts,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return claimedDelivery{}, false, nil
	}
	if err != nil {
		return claimedDelivery{}, false, fmt.Errorf("claim async task webhook delivery: %w", err)
	}
	return delivery, true, nil
}

func (s *postgresStore) finishDelivery(ctx context.Context, deliveryID, workerID string, outcome deliveryOutcome) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE ai_async_task_deliveries
		SET status           = $3,
		    last_status_code = NULLIF($4, 0),
		    last_error       = NULLIF($5, ''),
		    available_at     = CASE WHEN $3 = 'pending'
		                            THEN now() + make_interval(secs => $6)
		                            ELSE available_at END,
		    worker_id        = NULL,
		    lease_expires_at = NULL,
		    completed_at     = CASE WHEN $3 IN ('delivered', 'failed') THEN now() ELSE NULL END
		WHERE id = $1::uuid AND worker_id = $2 AND status = 'running'
	`, deliveryID, workerID, outcome.Status, outcome.StatusCode, outcome.LastError, outcome.RetryAfter.Seconds())
	if err != nil {
		return false, fmt.Errorf("finish async task webhook delivery: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (s *postgresStore) reapDeadDeliveries(ctx context.Context, limit int) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE ai_async_task_deliveries
		SET status           = 'failed',
		    worker_id        = NULL,
		    lease_expires_at = NULL,
		    last_error       = 'webhook worker stopped after the final attempt',
		    completed_at     = now()
		WHERE id IN (
		  SELECT id FROM ai_async_task_deliveries
		  WHERE status = 'running'
		    AND lease_expires_at < now()
		    AND attempt_count >= max_attempts
		  ORDER BY lease_expires_at
		  FOR UPDATE SKIP LOCKED
		  LIMIT $1
		)
	`, limit)
	if err != nil {
		return 0, fmt.Errorf("reap dead async task webhook deliveries: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (s *postgresStore) releaseDeliveries(ctx context.Context, workerID string) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE ai_async_task_deliveries
		SET status           = 'pending',
		    worker_id        = NULL,
		    lease_expires_at = NULL,
		    available_at     = now(),
		    attempt_count    = greatest(attempt_count - 1, 0)
		WHERE worker_id = $1 AND status = 'running'
	`, workerID)
	if err != nil {
		return 0, fmt.Errorf("release async task webhook deliveries: %w", err)
	}
	return tag.RowsAffected(), nil
}

// nullableJSON keeps empty JSON out of the row as SQL NULL rather than a
// literal "null", so `metadata IS NULL` means "client sent none".
func nullableJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return []byte(raw)
}
