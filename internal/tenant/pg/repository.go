package pg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"xiaodou/dai/internal/auth"
	authports "xiaodou/dai/internal/auth/ports"
	"xiaodou/dai/internal/billing"
	tenantports "xiaodou/dai/internal/tenant/ports"
)

type Pagination struct {
	Page   int64 `json:"page"`
	Size   int64 `json:"size"`
	Offset int64 `json:"-"`
}

func NewPagination(page, size int64) Pagination {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}
	return Pagination{Page: page, Size: size, Offset: (page - 1) * size}
}

type PaginatedResult[T any] struct {
	Records []T   `json:"records"`
	Total   int64 `json:"total"`
	Page    int64 `json:"page"`
	Size    int64 `json:"size"`
}

type TenantRepository struct {
	pool            *pgxpool.Pool
	activationStore activationStore
}

func (r *TenantRepository) RequestDeletion(ctx context.Context, tenantID, requestedBy string, executeAfter time.Time) (tenantports.TenantDeletionJob, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return tenantports.TenantDeletionJob{}, err
	}
	defer tx.Rollback(ctx)
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM iam_tenants WHERE tenant_id=$1)`, tenantID).Scan(&exists); err != nil {
		return tenantports.TenantDeletionJob{}, err
	}
	if !exists {
		return tenantports.TenantDeletionJob{}, pgx.ErrNoRows
	}
	if _, err := tx.Exec(ctx, `UPDATE iam_tenants SET status='deleting', updated_at=now() WHERE tenant_id=$1 AND status NOT IN ('deleting','purging')`, tenantID); err != nil {
		return tenantports.TenantDeletionJob{}, err
	}
	var job tenantports.TenantDeletionJob
	err = tx.QueryRow(ctx, `INSERT INTO tenant_deletion_jobs (tenant_id, requested_by, execute_after) VALUES ($1,$2,$3) ON CONFLICT (tenant_id) WHERE status IN ('pending','running') DO UPDATE SET requested_by=EXCLUDED.requested_by RETURNING job_id, tenant_id, status, requested_at, execute_after, started_at, completed_at, COALESCE(last_error,'')`, tenantID, requestedBy, executeAfter).Scan(&job.JobID, &job.TenantID, &job.Status, &job.RequestedAt, &job.ExecuteAfter, &job.StartedAt, &job.CompletedAt, &job.LastError)
	if err != nil {
		return tenantports.TenantDeletionJob{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return tenantports.TenantDeletionJob{}, err
	}
	return job, nil
}

func (r *TenantRepository) CancelDeletion(ctx context.Context, tenantID string) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE tenant_deletion_jobs SET status='cancelled', completed_at=now() WHERE tenant_id=$1 AND status='pending'`, tenantID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	if _, err := tx.Exec(ctx, `UPDATE iam_tenants SET status='disabled', updated_at=now() WHERE tenant_id=$1 AND status='deleting'`, tenantID); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (r *TenantRepository) GetDeletion(ctx context.Context, tenantID string) (tenantports.TenantDeletionJob, error) {
	var job tenantports.TenantDeletionJob
	err := r.pool.QueryRow(ctx, `SELECT job_id,tenant_id,status,requested_at,execute_after,started_at,completed_at,COALESCE(last_error,'') FROM tenant_deletion_jobs WHERE tenant_id=$1 ORDER BY requested_at DESC LIMIT 1`, tenantID).Scan(&job.JobID, &job.TenantID, &job.Status, &job.RequestedAt, &job.ExecuteAfter, &job.StartedAt, &job.CompletedAt, &job.LastError)
	return job, err
}

func (r *TenantRepository) GetDueDeletion(ctx context.Context) (tenantports.TenantDeletionJob, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return tenantports.TenantDeletionJob{}, err
	}
	defer tx.Rollback(ctx)
	var job tenantports.TenantDeletionJob
	err = tx.QueryRow(ctx, `SELECT job_id,tenant_id,status,requested_at,execute_after,started_at,completed_at,COALESCE(last_error,'') FROM tenant_deletion_jobs WHERE status IN ('pending','failed') AND attempts < 10 AND execute_after<=now() ORDER BY execute_after FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&job.JobID, &job.TenantID, &job.Status, &job.RequestedAt, &job.ExecuteAfter, &job.StartedAt, &job.CompletedAt, &job.LastError)
	if err != nil {
		return tenantports.TenantDeletionJob{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE tenant_deletion_jobs SET status='running',started_at=now(),attempts=attempts+1 WHERE job_id=$1`, job.JobID); err != nil {
		return tenantports.TenantDeletionJob{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return tenantports.TenantDeletionJob{}, err
	}
	return job, nil
}

