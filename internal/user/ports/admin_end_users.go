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

// AdminEndUserStatusUpdate carries the actor tenant scope into a status
// mutation. An empty TenantID is the explicit global scope used by platform
// administrators; a non-empty value makes the write tenant-scoped in SQL.
type AdminEndUserStatusUpdate struct {
	UserID   string
	TenantID string
	Status   string
}

// AdminEndUserPasswordReset carries the actor tenant scope into a password
// reset. The repository treats a cross-tenant target as not found so object
// ownership is not enumerable through the management API.
type AdminEndUserPasswordReset struct {
	UserID   string
	TenantID string
}

// AdminEndUserDeleteResult describes the atomic deletion decision. A found
// account with a non-zero balance is intentionally not deleted.
type AdminEndUserDeleteResult struct {
	Found           bool
	Deleted         bool
	BalanceMicroUSD int64
}

// AdminEndUserDeleteGuard is invoked after the account and balance rows are
// locked but before the soft-delete update is committed.
type AdminEndUserDeleteGuard func(ctx context.Context, userID string) error

// AdminEndUserDeleteGuardError marks an external safety guard failure so the
// HTTP boundary can return an availability error while the DB transaction is
// rolled back.
type AdminEndUserDeleteGuardError struct {
	Cause error
}

func (e *AdminEndUserDeleteGuardError) Error() string { return "end-user deletion guard failed" }
func (e *AdminEndUserDeleteGuardError) Unwrap() error { return e.Cause }

// AdminEndUserDeleteCommand carries the actor tenant scope and the optional
// pre-commit security guard into one atomic deletion command.
type AdminEndUserDeleteCommand struct {
	UserID       string
	TenantID     string
	BeforeCommit AdminEndUserDeleteGuard
}

// AdminEndUserSecurityError marks a committed end-user mutation whose
// post-commit security projection failed and can be retried independently.
type AdminEndUserSecurityError struct {
	Cause error
}

func (e *AdminEndUserSecurityError) Error() string { return "admin end-user security sync failed" }
func (e *AdminEndUserSecurityError) Unwrap() error { return e.Cause }

// AdminEndUserWriter owns end-user profile and status mutations. Session and
// blacklist effects are coordinated by the separate auth security command.
type AdminEndUserWriter interface {
	CreateEndUser(ctx context.Context, input AdminEndUserCreate) error
	UpdateEndUser(ctx context.Context, input AdminEndUserUpdate) (bool, error)
	UpdateEndUserStatus(ctx context.Context, input AdminEndUserStatusUpdate) (bool, error)
	ResetEndUserPassword(ctx context.Context, input AdminEndUserPasswordReset) (ActivationCredentialResult, error)
	DeleteEndUser(ctx context.Context, input AdminEndUserDeleteCommand) (AdminEndUserDeleteResult, error)
}

// AdminEndUserLifecycle coordinates persistence and account security effects
// for end-user status, password, and deletion commands.
type AdminEndUserLifecycle interface {
	UpdateEndUser(ctx context.Context, input AdminEndUserUpdate) (bool, error)
	UpdateEndUserStatus(ctx context.Context, input AdminEndUserStatusUpdate) (bool, error)
	ResetEndUserPassword(ctx context.Context, input AdminEndUserPasswordReset) (ActivationCredentialResult, error)
	DeleteEndUser(ctx context.Context, input AdminEndUserDeleteCommand) (AdminEndUserDeleteResult, error)
}
