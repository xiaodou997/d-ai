package postgres

import (
	"context"

	dbgen "xiaodou/unihub/ai-service/internal/db/gen"
	"xiaodou/unihub/ai-service/internal/domain"
	svcdashboard "xiaodou/unihub/ai-service/internal/service/dashboard"
)

// DashboardRepo implements service/dashboard.Repository on top of sqlc.
type DashboardRepo struct {
	q *dbgen.Queries
}

func NewDashboardRepo(q *dbgen.Queries) *DashboardRepo {
	return &DashboardRepo{q: q}
}

var _ svcdashboard.Repository = (*DashboardRepo)(nil)

// All dashboard queries take pgtype.Text scope params (empty → NULL), matching
// the original by-role handler which wrapped the scope in optionalTextValue.
func (r *DashboardRepo) Summary(ctx context.Context, f domain.DashboardFilter) (domain.DashboardSummary, error) {
	row, err := r.q.GetDashboardSummary(ctx, dbgen.GetDashboardSummaryParams{
		TenantID: akText(f.TenantID),
		UserID:   akText(f.UserID),
		Since:    akTimestamptz(f.Since),
	})
	if err != nil {
		return domain.DashboardSummary{}, err
	}
	return domain.DashboardSummary{
		TotalRequests:          row.TotalRequests,
		SuccessfulRequests:     row.SuccessfulRequests,
		FailedRequests:         row.FailedRequests,
		TotalTokens:            row.TotalTokens,
		TotalPromptTokens:      row.TotalPromptTokens,
		TotalCompletionTokens:  row.TotalCompletionTokens,
		TotalProviderCostMicro: row.TotalProviderCost,
		TotalPlatformCostMicro: row.TotalPlatformCost,
		TotalUserCostMicro:     row.TotalUserCost,
		AvgLatencyMs:           row.AvgLatencyMs,
	}, nil
}

func (r *DashboardRepo) TopModels(ctx context.Context, f domain.DashboardFilter, limit int32) ([]domain.DashboardTopModel, error) {
	rows, err := r.q.ListDashboardTopModels(ctx, dbgen.ListDashboardTopModelsParams{
		TenantID: akText(f.TenantID),
		UserID:   akText(f.UserID),
		Since:    akTimestamptz(f.Since),
		Limit:    limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.DashboardTopModel, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.DashboardTopModel{
			ModelCode:      row.ModelCode,
			RequestCount:   row.RequestCount,
			TotalTokens:    row.TotalTokens,
			TotalCostMicro: row.TotalCost,
		})
	}
	return out, nil
}

func (r *DashboardRepo) TopTenants(ctx context.Context, f domain.DashboardFilter, limit int32) ([]domain.DashboardTopTenant, error) {
	rows, err := r.q.ListDashboardTopTenants(ctx, dbgen.ListDashboardTopTenantsParams{
		TenantID: akText(f.TenantID),
		UserID:   akText(f.UserID),
		Since:    akTimestamptz(f.Since),
		Limit:    limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.DashboardTopTenant, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.DashboardTopTenant{
			TenantID:       row.TenantID,
			RequestCount:   row.RequestCount,
			TotalTokens:    row.TotalTokens,
			TotalCostMicro: row.TotalCost,
		})
	}
	return out, nil
}

func (r *DashboardRepo) RecentErrors(ctx context.Context, f domain.DashboardFilter, limit int32) ([]domain.DashboardRecentError, error) {
	rows, err := r.q.ListDashboardRecentErrors(ctx, dbgen.ListDashboardRecentErrorsParams{
		TenantID: akText(f.TenantID),
		UserID:   akText(f.UserID),
		Since:    akTimestamptz(f.Since),
		Limit:    limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.DashboardRecentError, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.DashboardRecentError{
			RequestID:     row.RequestID,
			ModelCode:     row.ModelCode,
			RequestStatus: row.RequestStatus,
			ErrorCode:     row.ErrorCode.String,
			ErrorMessage:  row.ErrorMessage.String,
			HTTPStatus:    akInt4StrPtr(row.HttpStatus),
			CreatedAt:     row.CreatedAt.Time,
		})
	}
	return out, nil
}