func (r *TenantRepository) RunDeletion(ctx context.Context, jobID, tenantID string) error {
	if _, err := r.DeleteTenant(ctx, tenantID); err != nil {
		_, _ = r.pool.Exec(ctx, `UPDATE tenant_deletion_jobs SET status='failed',last_error=$2,execute_after=now()+interval '5 minutes' WHERE job_id=$1`, jobID, err.Error())
		return err
	}
	_, err := r.pool.Exec(ctx, `UPDATE tenant_deletion_jobs SET status='completed',completed_at=now() WHERE job_id=$1`, jobID)
	return err
}

var _ tenantports.AdminTenantStatusWriter = (*TenantRepository)(nil)
var _ tenantports.AdminTenantWriter = (*TenantRepository)(nil)
var _ tenantports.AdminTenantReader = (*TenantRepository)(nil)

type activationStore interface {
	Store(ctx context.Context, tx pgx.Tx, userID, purpose string, credential auth.ActivationCredential) error
}

func NewTenantRepository(pool *pgxpool.Pool, activationStores ...activationStore) *TenantRepository {
	var activationStore activationStore
	if len(activationStores) > 0 {
		activationStore = activationStores[0]
	}
	return &TenantRepository{pool: pool, activationStore: activationStore}
}

// CreateTenant atomically creates a tenant and its optional initial tenant
// user. The activation record is written through the auth capability using
// the same transaction.
func (r *TenantRepository) CreateTenant(ctx context.Context, input tenantports.TenantCreateCommand) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
		INSERT INTO iam_tenants (tenant_id, tenant_name, contact_person, contact_email, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $6)
	`, input.TenantID, input.TenantName, input.ContactPerson, input.ContactEmail, input.Status, now); err != nil {
		return err
	}
	if input.InitialUser != nil {
		if r.activationStore == nil {
			return fmt.Errorf("tenant activation store is not configured")
		}
		user := input.InitialUser
		if _, err := tx.Exec(ctx, `
			INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, credential_state, email, user_type, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, 'pending_activation', $5, 3, 'active', $6, $6)
		`, user.UserID, input.TenantID, user.Username, user.PasswordHash, user.Email, now); err != nil {
			return translateAccountConstraint(err)
		}
		if err := r.activationStore.Store(ctx, tx, user.UserID, auth.ActivationPurposeAccount, auth.ActivationCredential{
			PasswordHash: user.PasswordHash,
			TokenHash:    user.ActivationTokenHash,
			ExpiresAt:    user.ActivationExpiresAt,
		}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func translateAccountConstraint(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "ux_iam_accounts_username_normalized":
			return authports.ErrUsernameTaken
		case "ux_iam_accounts_email_normalized":
			return authports.ErrEmailTaken
		}
	}
	return err
}

func (r *TenantRepository) UpdateTenant(ctx context.Context, input tenantports.TenantUpdateCommand) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var count int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM iam_tenants
		WHERE tenant_name = $1 AND tenant_id <> $2
	`, input.TenantName, input.TenantID).Scan(&count); err != nil {
		return false, err
	}
	if count > 0 {
		return false, fmt.Errorf("tenant name already exists: %w", ErrTenantNameTaken)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE iam_tenants
		SET tenant_name = $1, contact_person = $2, contact_email = $3, status = $4, updated_at = $5
		WHERE tenant_id = $6
	`, input.TenantName, input.ContactPerson, input.ContactEmail, input.Status, time.Now().UTC(), input.TenantID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() != 1 {
		return false, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (r *TenantRepository) DeleteTenant(ctx context.Context, tenantID string) (bool, error) {
	if err := r.deleteTenantArchivedPayloads(ctx, tenantID); err != nil {
		return false, err
	}
	if err := r.deleteTenantLargeTables(ctx, tenantID); err != nil {
		return false, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM iam_tenants WHERE tenant_id = $1)`, tenantID).Scan(&exists); err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	// Destructive tenant deletion is intentionally explicit and transactional.
	// Financial/usage rows are removed together with the tenant in this product
	// tier; keeping a partial tenant graph would leave unusable orphan records.
	deletes := []string{
		`DELETE FROM sys_notification_deliveries WHERE tenant_id = $1`,
		`DELETE FROM ann_audit_events WHERE actor_tenant_id = $1`,
		`DELETE FROM ann_audiences WHERE tenant_id = $1`,
		`DELETE FROM ann_receipts WHERE tenant_id = $1`,
		`DELETE FROM ann_announcements WHERE publisher_tenant_id = $1`,
		`DELETE FROM ledger_credit_leases WHERE tenant_id = $1`,
		`DELETE FROM ai_upstream_resource_tenant_policies WHERE tenant_id = $1`,
		`DELETE FROM ai_price_book_entries WHERE price_book_id IN (SELECT id FROM ai_price_books WHERE owner_type = 'tenant' AND owner_tenant_id = $1)`,
		`DELETE FROM ai_price_books WHERE owner_type = 'tenant' AND owner_tenant_id = $1`,
		`DELETE FROM ai_admin_audit_logs WHERE actor_tenant_id = $1`,
		`DELETE FROM ai_billing_settlement_outbox WHERE batch_id IN (SELECT batch_id FROM ai_billing_settlement_batches WHERE window_id IN (SELECT window_id FROM ai_billing_windows WHERE tenant_id = $1))`,
		`DELETE FROM ai_billing_settlement_batches WHERE window_id IN (SELECT window_id FROM ai_billing_windows WHERE tenant_id = $1)`,
		`DELETE FROM ai_billing_request_admissions WHERE window_id IN (SELECT window_id FROM ai_billing_windows WHERE tenant_id = $1)`,
		`DELETE FROM ai_billing_windows WHERE tenant_id = $1`,
		`DELETE FROM ai_async_task_deliveries WHERE task_id IN (SELECT id FROM ai_async_tasks WHERE tenant_id = $1)`,
		`DELETE FROM ai_sub_plan_purchase_policy_revisions WHERE plan_id IN (SELECT id FROM ai_sub_plans WHERE tenant_id = $1)`,
		`DELETE FROM ai_sub_plan_purchase_policies WHERE plan_id IN (SELECT id FROM ai_sub_plans WHERE tenant_id = $1)`,
		`DELETE FROM ai_sub_plan_groups WHERE plan_id IN (SELECT id FROM ai_sub_plans WHERE tenant_id = $1)`,
		`DELETE FROM ai_sub_subscriptions WHERE tenant_id = $1`,
		`DELETE FROM ai_sub_orders WHERE tenant_id = $1`,
		`DELETE FROM ai_sub_plans WHERE tenant_id = $1`,
		`DELETE FROM ai_user_groups WHERE tenant_id = $1`,
		`DELETE FROM ai_group_model_dispatch_rules WHERE group_id IN (SELECT id FROM ai_groups WHERE tenant_id = $1)`,
		`DELETE FROM ai_group_client_surfaces WHERE group_id IN (SELECT id FROM ai_groups WHERE tenant_id = $1)`,
		`DELETE FROM ai_group_targets WHERE group_id IN (SELECT id FROM ai_groups WHERE tenant_id = $1)`,
		`DELETE FROM ai_api_keys WHERE tenant_id = $1`,
		`DELETE FROM ai_workspace_messages WHERE thread_id IN (SELECT id FROM ai_workspace_threads WHERE tenant_id = $1)`,
		`DELETE FROM ai_workspace_threads WHERE tenant_id = $1`,
		`DELETE FROM ai_async_tasks WHERE tenant_id = $1`,
		`DELETE FROM ai_conversation_bindings WHERE tenant_id = $1`,
		`DELETE FROM ai_runtime_limit_policies WHERE (scope_type = 'tenant' AND scope_id = $1) OR (scope_type = 'user' AND scope_id IN (SELECT user_id FROM iam_accounts WHERE tenant_id = $1))`,
		`DELETE FROM ai_request_payloads WHERE request_id IN (SELECT request_id FROM ai_usage_logs WHERE tenant_id = $1)`,
		`DELETE FROM ai_usage_rollups_hourly WHERE tenant_id = $1`,
		`DELETE FROM ai_usage_logs WHERE tenant_id = $1`,
		`DELETE FROM ai_content_moderation_logs WHERE tenant_id = $1`,
		`DELETE FROM ai_prompt_audit_events WHERE tenant_id = $1`,
		`DELETE FROM ai_risk_events WHERE tenant_id = $1`,
		`DELETE FROM ai_audit_inbox WHERE payload->>'tenant_id' = $1`,
		`DELETE FROM bill_charge_outbox WHERE tenant_id = $1`,
		`DELETE FROM bill_credit_lots WHERE account_id IN (SELECT account_id FROM bill_accounts WHERE tenant_id = $1)`,
		`DELETE FROM bill_refund_reversal_effects WHERE account_id IN (SELECT account_id FROM bill_accounts WHERE tenant_id = $1)`,
		`DELETE FROM pay_refunds WHERE payment_order_id IN (SELECT order_id FROM pay_orders WHERE tenant_id = $1)`,
		`DELETE FROM bill_recharge_orders WHERE tenant_id = $1`,
		`DELETE FROM pay_orders WHERE tenant_id = $1`,
		`DELETE FROM pay_cash_ledger WHERE tenant_id = $1`,
		`DELETE FROM pay_withdrawals WHERE tenant_id = $1`,
		`DELETE FROM iam_user_legal_acceptances WHERE tenant_id = $1`,
		`DELETE FROM iam_invitation_codes WHERE tenant_id = $1`,
		`DELETE FROM pay_tenant_settings WHERE tenant_id = $1`,
		`DELETE FROM iam_tenant_portal_branding WHERE tenant_id = $1`,
		`DELETE FROM bill_accounts WHERE tenant_id = $1`,
		`DELETE FROM ai_groups WHERE tenant_id = $1`,
		`DELETE FROM iam_accounts WHERE tenant_id = $1`,
		`DELETE FROM iam_tenants WHERE tenant_id = $1`,
	}
	for _, query := range deletes {
		if _, err := tx.Exec(ctx, query, tenantID); err != nil {
			return false, fmt.Errorf("tenant deletion query failed: %s: %w", query, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (r *TenantRepository) deleteTenantArchivedPayloads(ctx context.Context, tenantID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT tablename FROM pg_catalog.pg_tables WHERE schemaname = current_schema() AND tablename ~ '^ai_request_payloads_archive_[0-9]{4}_[0-9]{2}$'`)
	if err != nil {
		return err
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, table := range tables {
		query := fmt.Sprintf(`DELETE FROM %s WHERE request_id IN (SELECT request_id FROM ai_usage_logs WHERE tenant_id = $1)`, pgx.Identifier{table}.Sanitize())
		if _, err := tx.Exec(ctx, query, tenantID); err != nil {
			return fmt.Errorf("tenant deletion archive query failed: %s: %w", table, err)
		}
	}
	return tx.Commit(ctx)
}

// deleteTenantLargeTables removes high-volume rows in short independent
// transactions. The final tenant graph cleanup below remains transactional,
// while these batches prevent a large tenant from holding locks for minutes.
func (r *TenantRepository) deleteTenantLargeTables(ctx context.Context, tenantID string) error {
	const batchSize = 1000
	for {
		tx, err := r.pool.Begin(ctx)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `DELETE FROM ai_usage_logs WHERE ctid IN (SELECT ctid FROM ai_usage_logs WHERE tenant_id = $1 LIMIT $2) RETURNING request_id`, tenantID, batchSize)
		if err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		requestIDs := make([]string, 0, batchSize)
		for rows.Next() {
			var requestID string
			if err := rows.Scan(&requestID); err != nil {
				rows.Close()
				_ = tx.Rollback(ctx)
				return err
			}
			requestIDs = append(requestIDs, requestID)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			_ = tx.Rollback(ctx)
			return err
		}
		rows.Close()
		if len(requestIDs) > 0 {
			if _, err := tx.Exec(ctx, `DELETE FROM ai_request_payloads WHERE request_id = ANY($1::text[])`, requestIDs); err != nil {
				_ = tx.Rollback(ctx)
				return err
			}
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
		if len(requestIDs) == 0 {
			break
		}
	}
	for _, table := range []string{"ai_usage_rollups_hourly", "ai_content_moderation_logs", "ai_prompt_audit_events", "ai_risk_events", "sys_notification_deliveries"} {
		for {
			tx, err := r.pool.Begin(ctx)
			if err != nil {
				return err
			}
			query := fmt.Sprintf(`DELETE FROM %s WHERE ctid IN (SELECT ctid FROM %s WHERE tenant_id = $1 LIMIT $2)`, pgx.Identifier{table}.Sanitize(), pgx.Identifier{table}.Sanitize())
			tag, err := tx.Exec(ctx, query, tenantID, batchSize)
			if err != nil {
				_ = tx.Rollback(ctx)
				return err
			}
			if err = tx.Commit(ctx); err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				break
			}
		}
	}
	for {
		tx, err := r.pool.Begin(ctx)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM ai_async_task_deliveries WHERE task_id IN (SELECT id FROM ai_async_tasks WHERE tenant_id = $1 LIMIT $2)`, tenantID, batchSize)
		if err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			break
		}
	}
	for {
		tx, err := r.pool.Begin(ctx)
		if err != nil {
			return err
		}
		query := `DELETE FROM ai_async_tasks WHERE ctid IN (SELECT ctid FROM ai_async_tasks WHERE tenant_id = $1 LIMIT $2)`
		tag, err := tx.Exec(ctx, query, tenantID, batchSize)
		if err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			break
		}
	}
	return nil
}

