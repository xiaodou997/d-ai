package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/ai/application"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
)

type ApplicationPromptRepo struct {
	pool *pgxpool.Pool
}

func NewApplicationPromptRepo(pool *pgxpool.Pool) *ApplicationPromptRepo {
	return &ApplicationPromptRepo{pool: pool}
}

type AppPromptWrite struct {
	OwnerType     string
	OwnerTenantID string
	OwnerUserID   string
	Name          string
	Description   string
	Status        string
	TemplateText  string
	Notes         string
	Actor         string
}

type AppPromptPatch struct {
	OwnerType     string
	OwnerTenantID string
	OwnerUserID   string
	PromptID      string
	Name          *string
	Description   *string
	Status        *string
	TemplateText  *string
	Notes         string
	Actor         string
}

func (r *ApplicationPromptRepo) CreatePrompt(ctx context.Context, in AppPromptWrite) (application.Prompt, error) {
	variables, err := application.ExtractPromptVariables(in.TemplateText)
	if err != nil {
		return application.Prompt{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return application.Prompt{}, err
	}
	defer tx.Rollback(ctx)

	var promptID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO ai_app_prompts (owner_type, owner_tenant_id, owner_user_id, name, description, status, current_version, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, 1, NULLIF($7, ''), NULLIF($7, ''))
		RETURNING id::text
	`, in.OwnerType, in.OwnerTenantID, in.OwnerUserID, in.Name, in.Description, in.Status, in.Actor).Scan(&promptID); err != nil {
		return application.Prompt{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO ai_app_prompt_versions (prompt_id, version, template_text, variables, notes, created_by)
		VALUES ($1, 1, $2, $3::jsonb, $4, NULLIF($5, ''))
	`, promptID, in.TemplateText, mustJSON(variables), in.Notes, in.Actor); err != nil {
		return application.Prompt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return application.Prompt{}, err
	}
	return r.GetPrompt(ctx, in.OwnerType, in.OwnerTenantID, in.OwnerUserID, promptID)
}

func (r *ApplicationPromptRepo) ListPrompts(ctx context.Context, ownerType, ownerTenantID, ownerUserID string) ([]application.Prompt, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT p.id::text,
		       p.owner_type,
		       p.owner_tenant_id,
		       p.owner_user_id,
		       p.name,
		       p.description,
		       p.status,
		       p.current_version,
		       v.template_text,
		       v.variables,
		       COALESCE(p.created_by, ''),
		       COALESCE(p.updated_by, ''),
		       p.created_at,
		       p.updated_at
		FROM ai_app_prompts p
		JOIN ai_app_prompt_versions v
		  ON v.prompt_id = p.id
		 AND v.version = p.current_version
		WHERE p.owner_type = $1
		  AND p.owner_tenant_id = $2
		  AND p.owner_user_id = $3
		ORDER BY p.updated_at DESC, p.name ASC
	`, ownerType, ownerTenantID, ownerUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]application.Prompt, 0)
	for rows.Next() {
		item, err := scanApplicationPrompt(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *ApplicationPromptRepo) GetPrompt(ctx context.Context, ownerType, ownerTenantID, ownerUserID, promptID string) (application.Prompt, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT p.id::text,
		       p.owner_type,
		       p.owner_tenant_id,
		       p.owner_user_id,
		       p.name,
		       p.description,
		       p.status,
		       p.current_version,
		       v.template_text,
		       v.variables,
		       COALESCE(p.created_by, ''),
		       COALESCE(p.updated_by, ''),
		       p.created_at,
		       p.updated_at
		FROM ai_app_prompts p
		JOIN ai_app_prompt_versions v
		  ON v.prompt_id = p.id
		 AND v.version = p.current_version
		WHERE p.id = $1
		  AND p.owner_type = $2
		  AND p.owner_tenant_id = $3
		  AND p.owner_user_id = $4
	`, promptID, ownerType, ownerTenantID, ownerUserID)
	item, err := scanApplicationPrompt(row.Scan)
	if err != nil {
		if err == pgx.ErrNoRows {
			return application.Prompt{}, domain.ErrNotFound
		}
		return application.Prompt{}, err
	}
	return item, nil
}

