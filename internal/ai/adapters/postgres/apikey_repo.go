package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
)

// APIKeyRepo implements core/identity.APIKeyRepository on top of the sqlc query layer.
type APIKeyRepo struct {
	q *dbgen.Queries
}

func NewAPIKeyRepo(q *dbgen.Queries) *APIKeyRepo {
	return &APIKeyRepo{q: q}
}

var _ coreidentity.APIKeyRepository = (*APIKeyRepo)(nil)

func (r *APIKeyRepo) Create(ctx context.Context, p coreidentity.APIKeyCreate) (coreidentity.APIKey, error) {
	groupID, err := akUUID(p.GroupID)
	if err != nil {
		return coreidentity.APIKey{}, err
	}
	row, err := r.q.CreateAPIKey(ctx, dbgen.CreateAPIKeyParams{
		OwnerType:     string(ownerScopeToLegacy(p.OwnerScope)),
		TenantID:      p.TenantID,
		UserID:        akText(p.UserID),
		GroupID:       groupID,
		KeyHash:       p.KeyHash,
		KeyCiphertext: p.KeyCiphertext,
		LastFour:      akText(p.LastFour),
		Name:          p.Name,
		QuotaLimit:    akInt8(p.QuotaLimitMicro),
		Status:        p.Status,
		ExpiresAt:     akTimestamptz(p.ExpiresAt),
		CreatedBy:     akText(p.CreatedBy),
	})
	if err != nil {
		return coreidentity.APIKey{}, err
	}
	return domain.APIKey{
		ID:              uuidToString(row.ID),
		OwnerType:       domain.OwnerType(row.OwnerType),
		TenantID:        row.TenantID,
		UserID:          row.UserID.String,
		GroupID:         uuidToString(row.GroupID),
		LastFour:        row.LastFour.String,
		Name:            row.Name,
		QuotaLimitMicro: akInt8Ptr(row.QuotaLimit),
		QuotaUsedMicro:  row.QuotaUsed,
		Status:          row.Status,
		ExpiresAt:       akTimePtr(row.ExpiresAt),
		LastUsedAt:      akTimePtr(row.LastUsedAt),
		CreatedBy:       row.CreatedBy.String,
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
	}.ToCore(), nil
}

func (r *APIKeyRepo) List(ctx context.Context, f coreidentity.APIKeyListFilter) ([]coreidentity.APIKey, error) {
	params := dbgen.ListAPIKeysParams{TenantID: f.TenantID}
	if f.OwnerScope != "" {
		params.OwnerType = pgtype.Text{String: string(ownerScopeToLegacy(f.OwnerScope)), Valid: true}
	}
	if f.UserID != "" {
		params.UserID = pgtype.Text{String: f.UserID, Valid: true}
	}
	rows, err := r.q.ListAPIKeys(ctx, params)
	if err != nil {
		return nil, err
	}
	out := make([]coreidentity.APIKey, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.APIKey{
			ID:              uuidToString(row.ID),
			OwnerType:       domain.OwnerType(row.OwnerType),
			TenantID:        row.TenantID,
			UserID:          row.UserID.String,
			GroupID:         uuidToString(row.GroupID),
			LastFour:        row.LastFour.String,
			Name:            row.Name,
			QuotaLimitMicro: akInt8Ptr(row.QuotaLimit),
			QuotaUsedMicro:  row.QuotaUsed,
			Status:          row.Status,
			ExpiresAt:       akTimePtr(row.ExpiresAt),
			LastUsedAt:      akTimePtr(row.LastUsedAt),
			CreatedBy:       row.CreatedBy.String,
			CreatedAt:       row.CreatedAt.Time,
			UpdatedAt:       row.UpdatedAt.Time,
		}.ToCore())
	}
	return out, nil
}

func (r *APIKeyRepo) Update(ctx context.Context, p coreidentity.APIKeyUpdate) (coreidentity.APIKey, string, error) {
	id, err := akUUID(p.ID)
	if err != nil {
		return coreidentity.APIKey{}, "", err
	}
	groupID, err := akUUID(p.GroupID)
	if err != nil {
		return coreidentity.APIKey{}, "", err
	}
	row, err := r.q.UpdateAPIKey(ctx, dbgen.UpdateAPIKeyParams{
		ID:         id,
		TenantID:   p.TenantID,
		GroupID:    groupID,
		Name:       p.Name,
		QuotaLimit: akInt8(p.QuotaLimitMicro),
		Status:     p.Status,
		ExpiresAt:  akTimestamptz(p.ExpiresAt),
	})
	if err != nil {
		return coreidentity.APIKey{}, "", err
	}
	return domain.APIKey{
		ID:              uuidToString(row.ID),
		OwnerType:       domain.OwnerType(row.OwnerType),
		TenantID:        row.TenantID,
		UserID:          row.UserID.String,
		GroupID:         uuidToString(row.GroupID),
		LastFour:        row.LastFour.String,
		Name:            row.Name,
		QuotaLimitMicro: akInt8Ptr(row.QuotaLimit),
		QuotaUsedMicro:  row.QuotaUsed,
		Status:          row.Status,
		ExpiresAt:       akTimePtr(row.ExpiresAt),
		LastUsedAt:      akTimePtr(row.LastUsedAt),
		CreatedBy:       row.CreatedBy.String,
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
	}.ToCore(), row.KeyHash, nil
}

