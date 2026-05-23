package httpserver

import (
	"net/http"
	"strconv"

	dbgen "xiaodou/uni-ai-api/internal/db/gen"
)

type usageStatsDTO struct {
	TotalRequests int64   `json:"total_requests"`
	SuccessCount  int64   `json:"success_count"`
	FailedCount   int64   `json:"failed_count"`
	TotalTokens   int64   `json:"total_tokens"`
	TotalCost     int64   `json:"total_cost"`    // micro-credits (1 credit = 10000 micro)
	TotalCredits  float64 `json:"total_credits"` // decimal credits = TotalCost / 10000
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
}

// microToCredits 把 micro-credit 精度的 int64 转成展示用的 decimal 积分 float。
// 用于在每个 DTO 中和原始 _cost 字段并列输出 _credits 字段，前端直接读
// _credits 不用再做 ÷10000 的折算。1 积分 = 10000 micro-credit。
func microToCredits(micro int64) float64 {
	return float64(micro) / 10000.0
}

type listUsageLogsResponse struct {
	Total   int64         `json:"total"`
	Stats   usageStatsDTO `json:"stats"`
	Records []usageLogDTO `json:"records"`
}

type listUsageLogsForTenantResponse struct {
	Total   int64                  `json:"total"`
	Stats   usageStatsDTO          `json:"stats"`
	Records []usageLogForTenantDTO `json:"records"`
}

