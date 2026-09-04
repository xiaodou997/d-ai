package domain

type ModelCatalogScope struct {
	TenantID string
	UserID   string
}

// RoutedModelPrice is the price projection for a model reachable through an
// active group target. ListAvailableModelPrices may return multiple rows for
// the same model when several visible groups route it.
type RoutedModelPrice struct {
	ModelCode         string
	CapabilityType    string
	TokenPriceTiers   []TokenPriceTier
	ImageDefaultPrice float64
	VideoDefaultPrice float64
	ImagePrices       []ResolutionUSDPrice
	VideoPrices       []ResolutionUSDPrice
	AudioTTSPerChar   float64
	AudioSTTPerMinute float64
}

type TenantUpstreamResource struct {
	ID                string
	Kind              UpstreamKind
	Name              string
	TenantMultiplier  float64
	PriceBookID       string
	PriceBookName     string
	PriceBookRevision int64
	APIFormats        []string
	Models            []TenantUpstreamModel
}

type TenantUpstreamModel struct {
	ModelCode      string
	CapabilityType string
	Price          *PriceBookEntry
}
