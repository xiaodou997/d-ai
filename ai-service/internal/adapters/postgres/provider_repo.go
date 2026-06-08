package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	dbgen "xiaodou/unihub/ai-service/internal/db/gen"
	"xiaodou/unihub/ai-service/internal/domain"
	svcprovider "xiaodou/unihub/ai-service/internal/service/provider"
)

// ProviderRepo implements service/provider.Repository on top of sqlc.
type ProviderRepo struct {
	q    *dbgen.Queries
	pool *pgxpool.Pool
}

func NewProviderRepo(q *dbgen.Queries, pool *pgxpool.Pool) *ProviderRepo {
	return &ProviderRepo{q: q, pool: pool}
}

var _ svcprovider.Repository = (*ProviderRepo)(nil)

const pvJSONObjectDefault = "{}"

func pvJSONObjectOrDefault(b []byte) []byte {
	if len(b) == 0 {
		return []byte(pvJSONObjectDefault)
	}
	return b
}

func (r *ProviderRepo) CreateProvider(ctx context.Context, w svcprovider.ProviderWrite) (domain.Provider, error) {
	row, err := r.q.CreateProvider(ctx, dbgen.CreateProviderParams{
		Code:   w.Code,
		Name:   w.Name,
		Config: pvJSONObjectOrDefault(w.Config),
		Status: w.Status,
	})
	if err != nil {
		return domain.Provider{}, err
	}
	return providerFromRow(row), nil
}

func (r *ProviderRepo) ListProviders(ctx context.Context) ([]domain.Provider, error) {
	rows, err := r.q.ListProviders(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Provider, 0, len(rows))
	for _, row := range rows {
		out = append(out, providerFromRow(row))
	}
	return out, nil
}

func (r *ProviderRepo) UpdateProvider(ctx context.Context, id string, w svcprovider.ProviderWrite) (domain.Provider, error) {
	uid, err := akUUID(id)
	if err != nil {
		return domain.Provider{}, err
	}
	row, err := r.q.UpdateProvider(ctx, dbgen.UpdateProviderParams{
		ID:     uid,
		Code:   w.Code,
		Name:   w.Name,
		Config: pvJSONObjectOrDefault(w.Config),
		Status: w.Status,
	})
	if err != nil {
		return domain.Provider{}, err
	}
	return providerFromRow(row), nil
}

func (r *ProviderRepo) UpdateProviderStatus(ctx context.Context, id, status string) (domain.Provider, error) {
	uid, err := akUUID(id)
	if err != nil {
		return domain.Provider{}, err
	}
	row, err := r.q.UpdateProviderStatus(ctx, dbgen.UpdateProviderStatusParams{ID: uid, Status: status})
	if err != nil {
		return domain.Provider{}, err
	}
	return providerFromRow(row), nil
}

func (r *ProviderRepo) CreateEndpoint(ctx context.Context, e svcprovider.EndpointCreate) (domain.ProviderEndpoint, error) {
	pid, err := akUUID(e.ProviderID)
	if err != nil {
		return domain.ProviderEndpoint{}, err
	}
	row, err := r.q.CreateProviderEndpoint(ctx, dbgen.CreateProviderEndpointParams{
		ProviderID:       pid,
		Name:             e.Name,
		BaseUrl:          e.BaseURL,
		ApiKeyCiphertext: e.Ciphertext,
		ExtraHeaders:     pvJSONObjectOrDefault(e.ExtraHeaders),
		Weight:           e.Weight,
		TimeoutMs:        e.TimeoutMs,
		DefaultProtocol:  e.DefaultProtocol,
		PriceBookID:      nullableUUID(e.PriceBookID),
		CostMultiplier:   floatPtrToNumeric(e.CostMultiplier),
		Status:           e.Status,
	})
	if err != nil {
		return domain.ProviderEndpoint{}, err
	}
	return domain.ProviderEndpoint{
		ID:              uuidToString(row.ID),
		ProviderID:      uuidToString(row.ProviderID),
		Name:            row.Name,
		BaseURL:         row.BaseUrl,
		ExtraHeaders:    row.ExtraHeaders,
		Weight:          row.Weight,
		TimeoutMs:       row.TimeoutMs,
		DefaultProtocol: row.DefaultProtocol,
		PriceBookID:     uuidToString(row.PriceBookID),
		CostMultiplier:  numericToFloatPtr(row.CostMultiplier),
		Status:          row.Status,
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
	}, nil
}

func (r *ProviderRepo) ListEndpoints(ctx context.Context, providerID string) ([]domain.ProviderEndpoint, error) {
	pid, err := akUUID(providerID)
	if err != nil {
		return nil, err
	}
	rows, err := r.q.ListProviderEndpoints(ctx, pid)
	if err != nil {
		return nil, err
	}
	out := make([]domain.ProviderEndpoint, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.ProviderEndpoint{
			ID:              uuidToString(row.ID),
			ProviderID:      uuidToString(row.ProviderID),
			Name:            row.Name,
			BaseURL:         row.BaseUrl,
			ExtraHeaders:    row.ExtraHeaders,
			Weight:          row.Weight,
			TimeoutMs:       row.TimeoutMs,
			DefaultProtocol: row.DefaultProtocol,
			PriceBookID:     uuidToString(row.PriceBookID),
			CostMultiplier:  numericToFloatPtr(row.CostMultiplier),
			Status:          row.Status,
			CreatedAt:       row.CreatedAt.Time,
			UpdatedAt:       row.UpdatedAt.Time,
		})
	}
	return out, nil
}

