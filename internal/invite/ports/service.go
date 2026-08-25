package ports

import "context"

type PublicInvitationStatus string

const (
	PublicInvitationStatusActive   PublicInvitationStatus = "active"
	PublicInvitationStatusExpired  PublicInvitationStatus = "expired"
	PublicInvitationStatusDisabled PublicInvitationStatus = "disabled"
	PublicInvitationStatusUsedUp   PublicInvitationStatus = "used_up"
	PublicInvitationStatusNotFound PublicInvitationStatus = "not_found"
)

type PublicInvitationView struct {
	Code             string
	TenantID         string
	TenantName       string
	CustomerSiteName string
	FaviconVersion   int64
	Description      string
	ExpireTime       *int64
	Status           PublicInvitationStatus
	CanRegister      bool
	Message          string
}

type RegisteredEndUser struct {
	UserID   string
	Username string
	TenantID string
	UserType int
}

type LegalAcceptance struct {
	TermsVersion   string
	PrivacyVersion string
}

type PublicService interface {
	DescribePublicInvitation(ctx context.Context, code string) (*PublicInvitationView, error)
	RegisterUser(ctx context.Context, code, username, password string, email, phone *string, legal LegalAcceptance) (*RegisteredEndUser, error)
}
