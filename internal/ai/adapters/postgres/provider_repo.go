package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/upstreamcontrol"
)

// AccountRepo implements upstreamcontrol.Repository (上游账号) on top of sqlc.
type AccountRepo struct {
	q    *dbgen.Queries
	pool *translatingPool
}

func NewAccountRepo(q *dbgen.Queries, pool *pgxpool.Pool) *AccountRepo {
	return &AccountRepo{q: q, pool: newTranslatingPool(pool)}
}

var _ upstreamcontrol.Repository = (*AccountRepo)(nil)

const pvJSONObjectDefault = "{}"

func pvJSONObjectOrDefault(b []byte) []byte {
	if len(b) == 0 {
		return []byte(pvJSONObjectDefault)
	}
	return b
}

func (r *AccountRepo) CreateAccount(ctx context.Context, e upstreamcontrol.AccountCreate) (domain.UpstreamAccount, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.UpstreamAccount{}, err
	}
	defer tx.Rollback(ctx)

	qtx := queriesWithTx(tx)
	row, err := qtx.CreateUpstreamAccount(ctx, dbgen.CreateUpstreamAccountParams{
		Name:              e.Name,
		TenantDisplayName: e.TenantDisplayName,
		TenantAccessMode:  e.TenantAccessMode,
		BaseUrl:           e.BaseURL,
		ApiKeyCiphertext:  e.Ciphertext,
		ExtraHeaders:      pvJSONObjectOrDefault(e.ExtraHeaders),
		DefaultProtocol:   e.DefaultProtocol,
		ConcurrencyLimit:  akInt4Ptr(intPtrToInt32Ptr(e.ConcurrencyLimit)),
		PriceBookID:       nullableUUID(e.PriceBookID),
		TenantMultiplier:  floatPtrToNumeric(e.TenantMultiplier),
		Status:            e.Status,
	})
	if err != nil {
		return domain.UpstreamAccount{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.UpstreamAccount{}, err
	}
	return r.GetAccount(ctx, uuidToString(row.ID))
}

func (r *AccountRepo) ListAccounts(ctx context.Context) ([]domain.UpstreamAccount, error) {
	rows, err := r.q.ListUpstreamAccounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.UpstreamAccount, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.UpstreamAccount{
			ID:                uuidToString(row.ID),
			Name:              row.Name,
			TenantDisplayName: row.TenantDisplayName,
			TenantAccessMode:  row.TenantAccessMode,
			BaseURL:           row.BaseUrl,
			ExtraHeaders:      row.ExtraHeaders,
			DefaultProtocol:   row.DefaultProtocol,
			ConcurrencyLimit:  int32PtrToIntPtr(akInt4StrPtr(row.ConcurrencyLimit)),
			PriceBookID:       uuidToString(row.PriceBookID),
			TenantMultiplier:  numericToFloatPtr(row.TenantMultiplier),
			Status:            row.Status,
			InvalidReason:     row.InvalidReason,
			InvalidAt:         akTimePtr(row.InvalidAt),
			CreatedAt:         row.CreatedAt.Time,
			UpdatedAt:         row.UpdatedAt.Time,
		})
	}
	return out, nil
}

func (r *AccountRepo) GetAccount(ctx context.Context, id string) (domain.UpstreamAccount, error) {
	aid, err := akUUID(id)
	if err != nil {
		return domain.UpstreamAccount{}, err
	}
	row, err := r.q.GetUpstreamAccount(ctx, aid)
	if err != nil {
		return domain.UpstreamAccount{}, err
	}
	return domain.UpstreamAccount{
		ID:                uuidToString(row.ID),
		Name:              row.Name,
		TenantDisplayName: row.TenantDisplayName,
		TenantAccessMode:  row.TenantAccessMode,
		BaseURL:           row.BaseUrl,
		ExtraHeaders:      row.ExtraHeaders,
		DefaultProtocol:   row.DefaultProtocol,
		ConcurrencyLimit:  int32PtrToIntPtr(akInt4StrPtr(row.ConcurrencyLimit)),
		PriceBookID:       uuidToString(row.PriceBookID),
		TenantMultiplier:  numericToFloatPtr(row.TenantMultiplier),
		Status:            row.Status,
		InvalidReason:     row.InvalidReason,
		InvalidAt:         akTimePtr(row.InvalidAt),
		CreatedAt:         row.CreatedAt.Time,
		UpdatedAt:         row.UpdatedAt.Time,
	}, nil
}

func (r *AccountRepo) GetAccountSecret(ctx context.Context, id string) (upstreamcontrol.AccountSecret, error) {
	aid, err := akUUID(id)
	if err != nil {
		return upstreamcontrol.AccountSecret{}, err
	}
	row, err := r.q.GetUpstreamAccount(ctx, aid)
	if err != nil {
		return upstreamcontrol.AccountSecret{}, err
	}
	return accountSecretFromRow(row), nil
}

