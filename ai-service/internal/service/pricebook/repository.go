// Package pricebook holds the business logic for the unified Price Book pricing
// model: a USD price catalog shared by upstream cost and outbound sell pricing,
// plus per-tenant/per-user sell bindings and the global USD→credit rate.
// See docs/PRICING_REFACTOR_PLAN.md. Service owns validation; persistence is
// reached through Repository.
package pricebook

import (
	"context"
	"encoding/json"

	"xiaodou/unihub/ai-service/internal/domain"
)

// Repository is the persistence port required by the pricebook service.
type Repository interface {
	// ---- price books ----
	CreatePriceBook(ctx context.Context, name, description string) (domain.PriceBook, error)
	GetPriceBook(ctx context.Context, id string) (domain.PriceBook, error)
	ListPriceBooks(ctx context.Context) ([]domain.PriceBook, error)
	UpdatePriceBook(ctx context.Context, id, name, description, status string) (domain.PriceBook, error)
	DeletePriceBook(ctx context.Context, id string) error

	// ---- entries ----
	// UpsertEntry persists a manual edit (source=manual, manually_edited=true).
	UpsertEntry(ctx context.Context, e domain.PriceBookEntry) (domain.PriceBookEntry, error)
	// ImportEntry fills a litellm-sourced entry, skipping rows already manually edited.
	ImportEntry(ctx context.Context, e domain.PriceBookEntry) error
	GetEntry(ctx context.Context, priceBookID, modelCode string) (domain.PriceBookEntry, error)
	ListEntries(ctx context.Context, priceBookID string) ([]domain.PriceBookEntry, error)
	DeleteEntry(ctx context.Context, priceBookID, modelCode string) error

	// ---- settings ----
	GetSetting(ctx context.Context, key string) (json.RawMessage, error)
	UpsertSetting(ctx context.Context, key string, value json.RawMessage) error

	// ---- sell bindings ----
	UpsertTenantSellBinding(ctx context.Context, b domain.TenantSellBinding) (domain.TenantSellBinding, error)
	GetTenantSellBinding(ctx context.Context, tenantID string) (domain.TenantSellBinding, error)
	ListTenantSellBindings(ctx context.Context) ([]domain.TenantSellBinding, error)
	DeleteTenantSellBinding(ctx context.Context, tenantID string) error

	UpsertUserSellBinding(ctx context.Context, b domain.UserSellBinding) (domain.UserSellBinding, error)
	GetUserSellBinding(ctx context.Context, tenantID string) (domain.UserSellBinding, error)
	DeleteUserSellBinding(ctx context.Context, tenantID string) error
}