func (s *Server) handleAdminListUsageLogs(w http.ResponseWriter, r *http.Request) {
	limit := int32(20)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || parsed <= 0 {
			writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid limit")
			return
		}
		if parsed > 100 {
			parsed = 100
		}
		limit = int32(parsed)
	}

	offset := int32(0)
	if raw := r.URL.Query().Get("offset"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || parsed < 0 {
			writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid offset")
			return
		}
		offset = int32(parsed)
	}

	filters, ok := scopedUsageFilters(w, r)
	if !ok {
		return
	}

	countParams := dbgen.CountUsageLogsParams{
		TenantID:      filters.tenantID,
		UserID:        optionalTextValue(filters.userID),
		ModelCode:     optionalTextValue(filters.modelCode),
		RequestStatus: optionalTextValue(filters.requestStatus),
		RequestSource: optionalTextValue(filters.requestSource),
		DateFrom:      filters.dateFrom,
		DateTo:        filters.dateTo,
	}
	total, err := s.queries.CountUsageLogs(r.Context(), countParams)
	if err != nil {
		s.writeAdminServerError(w, r, "count usage logs failed", err)
		return
	}

	const statsSQL = `
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
	var stats usageStatsDTO
	statsRow := s.postgres.QueryRow(r.Context(), statsSQL,
		filters.tenantID,
		optionalTextValue(filters.userID),
		optionalTextValue(filters.modelCode),
		optionalTextValue(filters.requestStatus),
		optionalTextValue(filters.requestSource),
		filters.dateFrom,
		filters.dateTo,
	)
	if err := statsRow.Scan(
		&stats.TotalRequests,
		&stats.SuccessCount,
		&stats.FailedCount,
		&stats.TotalTokens,
		&stats.TotalCost,
		&stats.AvgLatencyMs,
	); err != nil {
		s.writeAdminServerError(w, r, "query usage stats failed", err)
		return
	}
	stats.TotalCredits = microToCredits(stats.TotalCost)

	rows, err := s.queries.ListUsageLogs(r.Context(), dbgen.ListUsageLogsParams{
		TenantID:      filters.tenantID,
		Limit:         limit,
		Offset:        offset,
		UserID:        optionalTextValue(filters.userID),
		ModelCode:     optionalTextValue(filters.modelCode),
		RequestStatus: optionalTextValue(filters.requestStatus),
		RequestSource: optionalTextValue(filters.requestSource),
		DateFrom:      filters.dateFrom,
		DateTo:        filters.dateTo,
	})
	if err != nil {
		s.writeAdminServerError(w, r, "list usage logs failed", err)
		return
	}
	writeOK(w, listUsageLogsResponse{
		Total:   total,
		Stats:   stats,
		Records: fromListUsageLogs(rows),
	})
}

// ============================================================================
// Aggregation DTOs — wrap sqlc-generated rows to expose `_credits` floats
// alongside the raw micro-credit `_cost` ints.
// ============================================================================

type dashboardSummaryDTO struct {
	TotalRequests         int64   `json:"total_requests"`
	SuccessfulRequests    int64   `json:"successful_requests"`
	FailedRequests        int64   `json:"failed_requests"`
	TotalTokens           int64   `json:"total_tokens"`
	TotalPromptTokens     int64   `json:"total_prompt_tokens"`
	TotalCompletionTokens int64   `json:"total_completion_tokens"`
	TotalProviderCost     int64   `json:"total_provider_cost"`
	TotalPlatformCost     int64   `json:"total_platform_cost"`
	TotalUserCost         int64   `json:"total_user_cost"`
	TotalProviderCredits  float64 `json:"total_provider_credits"`
	TotalPlatformCredits  float64 `json:"total_platform_credits"`
	TotalUserCredits      float64 `json:"total_user_credits"`
	AvgLatencyMs          float64 `json:"avg_latency_ms"`
}

func fromDashboardSummary(r dbgen.GetDashboardSummaryRow) dashboardSummaryDTO {
	return dashboardSummaryDTO{
		TotalRequests:         r.TotalRequests,
		SuccessfulRequests:    r.SuccessfulRequests,
		FailedRequests:        r.FailedRequests,
		TotalTokens:           r.TotalTokens,
		TotalPromptTokens:     r.TotalPromptTokens,
		TotalCompletionTokens: r.TotalCompletionTokens,
		TotalProviderCost:     r.TotalProviderCost,
		TotalPlatformCost:     r.TotalPlatformCost,
		TotalUserCost:         r.TotalUserCost,
		TotalProviderCredits:  microToCredits(r.TotalProviderCost),
		TotalPlatformCredits:  microToCredits(r.TotalPlatformCost),
		TotalUserCredits:      microToCredits(r.TotalUserCost),
		AvgLatencyMs:          r.AvgLatencyMs,
	}
}

type usageSummaryRowDTO struct {
	ModelCode             string  `json:"model_code"`
	RequestCount          int64   `json:"request_count"`
	TotalPromptTokens     int64   `json:"total_prompt_tokens"`
	TotalCompletionTokens int64   `json:"total_completion_tokens"`
	TotalTokens           int64   `json:"total_tokens"`
	TotalProviderCost     int64   `json:"total_provider_cost"`
	TotalPlatformCost     int64   `json:"total_platform_cost"`
	TotalUserCost         int64   `json:"total_user_cost"`
	TotalQuotaCost        int64   `json:"total_quota_cost"`
	TotalProviderCredits  float64 `json:"total_provider_credits"`
	TotalPlatformCredits  float64 `json:"total_platform_credits"`
	TotalUserCredits      float64 `json:"total_user_credits"`
	TotalQuotaCredits     float64 `json:"total_quota_credits"`
}

func fromUsageSummary(rows []dbgen.ListUsageSummaryRow) []usageSummaryRowDTO {
	out := make([]usageSummaryRowDTO, len(rows))
	for i, r := range rows {
		out[i] = usageSummaryRowDTO{
			ModelCode:             r.ModelCode,
			RequestCount:          r.RequestCount,
			TotalPromptTokens:     r.TotalPromptTokens,
			TotalCompletionTokens: r.TotalCompletionTokens,
			TotalTokens:           r.TotalTokens,
			TotalProviderCost:     r.TotalProviderCost,
			TotalPlatformCost:     r.TotalPlatformCost,
			TotalUserCost:         r.TotalUserCost,
			TotalQuotaCost:        r.TotalQuotaCost,
			TotalProviderCredits:  microToCredits(r.TotalProviderCost),
			TotalPlatformCredits:  microToCredits(r.TotalPlatformCost),
			TotalUserCredits:      microToCredits(r.TotalUserCost),
			TotalQuotaCredits:     microToCredits(r.TotalQuotaCost),
		}
	}
	return out
}

type usageUnitSummaryRowDTO struct {
	BillableUnitType     string  `json:"billable_unit_type"`
	RequestCount         int64   `json:"request_count"`
	TotalBillableUnits   int64   `json:"total_billable_units"`
	TotalProviderCost    int64   `json:"total_provider_cost"`
	TotalPlatformCost    int64   `json:"total_platform_cost"`
	TotalUserCost        int64   `json:"total_user_cost"`
	TotalProviderCredits float64 `json:"total_provider_credits"`
	TotalPlatformCredits float64 `json:"total_platform_credits"`
	TotalUserCredits     float64 `json:"total_user_credits"`
}

func fromUsageUnitSummary(rows []dbgen.ListUsageUnitSummaryRow) []usageUnitSummaryRowDTO {
	out := make([]usageUnitSummaryRowDTO, len(rows))
	for i, r := range rows {
		out[i] = usageUnitSummaryRowDTO{
			BillableUnitType:     r.BillableUnitType,
			RequestCount:         r.RequestCount,
			TotalBillableUnits:   r.TotalBillableUnits,
			TotalProviderCost:    r.TotalProviderCost,
			TotalPlatformCost:    r.TotalPlatformCost,
			TotalUserCost:        r.TotalUserCost,
			TotalProviderCredits: microToCredits(r.TotalProviderCost),
			TotalPlatformCredits: microToCredits(r.TotalPlatformCost),
			TotalUserCredits:     microToCredits(r.TotalUserCost),
		}
	}
	return out
}

type userUsageSummaryDTO struct {
	RequestCount          int64   `json:"request_count"`
	SuccessRequests       int64   `json:"success_requests"`
	FailedRequests        int64   `json:"failed_requests"`
	TotalTokens           int64   `json:"total_tokens"`
	TotalPromptTokens     int64   `json:"total_prompt_tokens"`
	TotalCompletionTokens int64   `json:"total_completion_tokens"`
	TotalUserCost         int64   `json:"total_user_cost"`
	TotalUserCredits      float64 `json:"total_user_credits"`
	AvgLatencyMs          float64 `json:"avg_latency_ms"`
}

func fromUserUsageSummary(r dbgen.ListUsageSummaryByTenantUserRow) userUsageSummaryDTO {
	return userUsageSummaryDTO{
		RequestCount:          r.RequestCount,
		SuccessRequests:       r.SuccessRequests,
		FailedRequests:        r.FailedRequests,
		TotalTokens:           r.TotalTokens,
		TotalPromptTokens:     r.TotalPromptTokens,
		TotalCompletionTokens: r.TotalCompletionTokens,
		TotalUserCost:         r.TotalUserCost,
		TotalUserCredits:      microToCredits(r.TotalUserCost),
		AvgLatencyMs:          r.AvgLatencyMs,
	}
}

type dashboardTopModelDTO struct {
	ModelCode      string  `json:"model_code"`
	RequestCount   int64   `json:"request_count"`
	TotalTokens    int64   `json:"total_tokens"`
	TotalCost      int64   `json:"total_cost"`
	TotalCredits   float64 `json:"total_credits"`
}

func fromDashboardTopModels(rows []dbgen.ListDashboardTopModelsRow) []dashboardTopModelDTO {
	out := make([]dashboardTopModelDTO, len(rows))
	for i, r := range rows {
		out[i] = dashboardTopModelDTO{
			ModelCode:    r.ModelCode,
			RequestCount: r.RequestCount,
			TotalTokens:  r.TotalTokens,
			TotalCost:    r.TotalCost,
			TotalCredits: microToCredits(r.TotalCost),
		}
	}
	return out
}

type dashboardTopTenantDTO struct {
	TenantID     string  `json:"tenant_id"`
	RequestCount int64   `json:"request_count"`
	TotalTokens  int64   `json:"total_tokens"`
	TotalCost    int64   `json:"total_cost"`
	TotalCredits float64 `json:"total_credits"`
}

func fromDashboardTopTenants(rows []dbgen.ListDashboardTopTenantsRow) []dashboardTopTenantDTO {
	out := make([]dashboardTopTenantDTO, len(rows))
	for i, r := range rows {
		out[i] = dashboardTopTenantDTO{
			TenantID:     r.TenantID,
			RequestCount: r.RequestCount,
			TotalTokens:  r.TotalTokens,
			TotalCost:    r.TotalCost,
			TotalCredits: microToCredits(r.TotalCost),
		}
	}
	return out
}

// handleTenantListUsageLogs returns usage logs filtered for tenant visibility
// (no upstream/internal fields) with Chinese billing_status labels.
func (s *Server) handleTenantListUsageLogs(w http.ResponseWriter, r *http.Request) {
	limit := int32(20)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || parsed <= 0 {
			writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid limit")
			return
		}
		if parsed > 100 {
			parsed = 100
		}
		limit = int32(parsed)
	}

	offset := int32(0)
	if raw := r.URL.Query().Get("offset"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || parsed < 0 {
			writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid offset")
			return
		}
		offset = int32(parsed)
	}

	filters, ok := scopedUsageFilters(w, r)
	if !ok {
		return
	}

	countParams := dbgen.CountUsageLogsParams{
		TenantID:      filters.tenantID,
		UserID:        optionalTextValue(filters.userID),
		ModelCode:     optionalTextValue(filters.modelCode),
		RequestStatus: optionalTextValue(filters.requestStatus),
		RequestSource: optionalTextValue(filters.requestSource),
		DateFrom:      filters.dateFrom,
		DateTo:        filters.dateTo,
	}
	total, err := s.queries.CountUsageLogs(r.Context(), countParams)
	if err != nil {
		s.writeAdminServerError(w, r, "count usage logs failed", err)
		return
	}

	const statsSQL = `
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
	var stats usageStatsDTO
	statsRow := s.postgres.QueryRow(r.Context(), statsSQL,
		filters.tenantID,
		optionalTextValue(filters.userID),
		optionalTextValue(filters.modelCode),
		optionalTextValue(filters.requestStatus),
		optionalTextValue(filters.requestSource),
		filters.dateFrom,
		filters.dateTo,
	)
	if err := statsRow.Scan(
		&stats.TotalRequests,
		&stats.SuccessCount,
		&stats.FailedCount,
		&stats.TotalTokens,
		&stats.TotalCost,
		&stats.AvgLatencyMs,
	); err != nil {
		s.writeAdminServerError(w, r, "query usage stats failed", err)
		return
	}
	stats.TotalCredits = microToCredits(stats.TotalCost)

	rows, err := s.queries.ListUsageLogs(r.Context(), dbgen.ListUsageLogsParams{
		TenantID:      filters.tenantID,
		Limit:         limit,
		Offset:        offset,
		UserID:        optionalTextValue(filters.userID),
		ModelCode:     optionalTextValue(filters.modelCode),
		RequestStatus: optionalTextValue(filters.requestStatus),
		RequestSource: optionalTextValue(filters.requestSource),
		DateFrom:      filters.dateFrom,
		DateTo:        filters.dateTo,
	})
	if err != nil {
		s.writeAdminServerError(w, r, "list usage logs failed", err)
		return
	}
	writeOK(w, listUsageLogsForTenantResponse{
		Total:   total,
		Stats:   stats,
		Records: fromListUsageLogsForTenant(rows),
	})
}

