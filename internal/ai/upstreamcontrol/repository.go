package upstreamcontrol

import (
	"context"

	"xiaodou/dai/internal/ai/domain"
)

// Repository is the persistence port required by the upstream account service.
type Repository interface {
	CreateAccount(ctx context.Context, e AccountCreate) (domain.UpstreamAccount, error)
	ListAccounts(ctx context.Context) ([]domain.UpstreamAccount, error)
	GetAccountSecret(ctx context.Context, id string) (AccountSecret, error)
	UpdateAccount(ctx context.Context, e AccountUpdate) (domain.UpstreamAccount, error)
	UpdateAccountStatus(ctx context.Context, id, status string) (domain.UpstreamAccount, error)
	MarkAccountInvalid(ctx context.Context, id, reason string) (domain.UpstreamAccount, error)
	PriceBookExists(ctx context.Context, id string) (bool, error)
	DeleteAccount(ctx context.Context, id string) error
}

type AccountCreate struct {
	Name              string
	TenantDisplayName string
	TenantAccessMode  string
	BaseURL           string
	Ciphertext        string
	ExtraHeaders      []byte
	DefaultProtocol   string
	ConcurrencyLimit  *int
	PriceBookID       string
	TenantMultiplier  *float64
	Status            string
}

type AccountUpdate struct {
	ID                string
	Name              string
	TenantDisplayName string
	TenantAccessMode  string
	BaseURL           string
	Ciphertext        string
	ExtraHeaders      []byte
	DefaultProtocol   string
	ConcurrencyLimit  *int
	PriceBookID       string
	TenantMultiplier  *float64
	Status            string
}

type AccountSecret struct {
	Ciphertext        string
	DefaultProtocol   string
	TenantDisplayName string
	TenantAccessMode  string
	Status            string
}
