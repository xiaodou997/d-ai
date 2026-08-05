package billing

import "context"

// Repository is the vNext persistence port for price books and runtime
// billing settings.
type Repository interface {
	ListPriceBooks(ctx context.Context) ([]PriceBook, error)
	GetPriceBook(ctx context.Context, id string) (PriceBook, error)
	CreatePriceBook(ctx context.Context, in PriceBookWrite) (PriceBook, error)
	UpdatePriceBook(ctx context.Context, id string, in PriceBookWrite) (PriceBook, error)
	DeletePriceBook(ctx context.Context, id string) error

	ListEntries(ctx context.Context, priceBookID string) ([]PriceBookEntry, error)
	UpsertEntry(ctx context.Context, in PriceBookEntryWrite) (PriceBookEntry, error)
	DeleteEntry(ctx context.Context, priceBookID, modelID string) error

	GetSetting(ctx context.Context, key string) (Setting, error)
	UpsertSetting(ctx context.Context, key string, value any) (Setting, error)
}

type PriceBookWrite struct {
	Code        string
	Name        string
	Description string
	Status      Status
}

type PriceBookEntryWrite struct {
	PriceBookID       string
	ModelID           string
	ModelCode         string
	Capability        string
	TokenPriceTiers   []TokenPriceTier
	ImageDefaultPrice float64
	VideoDefaultPrice float64
	ImagePricesJSON   []byte
	VideoPricesJSON   []byte
	AudioTTSPerChar   float64
	AudioSTTPerMinute float64
	Source            string
	ManuallyEdited    bool
	Metadata          map[string]any
}