func accountSecretFromRow(row dbgen.AiUpstreamAccount) upstreamcontrol.AccountSecret {
	return upstreamcontrol.AccountSecret{
		Ciphertext:        row.ApiKeyCiphertext,
		BaseURL:           row.BaseUrl,
		ExtraHeaders:      row.ExtraHeaders,
		DefaultProtocol:   row.DefaultProtocol,
		TenantDisplayName: row.TenantDisplayName,
		TenantAccessMode:  row.TenantAccessMode,
		Status:            row.Status,
	}
}

func (r *AccountRepo) UpdateAccount(ctx context.Context, e upstreamcontrol.AccountUpdate) (domain.UpstreamAccount, error) {
	aid, err := akUUID(e.ID)
	if err != nil {
		return domain.UpstreamAccount{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.UpstreamAccount{}, err
	}
	defer tx.Rollback(ctx)

	qtx := queriesWithTx(tx)
	row, err := qtx.UpdateUpstreamAccount(ctx, dbgen.UpdateUpstreamAccountParams{
		ID:                aid,
		Name:              e.Name,
		TenantDisplayName: e.TenantDisplayName,
		TenantAccessMode:  e.TenantAccessMode,
		BaseUrl:           e.BaseURL,
		ApiKeyCiphertext:  e.Ciphertext,
		ExtraHeaders:      pvJSONObjectOrDefault(e.ExtraHeaders),
		DefaultProtocol:   e.DefaultProtocol,
		ConcurrencyLimit:  akInt4Ptr(intPtrToInt32Ptr(e.ConcurrencyLimit)),
		PriceBookID:       nullableUUID(e.PriceBookID),
		TenantMultiplier:  floatPtrToNumeric(e.TenantMultiplier),
		Status:            e.Status,
	})
	if err != nil {
		return domain.UpstreamAccount{}, err
	}
	if e.TenantAccessMode == "public" {
		if _, err := tx.Exec(ctx, `
			UPDATE ai_upstream_resource_tenant_policies
			SET access_granted = false, updated_at = now()
			WHERE resource_kind = 'direct_upstream' AND resource_id = $1
		`, aid); err != nil {
			return domain.UpstreamAccount{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.UpstreamAccount{}, err
	}
	return r.GetAccount(ctx, uuidToString(row.ID))
}

func (r *AccountRepo) UpdateAccountStatus(ctx context.Context, id, status string) (domain.UpstreamAccount, error) {
	aid, err := akUUID(id)
	if err != nil {
		return domain.UpstreamAccount{}, err
	}
	row, err := r.q.UpdateUpstreamAccountStatus(ctx, dbgen.UpdateUpstreamAccountStatusParams{ID: aid, Status: status})
	if err != nil {
		return domain.UpstreamAccount{}, err
	}
	return r.GetAccount(ctx, uuidToString(row.ID))
}

func (r *AccountRepo) MarkAccountInvalid(ctx context.Context, id, reason string) (domain.UpstreamAccount, error) {
	aid, err := akUUID(id)
	if err != nil {
		return domain.UpstreamAccount{}, err
	}
	row, err := r.q.MarkUpstreamAccountInvalid(ctx, dbgen.MarkUpstreamAccountInvalidParams{ID: aid, InvalidReason: reason})
	if err != nil {
		return domain.UpstreamAccount{}, err
	}
	return r.GetAccount(ctx, uuidToString(row.ID))
}

func (r *AccountRepo) PriceBookExists(ctx context.Context, id string) (bool, error) {
	uid, err := akUUID(id)
	if err != nil {
		return false, err
	}
	var exists bool
	err = r.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM ai_price_books WHERE id = $1 AND owner_type = 'platform' AND status = 'active')
	`, uid).Scan(&exists)
	return exists, err
}

func (r *AccountRepo) DeleteAccount(ctx context.Context, id string) error {
	aid, err := akUUID(id)
	if err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	groupRows, err := tx.Query(ctx, `
		SELECT g.id::text
		FROM ai_groups g
		WHERE g.id IN (
			SELECT group_id
			FROM ai_group_targets
			WHERE target_kind = 'direct_upstream' AND target_id = $1
		)
		ORDER BY g.id
		FOR UPDATE OF g
	`, aid)
	if err != nil {
		return err
	}
	for groupRows.Next() {
		var groupID string
		if err := groupRows.Scan(&groupID); err != nil {
			groupRows.Close()
			return err
		}
	}
	if err := groupRows.Err(); err != nil {
		groupRows.Close()
		return err
	}
	groupRows.Close()

	if _, err := tx.Exec(ctx, `
		DELETE FROM ai_group_targets
		WHERE target_kind = 'direct_upstream' AND target_id = $1
	`, aid); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM ai_upstream_models
		WHERE upstream_kind = 'direct_upstream' AND upstream_id = $1
	`, aid); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM ai_upstream_resource_tenant_policies
		WHERE resource_kind = 'direct_upstream' AND resource_id = $1
	`, aid); err != nil {
		return err
	}

	tag, err := tx.Exec(ctx, "DELETE FROM ai_upstream_accounts WHERE id = $1", aid)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return tx.Commit(ctx)
}
