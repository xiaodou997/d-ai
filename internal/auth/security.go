package auth

import (
	"context"
	"errors"
	"time"

	authports "xiaodou/dai/internal/auth/ports"
)

// AccountSecurityService coordinates Redis-backed token and ban state after
// an account or tenant mutation has committed. Persistence adapters remain
// responsible for database state; this service is the application command
// boundary for the related security effects.
type AccountSecurityService struct {
	blacklist *BlacklistService
}

var _ authports.AccountSecurityWriter = (*AccountSecurityService)(nil)

func NewAccountSecurityService(blacklist *BlacklistService) *AccountSecurityService {
	return &AccountSecurityService{blacklist: blacklist}
}

// IsEnabled reports whether the security store can enforce a ban. Destructive
// account deletion uses this as a guard because it cannot safely continue if
// the post-lock ban callback has nowhere to write.
func (s *AccountSecurityService) IsEnabled() bool {
	return s != nil && s.blacklist != nil && s.blacklist.IsEnabled()
}

// BanUser enforces a user ban for operations that must not commit without an
// immediately visible deny marker (for example, account deletion).
func (s *AccountSecurityService) BanUser(ctx context.Context, userID string) error {
	if !s.IsEnabled() {
		return errors.New("account security ban service is unavailable")
	}
	return s.blacklist.BanUser(ctx, userID)
}

// RevokeAccessToken invalidates one access token for its remaining lifetime.
func (s *AccountSecurityService) RevokeAccessToken(ctx context.Context, tokenID string, expiration time.Duration) error {
	if s == nil || s.blacklist == nil {
		return nil
	}
	return s.blacklist.AddToBlacklist(ctx, tokenID, expiration)
}

// InvalidateUserSessions invalidates all access tokens issued before now.
func (s *AccountSecurityService) InvalidateUserSessions(ctx context.Context, userID string) error {
	if s == nil || s.blacklist == nil {
		return nil
	}
	return s.blacklist.LogoutUser(ctx, userID)
}

// SyncUserStatus mirrors the persisted account status into the ban key. A
// disabled/inherited/deleted account is fail-closed; active accounts are
// explicitly unbanned so re-enabling is immediate on every replica.
func (s *AccountSecurityService) SyncUserStatus(ctx context.Context, userID, status string) error {
	if s == nil || s.blacklist == nil {
		return nil
	}
	if status == "disabled" || status == "inherited_disabled" || status == "deleted" {
		return s.blacklist.BanUser(ctx, userID)
	}
	return s.blacklist.UnbanUser(ctx, userID)
}

// SyncTenantStatus mirrors a tenant status transition and clears inherited
// user bans only for the accounts the tenant transaction actually restored.
func (s *AccountSecurityService) SyncTenantStatus(ctx context.Context, tenantID, status string, restoredUserIDs []string) error {
	if s == nil || s.blacklist == nil {
		return nil
	}
	if status == "disabled" {
		return s.blacklist.BanTenant(ctx, tenantID)
	}
	if err := s.blacklist.UnbanTenant(ctx, tenantID); err != nil {
		return err
	}
	for _, userID := range restoredUserIDs {
		if err := s.blacklist.UnbanUser(ctx, userID); err != nil {
			return err
		}
	}
	return nil
}