// UpdateStatus atomically updates the tenant access state and cascades only
// the reversible inherited_disabled state to organization/end-user accounts.
// Redis blacklist synchronization is intentionally left to the caller.
func (r *TenantRepository) UpdateStatus(ctx context.Context, tenantID, status string) (tenantports.AdminTenantStatusResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return tenantports.AdminTenantStatusResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now().UTC()
	tag, err := tx.Exec(ctx, `
		UPDATE iam_tenants SET status = $1, updated_at = $2
		WHERE tenant_id = $3
	`, status, now, tenantID)
	if err != nil {
		return tenantports.AdminTenantStatusResult{}, err
	}
	if tag.RowsAffected() != 1 {
		return tenantports.AdminTenantStatusResult{}, nil
	}

	result := tenantports.AdminTenantStatusResult{Updated: true}
	if status == "disabled" {
		if _, err := tx.Exec(ctx, `
			UPDATE iam_accounts SET status = 'inherited_disabled', updated_at = $1
			WHERE tenant_id = $2 AND user_type IN (3, 4) AND status = 'active'
		`, now, tenantID); err != nil {
			return tenantports.AdminTenantStatusResult{}, err
		}
	} else {
		rows, err := tx.Query(ctx, `
			UPDATE iam_accounts SET status = 'active', updated_at = $1
			WHERE tenant_id = $2 AND user_type IN (3, 4) AND status = 'inherited_disabled'
			RETURNING user_id
		`, now, tenantID)
		if err != nil {
			return tenantports.AdminTenantStatusResult{}, err
		}
		for rows.Next() {
			var userID string
			if err := rows.Scan(&userID); err != nil {
				rows.Close()
				return tenantports.AdminTenantStatusResult{}, err
			}
			result.RestoredUserIDs = append(result.RestoredUserIDs, userID)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return tenantports.AdminTenantStatusResult{}, err
		}
		rows.Close()
	}
	if err := tx.Commit(ctx); err != nil {
		return tenantports.AdminTenantStatusResult{}, err
	}
	return result, nil
}

