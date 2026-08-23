package ports

import (
	"context"
	"time"
)

// AdminEndUserListFilter is the application-facing scope and filter contract
// for admin end-user lists. Status uses the persisted string representation.
type AdminEndUserListFilter struct {
	TenantID   string
	TenantName string
	Username   string
	Keyword    string
	Status     string
	Page       int
	Size       int
}

// AdminEndUserRow is the non-secret end-user management projection.
type AdminEndUserRow struct {
	UserID          string
	TenantID        string
	Username        string
	TenantName      *string
	Email           *string
	Phone           *string
	InternalNote    string
	Nickname        *string
	Avatar          *string
	Status          string
	CredentialState string
	BalanceMicroUSD int64
	LastLoginAt     *time.Time
	CreatedAt       time.Time
}

// AdminEndUserPage is the paginated end-user management projection.
type AdminEndUserPage struct {
	Records []AdminEndUserRow
	Total   int64
	Page    int
	Size    int
}

// AdminEndUserReader exposes the scoped read capability required by the
// management handler without exposing SQL or persistence types.
type AdminEndUserReader interface {
	ListEndUsers(ctx context.Context, filter AdminEndUserListFilter) (AdminEndUserPage, error)
}
