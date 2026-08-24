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

// AdminEndUserRepository owns the scoped admin end-user projections and
// account mutations. Activation token persistence is delegated to the auth
// service while this repository retains the surrounding transaction boundary.
type AdminEndUserRepository struct {
	pool              *pgxpool.Pool
	activationService activationService
}

var _ userports.AdminEndUserReader = (*AdminEndUserRepository)(nil)
var _ userports.AdminEndUserWriter = (*AdminEndUserRepository)(nil)

func NewAdminEndUserRepository(pool *pgxpool.Pool, activationServices ...activationService) *AdminEndUserRepository {
	var activationService activationService
	if len(activationServices) > 0 {
		activationService = activationServices[0]
	}
	return &AdminEndUserRepository{pool: pool, activationService: activationService}
}

// CreateEndUser atomically creates the pending-activation account and its
// one-time activation record. A failed activation insert rolls the account
// back so callers never receive an unusable half-created user.
func (r *AdminEndUserRepository) CreateEndUser(ctx context.Context, input userports.AdminEndUserCreate) error {
	if r.activationService == nil {
		return errors.New("admin end-user activation store is not configured")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
		INSERT INTO iam_accounts (user_id, tenant_id, username, password_hash, credential_state, email, phone, internal_note, user_type, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'pending_activation', $5, $6, $7, 4, 'active', $8, $8)
	`, input.UserID, input.TenantID, input.Username, input.PasswordHash, input.Email, input.Phone, input.InternalNote, now); err != nil {
		return err
	}
	if err := r.activationService.Store(ctx, tx, input.UserID, auth.ActivationPurposeAccount, auth.ActivationCredential{
		PasswordHash: input.PasswordHash,
		TokenHash:    input.ActivationTokenHash,
		ExpiresAt:    input.ActivationExpiresAt,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// UpdateEndUser updates only fields explicitly selected by the caller. The
// tenant and user-type predicates keep the write scoped even if a prior
// ownership check races with a delete or reassignment attempt.
func (r *AdminEndUserRepository) UpdateEndUser(ctx context.Context, input userports.AdminEndUserUpdate) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE iam_accounts
		SET email = CASE WHEN $1 THEN NULLIF($2, '') ELSE email END,
		    phone = CASE WHEN $3 THEN NULLIF($4, '') ELSE phone END,
		    internal_note = CASE WHEN $5 THEN $6 ELSE internal_note END,
		    updated_at = $7
		WHERE user_id = $8 AND tenant_id = $9 AND user_type = 4 AND status <> 'deleted'
	`, input.EmailSet, input.Email, input.PhoneSet, input.Phone, input.InternalNoteSet, input.InternalNote, time.Now().UTC(), input.UserID, input.TenantID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// UpdateEndUserStatus changes an active end-user account state. Deleted
// accounts are intentionally excluded so a stale request cannot resurrect
// one that has already been removed from operational views.
func (r *AdminEndUserRepository) UpdateEndUserStatus(ctx context.Context, userID, status string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE iam_accounts
		SET status = $1, updated_at = $2
		WHERE user_id = $3 AND user_type = 4 AND status <> 'deleted'
	`, status, time.Now().UTC(), userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (r *AdminEndUserRepository) ResetEndUserPassword(ctx context.Context, userID string) (userports.ActivationCredentialResult, error) {
	if r.activationService == nil {
		return userports.ActivationCredentialResult{}, errors.New("end-user activation service is not configured")
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
	if userType != 4 {
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

func (r *AdminEndUserRepository) DeleteEndUser(ctx context.Context, userID string, beforeCommit userports.AdminEndUserDeleteGuard) (userports.AdminEndUserDeleteResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return userports.AdminEndUserDeleteResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status string
	if err := tx.QueryRow(ctx, `
		SELECT status FROM iam_accounts
		WHERE user_id = $1 AND user_type = 4
		FOR UPDATE
	`, userID).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return userports.AdminEndUserDeleteResult{}, nil
		}
		return userports.AdminEndUserDeleteResult{}, err
	}
	if status == "deleted" {
		return userports.AdminEndUserDeleteResult{}, nil
	}

	var balanceMicroUSD int64
	if err := tx.QueryRow(ctx, `
		SELECT balance_micro FROM bill_accounts WHERE account_id = $1 FOR UPDATE
	`, userID).Scan(&balanceMicroUSD); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return userports.AdminEndUserDeleteResult{}, err
	}
	decision := userports.AdminEndUserDeleteResult{Found: true, BalanceMicroUSD: balanceMicroUSD}
	if balanceMicroUSD != 0 {
		return decision, nil
	}
	if beforeCommit != nil {
		if err := beforeCommit(ctx, userID); err != nil {
			return userports.AdminEndUserDeleteResult{}, &userports.AdminEndUserDeleteGuardError{Cause: err}
		}
	}
	result, err := tx.Exec(ctx, `
		UPDATE iam_accounts
		SET status = 'deleted', updated_at = $1
		WHERE user_id = $2 AND user_type = 4 AND status <> 'deleted'
	`, time.Now().UTC(), userID)
	if err != nil {
		return userports.AdminEndUserDeleteResult{}, err
	}
	if result.RowsAffected() != 1 {
		return userports.AdminEndUserDeleteResult{}, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return userports.AdminEndUserDeleteResult{}, err
	}
	decision.Deleted = true
	return decision, nil
}

func (r *AdminEndUserRepository) ListEndUsers(ctx context.Context, filter userports.AdminEndUserListFilter) (userports.AdminEndUserPage, error) {
	page := filter.Page
	if page < 1 {
		page = 1
	}
	size := filter.Size
	if size < 1 || size > 100 {
		size = 20
	}

	where := "WHERE eu.user_type = 4 AND eu.status <> 'deleted'"
	args := []any{}
	idx := 1
	if filter.TenantID != "" {
		where += fmt.Sprintf(" AND eu.tenant_id = $%d", idx)
		args = append(args, filter.TenantID)
		idx++
	}
	if filter.Keyword != "" {
		where += fmt.Sprintf(" AND (eu.username LIKE $%d OR eu.email LIKE $%d OR eu.phone LIKE $%d OR eu.internal_note LIKE $%d)", idx, idx, idx, idx)
		args = append(args, "%"+filter.Keyword+"%")
		idx++
	}
	if filter.TenantName != "" {
		where += fmt.Sprintf(" AND t.tenant_name LIKE $%d", idx)
		args = append(args, "%"+filter.TenantName+"%")
		idx++
	}
	if filter.Username != "" {
		where += fmt.Sprintf(" AND eu.username LIKE $%d", idx)
		args = append(args, "%"+filter.Username+"%")
		idx++
	}
	if filter.Status != "" {
		where += fmt.Sprintf(" AND eu.status = $%d", idx)
		args = append(args, filter.Status)
		idx++
	}

	from := "FROM iam_accounts eu LEFT JOIN iam_tenants t ON eu.tenant_id = t.tenant_id"
	var total int64
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*) "+from+" "+where, args...).Scan(&total); err != nil {
		return userports.AdminEndUserPage{}, err
	}

	offset := (page - 1) * size
	queryArgs := append(append([]any{}, args...), size, offset)
	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT eu.user_id, eu.tenant_id, eu.username, eu.email, eu.phone, eu.internal_note, eu.nickname, eu.avatar,
		       eu.status, eu.credential_state, eu.last_login_at, eu.created_at,
		       COALESCE(t.tenant_name, '') AS tenant_name,
		       COALESCE((SELECT b.balance_micro FROM bill_accounts b WHERE b.account_id = eu.user_id), 0) AS credits
		%s
		%s ORDER BY eu.created_at DESC LIMIT $%d OFFSET $%d
	`, from, where, idx, idx+1), queryArgs...)
	if err != nil {
		return userports.AdminEndUserPage{}, err
	}
	defer rows.Close()

	items := make([]userports.AdminEndUserRow, 0)
	for rows.Next() {
		var item userports.AdminEndUserRow
		if err := rows.Scan(
			&item.UserID, &item.TenantID, &item.Username, &item.Email, &item.Phone, &item.InternalNote,
			&item.Nickname, &item.Avatar, &item.Status, &item.CredentialState, &item.LastLoginAt,
			&item.CreatedAt, &item.TenantName, &item.BalanceMicroUSD,
		); err != nil {
			return userports.AdminEndUserPage{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return userports.AdminEndUserPage{}, err
	}
	return userports.AdminEndUserPage{Records: items, Total: total, Page: page, Size: size}, nil
}
