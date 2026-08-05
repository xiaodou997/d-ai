package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/ai/billingcontrol"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
)

// PriceBookRepo implements billingcontrol.Repository on top of sqlc.
// USD prices live in NUMERIC columns (pgtype.Numeric ↔ float64); image/video
// resolution prices are stored as JSON arrays of {resolution, price(USD)}.
type PriceBookRepo struct {
	q    *dbgen.Queries
	pool *pgxpool.Pool
}

func NewPriceBookRepo(q *dbgen.Queries, pool *pgxpool.Pool) *PriceBookRepo {
	return &PriceBookRepo{q: q, pool: pool}
}

var _ billingcontrol.Repository = (*PriceBookRepo)(nil)

// ---- price books ----

func (r *PriceBookRepo) CreatePriceBook(ctx context.Context, ownerType domain.PriceBookOwnerType, ownerTenantID, name, description string) (domain.PriceBook, error) {
	row, err := r.q.CreatePriceBook(ctx, dbgen.CreatePriceBookParams{OwnerType: string(ownerType), OwnerTenantID: ownerTenantID, Name: name, Description: description})
	if err != nil {
		return domain.PriceBook{}, err
	}
	return priceBookFrom(row), nil
}

func (r *PriceBookRepo) ListVisiblePriceBooks(ctx context.Context, tenantID string) ([]domain.PriceBook, error) {
	rows, err := r.q.ListVisiblePriceBooks(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.PriceBook, 0, len(rows))
	for _, row := range rows {
		out = append(out, priceBookFrom(row))
	}
	return out, nil
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
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.PriceBook{}, err
	}
	defer tx.Rollback(ctx)
	groups, err := lockGroupsForPriceBook(ctx, tx, uid)
	if err != nil {
		return domain.PriceBook{}, err
	}
	var lockedID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM ai_price_books WHERE id = $1 FOR UPDATE`, uid).Scan(&lockedID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.PriceBook{}, domain.ErrNotFound
		}
		return domain.PriceBook{}, err
	}
	if status == "disabled" {
		// Re-read after locking the price book to include a group created while
		// this transaction waited for an earlier price-book reader.
		groups, err = lockGroupsForPriceBook(ctx, tx, uid)
		if err != nil {
			return domain.PriceBook{}, err
		}
		if err := priceBookReferenceConflicts(groups); err != nil {
			return domain.PriceBook{}, err
		}
	}
	row, err := r.q.WithTx(tx).UpdatePriceBook(ctx, dbgen.UpdatePriceBookParams{
		ID: uid, Name: name, Description: description, Status: status,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.PriceBook{}, domain.ErrNotFound
		}
		return domain.PriceBook{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.PriceBook{}, err
	}
	return priceBookFrom(row), nil
}

func (r *PriceBookRepo) DeletePriceBook(ctx context.Context, id string) error {
	uid, err := akUUID(id)
	if err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM ai_price_book_entries WHERE price_book_id = $1`, uid); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM ai_price_books WHERE id = $1`, uid); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PriceBookRepo) PriceBookExists(ctx context.Context, id string) (bool, error) {
	uid, err := akUUID(id)
	if err != nil {
		return false, err
	}
	return existsByID(ctx, r.pool, "ai_price_books", uid)
}

func (r *PriceBookRepo) CountPriceBookReferences(ctx context.Context, id string) (int, error) {
	uid, err := akUUID(id)
	if err != nil {
		return 0, err
	}
	return countOne(ctx, r.pool, `
		SELECT
			(SELECT COUNT(*) FROM ai_upstream_accounts WHERE price_book_id = $1) +
			(SELECT COUNT(*) FROM ai_credential_pools WHERE price_book_id = $1) +
			(SELECT COUNT(*) FROM ai_groups WHERE retail_price_book_id = $1)
	`, uid)
}

// ---- entries ----

func (r *PriceBookRepo) UpsertEntry(ctx context.Context, e domain.PriceBookEntry) (domain.PriceBookEntry, error) {
	bid, err := akUUID(e.PriceBookID)
	if err != nil {
		return domain.PriceBookEntry{}, err
	}
	tokenPriceTiers, err := encodeTokenPriceTiers(e.TokenPriceTiers)
	if err != nil {
		return domain.PriceBookEntry{}, fmt.Errorf("encode token price tiers: %w", err)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.PriceBookEntry{}, err
	}
	defer tx.Rollback(ctx)
	qtx := r.q.WithTx(tx)
	row, err := qtx.UpsertPriceBookEntry(ctx, dbgen.UpsertPriceBookEntryParams{
		PriceBookID:       bid,
		ModelCode:         e.ModelCode,
		CapabilityType:    e.CapabilityType,
		TokenPriceTiers:   tokenPriceTiers,
		ImageDefaultPrice: floatToNumeric(e.ImageDefaultPrice),
		VideoDefaultPrice: floatToNumeric(e.VideoDefaultPrice),
		ImagePrices:       encodeUSDResolutions(e.ImagePrices),
		VideoPrices:       encodeUSDResolutions(e.VideoPrices),
		AudioTtsPerChar:   floatToNumeric(e.AudioTTSPerChar),
		AudioSttPerMinute: floatToNumeric(e.AudioSTTPerMinute),
	})
	if err != nil {
		return domain.PriceBookEntry{}, err
	}
	if err := bumpPriceBookRevision(ctx, tx, bid); err != nil {
		return domain.PriceBookEntry{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.PriceBookEntry{}, err
	}
	return priceBookEntryFrom(row)
}

func (r *PriceBookRepo) ImportEntry(ctx context.Context, e domain.PriceBookEntry) error {
	bid, err := akUUID(e.PriceBookID)
	if err != nil {
		return err
	}
	tokenPriceTiers, err := encodeTokenPriceTiers(e.TokenPriceTiers)
	if err != nil {
		return fmt.Errorf("encode token price tiers: %w", err)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := r.q.WithTx(tx).ImportLiteLLMEntry(ctx, dbgen.ImportLiteLLMEntryParams{
		PriceBookID:     bid,
		ModelCode:       e.ModelCode,
		CapabilityType:  e.CapabilityType,
		TokenPriceTiers: tokenPriceTiers,
	}); err != nil {
		return err
	}
	if err := bumpPriceBookRevision(ctx, tx, bid); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PriceBookRepo) GetEntry(ctx context.Context, priceBookID, modelCode, capabilityType string) (domain.PriceBookEntry, error) {
	bid, err := akUUID(priceBookID)
	if err != nil {
		return domain.PriceBookEntry{}, err
	}
	row, err := r.q.GetPriceBookEntry(ctx, dbgen.GetPriceBookEntryParams{PriceBookID: bid, ModelCode: modelCode, CapabilityType: capabilityType})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.PriceBookEntry{}, domain.ErrNotFound
		}
		return domain.PriceBookEntry{}, err
	}
	return priceBookEntryFrom(row)
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
		entry, err := priceBookEntryFrom(row)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, nil
}

func (r *PriceBookRepo) DeleteEntry(ctx context.Context, priceBookID, modelCode, capabilityType string) error {
	bid, err := akUUID(priceBookID)
	if err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := lockGroupsForPriceBook(ctx, tx, bid); err != nil {
		return err
	}
	if err := priceEntryReferenceConflicts(ctx, tx, bid, modelCode, capabilityType); err != nil {
		return err
	}
	if err := r.q.WithTx(tx).DeletePriceBookEntry(ctx, dbgen.DeletePriceBookEntryParams{PriceBookID: bid, ModelCode: modelCode, CapabilityType: capabilityType}); err != nil {
		return err
	}
	if err := bumpPriceBookRevision(ctx, tx, bid); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func bumpPriceBookRevision(ctx context.Context, tx pgx.Tx, priceBookID pgtype.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE ai_price_books SET revision = revision + 1, updated_at = now() WHERE id = $1`, priceBookID)
	return err
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