// GetTenantDetails returns the tenant detail projection used by management
// routes without exposing the SQL query to HTTP transport.
func (r *TenantRepository) GetTenantDetails(ctx context.Context, tenantID string) (*tenantports.TenantDetails, error) {
	var details tenantports.TenantDetails
	var createdAt time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT tenant_id, tenant_name, contact_person, contact_email, status, created_at
		FROM iam_tenants
		WHERE tenant_id = $1
	`, tenantID).Scan(
		&details.TenantID, &details.TenantName, &details.ContactPerson,
		&details.ContactEmail, &details.Status, &createdAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, tenantports.ErrTenantNotFound
	}
	if err != nil {
		return nil, err
	}
	details.CreatedTime = createdAt.UnixMilli()
	return &details, nil
}

// GetEndUserTenantID returns the active tenant ownership used by management
// permission checks. Deleted end users are intentionally invisible.
func (r *TenantRepository) GetEndUserTenantID(ctx context.Context, userID string) (string, error) {
	var tenantID string
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(tenant_id, '')
		FROM iam_accounts
		WHERE user_id = $1 AND user_type = 4 AND status <> 'deleted'
	`, userID).Scan(&tenantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", tenantports.ErrTenantEndUserNotFound
	}
	return tenantID, err
}

// LockManualRechargeTarget serializes a manual recharge with tenant/account
// lifecycle changes. It intentionally checks existence and the active end-user
// state while holding the same transaction lock used by the billing service.
// When tenantID is empty, an active user row supplies the tenant scope before
// the tenant row is locked. Callers must map pgx.ErrNoRows to request-level
// validation errors.
func (r *TenantRepository) LockManualRechargeTarget(ctx context.Context, tx pgx.Tx, tenantID, userID string) (string, error) {
	if tenantID == "" {
		if userID == "" {
			return "", pgx.ErrNoRows
		}
		if err := tx.QueryRow(ctx, `
			SELECT tenant_id
			FROM iam_accounts
			WHERE user_id = $1 AND user_type = 4 AND status = 'active'
		`, userID).Scan(&tenantID); err != nil {
			return "", err
		}
	}
	var lockedTenantID string
	if err := tx.QueryRow(ctx, `
		SELECT tenant_id
		FROM iam_tenants
		WHERE tenant_id = $1
		FOR UPDATE
	`, tenantID).Scan(&lockedTenantID); err != nil {
		return "", err
	}
	if userID == "" {
		return lockedTenantID, nil
	}
	if err := tx.QueryRow(ctx, `
		SELECT tenant_id
		FROM iam_accounts
		WHERE user_id = $1 AND tenant_id = $2 AND user_type = 4 AND status = 'active'
		FOR UPDATE
	`, userID, lockedTenantID).Scan(&lockedTenantID); err != nil {
		return "", err
	}
	return lockedTenantID, nil
}

