package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/ai/application"
	"xiaodou/dai/internal/ai/domain"
)

const (
	PromptStrategyNone            = "none"
	PromptStrategyCallerVariables = "caller_variables"
	PromptStrategyBoundExact      = "bound_prompt_exact"
)

type ApplicationAppRepo struct {
	pool *pgxpool.Pool
}

func NewApplicationAppRepo(pool *pgxpool.Pool) *ApplicationAppRepo {
	return &ApplicationAppRepo{pool: pool}
}

type AppOwner struct {
	Type     string
	TenantID string
	UserID   string
}

type AppViewer struct {
	TenantID string
	UserID   string
}

type AppPromptBindingRecord struct {
	PromptID        string
	PromptName      string
	PromptStatus    string
	CurrentRevision int32
	TemplateText    string
	Variables       []byte
	BindingRole     string
	DisplayOrder    int32
}

type AppAgentRecord struct {
	OwnerType         string
	OwnerTenantID     string
	OwnerUserID       string
	ID                string
	Name              string
	Description       string
	Status            string
	Capability        string
	PromptStrategy    string
	GroupID           string
	ModelCode         string
	DefaultOptions    []byte
	PromptBindings    []AppPromptBindingRecord
	PublishedByTenant bool
	CreatedBy         *string
	UpdatedBy         *string
	CreatedAt         *int64
	UpdatedAt         *int64
}

const appSelectColumns = `
	SELECT a.owner_type,
	       a.owner_tenant_id,
	       a.owner_user_id,
	       a.id::text,
	       a.name,
	       a.description,
	       a.status,
	       a.capability,
	       a.prompt_strategy,
	       COALESCE(a.group_id::text, ''),
	       a.model_code,
	       a.default_options,
	       EXISTS (
	         SELECT 1 FROM ai_app_publications pt
	         WHERE pt.app_id = a.id
	           AND pt.publisher_scope = 'tenant'
	           AND pt.publisher_tenant_id = $1
	           AND pt.status = 'active'
	       ) AS published_by_tenant,
	       NULLIF(a.created_by, ''),
	       NULLIF(a.updated_by, ''),
	       EXTRACT(EPOCH FROM a.created_at)::bigint * 1000,
	       EXTRACT(EPOCH FROM a.updated_at)::bigint * 1000
	FROM ai_apps a
`

