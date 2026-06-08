package console

import (
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"

	"xiaodou/unihub/ai-service/internal/credits"
	"xiaodou/unihub/ai-service/internal/domain"
	usagesvc "xiaodou/unihub/ai-service/internal/service/usage"
)

type usageStatsDTO struct {
	TotalRequests int64   `json:"total_requests"`
	SuccessCount  int64   `json:"success_count"`
	FailedCount   int64   `json:"failed_count"`
	TotalTokens   int64   `json:"total_tokens"`
	TotalCredits  float64 `json:"total_credits"` // 小数积分（micro÷10000）
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
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

func (s *Console) handleAdminListUsageLogs(w http.ResponseWriter, r *http.Request) {
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

	page, err := s.usageSvc.ListLogs(r.Context(), usageFilterFromScoped(filters), limit, offset)
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, listUsageLogsResponse{
		Total:   page.Total,
		Stats:   usageStatsFromDomain(page.Stats),
		Records: usageLogsFromDomain(page.Records),
	})
}

// ============================================================================
// Aggregation DTOs — wrap sqlc-generated rows to expose `_credits` floats only.
// 内部 BillingResult / DB 列保持 micro-credit int64；对外接口只暴露小数积分。
// ============================================================================

type dashboardSummaryDTO struct {
	TotalRequests         int64   `json:"total_requests"`
	SuccessfulRequests    int64   `json:"successful_requests"`
	FailedRequests        int64   `json:"failed_requests"`
	TotalTokens           int64   `json:"total_tokens"`
	TotalPromptTokens     int64   `json:"total_prompt_tokens"`
	TotalCompletionTokens int64   `json:"total_completion_tokens"`
	TotalProviderCredits  float64 `json:"total_provider_credits"`
	TotalPlatformCredits  float64 `json:"total_platform_credits"`
	TotalUserCredits      float64 `json:"total_user_credits"`
	AvgLatencyMs          float64 `json:"avg_latency_ms"`
}

type usageSummaryRowDTO struct {
	ModelCode             string  `json:"model_code"`
	RequestCount          int64   `json:"request_count"`
	TotalPromptTokens     int64   `json:"total_prompt_tokens"`
	TotalCompletionTokens int64   `json:"total_completion_tokens"`
	TotalTokens           int64   `json:"total_tokens"`
	TotalProviderCredits  float64 `json:"total_provider_credits"`
	TotalPlatformCredits  float64 `json:"total_platform_credits"`
	TotalUserCredits      float64 `json:"total_user_credits"`
	TotalQuotaCredits     float64 `json:"total_quota_credits"`
}

type usageUnitSummaryRowDTO struct {
	BillableUnitType     string  `json:"billable_unit_type"`
	RequestCount         int64   `json:"request_count"`
	TotalBillableUnits   int64   `json:"total_billable_units"`
	TotalProviderCredits float64 `json:"total_provider_credits"`
	TotalPlatformCredits float64 `json:"total_platform_credits"`
	TotalUserCredits     float64 `json:"total_user_credits"`
}

type userUsageSummaryDTO struct {
	RequestCount          int64   `json:"request_count"`
	SuccessRequests       int64   `json:"success_requests"`
	FailedRequests        int64   `json:"failed_requests"`
	TotalTokens           int64   `json:"total_tokens"`
	TotalPromptTokens     int64   `json:"total_prompt_tokens"`
	TotalCompletionTokens int64   `json:"total_completion_tokens"`
	TotalUserCredits      float64 `json:"total_user_credits"`
	AvgLatencyMs          float64 `json:"avg_latency_ms"`
}

type dashboardTopModelDTO struct {
	ModelCode    string  `json:"model_code"`
	RequestCount int64   `json:"request_count"`
	TotalTokens  int64   `json:"total_tokens"`
	TotalCredits float64 `json:"total_credits"`
}

type dashboardTopTenantDTO struct {
	TenantID     string  `json:"tenant_id"`
	RequestCount int64   `json:"request_count"`
	TotalTokens  int64   `json:"total_tokens"`
	TotalCredits float64 `json:"total_credits"`
}

