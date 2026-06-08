package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	dbgen "xiaodou/unihub/ai-service/internal/db/gen"
	"xiaodou/unihub/ai-service/internal/domain"
	"xiaodou/unihub/ai-service/internal/service/pricebook"
)

// PriceBookRepo implements service/pricebook.Repository on top of sqlc.
// USD prices live in NUMERIC columns (pgtype.Numeric ↔ float64); image/video
// resolution prices are stored as JSON arrays of {resolution, price(USD)}.
type PriceBookRepo struct {
	q *dbgen.Queries
}

func NewPriceBookRepo(q *dbgen.Queries) *PriceBookRepo {
	return &PriceBookRepo{q: q}
}

var _ pricebook.Repository = (*PriceBookRepo)(nil)

// ---- price books ----

func (r *PriceBookRepo) CreatePriceBook(ctx context.Context, name, description string) (domain.PriceBook, error) {
	row, err := r.q.CreatePriceBook(ctx, dbgen.CreatePriceBookParams{Name: name, Description: description})
	if err != nil {
		return domain.PriceBook{}, err
	}
	return priceBookFrom(row), nil
}

func (r *PriceBookRepo) GetPriceBook(ctx context.Context, id string) (domain.PriceBook, error) {
	uid, err := akUUID(id)
	if err != nil {
		return domain.PriceBook{}, err
	}
	row, err := r.q.GetPriceBook(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.PriceBook{}, domain.ErrNotFound
		}
		return domain.PriceBook{}, err
	}
	return priceBookFrom(row), nil
}

func (r *PriceBookRepo) ListPriceBooks(ctx context.Context) ([]domain.PriceBook, error) {
	rows, err := r.q.ListPriceBooks(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.PriceBook, 0, len(rows))
	for _, row := range rows {
		out = append(out, priceBookFrom(row))
	}
	return out, nil
}

func (r *PriceBookRepo) UpdatePriceBook(ctx context.Context, id, name, description, status string) (domain.PriceBook, error) {
	uid, err := akUUID(id)
	if err != nil {
		return domain.PriceBook{}, err
	}
	row, err := r.q.UpdatePriceBook(ctx, dbgen.UpdatePriceBookParams{
		ID: uid, Name: name, Description: description, Status: status,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.PriceBook{}, domain.ErrNotFound
		}
		return domain.PriceBook{}, err
	}
	return priceBookFrom(row), nil
}

func (r *PriceBookRepo) DeletePriceBook(ctx context.Context, id string) error {
	uid, err := akUUID(id)
	if err != nil {
		return err
	}
	return r.q.DeletePriceBook(ctx, uid)
}

// ---- entries ----

func (r *PriceBookRepo) UpsertEntry(ctx context.Context, e domain.PriceBookEntry) (domain.PriceBookEntry, error) {
	bid, err := akUUID(e.PriceBookID)
	if err != nil {
		return domain.PriceBookEntry{}, err
	}
	row, err := r.q.UpsertPriceBookEntry(ctx, dbgen.UpsertPriceBookEntryParams{
		PriceBookID:        bid,
		ModelCode:          e.ModelCode,
		CapabilityType:     e.CapabilityType,
		InputPerToken:      floatToNumeric(e.InputPerToken),
		OutputPerToken:     floatToNumeric(e.OutputPerToken),
		CacheWritePerToken: floatToNumeric(e.CacheWritePerToken),
		CacheReadPerToken:  floatToNumeric(e.CacheReadPerToken),
		ReasoningPerToken:  floatToNumeric(e.ReasoningPerToken),
		ImagePrices:        encodeUSDResolutions(e.ImagePrices),
		VideoPrices:        encodeUSDResolutions(e.VideoPrices),
		AudioTtsPerChar:    floatToNumeric(e.AudioTTSPerChar),
		AudioSttPerMinute:  floatToNumeric(e.AudioSTTPerMinute),
	})
	if err != nil {
		return domain.PriceBookEntry{}, err
	}
	return priceBookEntryFrom(row), nil
}