// ---- mappers ----

func priceBookFrom(row dbgen.AiPriceBook) domain.PriceBook {
	return domain.PriceBook{
		ID:            uuidToString(row.ID),
		OwnerType:     domain.PriceBookOwnerType(row.OwnerType),
		OwnerTenantID: row.OwnerTenantID,
		Name:          row.Name,
		Description:   row.Description,
		Status:        row.Status,
		Revision:      row.Revision,
		CreatedAt:     row.CreatedAt.Time,
		UpdatedAt:     row.UpdatedAt.Time,
	}
}

func priceBookEntryFrom(row dbgen.AiPriceBookEntry) (domain.PriceBookEntry, error) {
	tokenPriceTiers, err := decodeTokenPriceTiers(row.TokenPriceTiers)
	if err != nil {
		return domain.PriceBookEntry{}, fmt.Errorf("decode token price tiers for %q: %w", row.ModelCode, err)
	}
	return domain.PriceBookEntry{
		ID:                uuidToString(row.ID),
		PriceBookID:       uuidToString(row.PriceBookID),
		ModelCode:         row.ModelCode,
		CapabilityType:    row.CapabilityType,
		TokenPriceTiers:   tokenPriceTiers,
		ImageDefaultPrice: numericToFloat(row.ImageDefaultPrice),
		VideoDefaultPrice: numericToFloat(row.VideoDefaultPrice),
		ImagePrices:       decodeUSDResolutions(row.ImagePrices),
		VideoPrices:       decodeUSDResolutions(row.VideoPrices),
		AudioTTSPerChar:   numericToFloat(row.AudioTtsPerChar),
		AudioSTTPerMinute: numericToFloat(row.AudioSttPerMinute),
		Source:            row.Source,
		ManuallyEdited:    row.ManuallyEdited,
		CreatedAt:         row.CreatedAt.Time,
		UpdatedAt:         row.UpdatedAt.Time,
	}, nil
}

func encodeTokenPriceTiers(tiers []domain.TokenPriceTier) ([]byte, error) {
	if len(tiers) == 0 {
		return []byte("[]"), nil
	}
	return json.Marshal(tiers)
}

func decodeTokenPriceTiers(raw []byte) ([]domain.TokenPriceTier, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var tiers []domain.TokenPriceTier
	if err := json.Unmarshal(raw, &tiers); err != nil {
		return nil, err
	}
	return tiers, nil
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
