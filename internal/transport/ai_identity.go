package transport

import (
	"context"
	"errors"

	aitransport "xiaodou/dai/internal/ai/transport"
	tenantports "xiaodou/dai/internal/tenant/ports"
	userports "xiaodou/dai/internal/user/ports"
)

// aiIdentityAdapter exposes the unified identity domain to AI handlers without
// another transport protocol or a duplicate identity model.
type aiIdentityAdapter struct {
	users   userports.IdentityUserReader
	tenants tenantports.AdminTenantReader
}

var _ aitransport.IdentityProvider = (*aiIdentityAdapter)(nil)
var _ aitransport.TenantEndUserVerifier = (*aiIdentityAdapter)(nil)

func newAIIdentityAdapter(tenants tenantports.AdminTenantReader, users userports.IdentityUserReader) *aiIdentityAdapter {
	if tenants == nil || users == nil {
		return nil
	}
	return &aiIdentityAdapter{users: users, tenants: tenants}
}

func (a *aiIdentityAdapter) BatchGetUsers(ctx context.Context, userIDs []string) (map[string]*aitransport.IdentityUser, error) {
	users, err := a.users.BatchGetIdentityUsers(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	result := make(map[string]*aitransport.IdentityUser, len(users))
	for id, user := range users {
		result[id] = &aitransport.IdentityUser{
			UserID: user.UserID, TenantID: user.TenantID, Username: user.Username,
			Email: user.Email, Nickname: user.Nickname, Avatar: user.Avatar,
		}
	}
	return result, nil
}

func (a *aiIdentityAdapter) BatchGetTenants(ctx context.Context, tenantIDs []string) (map[string]*aitransport.IdentityTenant, error) {
	tenants, err := a.tenants.GetByTenantIDs(ctx, tenantIDs)
	if err != nil {
		return nil, err
	}
	result := make(map[string]*aitransport.IdentityTenant, len(tenants))
	for _, tenant := range tenants {
		result[tenant.TenantID] = &aitransport.IdentityTenant{
			TenantID: tenant.TenantID, TenantName: tenant.TenantName,
		}
	}
	return result, nil
}

func (a *aiIdentityAdapter) CheckTenantEndUser(ctx context.Context, tenantID, userID string) error {
	ownedTenantID, err := a.tenants.GetEndUserTenantID(ctx, userID)
	if err != nil {
		if errors.Is(err, tenantports.ErrTenantEndUserNotFound) {
			return aitransport.ErrEndUserNotFound
		}
		return err
	}
	if ownedTenantID != tenantID {
		return aitransport.ErrEndUserNotFound
	}
	return nil
}