// handleTenantListUsageLogs returns usage logs filtered for tenant visibility
// (no upstream/internal fields) with Chinese billing_status labels.
func (s *Console) handleTenantListUsageLogs(w http.ResponseWriter, r *http.Request) {
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

	page, err := s.usageSvc.ListLogs(r.Context(), usageFilterFromScoped(filters), limit, offset)
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, listUsageLogsForTenantResponse{
		Total:   page.Total,
		Stats:   usageStatsFromDomain(page.Stats),
		Records: usageLogsForTenantFromDomain(page.Records),
	})
}

func (s *Console) handleAdminListUsageSummary(w http.ResponseWriter, r *http.Request) {
	filters, ok := scopedUsageFilters(w, r)
	if !ok {
		return
	}
	since, ok := parseUsageSummarySince(w, r)
	if !ok {
		return
	}
	rows, err := s.usageSvc.Summary(r.Context(), summaryFilterFromScoped(filters, since))
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, usageSummaryFromDomain(rows))
}

func (s *Console) handleAdminListUsageUnitSummary(w http.ResponseWriter, r *http.Request) {
	filters, ok := scopedUsageFilters(w, r)
	if !ok {
		return
	}
	since, ok := parseUsageSummarySince(w, r)
	if !ok {
		return
	}
	rows, err := s.usageSvc.UnitSummary(r.Context(), summaryFilterFromScoped(filters, since))
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, usageUnitSummaryFromDomain(rows))
}

// handleAdminListDailyTrend returns daily aggregated usage data for trend charts.
//
//	GET /api/v1/analytics/daily-trend?days=30
//
// Response: array of daily rows ordered by date ASC, each containing:
// date, request_count, success_count, failed_count, total_tokens,
// prompt_tokens, completion_tokens, provider_cost, platform_cost, user_cost, avg_latency_ms
func (s *Console) handleAdminListDailyTrend(w http.ResponseWriter, r *http.Request) {
	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if n, err := strconv.Atoi(d); err == nil && n > 0 && n <= 365 {
			days = n
		}
	}

	type dailyRow struct {
		Date             string  `json:"date"`
		RequestCount     int64   `json:"request_count"`
		SuccessCount     int64   `json:"success_count"`
		FailedCount      int64   `json:"failed_count"`
		TotalTokens      int64   `json:"total_tokens"`
		PromptTokens     int64   `json:"prompt_tokens"`
		CompletionTokens int64   `json:"completion_tokens"`
		ProviderCredits  float64 `json:"provider_credits"`
		PlatformCredits  float64 `json:"platform_credits"`
		UserCredits      float64 `json:"user_credits"`
		AvgLatencyMs     int64   `json:"avg_latency_ms"`
	}

	trend, err := s.usageSvc.DailyTrend(r.Context(), days)
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}

	out := make([]dailyRow, 0, len(trend))
	for _, t := range trend {
		out = append(out, dailyRow{
			Date:             t.Date,
			RequestCount:     t.RequestCount,
			SuccessCount:     t.SuccessCount,
			FailedCount:      t.FailedCount,
			TotalTokens:      t.TotalTokens,
			PromptTokens:     t.PromptTokens,
			CompletionTokens: t.CompletionTokens,
			ProviderCredits:  credits.MicroToCredits(t.ProviderCostMicro),
			PlatformCredits:  credits.MicroToCredits(t.PlatformCostMicro),
			UserCredits:      credits.MicroToCredits(t.UserCostMicro),
			AvgLatencyMs:     t.AvgLatencyMs,
		})
	}
	writeOK(w, out)
}

// ===========================================================================
// usage domain → wire DTO mappers + scoped-filter adapters（保契约逐字段一致）
// ===========================================================================

func usageFilterFromScoped(f usageFilters) domain.UsageFilter {
	return domain.UsageFilter{
		TenantID:      f.tenantID,
		UserID:        f.userID,
		ModelCode:     f.modelCode,
		RequestStatus: f.requestStatus,
		RequestSource: f.requestSource,
		DateFrom:      pgTimestamptzToPtr(f.dateFrom),
		DateTo:        pgTimestamptzToPtr(f.dateTo),
	}
}

