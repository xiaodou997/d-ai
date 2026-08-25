package pg

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	tenantports "xiaodou/dai/internal/tenant/ports"
)

var (
	ErrTenantNotFound  = tenantports.ErrTenantNotFound
	ErrTenantNameTaken = tenantports.ErrTenantNameTaken
)

// PortalBranding remains an adapter-level alias for callers that still import
// the PostgreSQL package. New boundaries should use tenant/ports directly.
type PortalBranding = tenantports.PortalBranding

type PortalBrandingRepository struct {
	pool *pgxpool.Pool
}

var _ tenantports.PortalBrandingStore = (*PortalBrandingRepository)(nil)
var _ tenantports.PortalBrandingReader = (*PortalBrandingRepository)(nil)
var _ tenantports.PortalBrandingWriter = (*PortalBrandingRepository)(nil)

func NewPortalBrandingRepository(pool *pgxpool.Pool) *PortalBrandingRepository {
	return &PortalBrandingRepository{pool: pool}
}

func IsTenantNameTaken(err error) bool {
	if errors.Is(err, ErrTenantNameTaken) {
		return true
	}
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "ux_iam_tenants_tenant_name"
}

func IsTenantReferenced(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

func (r *PortalBrandingRepository) Get(ctx context.Context, tenantID string) (*PortalBranding, error) {
	branding := &PortalBranding{TenantID: tenantID}
	err := r.pool.QueryRow(ctx, `
		SELECT t.tenant_name,
		       COALESCE(b.customer_site_name, ''),
		       b.favicon_png,
		       b.favicon_updated_at
		FROM iam_tenants t
		LEFT JOIN iam_tenant_portal_branding b ON b.tenant_id = t.tenant_id
		WHERE t.tenant_id = $1
	`, tenantID).Scan(
		&branding.TenantName,
		&branding.CustomerSiteName,
		&branding.FaviconPNG,
		&branding.FaviconUpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTenantNotFound
	}
	if err != nil {
		return nil, err
	}
	return branding, nil
}

func (r *PortalBrandingRepository) UpdateSettings(ctx context.Context, tenantID, tenantName, customerSiteName string) (*PortalBranding, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var existingID string
	err = tx.QueryRow(ctx, `SELECT tenant_id FROM iam_tenants WHERE tenant_name = $1 AND tenant_id <> $2 LIMIT 1`, tenantName, tenantID).Scan(&existingID)
	if err == nil {
		return nil, ErrTenantNameTaken
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	command, err := tx.Exec(ctx, `UPDATE iam_tenants SET tenant_name = $1, updated_at = $2 WHERE tenant_id = $3`, tenantName, time.Now().UTC(), tenantID)
	if err != nil {
		if IsTenantNameTaken(err) {
			return nil, ErrTenantNameTaken
		}
		return nil, err
	}
	if command.RowsAffected() == 0 {
		return nil, ErrTenantNotFound
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO iam_tenant_portal_branding (tenant_id, customer_site_name, updated_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (tenant_id) DO UPDATE
		SET customer_site_name = EXCLUDED.customer_site_name,
		    updated_at = EXCLUDED.updated_at
	`, tenantID, customerSiteName, time.Now().UTC())
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.Get(ctx, tenantID)
}

func (r *PortalBrandingRepository) UpdateFavicon(ctx context.Context, tenantID string, faviconPNG []byte) (*PortalBranding, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM iam_tenants WHERE tenant_id = $1)`, tenantID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrTenantNotFound
	}
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		INSERT INTO iam_tenant_portal_branding (tenant_id, favicon_png, favicon_updated_at, updated_at)
		VALUES ($1, $2, $3, $3)
		ON CONFLICT (tenant_id) DO UPDATE
		SET favicon_png = EXCLUDED.favicon_png,
		    favicon_updated_at = EXCLUDED.favicon_updated_at,
		    updated_at = EXCLUDED.updated_at
	`, tenantID, faviconPNG, now)
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, tenantID)
}

func (r *PortalBrandingRepository) ClearFavicon(ctx context.Context, tenantID string) (*PortalBranding, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM iam_tenants WHERE tenant_id = $1)`, tenantID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrTenantNotFound
	}
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		INSERT INTO iam_tenant_portal_branding (tenant_id, favicon_png, favicon_updated_at, updated_at)
		VALUES ($1, NULL, NULL, $2)
		ON CONFLICT (tenant_id) DO UPDATE
		SET favicon_png = NULL,
		    favicon_updated_at = NULL,
		    updated_at = EXCLUDED.updated_at
	`, tenantID, now)
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, tenantID)
}