func (s *Server) handleAdminListUsageSummary(w http.ResponseWriter, r *http.Request) {
	filters, ok := scopedUsageFilters(w, r)
	if !ok {
		return
	}
	since, ok := parseUsageSummarySince(w, r)
	if !ok {
		return
	}
	rows, err := s.queries.ListUsageSummary(r.Context(), dbgen.ListUsageSummaryParams{
		TenantID:      optionalTextValue(filters.tenantID),
		UserID:        optionalTextValue(filters.userID),
		ModelCode:     optionalTextValue(filters.modelCode),
		RequestStatus: optionalTextValue(filters.requestStatus),
		RequestSource: optionalTextValue(filters.requestSource),
		Since:         since,
	})
	if err != nil {
		s.writeAdminServerError(w, r, "list usage summary failed", err)
		return
	}
	writeOK(w, fromUsageSummary(rows))
}

func (s *Server) handleAdminListUsageUnitSummary(w http.ResponseWriter, r *http.Request) {
	filters, ok := scopedUsageFilters(w, r)
	if !ok {
		return
	}
	since, ok := parseUsageSummarySince(w, r)
	if !ok {
		return
	}
	rows, err := s.queries.ListUsageUnitSummary(r.Context(), dbgen.ListUsageUnitSummaryParams{
		TenantID:      optionalTextValue(filters.tenantID),
		UserID:        optionalTextValue(filters.userID),
		ModelCode:     optionalTextValue(filters.modelCode),
		RequestStatus: optionalTextValue(filters.requestStatus),
		RequestSource: optionalTextValue(filters.requestSource),
		Since:         since,
	})
	if err != nil {
		s.writeAdminServerError(w, r, "list usage unit summary failed", err)
		return
	}
	writeOK(w, fromUsageUnitSummary(rows))
}

