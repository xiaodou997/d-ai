package billing

import (
	"time"

	"xiaodou/dai/internal/ai/core/catalog"
)

type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
)

// PriceBook is the reusable USD price directory for cost and sell bindings.
type PriceBook struct {
	ID          string
	Code        string
	Name        string
	Description string
	Status      Status
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// PriceBookEntry is a model-scoped pricing row.
type PriceBookEntry struct {
	ID                string
	PriceBookID       string
	ModelID           string
	ModelCode         string
	Capability        catalog.Capability
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
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Setting is a generic key-value config record scoped to billing/runtime
// configuration.
type Setting struct {
	Key       string
	Value     any
	UpdatedAt time.Time
}
