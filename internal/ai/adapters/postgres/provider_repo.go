package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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
		ApiKeyCiphertext:  e.Ciphertext,
		ConcurrencyLimit:  akInt4Ptr(intPtrToInt32Ptr(e.ConcurrencyLimit)),
		PriceBookID:       nullableUUID(e.PriceBookID),
		TenantMultiplier:  floatPtrToNumeric(e.TenantMultiplier),
		Status:            e.Status,
	})
	if err != nil {
		return domain.UpstreamAccount{}, err
	}
	for _, endpoint := range e.Endpoints {
		if _, err := qtx.CreateUpstreamAccountEndpoint(ctx, endpointCreateParams(row.ID, endpoint)); err != nil {
			return domain.UpstreamAccount{}, err
		}
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
	endpointRows, err := r.q.ListAllUpstreamAccountEndpoints(ctx)
	if err != nil {
		return nil, err
	}
	endpointsByAccount := make(map[string][]domain.UpstreamAccountEndpoint)
	for _, row := range endpointRows {
		endpoint := endpointFromRow(row)
		endpointsByAccount[endpoint.AccountID] = append(endpointsByAccount[endpoint.AccountID], endpoint)
	}
	out := make([]domain.UpstreamAccount, 0, len(rows))
	for _, row := range rows {
		account := domain.UpstreamAccount{
			ID:                uuidToString(row.ID),
			Name:              row.Name,
			TenantDisplayName: row.TenantDisplayName,
			TenantAccessMode:  row.TenantAccessMode,
			ConcurrencyLimit:  int32PtrToIntPtr(akInt4StrPtr(row.ConcurrencyLimit)),
			PriceBookID:       uuidToString(row.PriceBookID),
			TenantMultiplier:  numericToFloatPtr(row.TenantMultiplier),
			Status:            row.Status,
			InvalidReason:     row.InvalidReason,
			InvalidAt:         akTimePtr(row.InvalidAt),
			CreatedAt:         row.CreatedAt.Time,
			UpdatedAt:         row.UpdatedAt.Time,
		}
		account.Endpoints = endpointsByAccount[account.ID]
		out = append(out, account)
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
	account := domain.UpstreamAccount{
		ID:                uuidToString(row.ID),
		Name:              row.Name,
		TenantDisplayName: row.TenantDisplayName,
		TenantAccessMode:  row.TenantAccessMode,
		ConcurrencyLimit:  int32PtrToIntPtr(akInt4StrPtr(row.ConcurrencyLimit)),
		PriceBookID:       uuidToString(row.PriceBookID),
		TenantMultiplier:  numericToFloatPtr(row.TenantMultiplier),
		Status:            row.Status,
		InvalidReason:     row.InvalidReason,
		InvalidAt:         akTimePtr(row.InvalidAt),
		CreatedAt:         row.CreatedAt.Time,
		UpdatedAt:         row.UpdatedAt.Time,
	}
	account.Endpoints, err = r.ListEndpoints(ctx, account.ID)
	return account, err
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
	secret := accountSecretFromRow(row)
	secret.Endpoints, err = r.ListEndpoints(ctx, id)
	return secret, err
}

func accountSecretFromRow(row dbgen.AiUpstreamAccount) upstreamcontrol.AccountSecret {
	return upstreamcontrol.AccountSecret{
		Ciphertext:        row.ApiKeyCiphertext,
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
		ApiKeyCiphertext:  e.Ciphertext,
		ConcurrencyLimit:  akInt4Ptr(intPtrToInt32Ptr(e.ConcurrencyLimit)),
		PriceBookID:       nullableUUID(e.PriceBookID),
		TenantMultiplier:  floatPtrToNumeric(e.TenantMultiplier),
		Status:            e.Status,
	})
	if err != nil {
		return domain.UpstreamAccount{}, err
	}
	if e.Status == domain.UpstreamAccountStatusActive {
		if err := validateActiveDirectAccountConfiguration(ctx, tx, e.ID); err != nil {
			return domain.UpstreamAccount{}, err
		}
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

func endpointCreateParams(accountID pgtype.UUID, write domain.UpstreamAccountEndpointWrite) dbgen.CreateUpstreamAccountEndpointParams {
	return dbgen.CreateUpstreamAccountEndpointParams{
		AccountID:    accountID,
		ApiFormat:    string(write.APIFormat),
		BaseUrl:      write.BaseURL,
		PathOverride: write.PathOverride,
		AuthScheme:   write.AuthScheme,
		AuthHeader:   write.AuthHeader,
		ExtraHeaders: pvJSONObjectOrDefault(write.ExtraHeaders),
		Status:       write.Status,
	}
}

func endpointFromRow(row dbgen.AiUpstreamAccountEndpoint) domain.UpstreamAccountEndpoint {
	return domain.UpstreamAccountEndpoint{
		ID:            uuidToString(row.ID),
		AccountID:     uuidToString(row.AccountID),
		APIFormat:     domain.UpstreamProtocol(row.ApiFormat),
		BaseURL:       row.BaseUrl,
		PathOverride:  row.PathOverride,
		AuthScheme:    row.AuthScheme,
		AuthHeader:    row.AuthHeader,
		ExtraHeaders:  row.ExtraHeaders,
		Status:        row.Status,
		HealthStatus:  domain.HealthStatus(row.HealthStatus),
		LastError:     row.LastError,
		LastCheckedAt: akTimePtr(row.LastCheckedAt),
		CreatedAt:     row.CreatedAt.Time,
		UpdatedAt:     row.UpdatedAt.Time,
	}
}

func (r *AccountRepo) ListEndpoints(ctx context.Context, accountID string) ([]domain.UpstreamAccountEndpoint, error) {
	aid, err := akUUID(accountID)
	if err != nil {
		return nil, err
	}
	rows, err := r.q.ListUpstreamAccountEndpoints(ctx, aid)
	if err != nil {
		return nil, err
	}
	out := make([]domain.UpstreamAccountEndpoint, 0, len(rows))
	for _, row := range rows {
		out = append(out, endpointFromRow(row))
	}
	return out, nil
}

func (r *AccountRepo) GetEndpoint(ctx context.Context, accountID, endpointID string) (domain.UpstreamAccountEndpoint, error) {
	aid, err := akUUID(accountID)
	if err != nil {
		return domain.UpstreamAccountEndpoint{}, err
	}
	eid, err := akUUID(endpointID)
	if err != nil {
		return domain.UpstreamAccountEndpoint{}, err
	}
	row, err := r.q.GetUpstreamAccountEndpoint(ctx, dbgen.GetUpstreamAccountEndpointParams{ID: eid, AccountID: aid})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.UpstreamAccountEndpoint{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.UpstreamAccountEndpoint{}, err
	}
	return endpointFromRow(row), nil
}

func (r *AccountRepo) CreateEndpoint(ctx context.Context, accountID string, write domain.UpstreamAccountEndpointWrite) (domain.UpstreamAccountEndpoint, error) {
	aid, err := akUUID(accountID)
	if err != nil {
		return domain.UpstreamAccountEndpoint{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.UpstreamAccountEndpoint{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := lockDirectAccountStatus(ctx, tx, accountID); err != nil {
		return domain.UpstreamAccountEndpoint{}, err
	}
	row, err := queriesWithTx(tx).CreateUpstreamAccountEndpoint(ctx, endpointCreateParams(aid, write))
	if err != nil {
		return domain.UpstreamAccountEndpoint{}, translatePersistenceError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.UpstreamAccountEndpoint{}, err
	}
	return endpointFromRow(row), nil
}

func (r *AccountRepo) UpdateEndpoint(ctx context.Context, accountID, endpointID string, write domain.UpstreamAccountEndpointWrite) (domain.UpstreamAccountEndpoint, error) {
	aid, err := akUUID(accountID)
	if err != nil {
		return domain.UpstreamAccountEndpoint{}, err
	}
	eid, err := akUUID(endpointID)
	if err != nil {
		return domain.UpstreamAccountEndpoint{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.UpstreamAccountEndpoint{}, err
	}
	defer tx.Rollback(ctx)
	accountStatus, err := lockDirectAccountStatus(ctx, tx, accountID)
	if err != nil {
		return domain.UpstreamAccountEndpoint{}, err
	}
	row, err := queriesWithTx(tx).UpdateUpstreamAccountEndpoint(ctx, dbgen.UpdateUpstreamAccountEndpointParams{
		ID: eid, AccountID: aid, ApiFormat: string(write.APIFormat), BaseUrl: write.BaseURL,
		PathOverride: write.PathOverride, AuthScheme: write.AuthScheme, AuthHeader: write.AuthHeader,
		ExtraHeaders: pvJSONObjectOrDefault(write.ExtraHeaders), Status: write.Status,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.UpstreamAccountEndpoint{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.UpstreamAccountEndpoint{}, translatePersistenceError(err)
	}
	if accountStatus == domain.UpstreamAccountStatusActive {
		if err := validateActiveDirectAccountConfiguration(ctx, tx, accountID); err != nil {
			return domain.UpstreamAccountEndpoint{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.UpstreamAccountEndpoint{}, err
	}
	return endpointFromRow(row), nil
}

func (r *AccountRepo) UpdateEndpointHealth(ctx context.Context, accountID, endpointID string, health domain.HealthStatus, lastError string) (domain.UpstreamAccountEndpoint, error) {
	aid, err := akUUID(accountID)
	if err != nil {
		return domain.UpstreamAccountEndpoint{}, err
	}
	eid, err := akUUID(endpointID)
	if err != nil {
		return domain.UpstreamAccountEndpoint{}, err
	}
	row, err := r.q.UpdateUpstreamAccountEndpointHealth(ctx, dbgen.UpdateUpstreamAccountEndpointHealthParams{
		ID: eid, AccountID: aid, HealthStatus: string(health), LastError: lastError,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.UpstreamAccountEndpoint{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.UpstreamAccountEndpoint{}, err
	}
	return endpointFromRow(row), nil
}

func (r *AccountRepo) DeleteEndpoint(ctx context.Context, accountID, endpointID string) error {
	aid, err := akUUID(accountID)
	if err != nil {
		return err
	}
	eid, err := akUUID(endpointID)
	if err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	accountStatus, err := lockDirectAccountStatus(ctx, tx, accountID)
	if err != nil {
		return err
	}
	count, err := queriesWithTx(tx).DeleteUpstreamAccountEndpoint(ctx, dbgen.DeleteUpstreamAccountEndpointParams{ID: eid, AccountID: aid})
	if err != nil {
		return err
	}
	if count == 0 {
		return domain.ErrNotFound
	}
	var remaining int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM ai_upstream_account_endpoints WHERE account_id = $1::uuid`, accountID).Scan(&remaining); err != nil {
		return err
	}
	if remaining == 0 {
		return domain.NewValidationError("endpoint_id", "account must keep at least one endpoint")
	}
	if accountStatus == domain.UpstreamAccountStatusActive {
		if err := validateActiveDirectAccountConfiguration(ctx, tx, accountID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *AccountRepo) UpdateAccountStatus(ctx context.Context, id, status string) (domain.UpstreamAccount, error) {
	aid, err := akUUID(id)
	if err != nil {
		return domain.UpstreamAccount{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.UpstreamAccount{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := lockDirectAccountStatus(ctx, tx, id); err != nil {
		return domain.UpstreamAccount{}, err
	}
	row, err := queriesWithTx(tx).UpdateUpstreamAccountStatus(ctx, dbgen.UpdateUpstreamAccountStatusParams{ID: aid, Status: status})
	if err != nil {
		return domain.UpstreamAccount{}, err
	}
	if status == domain.UpstreamAccountStatusActive {
		if err := validateActiveDirectAccountConfiguration(ctx, tx, id); err != nil {
			return domain.UpstreamAccount{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
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
	if _, err := lockDirectAccountStatus(ctx, tx, id); err != nil {
		return err
	}

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