// Legacy aliases keep adapter callers source-compatible while new callers use
// tenant/ports projections directly.
type ListTenantsParams = tenantports.TenantListQuery
type TenantRow = tenantports.TenantListItem
type TenantDetails = tenantports.TenantDetails
type Tenant = tenantports.TenantSummary

func (r *TenantRepository) List(ctx context.Context, params tenantports.TenantListQuery) (tenantports.TenantListPage, error) {
	var result tenantports.TenantListPage
	result.Page = params.Page
	result.Size = params.Size

	var keyword any = nil
	if params.Keyword != "" {
		keyword = "%" + params.Keyword + "%"
	}
	var status any = nil
	if params.Status != "" {
		status = params.Status
	}

	if err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM iam_tenants
		WHERE ($1::text IS NULL OR tenant_name LIKE $1::text OR contact_person LIKE $1::text OR contact_email LIKE $1::text)
		  AND ($2::text IS NULL OR status = $2::text)
	`, keyword, status).Scan(&result.Total); err != nil {
		return result, err
	}

	rows, err := r.pool.Query(ctx, `
		SELECT t.tenant_id, t.tenant_name, t.contact_person, t.contact_email, t.status, t.created_at,
		       t.balance_micro AS credits,
		       t.user_count
		FROM tenant_management_projection t
		WHERE ($1::text IS NULL OR t.tenant_name LIKE $1::text OR t.contact_person LIKE $1::text OR t.contact_email LIKE $1::text)
		  AND ($4::text IS NULL OR t.status = $4::text)
		ORDER BY t.created_at DESC
		LIMIT $3 OFFSET $2
	`, keyword, params.Offset, params.Size, status)
	if err != nil {
		return result, err
	}
	defer rows.Close()

	for rows.Next() {
		var row tenantports.TenantListItem
		var creditsMicro int64
		var createdAt time.Time
		if err := rows.Scan(
			&row.TenantID, &row.TenantName, &row.ContactPerson, &row.ContactEmail,
			&row.Status, &createdAt, &creditsMicro, &row.UserCount,
		); err != nil {
			return result, err
		}
		createdMillis := createdAt.UnixMilli()
		row.CreatedTime = &createdMillis
		row.BalanceUSD = billing.MicroToUSD(creditsMicro)
		result.Records = append(result.Records, row)
	}
	return result, rows.Err()
}

func (r *TenantRepository) GetByTenantIDs(ctx context.Context, tenantIDs []string) ([]*tenantports.TenantSummary, error) {
	if len(tenantIDs) == 0 {
		return []*tenantports.TenantSummary{}, nil
	}

	rows, err := r.pool.Query(ctx, `
		SELECT tenant_id, tenant_name, status
		FROM iam_tenants
		WHERE tenant_id = ANY($1)
	`, tenantIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tenants []*tenantports.TenantSummary
	for rows.Next() {
		var t tenantports.TenantSummary
		if err := rows.Scan(&t.TenantID, &t.TenantName, &t.Status); err != nil {
			return nil, err
		}
		tenants = append(tenants, &t)
	}
	return tenants, rows.Err()
}
