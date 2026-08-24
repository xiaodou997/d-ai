package pg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"xiaodou/dai/internal/auth"
	userports "xiaodou/dai/internal/user/ports"
)

// AdminAccountRepository owns management projections and lifecycle mutations
// for userType=2/3 accounts. Activation token persistence is delegated to the
// auth service while this repository retains account transaction boundaries.
type AdminAccountRepository struct {
	pool              *pgxpool.Pool
	activationService activationService
}

var _ userports.AdminAccountReader = (*AdminAccountRepository)(nil)
var _ userports.AdminAccountWriter = (*AdminAccountRepository)(nil)

func NewAdminAccountRepository(pool *pgxpool.Pool, activationServices ...activationService) *AdminAccountRepository {
	var activationService activationService
	if len(activationServices) > 0 {
		activationService = activationServices[0]
	}
	return &AdminAccountRepository{pool: pool, activationService: activationService}
}

// CreateSystemAdmin atomically persists a pending platform-admin account and
// its one-time activation credential.
func (r *AdminAccountRepository) CreateSystemAdmin(ctx context.Context, input userports.AdminAccountCreate) error {
	return r.create(ctx, input, 2)
}

// CreateTenantUser atomically persists a pending tenant-user account and its
// one-time activation credential.
func (r *AdminAccountRepository) CreateTenantUser(ctx context.Context, input userports.AdminAccountCreate) error {
	return r.create(ctx, input, 3)
}

