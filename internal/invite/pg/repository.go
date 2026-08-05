package pg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvitationCodeNotFound    = errors.New("invitation code not found")
	ErrInvitationCodeUnavailable = errors.New("invitation code unavailable")
)

// InvitationCode 邀请码实体
type InvitationCode struct {
	ID          int64  `json:"id"`
	Code        string `json:"code"`
	TenantID    string `json:"tenantId"`
	CreatedBy   string `json:"createdBy"`
	Description string `json:"description"`
	MaxUses     int    `json:"maxUses"`
	UsedCount   int    `json:"usedCount"`
	Status      int    `json:"status"` // 1: 有效, 2: 禁用
	ExpireTime  *int64 `json:"expireTime,omitempty"`
	CreatedTime int64  `json:"createdAt"`
	UpdatedTime int64  `json:"updatedAt"`
}

// TenantPublicBrand is the subset of tenant presentation data that may be shown
// on a public invitation page.
type TenantPublicBrand struct {
	TenantName       string
	CustomerSiteName string
	FaviconUpdatedAt *time.Time
}

type LegalAcceptance struct {
	DocumentKey string
	Version     string
}

type EndUserRegistration struct {
	InvitationCode   string
	TenantID         string
	UserID           string
	Username         string
	PasswordHash     string
	Email            *string
	Phone            *string
	LegalAcceptances []LegalAcceptance
}

// IsValid 检查邀请码是否可用
func (ic *InvitationCode) IsValid() bool {
	if ic.Status != 1 {
		return false
	}
	if ic.MaxUses > 0 && ic.UsedCount >= ic.MaxUses {
		return false
	}
	if ic.ExpireTime != nil && *ic.ExpireTime < time.Now().UnixMilli() {
		return false
	}
	return true
}

type InviteRepository struct {
	pool *pgxpool.Pool
}

func NewInviteRepository(pool *pgxpool.Pool) *InviteRepository {
	return &InviteRepository{pool: pool}
}

// GetByCode 根据邀请码获取记录
func (r *InviteRepository) GetByCode(code string) (*InvitationCode, error) {
	ctx := context.Background()
	ic := &InvitationCode{}
	var status string
	var description *string
	var expiresAt *time.Time
	var createdAt, updatedAt time.Time
	err := r.pool.QueryRow(ctx, `
        SELECT id, code, tenant_id, created_by, description, max_uses, used_count, status, expires_at, created_at, updated_at
        FROM iam_invitation_codes WHERE code = $1
    `, code).Scan(
		&ic.ID, &ic.Code, &ic.TenantID, &ic.CreatedBy, &description,
		&ic.MaxUses, &ic.UsedCount, &status, &expiresAt,
		&createdAt, &updatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, ErrInvitationCodeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query invitation code: %w", err)
	}
	if description != nil {
		ic.Description = *description
	}
	ic.Status = inviteCodeStatusToInt(status)
	ic.ExpireTime = inviteTimeToMillisPtr(expiresAt)
	ic.CreatedTime = createdAt.UnixMilli()
	ic.UpdatedTime = updatedAt.UnixMilli()
	return ic, nil
}