func summaryFilterFromScoped(f usageFilters, since pgtype.Timestamptz) usagesvc.SummaryFilter {
	return usagesvc.SummaryFilter{
		TenantID:      f.tenantID,
		UserID:        f.userID,
		ModelCode:     f.modelCode,
		RequestStatus: f.requestStatus,
		RequestSource: f.requestSource,
		Since:         pgTimestamptzToPtr(since),
	}
}

func usageStatsFromDomain(s domain.UsageStats) usageStatsDTO {
	return usageStatsDTO{
		TotalRequests: s.TotalRequests,
		SuccessCount:  s.SuccessCount,
		FailedCount:   s.FailedCount,
		TotalTokens:   s.TotalTokens,
		TotalCredits:  credits.MicroToCredits(s.TotalCostMicro),
		AvgLatencyMs:  s.AvgLatencyMs,
	}
}

func usageLogFromDomain(l domain.UsageLog) usageLogDTO {
	return usageLogDTO{
		ID:                   pgUUIDFromString(l.ID),
		RequestID:            l.RequestID,
		TraceID:              optionalTextValue(l.TraceID),
		ApiKeyID:             pgUUIDFromString(l.APIKeyID),
		KeyOwnerType:         l.KeyOwnerType,
		AuthMethod:           l.AuthMethod,
		RequestSource:        l.RequestSource,
		TenantID:             l.TenantID,
		UserID:               optionalTextValue(l.UserID),
		ExternalUserID:       optionalTextValue(l.ExternalUserID),
		ModelID:              pgUUIDFromString(l.ModelID),
		ModelCode:            l.ModelCode,
		CapabilityType:       l.CapabilityType,
		ModelRouteID:         pgUUIDFromString(l.ModelRouteID),
		UpstreamDeploymentID: pgUUIDFromString(l.UpstreamDeploymentID),
		EndpointID:           pgUUIDFromString(l.EndpointID),
		ProviderCode:         optionalTextValue(l.ProviderCode),
		UpstreamModel:        optionalTextValue(l.UpstreamModel),
		ConversationID:       optionalTextValue(l.ConversationID),
		Stream:               l.Stream,
		PromptTokens:         l.PromptTokens,
		CompletionTokens:     l.CompletionTokens,
		TotalTokens:          l.TotalTokens,
		BillableUnitType:     l.BillableUnitType,
		BillableUnits:        l.BillableUnits,
		ProviderCredits:      credits.MicroToCredits(l.ProviderCostMicro),
		PlatformCredits:      credits.MicroToCredits(l.PlatformCostMicro),
		UserCredits:          credits.MicroToCredits(l.UserCostMicro),
		ApiKeyQuotaCredits:   credits.MicroToCredits(l.APIKeyQuotaCostMicro),
		BillingStatus:        l.BillingStatus,
		RequestStatus:        l.RequestStatus,
		HttpStatus:           int32PtrToPg(l.HTTPStatus),
		UpstreamStatus:       int32PtrToPg(l.UpstreamStatus),
		LatencyMs:            int32PtrToPg(l.LatencyMs),
		FirstTokenLatencyMs:  int32PtrToPg(l.FirstTokenLatencyMs),
		ErrorCode:            optionalTextValue(l.ErrorCode),
		ErrorMessage:         optionalTextValue(l.ErrorMessage),
		UsageEstimated:       l.UsageEstimated,
		TokenUsageSource:     l.TokenUsageSource,
		CreatedAt:            timeToMillisPtr(l.CreatedAt),
	}
}

func usageLogsFromDomain(logs []domain.UsageLog) []usageLogDTO {
	out := make([]usageLogDTO, len(logs))
	for i, l := range logs {
		out[i] = usageLogFromDomain(l)
	}
	return out
}

