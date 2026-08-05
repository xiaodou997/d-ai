package application

import (
	"time"
)

type PromptWrite struct {
	OwnerScope    string
	OwnerTenantID string
	Code          string
	Name          string
	Description   string
	Status        Status
	CreatedBy     string
	UpdatedBy     string
}

type PromptFilter struct {
	OwnerScope    string
	OwnerTenantID string
	Status        Status
}

type PromptVersionWrite struct {
	PromptID     string
	Version      int
	TemplateText string
	Variables    []string
	Notes        string
	CreatedBy    string
}

type AppWrite struct {
	OwnerScope     string
	OwnerTenantID  string
	AppType        AppType
	Code           string
	Name           string
	Description    string
	BoundModelID   string
	DefaultOptions map[string]any
	Metadata       map[string]any
	Status         Status
	CreatedBy      string
	UpdatedBy      string
}

type AppFilter struct {
	OwnerScope    string
	OwnerTenantID string
	Status        Status
}

type AppPromptBindingWrite struct {
	AppID             string
	PromptID          string
	PromptVersion     int
	Role              PromptBindingRole
	BindingOrder      int
	VariablesRequired []string
}

type InvokeKeyWrite struct {
	OwnerScope    string
	TenantID      string
	UserID        string
	Name          string
	KeyHash       string
	KeyCiphertext string
	LastFour      string
	Status        Status
	AppID         string
	ExpiresAt     *time.Time
	CreatedBy     string
}

type InvokeKeyFilter struct {
	OwnerScope string
	TenantID   string
	UserID     string
	Status     Status
}

// InvokeKeyRotate mints a new secret for an existing invoke key while
// preserving its app binding and other metadata.
type InvokeKeyRotate struct {
	OwnerScope    string
	TenantID      string
	UserID        string
	KeyHash       string
	KeyCiphertext string
	LastFour      string
}
