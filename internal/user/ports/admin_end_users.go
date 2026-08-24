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

// AdminEndUserUpdate describes the optional profile fields that may be
// changed by a tenant-scoped management request. The set flags distinguish an
// omitted field from an explicit clear.
type AdminEndUserUpdate struct {
	UserID          string
	TenantID        string
	EmailSet        bool
	Email           string
	PhoneSet        bool
	Phone           string
	InternalNoteSet bool
	InternalNote    string
}

// AdminEndUserCreate contains the already-normalized account fields and the
// one-time credential material prepared by the authentication application
// service. The repository persists both account and activation records in one
// transaction.
type AdminEndUserCreate struct {
	UserID              string
	TenantID            string
	Username            string
	Email               *string
	Phone               *string
	InternalNote        string
	PasswordHash        string
	ActivationTokenHash []byte
	ActivationExpiresAt time.Time
}

// AdminEndUserWriter owns end-user profile and status mutations. Session
// blacklist side effects remain at the application/HTTP orchestration layer.
type AdminEndUserWriter interface {
	CreateEndUser(ctx context.Context, input AdminEndUserCreate) error
	UpdateEndUser(ctx context.Context, input AdminEndUserUpdate) (bool, error)
	UpdateEndUserStatus(ctx context.Context, userID, status string) (bool, error)
	ResetEndUserPassword(ctx context.Context, userID string) (ActivationCredentialResult, error)
}
