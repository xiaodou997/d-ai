package postgres

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	dbgen "xiaodou/unihub/ai-service/internal/db/gen"
	"xiaodou/unihub/ai-service/internal/domain"
	svcusage "xiaodou/unihub/ai-service/internal/service/usage"
)

// UsageRepo implements service/usage.Repository. Most reads go through sqlc, but
// the per-filter stats panel and the daily-trend rollup use inline SQL (the
// previous handler did the same — these aggregates are not in the sqlc set).
type UsageRepo struct {
	q    *dbgen.Queries
	pool *pgxpool.Pool
}

func NewUsageRepo(q *dbgen.Queries, pool *pgxpool.Pool) *UsageRepo {
	return &UsageRepo{q: q, pool: pool}
}

var _ svcusage.Repository = (*UsageRepo)(nil)

func (r *UsageRepo) CountLogs(ctx context.Context, f domain.UsageFilter) (int64, error) {
	return r.q.CountUsageLogs(ctx, dbgen.CountUsageLogsParams{
		TenantID:      f.TenantID,
		UserID:        akText(f.UserID),
		ModelCode:     akText(f.ModelCode),
		RequestStatus: akText(f.RequestStatus),
		RequestSource: akText(f.RequestSource),
		DateFrom:      akTimestamptz(f.DateFrom),
		DateTo:        akTimestamptz(f.DateTo),
	})
}

const usageStatsSQL = `
	SELECT
		COUNT(*) AS total_requests,
		COUNT(*) FILTER (WHERE request_status = 'success') AS success_count,
		COUNT(*) FILTER (WHERE request_status != 'success') AS failed_count,
		COALESCE(SUM(total_tokens), 0)::bigint AS total_tokens,
		COALESCE(SUM(user_cost), 0)::bigint AS total_cost,
		COALESCE(AVG(latency_ms) FILTER (WHERE request_status = 'success' AND latency_ms IS NOT NULL), 0)::float8 AS avg_latency_ms
	FROM ai_usage_logs
	WHERE tenant_id = $1
	  AND ($2::text IS NULL OR user_id = $2::text)
	  AND ($3::text IS NULL OR model_code = $3::text)
	  AND ($4::text IS NULL OR request_status = $4::text)
	  AND ($5::text IS NULL OR request_source = $5::text)
	  AND ($6::timestamptz IS NULL OR created_at >= $6::timestamptz)
	  AND ($7::timestamptz IS NULL OR created_at <= $7::timestamptz)
`

func (r *UsageRepo) StatsFor(ctx context.Context, f domain.UsageFilter) (domain.UsageStats, error) {
	row := r.pool.QueryRow(ctx, usageStatsSQL,
		f.TenantID,
		akText(f.UserID),
		akText(f.ModelCode),
		akText(f.RequestStatus),
		akText(f.RequestSource),
		akTimestamptz(f.DateFrom),
		akTimestamptz(f.DateTo),
	)
	var s domain.UsageStats
	if err := row.Scan(
		&s.TotalRequests,
		&s.SuccessCount,
		&s.FailedCount,
		&s.TotalTokens,
		&s.TotalCostMicro,
		&s.AvgLatencyMs,
	); err != nil {
		return domain.UsageStats{}, err
	}
	return s, nil
}

