package billingcontrol

import (
	"context"
	"encoding/json"

	"xiaodou/dai/internal/ai/domain"
)

// Repository is the persistence port required by the billing control service.
type Repository interface {
	CreatePriceBook(ctx context.Context, ownerType domain.PriceBookOwnerType, ownerTenantID, name, description string) (domain.PriceBook, error)
	GetPriceBook(ctx context.Context, id string) (domain.PriceBook, error)
	ListPriceBooks(ctx context.Context) ([]domain.PriceBook, error)
	ListVisiblePriceBooks(ctx context.Context, tenantID string) ([]domain.PriceBook, error)
	UpdatePriceBook(ctx context.Context, id, name, description, status string) (domain.PriceBook, error)
	DeletePriceBook(ctx context.Context, id string) error
	CountPriceBookReferences(ctx context.Context, id string) (int, error)

	UpsertEntry(ctx context.Context, e domain.PriceBookEntry) (domain.PriceBookEntry, error)
	ImportEntry(ctx context.Context, e domain.PriceBookEntry) error
	// ImportEntries applies one LiteLLM snapshot atomically for a price book.
	// The returned count is the number of rows inserted or changed; manually
	// edited rows and identical rows are deliberately not counted.
	ImportEntries(ctx context.Context, priceBookID string, entries []domain.PriceBookEntry) (int, error)
	GetEntry(ctx context.Context, priceBookID, modelCode, capabilityType string) (domain.PriceBookEntry, error)
	ListEntries(ctx context.Context, priceBookID string) ([]domain.PriceBookEntry, error)
	DeleteEntry(ctx context.Context, priceBookID, modelCode, capabilityType string) error

	GetSetting(ctx context.Context, key string) (json.RawMessage, error)
	UpsertSetting(ctx context.Context, key string, value json.RawMessage) error
}