func (r *PriceBookRepo) ImportEntry(ctx context.Context, e domain.PriceBookEntry) error {
	bid, err := akUUID(e.PriceBookID)
	if err != nil {
		return err
	}
	return r.q.ImportLiteLLMEntry(ctx, dbgen.ImportLiteLLMEntryParams{
		PriceBookID:        bid,
		ModelCode:          e.ModelCode,
		CapabilityType:     e.CapabilityType,
		InputPerToken:      floatToNumeric(e.InputPerToken),
		OutputPerToken:     floatToNumeric(e.OutputPerToken),
		CacheWritePerToken: floatToNumeric(e.CacheWritePerToken),
		CacheReadPerToken:  floatToNumeric(e.CacheReadPerToken),
		ReasoningPerToken:  floatToNumeric(e.ReasoningPerToken),
	})
}

func (r *PriceBookRepo) GetEntry(ctx context.Context, priceBookID, modelCode string) (domain.PriceBookEntry, error) {
	bid, err := akUUID(priceBookID)
	if err != nil {
		return domain.PriceBookEntry{}, err
	}
	row, err := r.q.GetPriceBookEntry(ctx, dbgen.GetPriceBookEntryParams{PriceBookID: bid, ModelCode: modelCode})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.PriceBookEntry{}, domain.ErrNotFound
		}
		return domain.PriceBookEntry{}, err
	}
	return priceBookEntryFrom(row), nil
}

func (r *PriceBookRepo) ListEntries(ctx context.Context, priceBookID string) ([]domain.PriceBookEntry, error) {
	bid, err := akUUID(priceBookID)
	if err != nil {
		return nil, err
	}
	rows, err := r.q.ListPriceBookEntries(ctx, bid)
	if err != nil {
		return nil, err
	}
	out := make([]domain.PriceBookEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, priceBookEntryFrom(row))
	}
	return out, nil
}

func (r *PriceBookRepo) DeleteEntry(ctx context.Context, priceBookID, modelCode string) error {
	bid, err := akUUID(priceBookID)
	if err != nil {
		return err
	}
	return r.q.DeletePriceBookEntry(ctx, dbgen.DeletePriceBookEntryParams{PriceBookID: bid, ModelCode: modelCode})
}

// ---- settings ----

func (r *PriceBookRepo) GetSetting(ctx context.Context, key string) (json.RawMessage, error) {
	row, err := r.q.GetSetting(ctx, key)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return json.RawMessage(row.Value), nil
}

func (r *PriceBookRepo) UpsertSetting(ctx context.Context, key string, value json.RawMessage) error {
	return r.q.UpsertSetting(ctx, dbgen.UpsertSettingParams{Key: key, Value: []byte(value)})
}

// ---- sell bindings ----

func (r *PriceBookRepo) UpsertTenantSellBinding(ctx context.Context, b domain.TenantSellBinding) (domain.TenantSellBinding, error) {
	bid, err := akUUID(b.PriceBookID)
	if err != nil {
		return domain.TenantSellBinding{}, err
	}
	row, err := r.q.UpsertTenantSellBinding(ctx, dbgen.UpsertTenantSellBindingParams{
		TenantID:            b.TenantID,
		PriceBookID:         bid,
		SellMultiplier:      floatToNumeric(b.SellMultiplier),
		CacheBillingEnabled: b.CacheBillingEnabled,
	})
	if err != nil {
		return domain.TenantSellBinding{}, err
	}
	return tenantSellBindingFrom(row), nil
}

func (r *PriceBookRepo) GetTenantSellBinding(ctx context.Context, tenantID string) (domain.TenantSellBinding, error) {
	row, err := r.q.GetTenantSellBinding(ctx, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.TenantSellBinding{}, domain.ErrNotFound
		}
		return domain.TenantSellBinding{}, err
	}
	return tenantSellBindingFrom(row), nil
}

func (r *PriceBookRepo) ListTenantSellBindings(ctx context.Context) ([]domain.TenantSellBinding, error) {
	rows, err := r.q.ListTenantSellBindings(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.TenantSellBinding, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.TenantSellBinding{
			ID:                  uuidToString(row.ID),
			TenantID:            row.TenantID,
			PriceBookID:         uuidToString(row.PriceBookID),
			SellMultiplier:      numericToFloat(row.SellMultiplier),
			CacheBillingEnabled: row.CacheBillingEnabled,
			CreatedAt:           row.CreatedAt.Time,
			UpdatedAt:           row.UpdatedAt.Time,
			PriceBookName:       row.PriceBookName,
		})
	}
	return out, nil
}