func (r *AdminAccountRepository) create(ctx context.Context, input userports.AdminAccountCreate, userType int) error {
	if r.activationService == nil {
		return errors.New("admin account activation store is not configured")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now().UTC()
	var insertErr error
	if userType == 2 {
		_, insertErr = tx.Exec(ctx, `
			INSERT INTO iam_accounts (user_id, username, password_hash, credential_state, email, user_type, status, created_at, updated_at)
			VALUES ($1, $2, $3, 'pending_activation', $4, 2, 'active', $5, $5)
		`, input.UserID, input.Username, input.PasswordHash, input.Email, now)
	} else {
		_, insertErr = tx.Exec(ctx, `
			INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, credential_state, email, user_type, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, 'pending_activation', $5, 3, 'active', $6, $6)
		`, input.UserID, input.TenantID, input.Username, input.PasswordHash, input.Email, now)
	}
	if insertErr != nil {
		return insertErr
	}
	if err := r.activationService.Store(ctx, tx, input.UserID, auth.ActivationPurposeAccount, authActivationCredential(input)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func authActivationCredential(input userports.AdminAccountCreate) auth.ActivationCredential {
	return auth.ActivationCredential{
		PasswordHash: input.PasswordHash,
		TokenHash:    input.ActivationTokenHash,
		ExpiresAt:    input.ActivationExpiresAt,
	}
}

// UpdateSystemAdmin preserves the legacy rule that a super administrator is
// not mutable through the platform-admin endpoint.
func (r *AdminAccountRepository) UpdateSystemAdmin(ctx context.Context, input userports.AdminAccountUpdate) (userports.AdminAccountMutationResult, error) {
	var userType int
	if err := r.pool.QueryRow(ctx, `
		SELECT user_type FROM iam_accounts
		WHERE user_id = $1 AND user_type IN (1, 2)
	`, input.UserID).Scan(&userType); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return userports.AdminAccountMutationResult{}, nil
		}
		return userports.AdminAccountMutationResult{}, err
	}
	if userType != 2 {
		return userports.AdminAccountMutationResult{Forbidden: true}, nil
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE iam_accounts SET email = $1, status = $2, updated_at = $3
		WHERE user_id = $4 AND user_type = 2
	`, input.Email, input.Status, time.Now().UTC(), input.UserID)
	if err != nil {
		return userports.AdminAccountMutationResult{}, err
	}
	return userports.AdminAccountMutationResult{Updated: tag.RowsAffected() == 1}, nil
}

func (r *AdminAccountRepository) UpdateTenantUserStatus(ctx context.Context, userID, status string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE iam_accounts SET status = $1, updated_at = $2
		WHERE user_id = $3 AND user_type = 3
	`, status, time.Now().UTC(), userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (r *AdminAccountRepository) UpdateTenantUser(ctx context.Context, input userports.AdminAccountUpdate) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE iam_accounts SET email = $1, status = $2, updated_at = $3
		WHERE user_id = $4 AND user_type = 3
	`, input.Email, input.Status, time.Now().UTC(), input.UserID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (r *AdminAccountRepository) ResetSystemAdminPassword(ctx context.Context, userID string) (userports.ActivationCredentialResult, error) {
	return r.resetPassword(ctx, userID, 2)
}

func (r *AdminAccountRepository) ResetTenantUserPassword(ctx context.Context, userID string) (userports.ActivationCredentialResult, error) {
	return r.resetPassword(ctx, userID, 3)
}

func (r *AdminAccountRepository) resetPassword(ctx context.Context, userID string, expectedType int) (userports.ActivationCredentialResult, error) {
	if r.activationService == nil {
		return userports.ActivationCredentialResult{}, errors.New("admin account activation service is not configured")
	}
	var userType int
	if err := r.pool.QueryRow(ctx, `
		SELECT user_type FROM iam_accounts
		WHERE user_id = $1 AND status <> 'deleted'
	`, userID).Scan(&userType); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return userports.ActivationCredentialResult{}, nil
		}
		return userports.ActivationCredentialResult{}, err
	}
	if userType != expectedType {
		return userports.ActivationCredentialResult{}, nil
	}
	result, err := r.activationService.Reset(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return userports.ActivationCredentialResult{}, nil
	}
	if err != nil {
		return userports.ActivationCredentialResult{}, err
	}
	return userports.ActivationCredentialResult{Token: result.Token, ExpiresIn: result.ExpiresIn}, nil
}

func (r *AdminAccountRepository) DeleteSystemAdmin(ctx context.Context, userID string) (userports.AdminAccountMutationResult, error) {
	var userType int
	if err := r.pool.QueryRow(ctx, `
		SELECT user_type FROM iam_accounts
		WHERE user_id = $1 AND user_type IN (1, 2)
	`, userID).Scan(&userType); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return userports.AdminAccountMutationResult{}, nil
		}
		return userports.AdminAccountMutationResult{}, err
	}
	if userType != 2 {
		return userports.AdminAccountMutationResult{Forbidden: true}, nil
	}
	tag, err := r.pool.Exec(ctx, `DELETE FROM iam_accounts WHERE user_id = $1 AND user_type = 2`, userID)
	if err != nil {
		return userports.AdminAccountMutationResult{}, err
	}
	return userports.AdminAccountMutationResult{Updated: tag.RowsAffected() == 1}, nil
}

func (r *AdminAccountRepository) ListSystemAdmins(ctx context.Context, keyword string, page, size int) (userports.AdminAccountPage, error) {
	return r.list(ctx, 2, "", keyword, page, size)
}

func (r *AdminAccountRepository) ListTenantUsers(ctx context.Context, tenantID, keyword string, page, size int) (userports.AdminAccountPage, error) {
	return r.list(ctx, 3, tenantID, keyword, page, size)
}

func (r *AdminAccountRepository) list(ctx context.Context, userType int, tenantID, keyword string, page, size int) (userports.AdminAccountPage, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}

	where := "WHERE user_type = $1"
	args := []any{userType}
	idx := 2
	if userType == 3 {
		where += fmt.Sprintf(" AND tenant_id = $%d", idx)
		args = append(args, tenantID)
		idx++
	}
	if keyword != "" {
		operator := "ILIKE"
		if userType == 2 {
			operator = "LIKE"
		}
		where += fmt.Sprintf(" AND (username %s $%d OR email %s $%d)", operator, idx, operator, idx)
		args = append(args, "%"+keyword+"%")
		idx++
	}

	var total int64
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM iam_accounts "+where, args...).Scan(&total); err != nil {
		return userports.AdminAccountPage{}, err
	}

	offset := (page - 1) * size
	queryArgs := append(append([]any{}, args...), size, offset)
	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT user_id, username, email, status, credential_state, created_at
		FROM iam_accounts %s
		ORDER BY created_at DESC LIMIT $%d OFFSET $%d
	`, where, idx, idx+1), queryArgs...)
	if err != nil {
		return userports.AdminAccountPage{}, err
	}
	defer rows.Close()

	items := make([]userports.AdminAccountRow, 0)
	for rows.Next() {
		var item userports.AdminAccountRow
		if err := rows.Scan(&item.UserID, &item.Username, &item.Email, &item.Status, &item.CredentialState, &item.CreatedAt); err != nil {
			return userports.AdminAccountPage{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return userports.AdminAccountPage{}, err
	}
	return userports.AdminAccountPage{Records: items, Total: total, Page: page, Size: size}, nil
}