func (r *InviteRepository) GetTenantPublicBrand(tenantID string) (*TenantPublicBrand, error) {
	ctx := context.Background()
	brand := &TenantPublicBrand{}
	err := r.pool.QueryRow(ctx, `
		SELECT t.tenant_name, COALESCE(b.customer_site_name, ''), b.favicon_updated_at
		FROM iam_tenants t
		LEFT JOIN iam_tenant_portal_branding b ON b.tenant_id = t.tenant_id
		WHERE t.tenant_id = $1
	`, tenantID).Scan(&brand.TenantName, &brand.CustomerSiteName, &brand.FaviconUpdatedAt)
	if err == pgx.ErrNoRows {
		return &TenantPublicBrand{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query tenant public brand: %w", err)
	}
	return brand, nil
}

// GetByTenantID 查询租户邀请码列表
func (r *InviteRepository) GetByTenantID(tenantID string, page, size int) ([]*InvitationCode, int64, error) {
	ctx := context.Background()
	var total int64
	r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM iam_invitation_codes WHERE tenant_id = $1`, tenantID).Scan(&total)

	offset := (page - 1) * size
	rows, err := r.pool.Query(ctx, `
        SELECT id, code, tenant_id, created_by, description, max_uses, used_count, status, expires_at, created_at, updated_at
        FROM iam_invitation_codes WHERE tenant_id = $1
        ORDER BY created_at DESC LIMIT $2 OFFSET $3
    `, tenantID, size, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var list []*InvitationCode
	for rows.Next() {
		ic := &InvitationCode{}
		var status string
		var description *string
		var expiresAt *time.Time
		var createdAt, updatedAt time.Time
		if err := rows.Scan(
			&ic.ID, &ic.Code, &ic.TenantID, &ic.CreatedBy, &description,
			&ic.MaxUses, &ic.UsedCount, &status, &expiresAt,
			&createdAt, &updatedAt,
		); err != nil {
			continue
		}
		if description != nil {
			ic.Description = *description
		}
		ic.Status = inviteCodeStatusToInt(status)
		ic.ExpireTime = inviteTimeToMillisPtr(expiresAt)
		ic.CreatedTime = createdAt.UnixMilli()
		ic.UpdatedTime = updatedAt.UnixMilli()
		list = append(list, ic)
	}
	return list, total, rows.Err()
}

// Create 创建邀请码
func (r *InviteRepository) Create(ic *InvitationCode) error {
	ctx := context.Background()
	now := time.Now().UTC()
	var expiresAt *time.Time
	if ic.ExpireTime != nil {
		value := time.UnixMilli(*ic.ExpireTime).UTC()
		expiresAt = &value
	}
	var id int64
	err := r.pool.QueryRow(ctx, `
        INSERT INTO iam_invitation_codes (code, tenant_id, created_by, description, max_uses, used_count, status, expires_at, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, 0, 'active', $6, $7, $7)
        RETURNING id
    `, ic.Code, ic.TenantID, ic.CreatedBy, ic.Description, ic.MaxUses, expiresAt, now).Scan(&id)
	if err != nil {
		return err
	}
	ic.ID = id
	return nil
}

// Update 更新邀请码
func (r *InviteRepository) Update(id int64, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}
	ctx := context.Background()
	updates["updated_at"] = time.Now().UTC()

	setClause := ""
	args := make([]any, 0, len(updates)+1)
	argIdx := 1
	for k, v := range updates {
		if setClause != "" {
			setClause += ", "
		}
		setClause += k + " = $" + fmt.Sprintf("%d", argIdx)
		args = append(args, v)
		argIdx++
	}
	args = append(args, id)

	_, err := r.pool.Exec(ctx, fmt.Sprintf("UPDATE iam_invitation_codes SET %s WHERE id = $%d", setClause, argIdx), args...)
	return err
}

// Delete 删除邀请码
func (r *InviteRepository) Delete(id int64) error {
	ctx := context.Background()
	_, err := r.pool.Exec(ctx, `DELETE FROM iam_invitation_codes WHERE id = $1`, id)
	return err
}

// CheckEndUserUsernameExists 检查终端用户名是否已存在
func (r *InviteRepository) CheckEndUserUsernameExists(username string) (bool, error) {
	ctx := context.Background()
	var count int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM iam_users WHERE username = $1`, username).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// RegisterEndUser persists account creation, invitation consumption and versioned legal
// acknowledgements as one unit. A registration is never considered successful without
// its matching legal evidence.
func (r *InviteRepository) RegisterEndUser(ctx context.Context, input EndUserRegistration) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin end-user registration: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
		INSERT INTO iam_users (user_id, tenant_id, username, password_hash, email, phone, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'active', $7, $7)
	`, input.UserID, input.TenantID, input.Username, input.PasswordHash, input.Email, input.Phone, now); err != nil {
		return fmt.Errorf("create end user: %w", err)
	}

	result, err := tx.Exec(ctx, `
		UPDATE iam_invitation_codes
		SET used_count = used_count + 1, updated_at = $2
		WHERE code = $1 AND status = 'active' AND (max_uses = 0 OR used_count < max_uses)
	`, input.InvitationCode, now)
	if err != nil {
		return fmt.Errorf("consume invitation code: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrInvitationCodeUnavailable
	}

	for _, acceptance := range input.LegalAcceptances {
		if _, err := tx.Exec(ctx, `
			INSERT INTO iam_user_legal_acceptances
				(user_id, tenant_id, document_key, document_version, source, accepted_at)
			VALUES ($1, $2, $3, $4, 'public_registration', $5)
		`, input.UserID, input.TenantID, acceptance.DocumentKey, acceptance.Version, now); err != nil {
			return fmt.Errorf("record legal acceptance: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit end-user registration: %w", err)
	}
	return nil
}

func inviteCodeStatusToInt(status string) int {
	if status == "disabled" {
		return 2
	}
	return 1
}

func inviteTimeToMillisPtr(value *time.Time) *int64 {
	if value == nil {
		return nil
	}
	v := value.UnixMilli()
	return &v
}
