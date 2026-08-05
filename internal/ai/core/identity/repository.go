package identity

import (
	"context"
	"time"
)

// APIKeyRepository is the vNext persistence port for AI API key management.
type APIKeyRepository interface {
	Create(ctx context.Context, in APIKeyCreate) (APIKey, error)
	List(ctx context.Context, filter APIKeyListFilter) ([]APIKey, error)
	Update(ctx context.Context, in APIKeyUpdate) (APIKey, string, error)
	UpdateStatus(ctx context.Context, id, tenantID, status string) (APIKey, string, error)
	Rotate(ctx context.Context, in APIKeyRotate) (APIKey, string, error)
	Reveal(ctx context.Context, id, tenantID string) (string, error)
	Delete(ctx context.Context, id, tenantID string) (string, error)
}

type APIKeyCreate struct {
	OwnerScope      Scope
	TenantID        string
	UserID          string
	GroupID         string
	KeyHash         string
	KeyCiphertext   string
	LastFour        string
	Name            string
	QuotaLimitMicro *int64
	AllowedModelIDs []string
	Status          string
	ExpiresAt       *time.Time
	CreatedBy       string
}

type APIKeyListFilter struct {
	TenantID   string
	OwnerScope Scope
	UserID     string
}

type APIKeyUpdate struct {
	ID              string
	TenantID        string
	GroupID         string
	Name            string
	QuotaLimitMicro *int64
	AllowedModelIDs []string
	Status          string
	ExpiresAt       *time.Time
}

type APIKeyRotate struct {
	ID            string
	TenantID      string
	KeyHash       string
	KeyCiphertext string
	LastFour      string
}
