package application

import (
	"time"

	"xiaodou/dai/internal/ai/core/identity"
)

type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
)

// Prompt is a sensitive prompt asset that is never exposed directly to end
// users.
type Prompt struct {
	ID                  string
	OwnerScope          identity.Scope
	OwnerTenantID       string
	OwnerUserID         string
	Code                string
	Name                string
	Description         string
	Status              Status
	CurrentVersion      int
	CurrentTemplateText string
	CurrentVariables    []string
	CreatedBy           string
	UpdatedBy           string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// PromptVersion is immutable once created.
type PromptVersion struct {
	ID           string
	PromptID     string
	Version      int
	TemplateText string
	Variables    []string
	Notes        string
	CreatedBy    string
	CreatedAt    time.Time
}

type AppType string

const (
	AppTypeChatAgent            AppType = "chat_agent"
	AppTypeImageGenerationAgent AppType = "image_generation_agent"
	AppTypeImageEditAgent       AppType = "image_edit_agent"
)

type PromptStrategy string

const (
	PromptStrategyNone            PromptStrategy = "none"
	PromptStrategyCallerVariables PromptStrategy = "caller_variables"
	PromptStrategyBoundExact      PromptStrategy = "bound_prompt_exact"
)

// App is the unified application abstraction replacing the narrower "agent"
// concept.
type App struct {
	ID             string
	OwnerScope     identity.Scope
	OwnerTenantID  string
	OwnerUserID    string
	AppType        AppType
	PromptStrategy PromptStrategy
	Code           string
	Name           string
	Description    string
	BoundModelID   string
	GroupID        string
	DefaultOptions map[string]any
	Metadata       map[string]any
	Status         Status
	CreatedBy      string
	UpdatedBy      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type PromptBindingRole string

const (
	PromptBindingSystem         PromptBindingRole = "system"
	PromptBindingInputTemplate  PromptBindingRole = "input_template"
	PromptBindingOutputTemplate PromptBindingRole = "output_template"
)

// AppPromptBinding attaches a logical prompt to an app. Runtime resolution
// always reads the prompt's current content.
type AppPromptBinding struct {
	ID                string
	AppID             string
	PromptID          string
	Role              PromptBindingRole
	BindingOrder      int
	VariablesRequired []string
	CreatedAt         time.Time
}

// InvokeKey is the public entry token for a single published app. It is a
// minimal-privilege key: it can only ever invoke the one app it is bound to.
type InvokeKey struct {
	ID            string
	OwnerScope    identity.Scope
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
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
