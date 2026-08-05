package postgres

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/ai/application"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
)

// ApplicationInvokeKeyRepo persists app keys: minimal-privilege tokens bound
// to exactly one published app.
type ApplicationInvokeKeyRepo struct {
	pool *pgxpool.Pool
}

func NewApplicationInvokeKeyRepo(pool *pgxpool.Pool) *ApplicationInvokeKeyRepo {
	return &ApplicationInvokeKeyRepo{pool: pool}
}

func (r *ApplicationInvokeKeyRepo) CreateInvokeKey(ctx context.Context, in application.InvokeKeyWrite) (application.InvokeKey, error) {
	ownerType, err := normalizeInvokeKeyOwnerScope(in.OwnerScope)
	if err != nil {
		return application.InvokeKey{}, err
	}
	row, err := r.pool.Query(ctx, `
		INSERT INTO ai_app_keys (
		  owner_type, tenant_id, user_id, name, key_hash, key_ciphertext, last_four, status, app_id, expires_at, created_by
		) VALUES (
		  $1, $2, $3, $4, $5, $6, $7, $8, $9::uuid, $10, NULLIF($11, '')
		)
		RETURNING id::text,
		          owner_type,
		          tenant_id,
		          COALESCE(user_id, ''),
		          name,
		          key_hash,
		          last_four,
		          status,
		          app_id::text,
		          expires_at,
		          COALESCE(created_by, ''),
		          created_at,
		          updated_at
	`, ownerType, in.TenantID, in.UserID, in.Name, in.KeyHash, in.KeyCiphertext, in.LastFour, in.Status, in.AppID, in.ExpiresAt, in.CreatedBy)
	if err != nil {
		return application.InvokeKey{}, err
	}
	defer row.Close()
	if !row.Next() {
		return application.InvokeKey{}, pgx.ErrNoRows
	}
	item, err := scanApplicationInvokeKey(row.Scan)
	if err != nil {
		return application.InvokeKey{}, err
	}
	return item, row.Err()
}

func (r *ApplicationInvokeKeyRepo) ListInvokeKeys(ctx context.Context, filter application.InvokeKeyFilter) ([]application.InvokeKey, error) {
	ownerType, err := normalizeInvokeKeyOwnerScope(filter.OwnerScope)
	if err != nil {
		return nil, err
	}
	query := `
		SELECT id::text,
		       owner_type,
		       tenant_id,
		       COALESCE(user_id, ''),
		       name,
		       key_hash,
		       last_four,
		       status,
		       app_id::text,
		       expires_at,
		       COALESCE(created_by, ''),
		       created_at,
		       updated_at
		FROM ai_app_keys
		WHERE owner_type = $1
		  AND tenant_id = $2
		  AND COALESCE(user_id, '') = $3`
	args := []any{ownerType, filter.TenantID, filter.UserID}
	if strings.TrimSpace(string(filter.Status)) != "" {
		query += ` AND status = $4`
		args = append(args, string(filter.Status))
	}
	query += ` ORDER BY updated_at DESC, created_at DESC`
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]application.InvokeKey, 0)
	for rows.Next() {
		item, err := scanApplicationInvokeKey(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *ApplicationInvokeKeyRepo) GetInvokeKeyByID(ctx context.Context, ownerScope, tenantID, userID, id string) (application.InvokeKey, error) {
	ownerType, err := normalizeInvokeKeyOwnerScope(ownerScope)
	if err != nil {
		return application.InvokeKey{}, err
	}
	row := r.pool.QueryRow(ctx, `
		SELECT id::text,
		       owner_type,
		       tenant_id,
		       COALESCE(user_id, ''),
		       name,
		       key_hash,
		       last_four,
		       status,
		       app_id::text,
		       expires_at,
		       COALESCE(created_by, ''),
		       created_at,
		       updated_at
		FROM ai_app_keys
		WHERE id = $1
		  AND owner_type = $2
		  AND tenant_id = $3
		  AND COALESCE(user_id, '') = $4
	`, id, ownerType, tenantID, userID)
	item, err := scanApplicationInvokeKey(row.Scan)
	if err != nil {
		if err == pgx.ErrNoRows {
			return application.InvokeKey{}, domain.ErrNotFound
		}
		return application.InvokeKey{}, err
	}
	return item, nil
}

func (r *ApplicationInvokeKeyRepo) GetInvokeKeyByHash(ctx context.Context, keyHash string) (application.InvokeKey, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id::text,
		       owner_type,
		       tenant_id,
		       COALESCE(user_id, ''),
		       name,
		       key_hash,
		       last_four,
		       status,
		       app_id::text,
		       expires_at,
		       COALESCE(created_by, ''),
		       created_at,
		       updated_at
		FROM ai_app_keys
		WHERE key_hash = $1
	`, keyHash)
	item, err := scanApplicationInvokeKey(row.Scan)
	if err != nil {
		if err == pgx.ErrNoRows {
			return application.InvokeKey{}, application.ErrRuntimeInvocationNotFound
		}
		return application.InvokeKey{}, err
	}
	return item, nil
}

func (r *ApplicationInvokeKeyRepo) UpdateInvokeKey(ctx context.Context, id string, in application.InvokeKeyWrite) (application.InvokeKey, error) {
	ownerType, err := normalizeInvokeKeyOwnerScope(in.OwnerScope)
	if err != nil {
		return application.InvokeKey{}, err
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE ai_app_keys
		SET name = $2,
		    status = $3,
		    app_id = $4::uuid,
		    expires_at = $5,
		    updated_at = now()
		WHERE id = $1
		  AND owner_type = $6
		  AND tenant_id = $7
		  AND COALESCE(user_id, '') = $8
	`, id, in.Name, in.Status, in.AppID, in.ExpiresAt, ownerType, in.TenantID, in.UserID)
	if err != nil {
		return application.InvokeKey{}, err
	}
	if tag.RowsAffected() == 0 {
		return application.InvokeKey{}, domain.ErrNotFound
	}
	return r.GetInvokeKeyByID(ctx, in.OwnerScope, in.TenantID, in.UserID, id)
}

