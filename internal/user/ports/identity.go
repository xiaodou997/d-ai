package ports

import "context"

// IdentityUser is the non-secret user projection used by cross-domain
// identity enrichment. Persistence-specific status and timestamps stay out
// of this contract.
type IdentityUser struct {
	UserID   string
	TenantID string
	Username string
	Email    *string
	Nickname *string
	Avatar   *string
}

// IdentityUserReader exposes the batch lookup required by downstream
// identity adapters without leaking the user PostgreSQL model.
type IdentityUserReader interface {
	BatchGetIdentityUsers(ctx context.Context, userIDs []string) (map[string]IdentityUser, error)
}