// handleAdminListDailyTrend returns daily aggregated usage data for trend charts.
//
//	GET /api/v1/analytics/daily-trend?days=30
//
// Response: array of daily rows ordered by date ASC, each containing:
// date, request_count, success_count, failed_count, total_tokens,
// prompt_tokens, completion_tokens, provider_cost, platform_cost, user_cost, avg_latency_ms
func (s *Server) handleAdminListDailyTrend(w http.ResponseWriter, r *http.Request) {
	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if n, err := strconv.Atoi(d); err == nil && n > 0 && n <= 365 {
			days = n
		}
	}

	const q = `
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

	type dailyRow struct {
		Date             string  `json:"date"`
		RequestCount     int64   `json:"request_count"`
		SuccessCount     int64   `json:"success_count"`
		FailedCount      int64   `json:"failed_count"`
		TotalTokens      int64   `json:"total_tokens"`
		PromptTokens     int64   `json:"prompt_tokens"`
		CompletionTokens int64   `json:"completion_tokens"`
		ProviderCost     int64   `json:"provider_cost"`
		PlatformCost     int64   `json:"platform_cost"`
		UserCost         int64   `json:"user_cost"`
		ProviderCredits  float64 `json:"provider_credits"`
		PlatformCredits  float64 `json:"platform_credits"`
		UserCredits      float64 `json:"user_credits"`
		AvgLatencyMs     int64   `json:"avg_latency_ms"`
	}

	rows, err := s.postgres.Query(r.Context(), q, strconv.Itoa(days))
	if err != nil {
		s.writeAdminServerError(w, r, "list daily trend failed", err)
		return
	}
	defer rows.Close()

	out := make([]dailyRow, 0)
	for rows.Next() {
		var row dailyRow
		if err := rows.Scan(
			&row.Date, &row.RequestCount, &row.SuccessCount, &row.FailedCount,
			&row.TotalTokens, &row.PromptTokens, &row.CompletionTokens,
			&row.ProviderCost, &row.PlatformCost, &row.UserCost, &row.AvgLatencyMs,
		); err != nil {
			s.writeAdminServerError(w, r, "scan daily trend row failed", err)
			return
		}
		row.ProviderCredits = microToCredits(row.ProviderCost)
		row.PlatformCredits = microToCredits(row.PlatformCost)
		row.UserCredits = microToCredits(row.UserCost)
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		s.writeAdminServerError(w, r, "read daily trend rows failed", err)
		return
	}
	writeOK(w, out)
}
