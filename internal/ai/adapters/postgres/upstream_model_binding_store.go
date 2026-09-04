package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/ai/domain"
)

type UpstreamModelBindingStore struct {
	pool *translatingPool
}

func NewUpstreamModelBindingStore(pool *pgxpool.Pool) *UpstreamModelBindingStore {
	return &UpstreamModelBindingStore{pool: newTranslatingPool(pool)}
}

func (s *UpstreamModelBindingStore) List(ctx context.Context, scope domain.UpstreamModelBindingScope) ([]domain.UpstreamModelBinding, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			id::text,
			model_code,
			capability_type,
			upstream_model_name,
			status,
			config_json,
			created_at,
			updated_at
		FROM ai_upstream_models
		WHERE upstream_kind = $1 AND upstream_id = $2::uuid
		ORDER BY model_code ASC, upstream_model_name ASC
	`, string(scope.Kind), scope.ID)
	if err != nil {
		return nil, fmt.Errorf("list upstream model bindings: %w", err)
	}
	defer rows.Close()

	items := make([]domain.UpstreamModelBinding, 0)
	for rows.Next() {
		item, err := scanManagementBinding(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan upstream model binding: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate upstream model bindings: %w", err)
	}
	return items, nil
}

func (s *UpstreamModelBindingStore) ListModelCodes(ctx context.Context, scope domain.UpstreamModelBindingScope) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT model_code
		FROM ai_upstream_models
		WHERE upstream_kind = $1 AND upstream_id = $2::uuid
	`, string(scope.Kind), scope.ID)
	if err != nil {
		return nil, fmt.Errorf("list upstream model codes: %w", err)
	}
	defer rows.Close()

	codes := make([]string, 0)
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, fmt.Errorf("scan upstream model code: %w", err)
		}
		codes = append(codes, code)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate upstream model codes: %w", err)
	}
	return codes, nil
}

func (s *UpstreamModelBindingStore) FindByModel(ctx context.Context, scope domain.UpstreamModelBindingScope, modelCode string) (domain.UpstreamModelBinding, error) {
	item, err := scanManagementBinding(s.pool.QueryRow(ctx, `
		SELECT
			id::text,
			model_code,
			capability_type,
			upstream_model_name,
			status,
			config_json,
			created_at,
			updated_at
		FROM ai_upstream_models
		WHERE upstream_kind = $1 AND upstream_id = $2::uuid AND model_code = $3
		ORDER BY (status = 'active') DESC
		LIMIT 1
	`, string(scope.Kind), scope.ID, modelCode).Scan)
	err = translatePersistenceError(err)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.UpstreamModelBinding{}, err
	}
	if err != nil {
		return domain.UpstreamModelBinding{}, fmt.Errorf("find upstream model binding: %w", err)
	}
	return item, nil
}

func (s *UpstreamModelBindingStore) Get(ctx context.Context, scope domain.UpstreamModelBindingScope, bindingID string) (domain.UpstreamModelBinding, error) {
	item, err := scanManagementBinding(s.pool.QueryRow(ctx, `
		SELECT
			id::text,
			model_code,
			capability_type,
			upstream_model_name,
			status,
			config_json,
			created_at,
			updated_at
		FROM ai_upstream_models
		WHERE id = $1::uuid AND upstream_kind = $2 AND upstream_id = $3::uuid
	`, bindingID, string(scope.Kind), scope.ID).Scan)
	err = translatePersistenceError(err)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.UpstreamModelBinding{}, err
	}
	if err != nil {
		return domain.UpstreamModelBinding{}, fmt.Errorf("get upstream model binding: %w", err)
	}
	return item, nil
}

func (s *UpstreamModelBindingStore) Create(ctx context.Context, scope domain.UpstreamModelBindingScope, write domain.UpstreamModelBindingWrite) (domain.UpstreamModelBinding, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.UpstreamModelBinding{}, fmt.Errorf("begin create upstream model binding: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := validateActiveModelBindingWrite(ctx, tx, scope, write); err != nil {
		return domain.UpstreamModelBinding{}, err
	}
	item, err := scanManagementBinding(tx.QueryRow(ctx, `
		INSERT INTO ai_upstream_models (
			upstream_kind, upstream_id, model_code, capability_type,
			upstream_model_name, status, config_json
		) VALUES ($1, $2::uuid, $3, $4, $5, $6, $7::jsonb)
		RETURNING
			id::text,
			model_code,
			capability_type,
			upstream_model_name,
			status,
			config_json,
			created_at,
			updated_at
	`,
		string(scope.Kind), scope.ID, write.ModelCode, write.CapabilityType,
		write.UpstreamModelName, write.Status, nonEmptyBindingConfig(write.ConfigJSON),
	).Scan)
	if err != nil {
		return domain.UpstreamModelBinding{}, fmt.Errorf("create upstream model binding: %w", translatePersistenceError(err))
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.UpstreamModelBinding{}, fmt.Errorf("commit create upstream model binding: %w", err)
	}
	return item, nil
}

