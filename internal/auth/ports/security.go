package ports

import (
	"context"
	"time"
)

// AccountSecurityWriter owns the non-persistence security effects that must
// follow account and tenant lifecycle changes. HTTP handlers pass commands to
// this port instead of coordinating token and ban state against Redis.
type AccountSecurityWriter interface {
	IsEnabled() bool
	BanUser(ctx context.Context, userID string) error
	RevokeAccessToken(ctx context.Context, tokenID string, expiration time.Duration) error
	InvalidateUserSessions(ctx context.Context, userID string) error
	SyncUserStatus(ctx context.Context, userID, status string) error
	SyncTenantStatus(ctx context.Context, tenantID, status string, restoredUserIDs []string) error
}
