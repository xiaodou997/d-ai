package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/ai/core/upstream"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/upstreamaccess"
)

type UpstreamAccessRepo struct {
	pool *translatingPool
}

func NewUpstreamAccessRepo(pool *pgxpool.Pool) *UpstreamAccessRepo {
	return &UpstreamAccessRepo{pool: newTranslatingPool(pool)}
}

var _ upstreamaccess.Repository = (*UpstreamAccessRepo)(nil)

func (r *UpstreamAccessRepo) ListForTenant(ctx context.Context, tenantID string) ([]upstreamaccess.ResourceAccess, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT r.resource_kind, r.id::text, r.name, r.tenant_display_name,
		       r.tenant_access_mode, r.status,
		       COALESCE(p.access_granted, false) AS access_granted,
		       r.status = 'active' AND (
		         r.tenant_access_mode = 'public' OR COALESCE(p.access_granted, false)
		       ) AS allowed,
		       COALESCE(r.tenant_multiplier, 1)::float8 AS default_multiplier,
		       p.tenant_multiplier_override::float8,
		       COALESCE(p.tenant_multiplier_override, r.tenant_multiplier, 1)::float8 AS effective_multiplier
		FROM ai_upstream_resources r
		LEFT JOIN ai_upstream_resource_tenant_policies p
		  ON p.resource_kind = r.resource_kind AND p.resource_id = r.id AND p.tenant_id = $1
		ORDER BY r.resource_kind, r.name, r.id
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]upstreamaccess.ResourceAccess, 0)
	for rows.Next() {
		var item upstreamaccess.ResourceAccess
		if err := rows.Scan(
			&item.Kind, &item.ID, &item.InternalName, &item.TenantDisplayName,
			&item.AccessMode, &item.Status, &item.AccessGranted, &item.Allowed,
			&item.DefaultTenantMultiplier, &item.TenantMultiplierOverride,
			&item.EffectiveTenantMultiplier,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *UpstreamAccessRepo) ReplacePolicies(ctx context.Context, tenantID string, policies []upstreamaccess.TenantResourcePolicy) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM ai_upstream_resource_tenant_policies WHERE tenant_id = $1`, tenantID); err != nil {
		return err
	}
	for _, policy := range policies {
		tag, err := tx.Exec(ctx, `
			INSERT INTO ai_upstream_resource_tenant_policies (
			  resource_kind, resource_id, tenant_id, access_granted,
			  tenant_multiplier_override
			)
			SELECT resource_kind, id, $3,
			       CASE WHEN tenant_access_mode = 'restricted' THEN $4 ELSE false END,
			       $5
			FROM ai_upstream_resources
			WHERE resource_kind = $1 AND id = $2::uuid
		`, policy.Kind, policy.ID, tenantID, policy.AccessGranted, policy.TenantMultiplierOverride)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.NewValidationError("policies", "policy target must exist")
		}
	}
	return tx.Commit(ctx)
}

func (r *UpstreamAccessRepo) CanAccess(ctx context.Context, tenantID string, ref upstreamaccess.ResourceRef) (bool, error) {
	var allowed bool
	err := r.pool.QueryRow(ctx, `
		SELECT r.tenant_access_mode = 'public' OR EXISTS (
		  SELECT 1 FROM ai_upstream_resource_tenant_policies p
		  WHERE p.resource_kind = r.resource_kind AND p.resource_id = r.id
		    AND p.tenant_id = $3 AND p.access_granted
		)
		FROM ai_upstream_resources r
		WHERE r.resource_kind = $1 AND r.id = $2::uuid AND r.status = 'active'
	`, ref.Kind, ref.ID, tenantID).Scan(&allowed)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return allowed, err
}

type RuntimeBindingAuthorizer struct {
	access *upstreamaccess.Service
}

func NewRuntimeBindingAuthorizer(pool *pgxpool.Pool) *RuntimeBindingAuthorizer {
	return &RuntimeBindingAuthorizer{access: upstreamaccess.New(NewUpstreamAccessRepo(pool))}
}

func (a *RuntimeBindingAuthorizer) AuthorizeRuntimeBinding(ctx context.Context, req upstream.RuntimeBindingRequest) error {
	kind := upstreamaccess.KindDirectUpstream
	if req.TargetMode == upstream.AccessModeOAuthPool {
		kind = upstreamaccess.KindOAuthPool
	}
	allowed, err := a.access.CanAccess(ctx, req.TenantID, upstreamaccess.ResourceRef{Kind: kind, ID: req.TargetID})
	if err != nil {
		return err
	}
	if !allowed {
		return upstream.NewRuntimeBindingRejection(upstream.BindingRejectionAccessDenied, "tenant cannot access target")
	}
	return nil
}