func usageLogForTenantFromDomain(l domain.UsageLog) usageLogForTenantDTO {
	return usageLogForTenantDTO{
		ID:                  pgUUIDFromString(l.ID),
		RequestID:           l.RequestID,
		RequestSource:       l.RequestSource,
		TenantID:            l.TenantID,
		UserID:              optionalTextValue(l.UserID),
		ExternalUserID:      optionalTextValue(l.ExternalUserID),
		ModelCode:           l.ModelCode,
		CapabilityType:      l.CapabilityType,
		Stream:              l.Stream,
		PromptTokens:        l.PromptTokens,
		CompletionTokens:    l.CompletionTokens,
		TotalTokens:         l.TotalTokens,
		PlatformCredits:     credits.MicroToCredits(l.PlatformCostMicro),
		UserCredits:         credits.MicroToCredits(l.UserCostMicro),
		BillingStatus:       l.BillingStatus,
		BillingStatusLabel:  billingStatusLabel(l.BillingStatus),
		RequestStatus:       l.RequestStatus,
		HttpStatus:          int32PtrToPg(l.HTTPStatus),
		LatencyMs:           int32PtrToPg(l.LatencyMs),
		FirstTokenLatencyMs: int32PtrToPg(l.FirstTokenLatencyMs),
		ErrorCode:           optionalTextValue(l.ErrorCode),
		ErrorMessage:        optionalTextValue(l.ErrorMessage),
		CreatedAt:           timeToMillisPtr(l.CreatedAt),
	}
}

func usageLogsForTenantFromDomain(logs []domain.UsageLog) []usageLogForTenantDTO {
	out := make([]usageLogForTenantDTO, len(logs))
	for i, l := range logs {
		out[i] = usageLogForTenantFromDomain(l)
	}
	return out
}

func usageSummaryFromDomain(rows []domain.UsageSummaryRow) []usageSummaryRowDTO {
	out := make([]usageSummaryRowDTO, len(rows))
	for i, r := range rows {
		out[i] = usageSummaryRowDTO{
			ModelCode:             r.ModelCode,
			RequestCount:          r.RequestCount,
			TotalPromptTokens:     r.TotalPromptTokens,
			TotalCompletionTokens: r.TotalCompletionTokens,
			TotalTokens:           r.TotalTokens,
			TotalProviderCredits:  credits.MicroToCredits(r.TotalProviderCostMicro),
			TotalPlatformCredits:  credits.MicroToCredits(r.TotalPlatformCostMicro),
			TotalUserCredits:      credits.MicroToCredits(r.TotalUserCostMicro),
			TotalQuotaCredits:     credits.MicroToCredits(r.TotalQuotaCostMicro),
		}
	}
	return out
}

func usageUnitSummaryFromDomain(rows []domain.UsageUnitSummaryRow) []usageUnitSummaryRowDTO {
	out := make([]usageUnitSummaryRowDTO, len(rows))
	for i, r := range rows {
		out[i] = usageUnitSummaryRowDTO{
			BillableUnitType:     r.BillableUnitType,
			RequestCount:         r.RequestCount,
			TotalBillableUnits:   r.TotalBillableUnits,
			TotalProviderCredits: credits.MicroToCredits(r.TotalProviderCostMicro),
			TotalPlatformCredits: credits.MicroToCredits(r.TotalPlatformCostMicro),
			TotalUserCredits:     credits.MicroToCredits(r.TotalUserCostMicro),
		}
	}
	return out
}

func userUsageSummaryFromDomain(s domain.UserUsageSummary) userUsageSummaryDTO {
	return userUsageSummaryDTO{
		RequestCount:          s.RequestCount,
		SuccessRequests:       s.SuccessRequests,
		FailedRequests:        s.FailedRequests,
		TotalTokens:           s.TotalTokens,
		TotalPromptTokens:     s.TotalPromptTokens,
		TotalCompletionTokens: s.TotalCompletionTokens,
		TotalUserCredits:      credits.MicroToCredits(s.TotalUserCostMicro),
		AvgLatencyMs:          s.AvgLatencyMs,
	}
}
