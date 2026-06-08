package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	dbgen "xiaodou/unihub/ai-service/internal/db/gen"
	"xiaodou/unihub/ai-service/internal/domain"
	svcapikey "xiaodou/unihub/ai-service/internal/service/apikey"
)

// APIKeyRepo implements service/apikey.Repository on top of the sqlc query layer.
type APIKeyRepo struct {
	q *dbgen.Queries
}

func NewAPIKeyRepo(q *dbgen.Queries) *APIKeyRepo {
	return &APIKeyRepo{q: q}
}

var _ svcapikey.Repository = (*APIKeyRepo)(nil)

func (r *APIKeyRepo) Create(ctx context.Context, p svcapikey.CreateParams) (domain.APIKey, error) {
	row, err := r.q.CreateAPIKey(ctx, dbgen.CreateAPIKeyParams{
		OwnerType:     string(p.OwnerType),
		TenantID:      p.TenantID,
		UserID:        akText(p.UserID),
		KeyHash:       p.KeyHash,
		LastFour:      akText(p.LastFour),
		Name:          p.Name,
		QuotaLimit:    akInt8(p.QuotaLimitMicro),
		AllowedModels: akModelsJSON(p.AllowedModels),
		Status:        p.Status,
		ExpiresAt:     akTimestamptz(p.ExpiresAt),
		CreatedBy:     akText(p.CreatedBy),
	})
	if err != nil {
		return domain.APIKey{}, err
	}
	return domain.APIKey{
		ID:                 uuidToString(row.ID),
		OwnerType:          domain.OwnerType(row.OwnerType),
		TenantID:           row.TenantID,
		UserID:             row.UserID.String,
		LastFour:           row.LastFour.String,
		Name:               row.Name,
		QuotaLimitMicro:    akInt8Ptr(row.QuotaLimit),
		QuotaUsedMicro:     row.QuotaUsed,
		QuotaReservedMicro: row.QuotaReserved,
		AllowedModels:      akDecodeModels(row.AllowedModels),
		Status:             row.Status,
		ExpiresAt:          akTimePtr(row.ExpiresAt),
		LastUsedAt:         akTimePtr(row.LastUsedAt),
		CreatedBy:          row.CreatedBy.String,
		CreatedAt:          row.CreatedAt.Time,
		UpdatedAt:          row.UpdatedAt.Time,
	}, nil
}

func (r *APIKeyRepo) List(ctx context.Context, f svcapikey.ListFilter) ([]domain.APIKey, error) {
	params := dbgen.ListAPIKeysParams{TenantID: f.TenantID}
	if f.OwnerType != "" {
		params.OwnerType = pgtype.Text{String: string(f.OwnerType), Valid: true}
	}
	if f.UserID != "" {
		params.UserID = pgtype.Text{String: f.UserID, Valid: true}
	}
	rows, err := r.q.ListAPIKeys(ctx, params)
	if err != nil {
		return nil, err
	}
	out := make([]domain.APIKey, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.APIKey{
			ID:                 uuidToString(row.ID),
			OwnerType:          domain.OwnerType(row.OwnerType),
			TenantID:           row.TenantID,
			UserID:             row.UserID.String,
			LastFour:           row.LastFour.String,
			Name:               row.Name,
			QuotaLimitMicro:    akInt8Ptr(row.QuotaLimit),
			QuotaUsedMicro:     row.QuotaUsed,
			QuotaReservedMicro: row.QuotaReserved,
			AllowedModels:      akDecodeModels(row.AllowedModels),
			Status:             row.Status,
			ExpiresAt:          akTimePtr(row.ExpiresAt),
			LastUsedAt:         akTimePtr(row.LastUsedAt),
			CreatedBy:          row.CreatedBy.String,
			CreatedAt:          row.CreatedAt.Time,
			UpdatedAt:          row.UpdatedAt.Time,
		})
	}
	return out, nil
}

func (r *APIKeyRepo) Update(ctx context.Context, p svcapikey.UpdateParams) (domain.APIKey, string, error) {
	id, err := akUUID(p.ID)
	if err != nil {
		return domain.APIKey{}, "", err
	}
	row, err := r.q.UpdateAPIKey(ctx, dbgen.UpdateAPIKeyParams{
		ID:            id,
		TenantID:      p.TenantID,
		Name:          p.Name,
		QuotaLimit:    akInt8(p.QuotaLimitMicro),
		AllowedModels: akModelsJSON(p.AllowedModels),
		Status:        p.Status,
		ExpiresAt:     akTimestamptz(p.ExpiresAt),
	})
	if err != nil {
		return domain.APIKey{}, "", err
	}
	return domain.APIKey{
		ID:                 uuidToString(row.ID),
		OwnerType:          domain.OwnerType(row.OwnerType),
		TenantID:           row.TenantID,
		UserID:             row.UserID.String,
		LastFour:           row.LastFour.String,
		Name:               row.Name,
		QuotaLimitMicro:    akInt8Ptr(row.QuotaLimit),
		QuotaUsedMicro:     row.QuotaUsed,
		QuotaReservedMicro: row.QuotaReserved,
		AllowedModels:      akDecodeModels(row.AllowedModels),
		Status:             row.Status,
		ExpiresAt:          akTimePtr(row.ExpiresAt),
		LastUsedAt:         akTimePtr(row.LastUsedAt),
		CreatedBy:          row.CreatedBy.String,
		CreatedAt:          row.CreatedAt.Time,
		UpdatedAt:          row.UpdatedAt.Time,
	}, row.KeyHash, nil
}

