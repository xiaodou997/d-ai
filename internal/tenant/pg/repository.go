package pg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"xiaodou/dai/internal/auth"
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
			return err
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
	tag, err := r.pool.Exec(ctx, `DELETE FROM iam_tenants WHERE tenant_id = $1`, tenantID)
	if err != nil {
		if IsTenantReferenced(err) {
			return false, tenantports.ErrTenantReferenced
		}
		return false, err
	}
	return tag.RowsAffected() == 1, nil
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
// state while holding the same transaction lock used by the billing service;
// callers must map pgx.ErrNoRows to their request-level validation error.
func (r *TenantRepository) LockManualRechargeTarget(ctx context.Context, tx pgx.Tx, tenantID, userID string) error {
	var lockedTenantID string
	if err := tx.QueryRow(ctx, `
		SELECT tenant_id
		FROM iam_tenants
		WHERE tenant_id = $1
		FOR UPDATE
	`, tenantID).Scan(&lockedTenantID); err != nil {
		return err
	}
	if userID == "" {
		return nil
	}
	return tx.QueryRow(ctx, `
		SELECT tenant_id
		FROM iam_accounts
		WHERE user_id = $1 AND tenant_id = $2 AND user_type = 4 AND status = 'active'
		FOR UPDATE
	`, userID, lockedTenantID).Scan(&lockedTenantID)
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
		       COALESCE(b.balance_micro, 0)::bigint AS credits,
		       (SELECT COUNT(*) FROM iam_accounts u WHERE u.tenant_id = t.tenant_id AND u.user_type = 4 AND u.status NOT IN ('locked', 'inherited_disabled', 'deleted'))::bigint AS user_count
		FROM iam_tenants t
		LEFT JOIN bill_accounts b ON b.account_id = t.tenant_id
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
