package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
)

func lockDirectAccountStatus(ctx context.Context, db dbgen.DBTX, accountID string) (string, error) {
	var status string
	if err := db.QueryRow(ctx, `
		SELECT status
		FROM ai_upstream_accounts
		WHERE id = $1::uuid
		FOR UPDATE
	`, accountID).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, domain.ErrNotFound) {
			return "", domain.ErrNotFound
		}
		return "", err
	}
	return status, nil
}

func lockOAuthPoolState(ctx context.Context, db dbgen.DBTX, poolID string) (string, domain.FixedProviderType, error) {
	var status string
	var providerType domain.FixedProviderType
	if err := db.QueryRow(ctx, `
		SELECT status, fixed_provider_type
		FROM ai_credential_pools
		WHERE id = $1::uuid
		FOR UPDATE
	`, poolID).Scan(&status, &providerType); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, domain.ErrNotFound) {
			return "", "", domain.ErrNotFound
		}
		return "", "", err
	}
	return status, providerType, nil
}

func loadActiveDirectProtocols(ctx context.Context, db dbgen.DBTX, accountID string) ([]domain.UpstreamProtocol, error) {
	rows, err := db.Query(ctx, `
		SELECT api_format
		FROM ai_upstream_account_endpoints
		WHERE account_id = $1::uuid AND status = 'active'
		ORDER BY api_format
	`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	protocols := make([]domain.UpstreamProtocol, 0)
	for rows.Next() {
		var protocol domain.UpstreamProtocol
		if err := rows.Scan(&protocol); err != nil {
			return nil, err
		}
		protocols = append(protocols, protocol)
	}
	return protocols, rows.Err()
}

func validateActiveModelBindingsAgainstProtocols(
	ctx context.Context,
	db dbgen.DBTX,
	kind domain.UpstreamKind,
	targetID string,
	protocols []domain.UpstreamProtocol,
) error {
	rows, err := db.Query(ctx, `
		SELECT DISTINCT capability_type
		FROM ai_upstream_models
		WHERE upstream_kind = $1 AND upstream_id = $2::uuid AND status = 'active'
		ORDER BY capability_type
	`, string(kind), targetID)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var capability domain.CapabilityType
		if err := rows.Scan(&capability); err != nil {
			return err
		}
		if !domain.AnyProtocolSupportsCapability(protocols, capability) {
			return domain.NewValidationError(
				"model_bindings",
				fmt.Sprintf("active target has no request protocol compatible with active %s model bindings", capability),
			)
		}
	}
	return rows.Err()
}

func validateActiveDirectAccountConfiguration(ctx context.Context, db dbgen.DBTX, accountID string) error {
	protocols, err := loadActiveDirectProtocols(ctx, db, accountID)
	if err != nil {
		return err
	}
	if len(protocols) == 0 {
		return domain.NewValidationError("status", "active account must keep at least one active endpoint")
	}
	return validateActiveModelBindingsAgainstProtocols(ctx, db, domain.UpstreamKindDirect, accountID, protocols)
}

func validateActiveOAuthPoolConfiguration(
	ctx context.Context,
	db dbgen.DBTX,
	poolID string,
	providerType domain.FixedProviderType,
) error {
	return validateActiveModelBindingsAgainstProtocols(
		ctx,
		db,
		domain.UpstreamKindPool,
		poolID,
		[]domain.UpstreamProtocol{domain.FixedProviderProtocol(providerType)},
	)
}

func validateActiveModelBindingWrite(
	ctx context.Context,
	db dbgen.DBTX,
	scope domain.UpstreamModelBindingScope,
	write domain.UpstreamModelBindingWrite,
) error {
	switch scope.Kind {
	case domain.UpstreamKindDirect:
		status, err := lockDirectAccountStatus(ctx, db, scope.ID)
		if err != nil {
			return err
		}
		if status != domain.UpstreamAccountStatusActive || write.Status != "active" {
			return nil
		}
		protocols, err := loadActiveDirectProtocols(ctx, db, scope.ID)
		if err != nil {
			return err
		}
		if !domain.AnyProtocolSupportsCapability(protocols, domain.CapabilityType(write.CapabilityType)) {
			return domain.NewValidationError("capability_type", "active account has no compatible active request endpoint")
		}
		return nil

	case domain.UpstreamKindPool:
		status, providerType, err := lockOAuthPoolState(ctx, db, scope.ID)
		if err != nil {
			return err
		}
		if status != "active" || write.Status != "active" {
			return nil
		}
		if !domain.ProtocolSupportsCapability(domain.FixedProviderProtocol(providerType), domain.CapabilityType(write.CapabilityType)) {
			return domain.NewValidationError("capability_type", "fixed-provider pool does not support this model capability")
		}
		return nil

	default:
		return domain.NewValidationError("upstream_kind", "unsupported upstream kind")
	}
}
