package ports

import (
	"context"
	"errors"
	"time"
)

var ErrAccountNotFound = errors.New("account not found")
var ErrUsernameTaken = errors.New("username already exists")
var ErrEmailTaken = errors.New("email already exists")

// CurrentUserSnapshot is the non-secret account projection returned by the
// authenticated /me endpoint.
type CurrentUserSnapshot struct {
	UserID     string
	Username   string
	UserType   int
	TenantID   string
	TenantName string
	MFAEnabled bool
	Status     string
}

// ProfileUpdate contains the already validated optional account profile
// fields. Set flags distinguish omission from an explicit clear.
type ProfileUpdate struct {
	UserID      string
	UserType    int
	UsernameSet bool
	Username    string
	EmailSet    bool
	Email       string
}

// AccountReader exposes authenticated account projections without SQL.
type AccountReader interface {
	GetCurrentUserSnapshot(ctx context.Context, userID string, userType int) (CurrentUserSnapshot, error)
	GetPasswordHash(ctx context.Context, userID string, userType int) (string, error)
	CheckTenantActive(ctx context.Context, tenantID string) (bool, error)
}

// AccountWriter owns authenticated account mutations. Session and token
// invalidation remain outside persistence.
type AccountWriter interface {
	UpdatePassword(ctx context.Context, userID string, userType int, passwordHash string) (bool, error)
	UpdateProfile(ctx context.Context, input ProfileUpdate) (bool, error)
}

// LoginAccount is the credential and session projection used by Portal login
// flows. Password material never crosses the HTTP boundary.
type LoginAccount struct {
	UserID            string
	TenantID          string
	Username          string
	PasswordHash      string
	UserType          int
	Status            string
	CredentialVersion int64
	CredentialState   string
	MFAEnabled        bool
}

// LoginReader exposes the account lookups needed by authentication handlers.
type LoginReader interface {
	GetPortalUserForLogin(ctx context.Context, identifier string) (LoginAccount, error)
	LookupTenantForLogin(ctx context.Context, identifier string) string
	CheckTenantActive(ctx context.Context, tenantID string) (bool, error)
	UpdateLoginTime(ctx context.Context, userID string, loginTime time.Time) error
}
