// Package postgres provides adapters that bridge the serving pipeline interfaces
// to the generated sqlc query layer.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	dbgen "xiaodou/uni-ai-api/internal/db/gen"
	"xiaodou/uni-ai-api/internal/domain"
	"xiaodou/uni-ai-api/internal/serving"
)

// APIKeyResolver resolves API keys from the database.
// It satisfies serving.APIKeyResolver.
type APIKeyResolver struct {
	q *dbgen.Queries
}

func NewAPIKeyResolver(q *dbgen.Queries) *APIKeyResolver {
	return &APIKeyResolver{q: q}
}

// ResolveAPIKey looks up an API key by the raw bearer token and validates it.
// It populates req.APIKey.
func (r *APIKeyResolver) ResolveAPIKey(ctx context.Context, token string, req *serving.Request) error {
	keyHash := hashAPIKey(token)

	row, err := r.q.GetAPIKeyByHash(ctx, keyHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("api key not found")
		}
		return fmt.Errorf("lookup api key: %w", err)
	}

	if row.Status != "active" {
		return fmt.Errorf("api key not active")
	}
	if row.ExpiresAt.Valid && !row.ExpiresAt.Time.After(nowFunc()) {
		return fmt.Errorf("api key expired")
	}

	var allowedModels []string
	if len(row.AllowedModels) > 0 && string(row.AllowedModels) != "null" {
		if err := json.Unmarshal(row.AllowedModels, &allowedModels); err != nil {
			return fmt.Errorf("parse allowed_models: %w", err)
		}
	}

	var quotaLimit *int64
	if row.QuotaLimit.Valid {
		v := row.QuotaLimit.Int64
		quotaLimit = &v
	}

	userID := ""
	if row.UserID.Valid {
		userID = row.UserID.String
	}

	key := domain.APIKeyAuth{
		KeyID:         uuidToString(row.ID),
		OwnerType:     domain.OwnerType(row.OwnerType),
		TenantID:      row.TenantID,
		UserID:        userID,
		AllowedModels: allowedModels,
		QuotaLimit:    quotaLimit,
		QuotaUsed:     row.QuotaUsed,
		QuotaReserved: row.QuotaReserved,
	}
	req.APIKey = &key
	req.Identity = domain.IdentityFromAPIKey(key)
	return nil
}

// nowFunc is a variable to allow test overrides.
var nowFunc = func() time.Time { return time.Now() }