func (r *ApplicationAppRepo) ListOwnedAgents(ctx context.Context, owner AppOwner) ([]AppAgentRecord, error) {
	rows, err := r.pool.Query(ctx, appSelectColumns+`
		WHERE a.owner_type = $2
		  AND a.owner_tenant_id = $3
		  AND a.owner_user_id = $4
		ORDER BY a.updated_at DESC, a.name ASC
	`, owner.TenantID, owner.Type, owner.TenantID, owner.UserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := collectAppRecords(rows)
	if err != nil {
		return nil, err
	}
	return r.attachPromptBindings(ctx, items)
}

func (r *ApplicationAppRepo) GetOwnedAgent(ctx context.Context, owner AppOwner, appID string) (AppAgentRecord, error) {
	row := r.pool.QueryRow(ctx, appSelectColumns+`
		WHERE a.id = $2
		  AND a.owner_type = $3
		  AND a.owner_tenant_id = $4
		  AND a.owner_user_id = $5
	`, owner.TenantID, appID, owner.Type, owner.TenantID, owner.UserID)
	item, err := scanAppRecord(row.Scan)
	if err != nil {
		if err == pgx.ErrNoRows {
			return AppAgentRecord{}, domain.ErrNotFound
		}
		return AppAgentRecord{}, err
	}
	items, err := r.attachPromptBindings(ctx, []AppAgentRecord{item})
	if err != nil {
		return AppAgentRecord{}, err
	}
	return items[0], nil
}

const appVisibilityPredicate = `
	(
	  ($2 = '' AND a.owner_type = 'tenant' AND a.owner_tenant_id = $1)
	  OR ($2 <> '' AND (
	    (a.owner_type = 'user' AND a.owner_tenant_id = $1 AND a.owner_user_id = $2)
	    OR (a.owner_type = 'tenant' AND a.owner_tenant_id = $1 AND EXISTS (
	      SELECT 1 FROM ai_app_publications pt
	      WHERE pt.app_id = a.id
	        AND pt.publisher_scope = 'tenant'
	        AND pt.publisher_tenant_id = $1
	        AND pt.status = 'active'
	    ))
	  ))
	)
`

func (r *ApplicationAppRepo) ListVisibleAgents(ctx context.Context, viewer AppViewer, capabilities []string) ([]AppAgentRecord, error) {
	if len(capabilities) == 0 {
		return []AppAgentRecord{}, nil
	}
	rows, err := r.pool.Query(ctx, appSelectColumns+`
		WHERE a.status = 'active'
		  AND a.capability = ANY($3::text[])
		  AND `+appVisibilityPredicate+`
		ORDER BY a.owner_type ASC, a.updated_at DESC, a.name ASC
	`, viewer.TenantID, viewer.UserID, capabilities)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := collectAppRecords(rows)
	if err != nil {
		return nil, err
	}
	return r.attachPromptBindings(ctx, items)
}

func (r *ApplicationAppRepo) GetVisibleAgentByID(ctx context.Context, viewer AppViewer, appID string, capabilities []string) (AppAgentRecord, error) {
	row := r.pool.QueryRow(ctx, appSelectColumns+`
		WHERE a.id = $4
		  AND a.status = 'active'
		  AND a.capability = ANY($3::text[])
		  AND `+appVisibilityPredicate+`
	`, viewer.TenantID, viewer.UserID, capabilities, appID)
	item, err := scanAppRecord(row.Scan)
	if err != nil {
		return AppAgentRecord{}, err
	}
	items, err := r.attachPromptBindings(ctx, []AppAgentRecord{item})
	if err != nil {
		return AppAgentRecord{}, err
	}
	return items[0], nil
}

func (r *ApplicationAppRepo) SetTenantPublication(ctx context.Context, tenantID, appID, actor string) error {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO ai_app_publications (app_id, publisher_scope, publisher_tenant_id, audience, status, created_by)
		SELECT a.id, 'tenant', $2, 'tenant_users', 'active', NULLIF($3, '')
		FROM ai_apps a
		WHERE a.id = $1 AND a.owner_type = 'tenant' AND a.owner_tenant_id = $2
		ON CONFLICT (app_id, publisher_scope, publisher_tenant_id)
		DO UPDATE SET status = 'active', updated_at = now()
	`, appID, tenantID, actor)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.NewValidationError("app_id", "app does not exist or is not publishable by current tenant")
	}
	return nil
}

func (r *ApplicationAppRepo) RemoveTenantPublication(ctx context.Context, tenantID, appID string) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM ai_app_publications
		WHERE app_id = $1 AND publisher_scope = 'tenant' AND publisher_tenant_id = $2
	`, appID, tenantID)
	return err
}

