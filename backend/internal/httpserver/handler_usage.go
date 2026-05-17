package httpserver

import (
	"net/http"
	"strconv"

	dbgen "uni-ai-api/backend/internal/db/gen"
)

func (s *Server) handleAdminListUsageLogs(w http.ResponseWriter, r *http.Request) {
	limit := int32(100)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || parsed <= 0 {
			writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid limit")
			return
		}
		if parsed > 500 {
			parsed = 500
		}
		limit = int32(parsed)
	}

	filters, ok := scopedUsageFilters(w, r)
	if !ok {
		return
	}
	rows, err := s.queries.ListUsageLogs(r.Context(), dbgen.ListUsageLogsParams{
		TenantID:      filters.tenantID,
		Limit:         limit,
		Offset:        0,
		UserID:        filters.userID,
		ModelCode:     filters.modelCode,
		RequestStatus: filters.requestStatus,
	})
	if err != nil {
		s.writeAdminServerError(w, r, "list usage logs failed", err)
		return
	}
	writeOK(w, fromListUsageLogs(rows))
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
		Since:         since,
	})
	if err != nil {
		s.writeAdminServerError(w, r, "list usage summary failed", err)
		return
	}
	writeOK(w, rows)
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
		Since:         since,
	})
	if err != nil {
		s.writeAdminServerError(w, r, "list usage unit summary failed", err)
		return
	}
	writeOK(w, rows)
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
		Date             string `json:"date"`
		RequestCount     int64  `json:"request_count"`
		SuccessCount     int64  `json:"success_count"`
		FailedCount      int64  `json:"failed_count"`
		TotalTokens      int64  `json:"total_tokens"`
		PromptTokens     int64  `json:"prompt_tokens"`
		CompletionTokens int64  `json:"completion_tokens"`
		ProviderCost     int64  `json:"provider_cost"`
		PlatformCost     int64  `json:"platform_cost"`
		UserCost         int64  `json:"user_cost"`
		AvgLatencyMs     int64  `json:"avg_latency_ms"`
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
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		s.writeAdminServerError(w, r, "read daily trend rows failed", err)
		return
	}
	writeOK(w, out)
}