func (r *APIKeyRepo) UpdateStatus(ctx context.Context, id, tenantID, status string) (coreidentity.APIKey, string, error) {
	uid, err := akUUID(id)
	if err != nil {
		return coreidentity.APIKey{}, "", err
	}
	row, err := r.q.UpdateAPIKeyStatus(ctx, dbgen.UpdateAPIKeyStatusParams{
		ID:       uid,
		TenantID: tenantID,
		Status:   status,
	})
	if err != nil {
		return coreidentity.APIKey{}, "", err
	}
	return domain.APIKey{
		ID:              uuidToString(row.ID),
		OwnerType:       domain.OwnerType(row.OwnerType),
		TenantID:        row.TenantID,
		UserID:          row.UserID.String,
		GroupID:         uuidToString(row.GroupID),
		LastFour:        row.LastFour.String,
		Name:            row.Name,
		QuotaLimitMicro: akInt8Ptr(row.QuotaLimit),
		QuotaUsedMicro:  row.QuotaUsed,
		Status:          row.Status,
		ExpiresAt:       akTimePtr(row.ExpiresAt),
		LastUsedAt:      akTimePtr(row.LastUsedAt),
		CreatedBy:       row.CreatedBy.String,
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
	}.ToCore(), row.KeyHash, nil
}

func (r *APIKeyRepo) Rotate(ctx context.Context, p coreidentity.APIKeyRotate) (coreidentity.APIKey, string, error) {
	uid, err := akUUID(p.ID)
	if err != nil {
		return coreidentity.APIKey{}, "", err
	}
	// Capture the previous hash first so the service can invalidate caches
	// keyed by the OLD secret (the rotate query returns only the new hash).
	prev, err := r.q.GetAPIKeyByID(ctx, dbgen.GetAPIKeyByIDParams{ID: uid, TenantID: p.TenantID})
	if err != nil {
		return coreidentity.APIKey{}, "", err
	}
	row, err := r.q.RotateAPIKey(ctx, dbgen.RotateAPIKeyParams{
		ID:            uid,
		TenantID:      p.TenantID,
		KeyHash:       p.KeyHash,
		KeyCiphertext: p.KeyCiphertext,
		LastFour:      pgtype.Text{String: p.LastFour, Valid: true},
	})
	if err != nil {
		return coreidentity.APIKey{}, "", err
	}
	return domain.APIKey{
		ID:              uuidToString(row.ID),
		OwnerType:       domain.OwnerType(row.OwnerType),
		TenantID:        row.TenantID,
		UserID:          row.UserID.String,
		GroupID:         uuidToString(row.GroupID),
		LastFour:        row.LastFour.String,
		Name:            row.Name,
		QuotaLimitMicro: akInt8Ptr(row.QuotaLimit),
		QuotaUsedMicro:  row.QuotaUsed,
		Status:          row.Status,
		ExpiresAt:       akTimePtr(row.ExpiresAt),
		LastUsedAt:      akTimePtr(row.LastUsedAt),
		CreatedBy:       row.CreatedBy.String,
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
	}.ToCore(), prev.KeyHash, nil
}

func (r *APIKeyRepo) Reveal(ctx context.Context, id, tenantID string) (string, error) {
	uid, err := akUUID(id)
	if err != nil {
		return "", err
	}
	return r.q.GetAPIKeySecretByID(ctx, dbgen.GetAPIKeySecretByIDParams{ID: uid, TenantID: tenantID})
}

func (r *APIKeyRepo) Delete(ctx context.Context, id, tenantID string) (string, error) {
	uid, err := akUUID(id)
	if err != nil {
		return "", err
	}
	return r.q.DeleteAPIKey(ctx, dbgen.DeleteAPIKeyParams{ID: uid, TenantID: tenantID})
}

func ownerScopeToLegacy(scope coreidentity.Scope) domain.OwnerType {
	switch scope {
	case coreidentity.ScopeUser:
		return domain.OwnerUser
	case coreidentity.ScopeTenant:
		return domain.OwnerTenant
	default:
		return ""
	}
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

func akUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return pgtype.UUID{}, err
	}
	return u, nil
}
