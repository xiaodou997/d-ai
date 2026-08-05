// Package postgres provides adapters that bridge the serving pipeline interfaces
// to the generated sqlc query layer.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
	dbgen "xiaodou/dai/internal/ai/db/gen"
)

// APIKeyResolver resolves API keys from the database.
// It satisfies serving.APIKeyResolver.
type APIKeyResolver struct {
	q *dbgen.Queries
}

func NewAPIKeyResolver(q *dbgen.Queries) *APIKeyResolver {
	return &APIKeyResolver{q: q}
}

// ResolveSubject looks up an API key by the raw bearer token and returns the
// canonical runtime subject for serving.
func (r *APIKeyResolver) ResolveSubject(ctx context.Context, token string) (coreidentity.Subject, error) {
	keyHash := hashAPIKey(token)

	row, err := r.q.GetAPIKeyByHash(ctx, keyHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return coreidentity.Subject{}, fmt.Errorf("api key not found")
		}
		return coreidentity.Subject{}, fmt.Errorf("lookup api key: %w", err)
	}

	if row.Status != "active" {
		return coreidentity.Subject{}, fmt.Errorf("api key not active")
	}
	if row.ExpiresAt.Valid && !row.ExpiresAt.Time.After(nowFunc()) {
		return coreidentity.Subject{}, fmt.Errorf("api key expired")
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

	scope := coreidentity.ScopeTenant
	switch row.OwnerType {
	case "user":
		scope = coreidentity.ScopeUser
	case "tenant":
		scope = coreidentity.ScopeTenant
	}

	return coreidentity.Subject{
		AuthMethod:    coreidentity.AuthMethodAPIKey,
		RequestSource: coreidentity.RequestSourceAPIKey,
		Scope:         scope,
		TenantID:      row.TenantID,
		UserID:        userID,
		APIKeyID:      uuidToString(row.ID),
		GroupID:       uuidToString(row.GroupID),
		QuotaLimit:    quotaLimit,
		QuotaUsed:     row.QuotaUsed,
	}, nil
}

// nowFunc is a variable to allow test overrides.
var nowFunc = func() time.Time { return time.Now() }