func (r *ApplicationInvokeKeyRepo) DeleteInvokeKey(ctx context.Context, ownerScope, tenantID, userID, id string) error {
	ownerType, err := normalizeInvokeKeyOwnerScope(ownerScope)
	if err != nil {
		return err
	}
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM ai_app_keys
		WHERE id = $1
		  AND owner_type = $2
		  AND tenant_id = $3
		  AND COALESCE(user_id, '') = $4
	`, id, ownerType, tenantID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// RevealInvokeKey returns the decryptable ciphertext for an app key already
// scoped to its owner, mirroring the API key reveal flow.
func (r *ApplicationInvokeKeyRepo) RevealInvokeKey(ctx context.Context, ownerScope, tenantID, userID, id string) (string, error) {
	ownerType, err := normalizeInvokeKeyOwnerScope(ownerScope)
	if err != nil {
		return "", err
	}
	var ciphertext string
	err = r.pool.QueryRow(ctx, `
		SELECT key_ciphertext
		FROM ai_app_keys
		WHERE id = $1
		  AND owner_type = $2
		  AND tenant_id = $3
		  AND COALESCE(user_id, '') = $4
	`, id, ownerType, tenantID, userID).Scan(&ciphertext)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", domain.ErrNotFound
		}
		return "", err
	}
	return ciphertext, nil
}

// RotateInvokeKey mints a new secret for an existing app key, preserving its
// app binding, name and expiry.
func (r *ApplicationInvokeKeyRepo) RotateInvokeKey(ctx context.Context, id string, in application.InvokeKeyRotate) (application.InvokeKey, error) {
	ownerType, err := normalizeInvokeKeyOwnerScope(in.OwnerScope)
	if err != nil {
		return application.InvokeKey{}, err
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE ai_app_keys
		SET key_hash = $2,
		    key_ciphertext = $3,
		    last_four = $4,
		    updated_at = now()
		WHERE id = $1
		  AND owner_type = $5
		  AND tenant_id = $6
		  AND COALESCE(user_id, '') = $7
	`, id, in.KeyHash, in.KeyCiphertext, in.LastFour, ownerType, in.TenantID, in.UserID)
	if err != nil {
		return application.InvokeKey{}, err
	}
	if tag.RowsAffected() == 0 {
		return application.InvokeKey{}, domain.ErrNotFound
	}
	return r.GetInvokeKeyByID(ctx, in.OwnerScope, in.TenantID, in.UserID, id)
}

func normalizeInvokeKeyOwnerScope(ownerScope string) (string, error) {
	switch strings.TrimSpace(ownerScope) {
	case string(coreidentity.ScopeTenant):
		return "tenant", nil
	case string(coreidentity.ScopeUser):
		return "user", nil
	default:
		return "", domain.NewValidationError("owner_scope", "must be tenant or user")
	}
}

func scanApplicationInvokeKey(scan func(dest ...any) error) (application.InvokeKey, error) {
	var (
		item      application.InvokeKey
		ownerType string
		userID    string
		expiresAt *time.Time
	)
	if err := scan(
		&item.ID,
		&ownerType,
		&item.TenantID,
		&userID,
		&item.Name,
		&item.KeyHash,
		&item.LastFour,
		&item.Status,
		&item.AppID,
		&expiresAt,
		&item.CreatedBy,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return application.InvokeKey{}, err
	}
	scope, err := ownerTypeToIdentityScope(ownerType)
	if err != nil {
		return application.InvokeKey{}, err
	}
	item.OwnerScope = scope
	item.UserID = userID
	item.ExpiresAt = expiresAt
	return item, nil
}

func ownerTypeToIdentityScope(ownerType string) (coreidentity.Scope, error) {
	switch strings.TrimSpace(ownerType) {
	case "tenant":
		return coreidentity.ScopeTenant, nil
	case "user":
		return coreidentity.ScopeUser, nil
	default:
		return "", domain.NewValidationError("owner_type", "must be tenant or user")
	}
}