func (r *ProviderRepo) GetEndpointSecret(ctx context.Context, providerID, id string) (svcprovider.EndpointSecret, error) {
	pid, err := akUUID(providerID)
	if err != nil {
		return svcprovider.EndpointSecret{}, err
	}
	eid, err := akUUID(id)
	if err != nil {
		return svcprovider.EndpointSecret{}, err
	}
	row, err := r.q.GetProviderEndpoint(ctx, dbgen.GetProviderEndpointParams{ProviderID: pid, ID: eid})
	if err != nil {
		return svcprovider.EndpointSecret{}, err
	}
	return svcprovider.EndpointSecret{
		Ciphertext:      row.ApiKeyCiphertext,
		DefaultProtocol: row.DefaultProtocol,
	}, nil
}

func (r *ProviderRepo) UpdateEndpoint(ctx context.Context, e svcprovider.EndpointUpdate) (domain.ProviderEndpoint, error) {
	pid, err := akUUID(e.ProviderID)
	if err != nil {
		return domain.ProviderEndpoint{}, err
	}
	eid, err := akUUID(e.ID)
	if err != nil {
		return domain.ProviderEndpoint{}, err
	}
	row, err := r.q.UpdateProviderEndpoint(ctx, dbgen.UpdateProviderEndpointParams{
		ProviderID:       pid,
		ID:               eid,
		Name:             e.Name,
		BaseUrl:          e.BaseURL,
		ApiKeyCiphertext: e.Ciphertext,
		ExtraHeaders:     pvJSONObjectOrDefault(e.ExtraHeaders),
		Weight:           e.Weight,
		TimeoutMs:        e.TimeoutMs,
		DefaultProtocol:  e.DefaultProtocol,
		PriceBookID:      nullableUUID(e.PriceBookID),
		CostMultiplier:   floatPtrToNumeric(e.CostMultiplier),
		Status:           e.Status,
	})
	if err != nil {
		return domain.ProviderEndpoint{}, err
	}
	return endpointFromUpdateRow(row), nil
}

func (r *ProviderRepo) UpdateEndpointStatus(ctx context.Context, providerID, id, status string) (domain.ProviderEndpoint, error) {
	pid, err := akUUID(providerID)
	if err != nil {
		return domain.ProviderEndpoint{}, err
	}
	eid, err := akUUID(id)
	if err != nil {
		return domain.ProviderEndpoint{}, err
	}
	row, err := r.q.UpdateProviderEndpointStatus(ctx, dbgen.UpdateProviderEndpointStatusParams{
		ProviderID: pid,
		ID:         eid,
		Status:     status,
	})
	if err != nil {
		return domain.ProviderEndpoint{}, err
	}
	return domain.ProviderEndpoint{
		ID:              uuidToString(row.ID),
		ProviderID:      uuidToString(row.ProviderID),
		Name:            row.Name,
		BaseURL:         row.BaseUrl,
		ExtraHeaders:    row.ExtraHeaders,
		Weight:          row.Weight,
		TimeoutMs:       row.TimeoutMs,
		DefaultProtocol: row.DefaultProtocol,
		PriceBookID:     uuidToString(row.PriceBookID),
		CostMultiplier:  numericToFloatPtr(row.CostMultiplier),
		Status:          row.Status,
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
	}, nil
}

func (r *ProviderRepo) DeleteEndpoint(ctx context.Context, providerID, id string) error {
	pid, err := akUUID(providerID)
	if err != nil {
		return err
	}
	eid, err := akUUID(id)
	if err != nil {
		return err
	}
	tag, err := r.pool.Exec(ctx,
		"DELETE FROM ai_provider_endpoints WHERE provider_id = $1 AND id = $2",
		pid, eid)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func providerFromRow(row dbgen.AiProvider) domain.Provider {
	return domain.Provider{
		ID:        uuidToString(row.ID),
		Code:      row.Code,
		Name:      row.Name,
		Config:    row.Config,
		Status:    row.Status,
		CreatedAt: row.CreatedAt.Time,
		UpdatedAt: row.UpdatedAt.Time,
	}
}

func endpointFromUpdateRow(row dbgen.UpdateProviderEndpointRow) domain.ProviderEndpoint {
	return domain.ProviderEndpoint{
		ID:              uuidToString(row.ID),
		ProviderID:      uuidToString(row.ProviderID),
		Name:            row.Name,
		BaseURL:         row.BaseUrl,
		ExtraHeaders:    row.ExtraHeaders,
		Weight:          row.Weight,
		TimeoutMs:       row.TimeoutMs,
		DefaultProtocol: row.DefaultProtocol,
		PriceBookID:     uuidToString(row.PriceBookID),
		CostMultiplier:  numericToFloatPtr(row.CostMultiplier),
		Status:          row.Status,
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
	}
}