func (r *PriceBookRepo) DeleteTenantSellBinding(ctx context.Context, tenantID string) error {
	return r.q.DeleteTenantSellBinding(ctx, tenantID)
}

func (r *PriceBookRepo) UpsertUserSellBinding(ctx context.Context, b domain.UserSellBinding) (domain.UserSellBinding, error) {
	row, err := r.q.UpsertUserSellBinding(ctx, dbgen.UpsertUserSellBindingParams{
		TenantID:            b.TenantID,
		UserMultiplier:      floatToNumeric(b.UserMultiplier),
		CacheBillingEnabled: b.CacheBillingEnabled,
	})
	if err != nil {
		return domain.UserSellBinding{}, err
	}
	return userSellBindingFrom(row), nil
}

func (r *PriceBookRepo) GetUserSellBinding(ctx context.Context, tenantID string) (domain.UserSellBinding, error) {
	row, err := r.q.GetUserSellBinding(ctx, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.UserSellBinding{}, domain.ErrNotFound
		}
		return domain.UserSellBinding{}, err
	}
	return userSellBindingFrom(row), nil
}

func (r *PriceBookRepo) DeleteUserSellBinding(ctx context.Context, tenantID string) error {
	return r.q.DeleteUserSellBinding(ctx, tenantID)
}

// ---- mappers ----

func priceBookFrom(row dbgen.AiPriceBook) domain.PriceBook {
	return domain.PriceBook{
		ID:          uuidToString(row.ID),
		Name:        row.Name,
		Description: row.Description,
		Status:      row.Status,
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}
}

func priceBookEntryFrom(row dbgen.AiPriceBookEntry) domain.PriceBookEntry {
	return domain.PriceBookEntry{
		ID:                 uuidToString(row.ID),
		PriceBookID:        uuidToString(row.PriceBookID),
		ModelCode:          row.ModelCode,
		CapabilityType:     row.CapabilityType,
		InputPerToken:      numericToFloat(row.InputPerToken),
		OutputPerToken:     numericToFloat(row.OutputPerToken),
		CacheWritePerToken: numericToFloat(row.CacheWritePerToken),
		CacheReadPerToken:  numericToFloat(row.CacheReadPerToken),
		ReasoningPerToken:  numericToFloat(row.ReasoningPerToken),
		ImagePrices:        decodeUSDResolutions(row.ImagePrices),
		VideoPrices:        decodeUSDResolutions(row.VideoPrices),
		AudioTTSPerChar:    numericToFloat(row.AudioTtsPerChar),
		AudioSTTPerMinute:  numericToFloat(row.AudioSttPerMinute),
		Source:             row.Source,
		ManuallyEdited:     row.ManuallyEdited,
		CreatedAt:          row.CreatedAt.Time,
		UpdatedAt:          row.UpdatedAt.Time,
	}
}

func tenantSellBindingFrom(row dbgen.AiTenantSellBinding) domain.TenantSellBinding {
	return domain.TenantSellBinding{
		ID:                  uuidToString(row.ID),
		TenantID:            row.TenantID,
		PriceBookID:         uuidToString(row.PriceBookID),
		SellMultiplier:      numericToFloat(row.SellMultiplier),
		CacheBillingEnabled: row.CacheBillingEnabled,
		CreatedAt:           row.CreatedAt.Time,
		UpdatedAt:           row.UpdatedAt.Time,
	}
}

func userSellBindingFrom(row dbgen.AiUserSellBinding) domain.UserSellBinding {
	return domain.UserSellBinding{
		ID:                  uuidToString(row.ID),
		TenantID:            row.TenantID,
		UserMultiplier:      numericToFloat(row.UserMultiplier),
		CacheBillingEnabled: row.CacheBillingEnabled,
		CreatedAt:           row.CreatedAt.Time,
		UpdatedAt:           row.UpdatedAt.Time,
	}
}

func encodeUSDResolutions(rs []domain.ResolutionUSDPrice) []byte {
	if len(rs) == 0 {
		return []byte("[]")
	}
	b, err := json.Marshal(rs)
	if err != nil {
		return []byte("[]")
	}
	return b
}

func decodeUSDResolutions(raw []byte) []domain.ResolutionUSDPrice {
	if len(raw) == 0 {
		return nil
	}
	var out []domain.ResolutionUSDPrice
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}
