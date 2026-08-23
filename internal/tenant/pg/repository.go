package pg

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"xiaodou/dai/internal/billing"
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
	pool *pgxpool.Pool
}

func NewTenantRepository(pool *pgxpool.Pool) *TenantRepository {
	return &TenantRepository{pool: pool}
}

// GetTenantDetails returns the tenant detail projection used by management
// routes without exposing the SQL query to HTTP transport.
func (r *TenantRepository) GetTenantDetails(ctx context.Context, tenantID string) (*TenantDetails, error) {
	var details TenantDetails
	var createdAt time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT tenant_id, tenant_name, contact_person, contact_email, status, created_at
		FROM iam_tenants
		WHERE tenant_id = $1
	`, tenantID).Scan(
		&details.TenantID, &details.TenantName, &details.ContactPerson,
		&details.ContactEmail, &details.Status, &createdAt,
	)
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
	return tenantID, err
}

type ListTenantsParams struct {
	Keyword string
	Status  string // 存储层状态字符串（active/disabled/suspended），空表示不过滤
	Pagination
}

type TenantRow struct {
	TenantID      string  `json:"tenantId"`
	TenantName    string  `json:"tenantName"`
	ContactPerson *string `json:"contactPerson"`
	ContactEmail  *string `json:"contactEmail"`
	Status        *string `json:"status"`
	CreatedTime   *int64  `json:"createdTime"`
	BalanceUSD    float64 `json:"balanceUsd"`
	UserCount     int64   `json:"userCount"`
}

// TenantDetails is the scoped tenant projection used by admin detail and
// tenant-self read handlers. It intentionally excludes aggregate balances and
// user counts from the list projection.
type TenantDetails struct {
	TenantID      string
	TenantName    string
	ContactPerson *string
	ContactEmail  *string
	Status        string
	CreatedTime   int64
}

type Tenant struct {
	TenantID   string  `json:"tenantId"`
	TenantName string  `json:"tenantName"`
	Status     *string `json:"status,omitempty"`
}

func (r *TenantRepository) List(ctx context.Context, params ListTenantsParams) (PaginatedResult[TenantRow], error) {
	var result PaginatedResult[TenantRow]
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
		var row TenantRow
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

func (r *TenantRepository) GetByTenantIDs(ctx context.Context, tenantIDs []string) ([]*Tenant, error) {
	if len(tenantIDs) == 0 {
		return []*Tenant{}, nil
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

	var tenants []*Tenant
	for rows.Next() {
		var t Tenant
		if err := rows.Scan(&t.TenantID, &t.TenantName, &t.Status); err != nil {
			return nil, err
		}
		tenants = append(tenants, &t)
	}
	return tenants, rows.Err()
}