func (r *ApplicationAppRepo) CreateOwnedAgent(
	ctx context.Context,
	owner AppOwner,
	name, description, status, capability, promptStrategy string,
	promptIDs []string,
	groupID, modelCode string,
	defaultOptions map[string]any,
	actor string,
) (AppAgentRecord, error) {
	if err := r.validatePromptBindings(ctx, owner, promptStrategy, promptIDs); err != nil {
		return AppAgentRecord{}, err
	}
	raw, _ := json.Marshal(defaultOptions)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return AppAgentRecord{}, err
	}
	defer tx.Rollback(ctx)
	var appID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO ai_apps (
		  owner_type, owner_tenant_id, owner_user_id, name, description, status,
		  capability, prompt_strategy, group_id, model_code, default_options, created_by, updated_by
		) VALUES (
		  $1, $2, $3, $4, $5, $6, $7, $8, $9::uuid, $10, $11::jsonb, NULLIF($12, ''), NULLIF($12, '')
		)
		RETURNING id::text
	`, owner.Type, owner.TenantID, owner.UserID, name, description, status, capability, promptStrategy, groupID, modelCode, raw, actor).Scan(&appID); err != nil {
		return AppAgentRecord{}, err
	}
	if err := replacePromptBindings(ctx, tx, appID, promptStrategy, promptIDs); err != nil {
		return AppAgentRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return AppAgentRecord{}, err
	}
	return r.GetOwnedAgent(ctx, owner, appID)
}

func (r *ApplicationAppRepo) UpdateOwnedAgent(
	ctx context.Context,
	owner AppOwner,
	appID, name, description, status, capability, promptStrategy string,
	promptIDs []string,
	groupID, modelCode string,
	defaultOptions map[string]any,
	actor string,
) (AppAgentRecord, error) {
	if err := r.validatePromptBindings(ctx, owner, promptStrategy, promptIDs); err != nil {
		return AppAgentRecord{}, err
	}
	raw, _ := json.Marshal(defaultOptions)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return AppAgentRecord{}, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `
		UPDATE ai_apps
		SET name = $2, description = $3, status = $4, capability = $5,
		    prompt_strategy = $6, group_id = $7::uuid, model_code = $8,
		    default_options = $9::jsonb, updated_by = NULLIF($10, ''), updated_at = now()
		WHERE id = $1 AND owner_type = $11 AND owner_tenant_id = $12 AND owner_user_id = $13
	`, appID, name, description, status, capability, promptStrategy, groupID, modelCode, raw, actor, owner.Type, owner.TenantID, owner.UserID)
	if err != nil {
		return AppAgentRecord{}, err
	}
	if tag.RowsAffected() == 0 {
		return AppAgentRecord{}, domain.ErrNotFound
	}
	if err := replacePromptBindings(ctx, tx, appID, promptStrategy, promptIDs); err != nil {
		return AppAgentRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return AppAgentRecord{}, err
	}
	return r.GetOwnedAgent(ctx, owner, appID)
}

func (r *ApplicationAppRepo) DeleteOwnedAgent(ctx context.Context, owner AppOwner, appID string) error {
	var refs int
	if err := r.pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM ai_app_keys WHERE app_id = $1) +
			(SELECT COUNT(*) FROM ai_workspace_threads WHERE app_id = $1 AND status <> 'deleted')
	`, appID).Scan(&refs); err != nil {
		return err
	}
	if refs > 0 {
		return fmt.Errorf("app is still referenced by %d runtime record(s), clear app keys/sessions before deleting: %w", refs, domain.ErrConflict)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, statement := range []string{
		`DELETE FROM ai_app_publications WHERE app_id = $1`,
		`DELETE FROM ai_app_prompt_bindings WHERE app_id = $1`,
	} {
		if _, err := tx.Exec(ctx, statement, appID); err != nil {
			return err
		}
	}
	tag, err := tx.Exec(ctx, `
		DELETE FROM ai_apps
		WHERE id = $1 AND owner_type = $2 AND owner_tenant_id = $3 AND owner_user_id = $4
	`, appID, owner.Type, owner.TenantID, owner.UserID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return tx.Commit(ctx)
}

func (r *ApplicationAppRepo) validatePromptBindings(ctx context.Context, owner AppOwner, strategy string, promptIDs []string) error {
	promptIDs = uniqueNonEmpty(promptIDs)
	switch strategy {
	case PromptStrategyNone:
		if len(promptIDs) != 0 {
			return domain.NewValidationError("prompt_ids", "must be empty for none strategy")
		}
		return nil
	case PromptStrategyCallerVariables:
		if len(promptIDs) != 1 {
			return domain.NewValidationError("prompt_ids", "must contain exactly one prompt for caller_variables strategy")
		}
	case PromptStrategyBoundExact:
		if len(promptIDs) == 0 {
			return domain.NewValidationError("prompt_ids", "must contain at least one prompt for bound_prompt_exact strategy")
		}
		if len(promptIDs) > application.MaxPromptPlaceholders {
			return domain.NewValidationError("prompt_ids", "contains too many prompts for bound_prompt_exact strategy")
		}
	default:
		return domain.NewValidationError("prompt_strategy", "unsupported prompt strategy")
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, name, status
		FROM ai_app_prompts
		WHERE id = ANY($1::uuid[])
		  AND owner_type = $2 AND owner_tenant_id = $3 AND owner_user_id = $4
	`, promptIDs, owner.Type, owner.TenantID, owner.UserID)
	if err != nil {
		return err
	}
	defer rows.Close()
	seenIDs := map[string]struct{}{}
	seenNames := map[string]struct{}{}
	for rows.Next() {
		var id, name, status string
		if err := rows.Scan(&id, &name, &status); err != nil {
			return err
		}
		if status != "active" {
			return domain.NewValidationError("prompt_ids", "all bound prompts must be active")
		}
		seenIDs[id] = struct{}{}
		normalizedName, err := application.NormalizePromptName(name)
		if err != nil {
			return domain.NewValidationError("prompt_ids", "bound prompt names must be valid placeholder names")
		}
		if _, exists := seenNames[normalizedName]; exists {
			return domain.NewValidationError("prompt_ids", "bound prompt names must be unique within an app")
		}
		seenNames[normalizedName] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(seenIDs) != len(promptIDs) {
		return domain.NewValidationError("prompt_ids", "all prompts must exist in the current owner scope")
	}
	return nil
}

func replacePromptBindings(ctx context.Context, tx pgx.Tx, appID, strategy string, promptIDs []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM ai_app_prompt_bindings WHERE app_id = $1`, appID); err != nil {
		return err
	}
	role := "fragment"
	if strategy == PromptStrategyCallerVariables {
		role = "primary"
	}
	for index, promptID := range uniqueNonEmpty(promptIDs) {
		if _, err := tx.Exec(ctx, `
			INSERT INTO ai_app_prompt_bindings (app_id, prompt_id, binding_role, display_order)
			VALUES ($1, $2::uuid, $3, $4)
		`, appID, promptID, role, index); err != nil {
			return err
		}
	}
	return nil
}

func (r *ApplicationAppRepo) attachPromptBindings(ctx context.Context, apps []AppAgentRecord) ([]AppAgentRecord, error) {
	if len(apps) == 0 {
		return apps, nil
	}
	ids := make([]string, 0, len(apps))
	indexByID := make(map[string]int, len(apps))
	for index := range apps {
		apps[index].PromptBindings = []AppPromptBindingRecord{}
		ids = append(ids, apps[index].ID)
		indexByID[apps[index].ID] = index
	}
	rows, err := r.pool.Query(ctx, `
		SELECT b.app_id::text, p.id::text, p.name, p.status, p.current_version,
		       v.template_text, v.variables, b.binding_role, b.display_order
		FROM ai_app_prompt_bindings b
		JOIN ai_app_prompts p ON p.id = b.prompt_id
		JOIN ai_app_prompt_versions v ON v.prompt_id = p.id AND v.version = p.current_version
		WHERE b.app_id = ANY($1::uuid[])
		ORDER BY b.app_id, b.display_order, p.name
	`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var appID string
		var binding AppPromptBindingRecord
		if err := rows.Scan(
			&appID, &binding.PromptID, &binding.PromptName, &binding.PromptStatus,
			&binding.CurrentRevision, &binding.TemplateText, &binding.Variables,
			&binding.BindingRole, &binding.DisplayOrder,
		); err != nil {
			return nil, err
		}
		if index, ok := indexByID[appID]; ok {
			apps[index].PromptBindings = append(apps[index].PromptBindings, binding)
		}
	}
	return apps, rows.Err()
}

func collectAppRecords(rows pgx.Rows) ([]AppAgentRecord, error) {
	out := make([]AppAgentRecord, 0)
	for rows.Next() {
		item, err := scanAppRecord(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func scanAppRecord(scan func(dest ...any) error) (AppAgentRecord, error) {
	var item AppAgentRecord
	if err := scan(
		&item.OwnerType, &item.OwnerTenantID, &item.OwnerUserID, &item.ID,
		&item.Name, &item.Description, &item.Status, &item.Capability,
		&item.PromptStrategy, &item.GroupID, &item.ModelCode, &item.DefaultOptions,
		&item.PublishedByTenant, &item.CreatedBy,
		&item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return AppAgentRecord{}, err
	}
	return item, nil
}

func uniqueNonEmpty(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
