package gateway

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"xiaodou/dai/internal/ai/asynctask"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	dbgen "xiaodou/dai/internal/ai/db/gen"
)

type taskSubjectResolver struct {
	queries *dbgen.Queries
}

// NewTaskSubjectResolver builds the credential resolver used by async workers.
// It stores no credential snapshot: every attempt reloads the current key.
func NewTaskSubjectResolver(queries *dbgen.Queries) asynctask.SubjectResolver {
	return &taskSubjectResolver{queries: queries}
}

func (r *taskSubjectResolver) Resolve(ctx context.Context, ref asynctask.SubjectRef) (coreidentity.Subject, error) {
	switch ref.AuthMethod {
	case coreidentity.AuthMethodAPIKey:
		if r == nil || r.queries == nil {
			return coreidentity.Subject{}, fmt.Errorf("API key task subject resolver is not configured")
		}
		var keyID pgtype.UUID
		if err := keyID.Scan(ref.APIKeyID); err != nil || !keyID.Valid {
			return coreidentity.Subject{}, fmt.Errorf("invalid API key id %q", ref.APIKeyID)
		}
		row, err := r.queries.GetAPIKeyByID(ctx, dbgen.GetAPIKeyByIDParams{ID: keyID, TenantID: ref.TenantID})
		if err != nil {
			return coreidentity.Subject{}, fmt.Errorf("load API key: %w", err)
		}
		if row.Status != "active" {
			return coreidentity.Subject{}, fmt.Errorf("API key is not active")
		}
		if row.ExpiresAt.Valid && !time.Now().Before(row.ExpiresAt.Time) {
			return coreidentity.Subject{}, fmt.Errorf("API key is expired")
		}
		subject, err := buildRuntimeAuthSubjectRecord(runtimeAPIKeyRecordFromModel(row))
		if err != nil {
			return coreidentity.Subject{}, err
		}
		if subject.GroupID == "" {
			return coreidentity.Subject{}, fmt.Errorf("API key has no bound group")
		}
		return subject, nil
	case coreidentity.AuthMethodJWT:
		scope := coreidentity.ScopeTenant
		if ref.UserID != "" {
			scope = coreidentity.ScopeUser
		}
		return coreidentity.Subject{
			AuthMethod:    coreidentity.AuthMethodJWT,
			RequestSource: coreidentity.RequestSourceWebImage,
			Scope:         scope,
			TenantID:      ref.TenantID,
			UserID:        ref.UserID,
		}, nil
	default:
		return coreidentity.Subject{}, fmt.Errorf("unsupported async task auth method %q", ref.AuthMethod)
	}
}

type runtimeAPIKeyRecord struct {
	ID         pgtype.UUID
	OwnerType  string
	TenantID   string
	UserID     pgtype.Text
	GroupID    pgtype.UUID
	QuotaLimit pgtype.Int8
	QuotaUsed  int64
}

func runtimeAPIKeyRecordFromHashRow(row dbgen.GetAPIKeyByHashRow) runtimeAPIKeyRecord {
	return runtimeAPIKeyRecord{
		ID: row.ID, OwnerType: row.OwnerType, TenantID: row.TenantID, UserID: row.UserID, GroupID: row.GroupID,
		QuotaLimit: row.QuotaLimit, QuotaUsed: row.QuotaUsed,
	}
}

func runtimeAPIKeyRecordFromModel(row dbgen.AiApiKey) runtimeAPIKeyRecord {
	return runtimeAPIKeyRecord{
		ID: row.ID, OwnerType: row.OwnerType, TenantID: row.TenantID, UserID: row.UserID, GroupID: row.GroupID,
		QuotaLimit: row.QuotaLimit, QuotaUsed: row.QuotaUsed,
	}
}

func buildRuntimeAuthSubjectRecord(row runtimeAPIKeyRecord) (coreidentity.Subject, error) {
	var quotaLimit *int64
	if row.QuotaLimit.Valid {
		value := row.QuotaLimit.Int64
		quotaLimit = &value
	}
	userID := ""
	if row.UserID.Valid {
		userID = row.UserID.String
	}
	scope := coreidentity.ScopeTenant
	if row.OwnerType == "user" {
		scope = coreidentity.ScopeUser
	}
	return coreidentity.Subject{
		AuthMethod:    coreidentity.AuthMethodAPIKey,
		RequestSource: coreidentity.RequestSourceAPIKey,
		Scope:         scope,
		TenantID:      row.TenantID,
		UserID:        userID,
		APIKeyID:      runtimeAuthUUIDString(row.ID),
		GroupID:       runtimeAuthUUIDString(row.GroupID),
		QuotaLimit:    quotaLimit,
		QuotaUsed:     row.QuotaUsed,
	}, nil
}