func (r *UsageRepo) ListLogs(ctx context.Context, f domain.UsageFilter, limit, offset int32) ([]domain.UsageLog, error) {
	rows, err := r.q.ListUsageLogs(ctx, dbgen.ListUsageLogsParams{
		TenantID:      f.TenantID,
		Limit:         limit,
		Offset:        offset,
		UserID:        akText(f.UserID),
		ModelCode:     akText(f.ModelCode),
		RequestStatus: akText(f.RequestStatus),
		RequestSource: akText(f.RequestSource),
		DateFrom:      akTimestamptz(f.DateFrom),
		DateTo:        akTimestamptz(f.DateTo),
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.UsageLog, 0, len(rows))
	for _, row := range rows {
		out = append(out, usageLogFromRow(row))
	}
	return out, nil
}

func (r *UsageRepo) Summary(ctx context.Context, f svcusage.SummaryFilter) ([]domain.UsageSummaryRow, error) {
	rows, err := r.q.ListUsageSummary(ctx, dbgen.ListUsageSummaryParams{
		TenantID:      akText(f.TenantID),
		UserID:        akText(f.UserID),
		ModelCode:     akText(f.ModelCode),
		RequestStatus: akText(f.RequestStatus),
		RequestSource: akText(f.RequestSource),
		Since:         akTimestamptz(f.Since),
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.UsageSummaryRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.UsageSummaryRow{
			ModelCode:              row.ModelCode,
			RequestCount:           row.RequestCount,
			TotalPromptTokens:      row.TotalPromptTokens,
			TotalCompletionTokens:  row.TotalCompletionTokens,
			TotalTokens:            row.TotalTokens,
			TotalProviderCostMicro: row.TotalProviderCost,
			TotalPlatformCostMicro: row.TotalPlatformCost,
			TotalUserCostMicro:     row.TotalUserCost,
			TotalQuotaCostMicro:    row.TotalQuotaCost,
		})
	}
	return out, nil
}

func (r *UsageRepo) UnitSummary(ctx context.Context, f svcusage.SummaryFilter) ([]domain.UsageUnitSummaryRow, error) {
	rows, err := r.q.ListUsageUnitSummary(ctx, dbgen.ListUsageUnitSummaryParams{
		TenantID:      akText(f.TenantID),
		UserID:        akText(f.UserID),
		ModelCode:     akText(f.ModelCode),
		RequestStatus: akText(f.RequestStatus),
		RequestSource: akText(f.RequestSource),
		Since:         akTimestamptz(f.Since),
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.UsageUnitSummaryRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.UsageUnitSummaryRow{
			BillableUnitType:       row.BillableUnitType,
			RequestCount:           row.RequestCount,
			TotalBillableUnits:     row.TotalBillableUnits,
			TotalProviderCostMicro: row.TotalProviderCost,
			TotalPlatformCostMicro: row.TotalPlatformCost,
			TotalUserCostMicro:     row.TotalUserCost,
		})
	}
	return out, nil
}

func (r *UsageRepo) UserSummary(ctx context.Context, tenantID, userID, requestSource string) (domain.UserUsageSummary, error) {
	row, err := r.q.ListUsageSummaryByTenantUser(ctx, dbgen.ListUsageSummaryByTenantUserParams{
		TenantID:      tenantID,
		UserID:        userID,
		RequestSource: akText(requestSource),
	})
	if err != nil {
		return domain.UserUsageSummary{}, err
	}
	return domain.UserUsageSummary{
		RequestCount:          row.RequestCount,
		SuccessRequests:       row.SuccessRequests,
		FailedRequests:        row.FailedRequests,
		TotalTokens:           row.TotalTokens,
		TotalPromptTokens:     row.TotalPromptTokens,
		TotalCompletionTokens: row.TotalCompletionTokens,
		TotalUserCostMicro:    row.TotalUserCost,
		AvgLatencyMs:          row.AvgLatencyMs,
	}, nil
}

const dailyTrendSQL = `
	SELECT
	    date_trunc('day', bucket_start AT TIME ZONE 'UTC')::date::text AS date,
	    COALESCE(SUM(request_count), 0)::bigint                         AS request_count,
	    COALESCE(SUM(success_count), 0)::bigint                         AS success_count,
	    COALESCE(SUM(failed_count), 0)::bigint                          AS failed_count,
	    COALESCE(SUM(total_tokens), 0)::bigint                          AS total_tokens,
	    COALESCE(SUM(prompt_tokens), 0)::bigint                         AS prompt_tokens,
	    COALESCE(SUM(completion_tokens), 0)::bigint                     AS completion_tokens,
	    COALESCE(SUM(provider_cost), 0)::bigint                         AS provider_cost,
	    COALESCE(SUM(platform_cost), 0)::bigint                         AS platform_cost,
	    COALESCE(SUM(user_cost), 0)::bigint                             AS user_cost,
	    CASE WHEN SUM(latency_success_count) > 0
	         THEN (SUM(latency_success_sum_ms) / SUM(latency_success_count))::bigint
	         ELSE 0
	    END AS avg_latency_ms
	FROM ai_usage_rollups_hourly
	WHERE bucket_start >= now() - ($1 || ' days')::interval
	GROUP BY 1
	ORDER BY 1 ASC
`

func (r *UsageRepo) DailyTrend(ctx context.Context, days int) ([]domain.DailyTrendRow, error) {
	rows, err := r.pool.Query(ctx, dailyTrendSQL, strconv.Itoa(days))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.DailyTrendRow, 0)
	for rows.Next() {
		var row domain.DailyTrendRow
		if err := rows.Scan(
			&row.Date, &row.RequestCount, &row.SuccessCount, &row.FailedCount,
			&row.TotalTokens, &row.PromptTokens, &row.CompletionTokens,
			&row.ProviderCostMicro, &row.PlatformCostMicro, &row.UserCostMicro, &row.AvgLatencyMs,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func usageLogFromRow(row dbgen.ListUsageLogsRow) domain.UsageLog {
	return domain.UsageLog{
		ID:                   uuidToString(row.ID),
		RequestID:            row.RequestID,
		TraceID:              row.TraceID.String,
		APIKeyID:             uuidToString(row.ApiKeyID),
		KeyOwnerType:         row.KeyOwnerType,
		AuthMethod:           row.AuthMethod,
		RequestSource:        row.RequestSource,
		TenantID:             row.TenantID,
		UserID:               row.UserID.String,
		ExternalUserID:       row.ExternalUserID.String,
		ModelID:              uuidToString(row.ModelID),
		ModelCode:            row.ModelCode,
		CapabilityType:       row.CapabilityType,
		ModelRouteID:         uuidToString(row.ModelRouteID),
		UpstreamDeploymentID: uuidToString(row.UpstreamDeploymentID),
		EndpointID:           uuidToString(row.EndpointID),
		ProviderCode:         row.ProviderCode.String,
		UpstreamModel:        row.UpstreamModel.String,
		ConversationID:       row.ConversationID.String,
		Stream:               row.Stream,
		PromptTokens:         row.PromptTokens,
		CompletionTokens:     row.CompletionTokens,
		TotalTokens:          row.TotalTokens,
		BillableUnitType:     row.BillableUnitType,
		BillableUnits:        row.BillableUnits,
		ProviderCostMicro:    row.ProviderCost,
		PlatformCostMicro:    row.PlatformCost,
		UserCostMicro:        row.UserCost,
		APIKeyQuotaCostMicro: row.ApiKeyQuotaCost,
		URMTransactionID:     row.UrmTransactionID.String,
		BillingStatus:        row.BillingStatus,
		RequestStatus:        row.RequestStatus,
		HTTPStatus:           akInt4StrPtr(row.HttpStatus),
		UpstreamStatus:       akInt4StrPtr(row.UpstreamStatus),
		LatencyMs:            akInt4StrPtr(row.LatencyMs),
		FirstTokenLatencyMs:  akInt4StrPtr(row.FirstTokenLatencyMs),
		ErrorCode:            row.ErrorCode.String,
		ErrorMessage:         row.ErrorMessage.String,
		UsageEstimated:       row.UsageEstimated,
		TokenUsageSource:     row.TokenUsageSource,
		CreatedAt:            row.CreatedAt.Time,
	}
}

// silence unused import if pgtype ends up unreferenced after edits.
var _ = pgtype.Text{}