func (r *ApplicationPromptRepo) UpdatePrompt(ctx context.Context, in AppPromptPatch) (application.Prompt, error) {
	var nextVariables []string
	if in.TemplateText != nil {
		var err error
		nextVariables, err = application.ExtractPromptVariables(*in.TemplateText)
		if err != nil {
			return application.Prompt{}, err
		}
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return application.Prompt{}, err
	}
	defer tx.Rollback(ctx)

	var (
		currentName        string
		currentDescription string
		currentStatus      string
		currentVersion     int32
	)
	if err := tx.QueryRow(ctx, `
		SELECT name, description, status, current_version
		FROM ai_app_prompts
		WHERE id = $1
		  AND owner_type = $2
		  AND owner_tenant_id = $3
		  AND owner_user_id = $4
		FOR UPDATE
	`, in.PromptID, in.OwnerType, in.OwnerTenantID, in.OwnerUserID).Scan(&currentName, &currentDescription, &currentStatus, &currentVersion); err != nil {
		if err == pgx.ErrNoRows {
			return application.Prompt{}, domain.ErrNotFound
		}
		return application.Prompt{}, err
	}

	name := currentName
	if in.Name != nil {
		name = *in.Name
	}
	description := currentDescription
	if in.Description != nil {
		description = *in.Description
	}
	status := currentStatus
	if in.Status != nil {
		status = *in.Status
	}

	nextVersion := currentVersion
	if in.TemplateText != nil {
		nextVersion++
		if _, err := tx.Exec(ctx, `
			INSERT INTO ai_app_prompt_versions (prompt_id, version, template_text, variables, notes, created_by)
			VALUES ($1, $2, $3, $4::jsonb, $5, NULLIF($6, ''))
		`, in.PromptID, nextVersion, *in.TemplateText, mustJSON(nextVariables), in.Notes, in.Actor); err != nil {
			return application.Prompt{}, err
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE ai_app_prompts
		SET name = $2,
		    description = $3,
		    status = $4,
		    current_version = $5,
		    updated_by = NULLIF($6, ''),
		    updated_at = now()
		WHERE id = $1
	`, in.PromptID, name, description, status, nextVersion, in.Actor); err != nil {
		return application.Prompt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return application.Prompt{}, err
	}
	return r.GetPrompt(ctx, in.OwnerType, in.OwnerTenantID, in.OwnerUserID, in.PromptID)
}

func (r *ApplicationPromptRepo) ListPromptVersions(ctx context.Context, promptID string) ([]application.PromptVersion, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text,
		       prompt_id::text,
		       version,
		       template_text,
		       variables,
		       COALESCE(notes, ''),
		       COALESCE(created_by, ''),
		       created_at
		FROM ai_app_prompt_versions
		WHERE prompt_id = $1
		ORDER BY version DESC
	`, promptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]application.PromptVersion, 0)
	for rows.Next() {
		var row application.PromptVersion
		var variablesRaw []byte
		if err := rows.Scan(&row.ID, &row.PromptID, &row.Version, &row.TemplateText, &variablesRaw, &row.Notes, &row.CreatedBy, &row.CreatedAt); err != nil {
			return nil, err
		}
		row.Variables = decodeStringArray(variablesRaw)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *ApplicationPromptRepo) DeletePrompt(ctx context.Context, ownerType, ownerTenantID, ownerUserID, promptID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(
		  SELECT 1
		  FROM ai_app_prompts
		  WHERE id = $1
		    AND owner_type = $2
		    AND owner_tenant_id = $3
		    AND owner_user_id = $4
		)
	`, promptID, ownerType, ownerTenantID, ownerUserID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return domain.ErrNotFound
	}

	var refs int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM ai_app_prompt_bindings
		WHERE prompt_id = $1
	`, promptID).Scan(&refs); err != nil {
		return err
	}
	if refs > 0 {
		return fmt.Errorf("prompt is still used by %d app(s), remove app bindings before deleting: %w", refs, domain.ErrConflict)
	}

	if _, err := tx.Exec(ctx, `
		DELETE FROM ai_app_prompt_versions
		WHERE prompt_id = $1
	`, promptID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		DELETE FROM ai_app_prompts
		WHERE id = $1
		  AND owner_type = $2
		  AND owner_tenant_id = $3
		  AND owner_user_id = $4
	`, promptID, ownerType, ownerTenantID, ownerUserID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return tx.Commit(ctx)
}

func scanApplicationPrompt(scan func(dest ...any) error) (application.Prompt, error) {
	var (
		item          application.Prompt
		variablesRaw  []byte
		createdBy     string
		updatedBy     string
		createdAt     time.Time
		updatedAt     time.Time
		ownerType     string
		ownerTenantID string
		ownerUserID   string
	)
	if err := scan(
		&item.ID,
		&ownerType,
		&ownerTenantID,
		&ownerUserID,
		&item.Name,
		&item.Description,
		&item.Status,
		&item.CurrentVersion,
		&item.CurrentTemplateText,
		&variablesRaw,
		&createdBy,
		&updatedBy,
		&createdAt,
		&updatedAt,
	); err != nil {
		return application.Prompt{}, err
	}
	item.OwnerScope = promptOwnerScope(ownerType)
	item.OwnerTenantID = ownerTenantID
	item.OwnerUserID = ownerUserID
	item.Code = item.Name
	item.CurrentVariables = decodeStringArray(variablesRaw)
	item.CreatedBy = createdBy
	item.UpdatedBy = updatedBy
	item.CreatedAt = createdAt
	item.UpdatedAt = updatedAt
	return item, nil
}

func promptOwnerScope(ownerType string) coreidentity.Scope {
	switch strings.TrimSpace(ownerType) {
	case "platform":
		return coreidentity.ScopePlatform
	case "tenant":
		return coreidentity.ScopeTenant
	case "user":
		return coreidentity.ScopeUser
	default:
		return coreidentity.Scope(strings.TrimSpace(ownerType))
	}
}

func decodeStringArray(raw []byte) []string {
	if len(raw) == 0 {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return []string{}
	}
	return out
}

func mustJSON(v any) []byte {
	raw, _ := json.Marshal(v)
	return raw
}