func (r *APIKeyRepo) UpdateStatus(ctx context.Context, id, tenantID, status string) (domain.APIKey, string, error) {
	uid, err := akUUID(id)
	if err != nil {
		return domain.APIKey{}, "", err
	}
	row, err := r.q.UpdateAPIKeyStatus(ctx, dbgen.UpdateAPIKeyStatusParams{
		ID:       uid,
		TenantID: tenantID,
		Status:   status,
	})
	if err != nil {
		return domain.APIKey{}, "", err
	}
	return domain.APIKey{
		ID:                 uuidToString(row.ID),
		OwnerType:          domain.OwnerType(row.OwnerType),
		TenantID:           row.TenantID,
		UserID:             row.UserID.String,
		LastFour:           row.LastFour.String,
		Name:               row.Name,
		QuotaLimitMicro:    akInt8Ptr(row.QuotaLimit),
		QuotaUsedMicro:     row.QuotaUsed,
		QuotaReservedMicro: row.QuotaReserved,
		AllowedModels:      akDecodeModels(row.AllowedModels),
		Status:             row.Status,
		ExpiresAt:          akTimePtr(row.ExpiresAt),
		LastUsedAt:         akTimePtr(row.LastUsedAt),
		CreatedBy:          row.CreatedBy.String,
		CreatedAt:          row.CreatedAt.Time,
		UpdatedAt:          row.UpdatedAt.Time,
	}, row.KeyHash, nil
}

func (r *APIKeyRepo) Rotate(ctx context.Context, p svcapikey.RotateParams) (domain.APIKey, string, error) {
	uid, err := akUUID(p.ID)
	if err != nil {
		return domain.APIKey{}, "", err
	}
	// Capture the previous hash first so the service can invalidate caches
	// keyed by the OLD secret (the rotate query returns only the new hash).
	prev, err := r.q.GetAPIKeyByID(ctx, dbgen.GetAPIKeyByIDParams{ID: uid, TenantID: p.TenantID})
	if err != nil {
		return domain.APIKey{}, "", err
	}
	row, err := r.q.RotateAPIKey(ctx, dbgen.RotateAPIKeyParams{
		ID:       uid,
		TenantID: p.TenantID,
		KeyHash:  p.KeyHash,
		LastFour: pgtype.Text{String: p.LastFour, Valid: true},
	})
	if err != nil {
		return domain.APIKey{}, "", err
	}
	return domain.APIKey{
		ID:                 uuidToString(row.ID),
		OwnerType:          domain.OwnerType(row.OwnerType),
		TenantID:           row.TenantID,
		UserID:             row.UserID.String,
		LastFour:           row.LastFour.String,
		Name:               row.Name,
		QuotaLimitMicro:    akInt8Ptr(row.QuotaLimit),
		QuotaUsedMicro:     row.QuotaUsed,
		QuotaReservedMicro: row.QuotaReserved,
		AllowedModels:      akDecodeModels(row.AllowedModels),
		Status:             row.Status,
		ExpiresAt:          akTimePtr(row.ExpiresAt),
		LastUsedAt:         akTimePtr(row.LastUsedAt),
		CreatedBy:          row.CreatedBy.String,
		CreatedAt:          row.CreatedAt.Time,
		UpdatedAt:          row.UpdatedAt.Time,
	}, prev.KeyHash, nil
}

func (r *APIKeyRepo) Delete(ctx context.Context, id, tenantID string) (string, error) {
	uid, err := akUUID(id)
	if err != nil {
		return "", err
	}
	return r.q.DeleteAPIKey(ctx, dbgen.DeleteAPIKeyParams{ID: uid, TenantID: tenantID})
}

// ---- apikey-local pgtype <-> domain conversion helpers (ak* prefix avoids
// collisions with other adapter files in this package) ----

func akText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func akInt8(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

func akInt8Ptr(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	n := v.Int64
	return &n
}

func akTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func akTimePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	tt := t.Time
	return &tt
}

func akModelsJSON(models []string) []byte {
	b, _ := json.Marshal(models)
	return b
}

func akDecodeModels(b []byte) []string {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	var m []string
	_ = json.Unmarshal(b, &m)
	return m
}

func akUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return pgtype.UUID{}, err
	}
	return u, nil
}
