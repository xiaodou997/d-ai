package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/observabilitycontrol"
)

// UsageRepo implements observabilitycontrol.UsageRepository. Most reads go through sqlc, but
// the per-filter stats panel and the daily trend query use inline SQL (the
// previous handler did the same — these aggregates are not in the sqlc set).
type UsageRepo struct {
	q    *dbgen.Queries
	pool *pgxpool.Pool
}

func NewUsageRepo(q *dbgen.Queries, pool *pgxpool.Pool) *UsageRepo {
	return &UsageRepo{q: q, pool: pool}
}

var _ observabilitycontrol.UsageRepository = (*UsageRepo)(nil)

func (r *UsageRepo) CountLogs(ctx context.Context, f domain.UsageFilter) (int64, error) {
	return r.q.CountUsageLogs(ctx, dbgen.CountUsageLogsParams{
		TenantID:      akText(f.TenantID),
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
		COALESCE(SUM(catalog_base), 0)::bigint AS total_catalog_base,
		COALESCE(SUM(tenant_payable), 0)::bigint AS total_tenant_payable,
		COALESCE(SUM(user_charged), 0)::bigint AS total_user_charged,
		COALESCE(AVG(latency_ms) FILTER (WHERE request_status = 'success' AND latency_ms IS NOT NULL), 0)::float8 AS avg_latency_ms,
		COALESCE(AVG(request_total_ms) FILTER (WHERE request_status = 'success' AND request_total_ms IS NOT NULL), 0)::float8 AS avg_request_total_ms,
		COALESCE(AVG(first_response_byte_ms) FILTER (WHERE request_status = 'success' AND first_response_byte_ms IS NOT NULL), 0)::float8 AS avg_first_response_byte_ms
	FROM ai_usage_logs
	WHERE ($1::text IS NULL OR tenant_id = $1::text)
	  AND ($2::text IS NULL OR user_id = $2::text)
	  AND ($3::text IS NULL OR model_code = $3::text)
	  AND ($4::text IS NULL OR request_status = $4::text)
	  AND ($5::text IS NULL OR request_source = $5::text)
		  AND ($6::timestamptz IS NULL OR created_at >= $6::timestamptz)
		  AND ($7::timestamptz IS NULL OR created_at < $7::timestamptz)
	`

func (r *UsageRepo) StatsFor(ctx context.Context, f domain.UsageFilter) (domain.UsageStats, error) {
	row := r.pool.QueryRow(ctx, usageStatsSQL,
		akText(f.TenantID),
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
		&s.TotalCatalogBaseMicro,
		&s.TotalTenantPayableMicro,
		&s.TotalUserChargedMicro,
		&s.AvgLatencyMs,
		&s.AvgRequestTotalMs,
		&s.AvgFirstResponseByteMs,
	); err != nil {
		return domain.UsageStats{}, err
	}
	return s, nil
}

func (r *UsageRepo) ListLogs(ctx context.Context, f domain.UsageFilter, limit, offset int32) ([]domain.UsageLog, error) {
	rows, err := r.q.ListUsageLogs(ctx, dbgen.ListUsageLogsParams{
		TenantID:      akText(f.TenantID),
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

func (r *UsageRepo) GetLogDetail(ctx context.Context, requestID string) (domain.UsageLogDetail, error) {
	row, err := r.q.GetUsageLogByRequestID(ctx, requestID)
	if err != nil {
		return domain.UsageLogDetail{}, err
	}
	detail := domain.UsageLogDetail{UsageLog: usageLogFromGetByRequestIDRow(row)}
	detail.UserAgent = detail.ClientUserAgent
	if detail.CredentialPoolID != "" {
		detail.SelectedUpstreamTargetType = "pool"
	} else if detail.EndpointID != "" {
		detail.SelectedUpstreamTargetType = "account"
	}
	auditRec, err := NewAuditStore(r.pool).GetByRequestID(ctx, requestID)
	if err != nil {
		return domain.UsageLogDetail{}, err
	}
	if auditRec != nil {
		detail.ClientProtocol = auditRec.ClientProtocol
		detail.RequestPath = auditRec.RequestPath
		detail.ClientIP = auditRec.ClientIP
		detail.UserAgent = auditRec.UserAgent
		detail.AuthMasked = auditRec.AuthMasked
		detail.RequestMessages = auditRec.RequestMessages
		detail.RequestParams = auditRec.RequestParams
		detail.ResponseMessage = auditRec.ResponseMessage
		detail.MediaRefs = auditRec.MediaRefs
		if detail.RequestedModel == "" {
			detail.RequestedModel = auditRec.RequestModel
		}
		if detail.MatchedDispatchRuleID == "" {
			detail.MatchedDispatchRuleID = auditRec.MatchedDispatchRuleID
		}
		if detail.MatchedDispatchRuleSummary == "" {
			detail.MatchedDispatchRuleSummary = auditRec.MatchedDispatchRuleSummary
		}
		if detail.ResolvedLogicalModel == "" {
			detail.ResolvedLogicalModel = auditRec.ResolvedLogicalModel
		}
		if detail.ResolvedProviderFamily == "" {
			detail.ResolvedProviderFamily = auditRec.ResolvedProviderFamily
		}
		if detail.SelectedUpstreamProtocol == "" {
			detail.SelectedUpstreamProtocol = auditRec.SelectedUpstreamProtocol
		}
		if detail.SelectedUpstreamModel == "" {
			detail.SelectedUpstreamModel = auditRec.SelectedUpstreamModel
		}
		if detail.PublicResponseModel == "" {
			detail.PublicResponseModel = auditRec.PublicResponseModel
		}
		detail.InternalErrorDetail = auditRec.InternalErrorDetail
		detail.FailedStep = auditRec.FailedStep
		detail.AttemptsDetail = auditRec.AttemptsDetail
	}
	return detail, nil
}

func (r *UsageRepo) Summary(ctx context.Context, f observabilitycontrol.SummaryFilter) ([]domain.UsageSummaryRow, error) {
	rows, err := r.q.ListUsageSummary(ctx, dbgen.ListUsageSummaryParams{
		TenantID:      akText(f.TenantID),
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
	out := make([]domain.UsageSummaryRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.UsageSummaryRow{
			ModelCode:               row.ModelCode,
			RequestCount:            row.RequestCount,
			TotalPromptTokens:       row.TotalPromptTokens,
			TotalCompletionTokens:   row.TotalCompletionTokens,
			TotalTokens:             row.TotalTokens,
			TotalCatalogBaseMicro:   row.TotalCatalogBase,
			TotalTenantPayableMicro: row.TotalTenantPayable,
			TotalRetailBaseMicro:    row.TotalRetailBase,
			TotalUserPayableMicro:   row.TotalUserPayable,
			TotalUserChargedMicro:   row.TotalUserCharged,
			TotalQuotaCostMicro:     row.TotalQuotaCost,
		})
	}
	return out, nil
}

func (r *UsageRepo) UnitSummary(ctx context.Context, f observabilitycontrol.SummaryFilter) ([]domain.UsageUnitSummaryRow, error) {
	rows, err := r.q.ListUsageUnitSummary(ctx, dbgen.ListUsageUnitSummaryParams{
		TenantID:      akText(f.TenantID),
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
	out := make([]domain.UsageUnitSummaryRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.UsageUnitSummaryRow{
			BillableUnitType:        row.BillableUnitType,
			RequestCount:            row.RequestCount,
			TotalBillableUnits:      row.TotalBillableUnits,
			TotalCatalogBaseMicro:   row.TotalCatalogBase,
			TotalTenantPayableMicro: row.TotalTenantPayable,
			TotalRetailBaseMicro:    row.TotalRetailBase,
			TotalUserPayableMicro:   row.TotalUserPayable,
			TotalUserChargedMicro:   row.TotalUserCharged,
		})
	}
	return out, nil
}

// upstreamUsageSummarySQL reports what each upstream account or pool actually
// produced. It is the denominator for reconciling against the operator's own
// spend ledger, which is where platform cost lives — this system stores no
// purchase price, so nothing here is a margin figure.
//
// The resource name comes from a LEFT JOIN and may be missing: deleting an
// account must not erase its history, so the caller falls back to provider_code
// or the raw ID rather than dropping the row.
const upstreamUsageSummarySQL = `
	SELECT
		CASE WHEN l.endpoint_id IS NOT NULL THEN 'direct_upstream' ELSE 'oauth_pool' END AS target_kind,
		COALESCE(l.endpoint_id::text, l.credential_pool_id::text) AS target_id,
		COALESCE(a.name, p.name, '') AS target_name,
		COALESCE(l.provider_code, '') AS provider_code,
		COUNT(*)::bigint AS request_count,
		COUNT(*) FILTER (WHERE l.request_status = 'success')::bigint AS success_count,
		COUNT(*) FILTER (WHERE l.request_status <> 'success')::bigint AS failed_count,
		COALESCE(SUM(l.prompt_tokens), 0)::bigint AS total_prompt_tokens,
		COALESCE(SUM(l.completion_tokens), 0)::bigint AS total_completion_tokens,
		COALESCE(SUM(l.total_tokens), 0)::bigint AS total_tokens,
		-- billable_units mixes units across capabilities, so split it by type
		-- instead of summing tokens and images into one meaningless number.
		COALESCE(SUM(l.billable_units) FILTER (
			WHERE l.billable_unit_type IN ('token', 'input_token', 'output_token')), 0)::bigint AS token_units,
		COALESCE(SUM(l.billable_units) FILTER (WHERE l.billable_unit_type = 'image'), 0)::bigint AS image_units,
		COALESCE(SUM(l.catalog_base), 0)::bigint AS catalog_base,
		COALESCE(SUM(l.tenant_payable), 0)::bigint AS tenant_payable
	FROM ai_usage_logs l
	LEFT JOIN ai_upstream_accounts a ON a.id = l.endpoint_id
	LEFT JOIN ai_credential_pools  p ON p.id = l.credential_pool_id
	WHERE (l.endpoint_id IS NOT NULL OR l.credential_pool_id IS NOT NULL)
	  AND ($1::text IS NULL OR l.tenant_id = $1::text)
	  AND ($2::text IS NULL OR l.user_id = $2::text)
	  AND ($3::text IS NULL OR l.model_code = $3::text)
	  AND ($4::text IS NULL OR l.request_status = $4::text)
	  AND ($5::text IS NULL OR l.request_source = $5::text)
	  AND ($6::timestamptz IS NULL OR l.created_at >= $6::timestamptz)
	  AND ($7::timestamptz IS NULL OR l.created_at < $7::timestamptz)
	GROUP BY 1, 2, 3, 4
	ORDER BY request_count DESC, total_tokens DESC
`

func (r *UsageRepo) UpstreamSummary(ctx context.Context, f observabilitycontrol.SummaryFilter) ([]domain.UsageUpstreamSummaryRow, error) {
	rows, err := r.pool.Query(ctx, upstreamUsageSummarySQL,
		akText(f.TenantID), akText(f.UserID), akText(f.ModelCode),
		akText(f.RequestStatus), akText(f.RequestSource),
		akTimestamptz(f.DateFrom), akTimestamptz(f.DateTo),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.UsageUpstreamSummaryRow, 0)
	for rows.Next() {
		var row domain.UsageUpstreamSummaryRow
		if err := rows.Scan(
			&row.TargetKind, &row.TargetID, &row.TargetName, &row.ProviderCode,
			&row.RequestCount, &row.SuccessCount, &row.FailedCount,
			&row.TotalPromptTokens, &row.TotalCompletionTokens, &row.TotalTokens,
			&row.TokenUnits, &row.ImageUnits,
			&row.CatalogBaseMicro, &row.TenantPayableMicro,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

const userRankingSQL = `
	SELECT
		tenant_id,
		user_id,
		COUNT(*)::bigint AS request_count,
		COUNT(*) FILTER (WHERE request_status = 'success')::bigint AS success_count,
		COUNT(*) FILTER (WHERE request_status != 'success')::bigint AS failed_count,
		COALESCE(SUM(total_tokens), 0)::bigint AS total_tokens,
		COALESCE(SUM(user_charged), 0)::bigint AS total_user_charged,
		MAX(created_at) AS last_requested_at
	FROM ai_usage_logs
	WHERE COALESCE(user_id, '') <> ''
	  AND ($1::text IS NULL OR tenant_id = $1::text)
	  AND ($2::text IS NULL OR user_id = $2::text)
	  AND ($3::text IS NULL OR model_code = $3::text)
	  AND ($4::text IS NULL OR request_status = $4::text)
	  AND ($5::text IS NULL OR request_source = $5::text)
	  AND ($6::timestamptz IS NULL OR created_at >= $6::timestamptz)
	  AND ($7::timestamptz IS NULL OR created_at < $7::timestamptz)
	GROUP BY tenant_id, user_id
	ORDER BY total_user_charged DESC, total_tokens DESC, request_count DESC, last_requested_at DESC
	LIMIT $8
`

func (r *UsageRepo) UserRanking(ctx context.Context, f observabilitycontrol.SummaryFilter, limit int32) ([]domain.UsageUserRankingRow, error) {
	rows, err := r.pool.Query(ctx, userRankingSQL,
		akText(f.TenantID),
		akText(f.UserID),
		akText(f.ModelCode),
		akText(f.RequestStatus),
		akText(f.RequestSource),
		akTimestamptz(f.DateFrom),
		akTimestamptz(f.DateTo),
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.UsageUserRankingRow, 0)
	for rows.Next() {
		var row domain.UsageUserRankingRow
		if err := rows.Scan(
			&row.TenantID,
			&row.UserID,
			&row.RequestCount,
			&row.SuccessCount,
			&row.FailedCount,
			&row.TotalTokens,
			&row.TotalUserChargedMicro,
			&row.LastRequestedAt,
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
		TotalUserChargedMicro: row.TotalUserCharged,
		AvgLatencyMs:          row.AvgLatencyMs,
	}, nil
}

const dailyTrendSQL = `
	SELECT
	    to_char(date_trunc('day', created_at AT TIME ZONE 'UTC'), 'YYYY-MM-DD') AS date,
	    COUNT(*)::bigint AS request_count,
	    COUNT(*) FILTER (WHERE request_status = 'success')::bigint AS success_count,
	    COUNT(*) FILTER (WHERE request_status != 'success')::bigint AS failed_count,
	    COALESCE(SUM(total_tokens), 0)::bigint AS total_tokens,
	    COALESCE(SUM(prompt_tokens), 0)::bigint AS prompt_tokens,
	    COALESCE(SUM(completion_tokens), 0)::bigint AS completion_tokens,
	    COALESCE(SUM(catalog_base), 0)::bigint AS catalog_base,
	    COALESCE(SUM(tenant_payable), 0)::bigint AS tenant_payable,
	    COALESCE(SUM(retail_base), 0)::bigint AS retail_base,
	    COALESCE(SUM(user_payable), 0)::bigint AS user_payable,
	    COALESCE(SUM(user_charged), 0)::bigint AS user_charged,
	    COALESCE(AVG(latency_ms) FILTER (WHERE request_status = 'success'), 0)::bigint AS avg_latency_ms,
	    COALESCE(AVG(request_total_ms) FILTER (WHERE request_status = 'success'), 0)::bigint AS avg_request_total_ms,
	    COALESCE(AVG(first_response_byte_ms) FILTER (WHERE request_status = 'success'), 0)::bigint AS avg_first_response_byte_ms
	FROM ai_usage_logs
	WHERE ($1::timestamptz IS NULL OR created_at >= $1::timestamptz)
	  AND ($2::timestamptz IS NULL OR created_at < $2::timestamptz)
	GROUP BY 1
	ORDER BY 1 ASC
`

func (r *UsageRepo) DailyTrend(ctx context.Context, dateFrom, dateTo *time.Time) ([]domain.DailyTrendRow, error) {
	rows, err := r.pool.Query(ctx, dailyTrendSQL, akTimestamptz(dateFrom), akTimestamptz(dateTo))
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
			&row.CatalogBaseMicro, &row.TenantPayableMicro, &row.RetailBaseMicro, &row.UserPayableMicro, &row.UserChargedMicro,
			&row.AvgLatencyMs, &row.AvgRequestTotalMs, &row.AvgFirstResponseByteMs,
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
		ID:                                 uuidToString(row.ID),
		RequestID:                          row.RequestID,
		TraceID:                            row.TraceID.String,
		APIKeyID:                           uuidToString(row.ApiKeyID),
		KeyOwnerType:                       row.KeyOwnerType,
		AuthMethod:                         row.AuthMethod,
		RequestSource:                      row.RequestSource,
		TenantID:                           row.TenantID,
		UserID:                             row.UserID.String,
		ClientUserAgent:                    row.ClientUserAgent,
		ExternalUserID:                     row.ExternalUserID.String,
		GroupID:                            uuidToString(row.GroupID),
		GroupNameSnapshot:                  row.GroupNameSnapshot,
		GroupDefaultUserMultiplierSnapshot: numericToFloat(row.GroupDefaultUserMultiplierSnapshot),
		UserMultiplierOverrideSnapshot:     numericToFloatPtr(row.UserMultiplierOverrideSnapshot),
		EffectiveUserMultiplierSnapshot:    numericToFloat(row.EffectiveUserMultiplierSnapshot),
		BillingGroupLabelSnapshot:          row.BillingGroupLabelSnapshot,
		ModelCode:                          row.ModelCode,
		AppID:                              uuidToString(row.AppID),
		AppName:                            row.AppNameSnapshot,
		AppOwnerType:                       row.AppOwnerType,
		AppOwnerTenantID:                   row.AppOwnerTenantID,
		AppOwnerUserID:                     row.AppOwnerUserID,
		RequestedModel:                     row.RequestedModel,
		MatchedDispatchRuleID:              uuidToString(row.MatchedDispatchRuleID),
		MatchedDispatchRuleSummary:         row.MatchedDispatchRuleSummary.String,
		ResolvedLogicalModel:               row.ResolvedLogicalModel.String,
		ResolvedProviderFamily:             row.ResolvedProviderFamily.String,
		CapabilityType:                     row.CapabilityType,
		GroupTargetID:                      uuidToString(row.GroupTargetID),
		EndpointID:                         uuidToString(row.EndpointID),
		CredentialPoolID:                   uuidToString(row.CredentialPoolID),
		ProviderCode:                       row.ProviderCode.String,
		UpstreamModel:                      row.UpstreamModel.String,
		ConversationID:                     row.ConversationID.String,
		Stream:                             row.Stream,
		PromptTokens:                       row.PromptTokens,
		CompletionTokens:                   row.CompletionTokens,
		CacheWriteTokens:                   row.CacheWriteTokens,
		CacheReadTokens:                    row.CacheReadTokens,
		ReasoningTokens:                    row.ReasoningTokens,
		ReasoningEffort:                    row.ReasoningEffort.String,
		TotalTokens:                        row.TotalTokens,
		BillableUnitType:                   row.BillableUnitType,
		BillableUnits:                      row.BillableUnits,
		Resolution:                         row.Resolution.String,
		CatalogBaseMicro:                   row.CatalogBase,
		TenantPayableMicro:                 row.TenantPayable,
		RetailBaseMicro:                    row.RetailBase,
		UserPayableMicro:                   row.UserPayable,
		UserChargedMicro:                   row.UserCharged,
		APIKeyQuotaCostMicro:               row.ApiKeyQuotaCost,
		ServiceTier:                        row.ServiceTier,
		BillingBreakdownJSON:               row.BillingBreakdown,
		URMTransactionID:                   row.UrmTransactionID.String,
		BillingStatus:                      row.BillingStatus,
		RequestStatus:                      row.RequestStatus,
		HTTPStatus:                         akInt4StrPtr(row.HttpStatus),
		UpstreamStatus:                     akInt4StrPtr(row.UpstreamStatus),
		LatencyMs:                          akInt4StrPtr(row.LatencyMs),
		FirstTokenLatencyMs:                akInt4StrPtr(row.FirstTokenLatencyMs),
		RequestTotalMs:                     akInt4StrPtr(row.RequestTotalMs),
		RequestSetupMs:                     akInt4StrPtr(row.RequestSetupMs),
		FirstResponseByteMs:                akInt4StrPtr(row.FirstResponseByteMs),
		ResponseTailMs:                     akInt4StrPtr(row.ResponseTailMs),
		FinalAttemptHeaderMs:               akInt4StrPtr(row.FinalAttemptHeaderMs),
		FinalAttemptTotalMs:                akInt4StrPtr(row.FinalAttemptTotalMs),
		ErrorCode:                          row.ErrorCode.String,
		ErrorMessage:                       row.ErrorMessage.String,
		AttemptsCount:                      row.AttemptsCount,
		ProtocolConversionEnabled:          row.ProtocolConversionEnabled,
		UpstreamModelMappingApplied:        row.UpstreamModelMappingApplied,
		PublicResponseModel:                row.PublicResponseModel.String,
		UsageEstimated:                     row.UsageEstimated,
		TokenUsageSource:                   row.TokenUsageSource,
		BillingSource:                      row.BillingSource,
		CreatedAt:                          row.CreatedAt.Time,
	}
}

func usageLogFromGetByRequestIDRow(row dbgen.GetUsageLogByRequestIDRow) domain.UsageLog {
	return domain.UsageLog{
		ID:                                 uuidToString(row.ID),
		RequestID:                          row.RequestID,
		TraceID:                            row.TraceID.String,
		APIKeyID:                           uuidToString(row.ApiKeyID),
		KeyOwnerType:                       row.KeyOwnerType,
		AuthMethod:                         row.AuthMethod,
		RequestSource:                      row.RequestSource,
		TenantID:                           row.TenantID,
		UserID:                             row.UserID.String,
		ClientUserAgent:                    row.ClientUserAgent,
		ExternalUserID:                     row.ExternalUserID.String,
		GroupID:                            uuidToString(row.GroupID),
		GroupNameSnapshot:                  row.GroupNameSnapshot,
		GroupDefaultUserMultiplierSnapshot: numericToFloat(row.GroupDefaultUserMultiplierSnapshot),
		UserMultiplierOverrideSnapshot:     numericToFloatPtr(row.UserMultiplierOverrideSnapshot),
		EffectiveUserMultiplierSnapshot:    numericToFloat(row.EffectiveUserMultiplierSnapshot),
		BillingGroupLabelSnapshot:          row.BillingGroupLabelSnapshot,
		ModelCode:                          row.ModelCode,
		AppID:                              uuidToString(row.AppID),
		AppName:                            row.AppNameSnapshot,
		AppOwnerType:                       row.AppOwnerType,
		AppOwnerTenantID:                   row.AppOwnerTenantID,
		AppOwnerUserID:                     row.AppOwnerUserID,
		RequestedModel:                     row.RequestedModel,
		MatchedDispatchRuleID:              uuidToString(row.MatchedDispatchRuleID),
		MatchedDispatchRuleSummary:         row.MatchedDispatchRuleSummary.String,
		ResolvedLogicalModel:               row.ResolvedLogicalModel.String,
		ResolvedProviderFamily:             row.ResolvedProviderFamily.String,
		CapabilityType:                     row.CapabilityType,
		GroupTargetID:                      uuidToString(row.GroupTargetID),
		EndpointID:                         uuidToString(row.EndpointID),
		CredentialPoolID:                   uuidToString(row.CredentialPoolID),
		ProviderCode:                       row.ProviderCode.String,
		UpstreamModel:                      row.UpstreamModel.String,
		ConversationID:                     row.ConversationID.String,
		Stream:                             row.Stream,
		PromptTokens:                       row.PromptTokens,
		CompletionTokens:                   row.CompletionTokens,
		CacheWriteTokens:                   row.CacheWriteTokens,
		CacheReadTokens:                    row.CacheReadTokens,
		ReasoningTokens:                    row.ReasoningTokens,
		ReasoningEffort:                    row.ReasoningEffort.String,
		TotalTokens:                        row.TotalTokens,
		BillableUnitType:                   row.BillableUnitType,
		BillableUnits:                      row.BillableUnits,
		Resolution:                         row.Resolution.String,
		CatalogBaseMicro:                   row.CatalogBase,
		TenantPayableMicro:                 row.TenantPayable,
		RetailBaseMicro:                    row.RetailBase,
		UserPayableMicro:                   row.UserPayable,
		UserChargedMicro:                   row.UserCharged,
		APIKeyQuotaCostMicro:               row.ApiKeyQuotaCost,
		ServiceTier:                        row.ServiceTier,
		BillingBreakdownJSON:               row.BillingBreakdown,
		URMTransactionID:                   row.UrmTransactionID.String,
		BillingStatus:                      row.BillingStatus,
		RequestStatus:                      row.RequestStatus,
		HTTPStatus:                         akInt4StrPtr(row.HttpStatus),
		UpstreamStatus:                     akInt4StrPtr(row.UpstreamStatus),
		LatencyMs:                          akInt4StrPtr(row.LatencyMs),
		FirstTokenLatencyMs:                akInt4StrPtr(row.FirstTokenLatencyMs),
		RequestTotalMs:                     akInt4StrPtr(row.RequestTotalMs),
		RequestSetupMs:                     akInt4StrPtr(row.RequestSetupMs),
		FirstResponseByteMs:                akInt4StrPtr(row.FirstResponseByteMs),
		ResponseTailMs:                     akInt4StrPtr(row.ResponseTailMs),
		FinalAttemptHeaderMs:               akInt4StrPtr(row.FinalAttemptHeaderMs),
		FinalAttemptTotalMs:                akInt4StrPtr(row.FinalAttemptTotalMs),
		ErrorCode:                          row.ErrorCode.String,
		ErrorMessage:                       row.ErrorMessage.String,
		AttemptsCount:                      row.AttemptsCount,
		ProtocolConversionEnabled:          row.ProtocolConversionEnabled,
		UpstreamModelMappingApplied:        row.UpstreamModelMappingApplied,
		PublicResponseModel:                row.PublicResponseModel.String,
		UsageEstimated:                     row.UsageEstimated,
		TokenUsageSource:                   row.TokenUsageSource,
		CreatedAt:                          row.CreatedAt.Time,
	}
}

// silence unused import if pgtype ends up unreferenced after edits.
var _ = pgtype.Text{}
