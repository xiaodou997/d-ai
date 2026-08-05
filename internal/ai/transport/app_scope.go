package transport

import (
	"context"
	"strings"

	pgadapter "xiaodou/dai/internal/ai/adapters/postgres"
	"xiaodou/dai/internal/ai/domain"
)

const (
	transportAppOwnerTenant         = "tenant"
	transportAppOwnerUser           = "user"
	transportAppCapabilityChat      = "chat"
	transportAppCapabilityImageGen  = "image_generation"
	transportAppCapabilityImageEdit = "image_edit"
)

type appScope struct {
	OwnerType     string
	OwnerTenantID string
	OwnerUserID   string
}

func tenantAppScope(ctx context.Context) appScope {
	return appScope{
		OwnerType:     transportAppOwnerTenant,
		OwnerTenantID: strings.TrimSpace(tenantIDFromContext(ctx)),
	}
}

func userAppScope(ctx context.Context) appScope {
	return appScope{
		OwnerType:     transportAppOwnerUser,
		OwnerTenantID: strings.TrimSpace(tenantIDFromContext(ctx)),
		OwnerUserID:   strings.TrimSpace(userIDFromContext(ctx)),
	}
}

func (s appScope) owner() pgadapter.AppOwner {
	return pgadapter.AppOwner{Type: s.OwnerType, TenantID: s.OwnerTenantID, UserID: s.OwnerUserID}
}

func validateAppScope(scope appScope) error {
	switch scope.OwnerType {
	case transportAppOwnerTenant:
		if strings.TrimSpace(scope.OwnerTenantID) == "" {
			return domain.NewValidationError("owner_tenant_id", "tenant scope requires tenant id")
		}
		if scope.OwnerUserID != "" {
			return domain.NewValidationError("owner_user_id", "tenant scope must not carry user id")
		}
	case transportAppOwnerUser:
		if strings.TrimSpace(scope.OwnerTenantID) == "" || strings.TrimSpace(scope.OwnerUserID) == "" {
			return domain.NewValidationError("owner_user_id", "user scope requires tenant id and user id")
		}
	default:
		return domain.NewValidationError("owner_type", "must be tenant or user")
	}
	return nil
}
