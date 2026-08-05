package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"

	corebilling "xiaodou/dai/internal/ai/core/billing"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
)

// CoreBillingRepo adapts the current price-book storage to the core billing
// repository port. It is intentionally implemented on top of the current
// sqlc-backed PriceBookRepo while the rebuilt billing schema is still converging.
type CoreBillingRepo struct {
	backing *PriceBookRepo
}

func NewCoreBillingRepo(q *dbgen.Queries, pool *pgxpool.Pool) *CoreBillingRepo {
	return &CoreBillingRepo{backing: NewPriceBookRepo(q, pool)}
}

var _ corebilling.Repository = (*CoreBillingRepo)(nil)

func (r *CoreBillingRepo) ListPriceBooks(ctx context.Context) ([]corebilling.PriceBook, error) {
	items, err := r.backing.ListPriceBooks(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]corebilling.PriceBook, 0, len(items))
	for _, item := range items {
		out = append(out, item.ToCore())
	}
	return out, nil
}

func (r *CoreBillingRepo) GetPriceBook(ctx context.Context, id string) (corebilling.PriceBook, error) {
	item, err := r.backing.GetPriceBook(ctx, id)
	if err != nil {
		return corebilling.PriceBook{}, err
	}
	return item.ToCore(), nil
}

func (r *CoreBillingRepo) CreatePriceBook(ctx context.Context, in corebilling.PriceBookWrite) (corebilling.PriceBook, error) {
	name := in.Name
	if name == "" {
		name = in.Code
	}
	item, err := r.backing.CreatePriceBook(ctx, domain.PriceBookOwnerPlatform, "", name, in.Description)
	if err != nil {
		return corebilling.PriceBook{}, err
	}
	return item.ToCore(), nil
}

func (r *CoreBillingRepo) UpdatePriceBook(ctx context.Context, id string, in corebilling.PriceBookWrite) (corebilling.PriceBook, error) {
	name := in.Name
	if name == "" {
		name = in.Code
	}
	item, err := r.backing.UpdatePriceBook(ctx, id, name, in.Description, string(in.Status))
	if err != nil {
		return corebilling.PriceBook{}, err
	}
	return item.ToCore(), nil
}

func (r *CoreBillingRepo) DeletePriceBook(ctx context.Context, id string) error {
	return r.backing.DeletePriceBook(ctx, id)
}

func (r *CoreBillingRepo) ListEntries(ctx context.Context, priceBookID string) ([]corebilling.PriceBookEntry, error) {
	items, err := r.backing.ListEntries(ctx, priceBookID)
	if err != nil {
		return nil, err
	}
	out := make([]corebilling.PriceBookEntry, 0, len(items))
	for _, item := range items {
		out = append(out, item.ToCore())
	}
	return out, nil
}

func (r *CoreBillingRepo) UpsertEntry(ctx context.Context, in corebilling.PriceBookEntryWrite) (corebilling.PriceBookEntry, error) {
	var imagePrices []domain.ResolutionUSDPrice
	if len(in.ImagePricesJSON) > 0 {
		_ = json.Unmarshal(in.ImagePricesJSON, &imagePrices)
	}
	var videoPrices []domain.ResolutionUSDPrice
	if len(in.VideoPricesJSON) > 0 {
		_ = json.Unmarshal(in.VideoPricesJSON, &videoPrices)
	}
	item, err := r.backing.UpsertEntry(ctx, domain.PriceBookEntry{
		PriceBookID:       in.PriceBookID,
		ModelCode:         in.ModelCode,
		CapabilityType:    in.Capability,
		TokenPriceTiers:   append([]corebilling.TokenPriceTier(nil), in.TokenPriceTiers...),
		ImageDefaultPrice: in.ImageDefaultPrice,
		VideoDefaultPrice: in.VideoDefaultPrice,
		ImagePrices:       imagePrices,
		VideoPrices:       videoPrices,
		AudioTTSPerChar:   in.AudioTTSPerChar,
		AudioSTTPerMinute: in.AudioSTTPerMinute,
		Source:            in.Source,
		ManuallyEdited:    in.ManuallyEdited,
	})
	if err != nil {
		return corebilling.PriceBookEntry{}, err
	}
	return item.ToCore(), nil
}

func (r *CoreBillingRepo) DeleteEntry(ctx context.Context, priceBookID, modelID string) error {
	entries, err := r.backing.ListEntries(ctx, priceBookID)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.ModelCode == modelID {
			if err := r.backing.DeleteEntry(ctx, priceBookID, modelID, entry.CapabilityType); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *CoreBillingRepo) GetSetting(ctx context.Context, key string) (corebilling.Setting, error) {
	raw, err := r.backing.GetSetting(ctx, key)
	if err != nil {
		return corebilling.Setting{}, err
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return corebilling.Setting{}, err
	}
	return corebilling.Setting{Key: key, Value: value}, nil
}

func (r *CoreBillingRepo) UpsertSetting(ctx context.Context, key string, value any) (corebilling.Setting, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return corebilling.Setting{}, err
	}
	if err := r.backing.UpsertSetting(ctx, key, raw); err != nil {
		return corebilling.Setting{}, err
	}
	return corebilling.Setting{Key: key, Value: value}, nil
}