func (s *UpstreamModelBindingStore) Update(ctx context.Context, scope domain.UpstreamModelBindingScope, bindingID string, write domain.UpstreamModelBindingWrite) (domain.UpstreamModelBinding, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.UpstreamModelBinding{}, fmt.Errorf("begin update upstream model binding: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := validateActiveModelBindingWrite(ctx, tx, scope, write); err != nil {
		return domain.UpstreamModelBinding{}, err
	}
	item, err := scanManagementBinding(tx.QueryRow(ctx, `
		UPDATE ai_upstream_models
		SET model_code = $4,
			capability_type = $5,
			upstream_model_name = $6,
			status = $7,
			config_json = $8::jsonb,
			updated_at = now()
		WHERE id = $1::uuid AND upstream_kind = $2 AND upstream_id = $3::uuid
		RETURNING
			id::text,
			model_code,
			capability_type,
			upstream_model_name,
			status,
			config_json,
			created_at,
			updated_at
	`,
		bindingID, string(scope.Kind), scope.ID, write.ModelCode, write.CapabilityType,
		write.UpstreamModelName, write.Status, nonEmptyBindingConfig(write.ConfigJSON),
	).Scan)
	err = translatePersistenceError(err)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.UpstreamModelBinding{}, err
	}
	if err != nil {
		return domain.UpstreamModelBinding{}, fmt.Errorf("update upstream model binding: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.UpstreamModelBinding{}, fmt.Errorf("commit update upstream model binding: %w", err)
	}
	return item, nil
}

func (s *UpstreamModelBindingStore) Delete(ctx context.Context, scope domain.UpstreamModelBindingScope, bindingID string) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM ai_upstream_models
		WHERE id = $1::uuid AND upstream_kind = $2 AND upstream_id = $3::uuid
	`, bindingID, string(scope.Kind), scope.ID)
	if err != nil {
		return fmt.Errorf("delete upstream model binding: %w", translatePersistenceError(err))
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *UpstreamModelBindingStore) BatchDelete(ctx context.Context, scope domain.UpstreamModelBindingScope, bindingIDs []string) (int64, error) {
	ids := make([]pgtype.UUID, 0, len(bindingIDs))
	for _, bindingID := range bindingIDs {
		id, err := akUUID(bindingID)
		if err != nil {
			return 0, err
		}
		ids = append(ids, id)
	}
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM ai_upstream_models
		WHERE upstream_kind = $1
		  AND upstream_id = $2::uuid
		  AND id = ANY($3::uuid[])
	`, string(scope.Kind), scope.ID, ids)
	if err != nil {
		return 0, fmt.Errorf("batch delete upstream model bindings: %w", translatePersistenceError(err))
	}
	return tag.RowsAffected(), nil
}

func (s *UpstreamModelBindingStore) Import(ctx context.Context, scope domain.UpstreamModelBindingScope, writes []domain.UpstreamModelBindingWrite) (domain.UpstreamModelBindingImportResult, error) {
	result := domain.UpstreamModelBindingImportResult{Created: []string{}, Skipped: []string{}}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return result, fmt.Errorf("begin upstream model import: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if len(writes) > 0 {
		if err := validateActiveModelBindingWrite(ctx, tx, scope, writes[0]); err != nil {
			return result, err
		}
	}

	rows, err := tx.Query(ctx, `
		SELECT model_code, capability_type
		FROM ai_upstream_models
		WHERE upstream_kind = $1 AND upstream_id = $2::uuid
	`, string(scope.Kind), scope.ID)
	if err != nil {
		return result, fmt.Errorf("list existing upstream models: %w", err)
	}
	existing := make(map[string]struct{})
	for rows.Next() {
		var code, capability string
		if err := rows.Scan(&code, &capability); err != nil {
			rows.Close()
			return result, fmt.Errorf("scan existing upstream model: %w", err)
		}
		existing[modelBindingImportKey(code, capability)] = struct{}{}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("iterate existing upstream models: %w", err)
	}

	for _, write := range writes {
		if err := validateActiveModelBindingWrite(ctx, tx, scope, write); err != nil {
			return result, err
		}
		key := modelBindingImportKey(write.ModelCode, write.CapabilityType)
		if _, ok := existing[key]; ok {
			result.Skipped = append(result.Skipped, write.ModelCode)
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO ai_upstream_models (
				upstream_kind, upstream_id, model_code, capability_type,
				upstream_model_name, status
			) VALUES ($1, $2::uuid, $3, $4, $5, $6)
		`, string(scope.Kind), scope.ID, write.ModelCode, write.CapabilityType,
			write.UpstreamModelName, write.Status); err != nil {
			return result, fmt.Errorf("insert imported upstream model: %w", translatePersistenceError(err))
		}
		existing[key] = struct{}{}
		result.Created = append(result.Created, write.ModelCode)
	}
	if err := tx.Commit(ctx); err != nil {
		return result, fmt.Errorf("commit upstream model import: %w", err)
	}
	return result, nil
}

func modelBindingImportKey(modelCode, capability string) string {
	return modelCode + "\x00" + capability
}

func scanManagementBinding(scan func(dest ...any) error) (domain.UpstreamModelBinding, error) {
	var item domain.UpstreamModelBinding
	err := scan(
		&item.ID,
		&item.ModelCode,
		&item.CapabilityType,
		&item.UpstreamModelName,
		&item.Status,
		&item.ConfigJSON,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	return item, err
}

func nonEmptyBindingConfig(config []byte) []byte {
	if len(config) == 0 {
		return []byte(`{}`)
	}
	return config
}
