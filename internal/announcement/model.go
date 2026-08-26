package announcement

import (
	"time"

	"xiaodou/dai/internal/auth"
)

type PublisherType string

const (
	PublisherPlatform PublisherType = "platform"
	PublisherTenant   PublisherType = "tenant"
)

type AudienceKind string

const (
	AudienceAdmin      AudienceKind = "admin"
	AudienceTenantUser AudienceKind = "tenant_user"
	AudienceEndUser    AudienceKind = "end_user"
)

type AudienceScopeType string

const (
	AudienceScopeAll    AudienceScopeType = "all"
	AudienceScopeTenant AudienceScopeType = "tenant"
)

type Category string

const (
	CategoryGeneral     Category = "general"
	CategoryMaintenance Category = "maintenance"
	CategoryUpgrade     Category = "upgrade"
	CategoryPricing     Category = "pricing"
	CategorySecurity    Category = "security"
)

type Severity string

const (
	SeverityInfo      Severity = "info"
	SeverityImportant Severity = "important"
	SeverityCritical  Severity = "critical"
)

type DisplayMode string

const (
	DisplayInbox DisplayMode = "inbox"
	DisplayPopup DisplayMode = "popup"
)

type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
	StatusArchived  Status = "archived"
)

type Actor struct {
	UserType auth.UserType
	UserID   auth.UserID
	TenantID auth.TenantID
}

type Principal = Actor

func NewActor(userID, tenantID string, userType int) Actor {
	return Actor{UserType: auth.UserType(userType), UserID: auth.UserID(userID), TenantID: auth.TenantID(tenantID)}
}

// AuthorizationActor bridges the announcement application model to the
// shared authorization policy without duplicating role/scope rules here.
func (a Actor) AuthorizationActor() auth.Actor {
	return auth.NewActor(string(a.UserID), string(a.TenantID), int(a.UserType))
}

func (a Actor) Has(capability auth.Capability) bool {
	return a.AuthorizationActor().Has(capability)
}

type AudienceRule struct {
	Kind      AudienceKind
	ScopeType AudienceScopeType
	TenantID  string
}

type DraftInput struct {
	Title           string
	ContentMarkdown string
	Category        Category
	Severity        Severity
	DisplayMode     DisplayMode
	StartsAt        *time.Time
	EndsAt          *time.Time
	Audiences       []AudienceRule
}

type Announcement struct {
	ID                    string
	PublisherType         PublisherType
	PublisherTenantID     string
	Title                 string
	ContentMarkdown       string
	Category              Category
	Severity              Severity
	DisplayMode           DisplayMode
	Status                Status
	StartsAt              *time.Time
	EndsAt                *time.Time
	PublishedAt           *time.Time
	ArchivedAt            *time.Time
	AudienceSizeAtPublish *int64
	CreatedBy             string
	CreatedByUserType     int
	UpdatedBy             string
	CreatedAt             time.Time
	UpdatedAt             time.Time
	Audiences             []AudienceRule
}

type ManageQuery struct {
	Status Status
	Search string
	Page   int
	Size   int
}

type ManagedPage struct {
	Items []Announcement
	Total int64
	Page  int
	Size  int
}

type RecipientQuery struct {
	Search string
	Page   int
	Size   int
}

type Recipient struct {
	UserType int
	UserID   string
	TenantID string
	Username string
	Email    string
	ReadAt   *time.Time
}

type RecipientPage struct {
	Items []Recipient
	Total int64
	Page  int
	Size  int
}

type InboxQuery struct {
	Page        int
	Size        int
	UnreadOnly  bool
	DisplayMode DisplayMode
}

type InboxItem struct {
	Announcement Announcement
	ReadAt       *time.Time
}

type InboxPage struct {
	Items       []InboxItem
	Total       int64
	UnreadCount int64
	Page        int
	Size        int
}

type Stats struct {
	AudienceSizeAtPublish int64
	CurrentAudienceSize   int64
	ReadCount             int64
}
