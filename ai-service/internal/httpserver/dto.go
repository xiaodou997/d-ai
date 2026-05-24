package httpserver

// dto.go — Phase 3: explicit DTO layer for admin handler responses.
//
// All sqlc-generated structs that contain pgtype.Timestamptz or []byte JSONB
// fields are wrapped here. Timestamps are converted to Unix milliseconds
// (int64, nil = absent), and JSONB bytes become json.RawMessage so the wire
// format is actual JSON rather than RFC3339 strings or base64.
//
// 单位约定（自 2026-05 计费精度升级 + API 统一积分单位重构起）：
//   - 数据库 / 内部计算使用「微积分」(micro-credits, int64)，
//     1 积分 = 10000 微积分 = 1 分人民币。
//   - API 边界（DTO 响应 + 请求）统一使用「积分」(float64)，
//     后端在 DTO 层做 ÷10000 / ×10000 转换。
//   - 前端只看到「积分」，不再需要感知微积分。
//   - usage DTO、价格 DTO、配额 DTO 都通过 internal/credits 在边界转换单位。

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"xiaodou/uni-ai-api/internal/credits"
	dbgen "xiaodou/uni-ai-api/internal/db/gen"
	"xiaodou/uni-ai-api/internal/domain"
)

// ---------------------------------------------------------------------------
// Price DTO helpers
// ---------------------------------------------------------------------------

type resolutionPriceDTO struct {
	Resolution   string  `json:"resolution"`
	PriceCredits float64 `json:"price_credits"`
}

func resolutionPricesMicroToCredits(raw []byte) []resolutionPriceDTO {
	if len(raw) == 0 || string(raw) == "null" {
		return []resolutionPriceDTO{}
	}
	var stored []domain.ResolutionCreditPrice
	if err := json.Unmarshal(raw, &stored); err != nil {
		return []resolutionPriceDTO{}
	}
	out := make([]resolutionPriceDTO, 0, len(stored))
	for _, item := range stored {
		out = append(out, resolutionPriceDTO{
			Resolution:   item.Resolution,
			PriceCredits: credits.MicroToCredits(item.Price),
		})
	}
	return out
}

func resolutionPricesCreditsToMicro(entries []resolutionPriceDTO, fieldName string) ([]byte, string) {
	if len(entries) == 0 {
		return []byte("[]"), ""
	}
	seen := make(map[string]struct{}, len(entries))
	stored := make([]domain.ResolutionCreditPrice, 0, len(entries))
	for i, entry := range entries {
		resolution := strings.TrimSpace(entry.Resolution)
		if resolution == "" {
			return nil, fmt.Sprintf("%s[%d].resolution is required", fieldName, i)
		}
		if _, ok := seen[resolution]; ok {
			return nil, fmt.Sprintf("%s contains duplicate resolution %q", fieldName, resolution)
		}
		seen[resolution] = struct{}{}
		if message := validateCreditAmount(fmt.Sprintf("%s[%d].price_credits", fieldName, i), entry.PriceCredits); message != "" {
			return nil, message
		}
		stored = append(stored, domain.ResolutionCreditPrice{
			Resolution: resolution,
			Price:      credits.CreditsToMicro(entry.PriceCredits),
		})
	}
	out, err := json.Marshal(stored)
	if err != nil {
		return nil, fmt.Sprintf("%s is invalid", fieldName)
	}
	return out, ""
}

func validateCreditAmount(name string, value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Sprintf("%s must be a finite credit value", name)
	}
	if value < 0 {
		return fmt.Sprintf("%s must be a non-negative credit value", name)
	}
	scaled := value * float64(credits.MicroPerCredit)
	if math.Abs(scaled-math.Round(scaled)) > 1e-7 {
		return fmt.Sprintf("%s must have at most 4 decimal places", name)
	}
	if value > maxCreditsPerField {
		return fmt.Sprintf("%s exceeds maximum allowed value (%.0f credits)", name, maxCreditsPerField)
	}
	return ""
}

func validateOptionalCreditAmount(name string, value *float64) string {
	if value == nil {
		return ""
	}
	return validateCreditAmount(name, *value)
}

// ---------------------------------------------------------------------------
// Timestamp / JSONB helpers
// ---------------------------------------------------------------------------

// millis converts a pgtype.Timestamptz to Unix milliseconds as *int64.
// Returns nil when the value is NULL or zero.
func millis(ts pgtype.Timestamptz) *int64 {
	if !ts.Valid || ts.Time.IsZero() {
		return nil
	}
	v := ts.Time.UnixMilli()
	return &v
}

// rawJSON converts a []byte JSONB column to json.RawMessage.
// Returns JSON null when the slice is empty or invalid JSON.
func rawJSON(b []byte) json.RawMessage {
	if len(b) == 0 || !json.Valid(b) {
		return json.RawMessage("null")
	}
	return json.RawMessage(b)
}

// ---------------------------------------------------------------------------
// Provider DTOs
// ---------------------------------------------------------------------------

type providerDTO struct {
	ID        string          `json:"id"`
	Code      string          `json:"code"`
	Name      string          `json:"name"`
	Config    json.RawMessage `json:"config"`
	Status    string          `json:"status"`
	CreatedAt *int64          `json:"created_at"`
	UpdatedAt *int64          `json:"updated_at"`
}

func fromProvider(r dbgen.AiProvider) providerDTO {
	return providerDTO{
		ID:        uuidStr(r.ID),
		Code:      r.Code,
		Name:      r.Name,
		Config:    rawJSON(r.Config),
		Status:    r.Status,
		CreatedAt: millis(r.CreatedAt),
		UpdatedAt: millis(r.UpdatedAt),
	}
}

func fromProviders(rows []dbgen.AiProvider) []providerDTO {
	out := make([]providerDTO, len(rows))
	for i, r := range rows {
		out[i] = fromProvider(r)
	}
	return out
}

// ---------------------------------------------------------------------------
// Provider endpoint DTOs
// ---------------------------------------------------------------------------

type providerEndpointDTO struct {
	ID              pgtype.UUID     `json:"id"`
	ProviderID      pgtype.UUID     `json:"provider_id"`
	Name            string          `json:"name"`
	BaseUrl         string          `json:"base_url"`
	ExtraHeaders    json.RawMessage `json:"extra_headers"`
	Weight          int32           `json:"weight"`
	TimeoutMs       int32           `json:"timeout_ms"`
	DefaultProtocol string          `json:"default_protocol"`
	Status          string          `json:"status"`
	CreatedAt       *int64          `json:"created_at"`
	UpdatedAt       *int64          `json:"updated_at"`
}

func fromCreateProviderEndpoint(r dbgen.CreateProviderEndpointRow) providerEndpointDTO {
	return providerEndpointDTO{
		ID:              r.ID,
		ProviderID:      r.ProviderID,
		Name:            r.Name,
		BaseUrl:         r.BaseUrl,
		ExtraHeaders:    rawJSON(r.ExtraHeaders),
		Weight:          r.Weight,
		TimeoutMs:       r.TimeoutMs,
		DefaultProtocol: r.DefaultProtocol,
		Status:          r.Status,
		CreatedAt:       millis(r.CreatedAt),
		UpdatedAt:       millis(r.UpdatedAt),
	}
}

type listProviderEndpointDTO struct {
	ID              pgtype.UUID     `json:"id"`
	ProviderID      pgtype.UUID     `json:"provider_id"`
	Name            string          `json:"name"`
	BaseUrl         string          `json:"base_url"`
	ExtraHeaders    json.RawMessage `json:"extra_headers"`
	Weight          int32           `json:"weight"`
	TimeoutMs       int32           `json:"timeout_ms"`
	DefaultProtocol string          `json:"default_protocol"`
	Status          string          `json:"status"`
	CreatedAt       *int64          `json:"created_at"`
	UpdatedAt       *int64          `json:"updated_at"`
}

func fromListProviderEndpoint(r dbgen.ListProviderEndpointsRow) listProviderEndpointDTO {
	return listProviderEndpointDTO{
		ID:              r.ID,
		ProviderID:      r.ProviderID,
		Name:            r.Name,
		BaseUrl:         r.BaseUrl,
		ExtraHeaders:    rawJSON(r.ExtraHeaders),
		Weight:          r.Weight,
		TimeoutMs:       r.TimeoutMs,
		DefaultProtocol: r.DefaultProtocol,
		Status:          r.Status,
		CreatedAt:       millis(r.CreatedAt),
		UpdatedAt:       millis(r.UpdatedAt),
	}
}

func fromListProviderEndpoints(rows []dbgen.ListProviderEndpointsRow) []listProviderEndpointDTO {
	out := make([]listProviderEndpointDTO, len(rows))
	for i, r := range rows {
		out[i] = fromListProviderEndpoint(r)
	}
	return out
}

// ---------------------------------------------------------------------------
// Model DTOs
// ---------------------------------------------------------------------------

type modelDTO struct {
	ID                     pgtype.UUID `json:"id"`
	ModelCode              string      `json:"model_code"`
	CapabilityType         string      `json:"capability_type"`
	ContextWindow          pgtype.Int4 `json:"context_window"`
	DefaultMaxOutputTokens int32       `json:"default_max_output_tokens"`
	MaxOutputTokens        pgtype.Int4 `json:"max_output_tokens"`
	Status                 string      `json:"status"`
	CreatedAt              *int64      `json:"created_at"`
	UpdatedAt              *int64      `json:"updated_at"`
}

func fromModel(r dbgen.AiModel) modelDTO {
	return modelDTO{
		ID:                     r.ID,
		ModelCode:              r.ModelCode,
		CapabilityType:         r.CapabilityType,
		ContextWindow:          r.ContextWindow,
		DefaultMaxOutputTokens: r.DefaultMaxOutputTokens,
		MaxOutputTokens:        r.MaxOutputTokens,
		Status:                 r.Status,
		CreatedAt:              millis(r.CreatedAt),
		UpdatedAt:              millis(r.UpdatedAt),
	}
}

func fromCreateModelRow(r dbgen.CreateModelRow) modelDTO {
	return modelDTO{
		ID:                     r.ID,
		ModelCode:              r.ModelCode,
		CapabilityType:         r.CapabilityType,
		ContextWindow:          r.ContextWindow,
		DefaultMaxOutputTokens: r.DefaultMaxOutputTokens,
		MaxOutputTokens:        r.MaxOutputTokens,
		Status:                 r.Status,
		CreatedAt:              millis(r.CreatedAt),
		UpdatedAt:              millis(r.UpdatedAt),
	}
}

func fromUpdateModelRow(r dbgen.UpdateModelRow) modelDTO {
	return modelDTO{
		ID:                     r.ID,
		ModelCode:              r.ModelCode,
		CapabilityType:         r.CapabilityType,
		ContextWindow:          r.ContextWindow,
		DefaultMaxOutputTokens: r.DefaultMaxOutputTokens,
		MaxOutputTokens:        r.MaxOutputTokens,
		Status:                 r.Status,
		CreatedAt:              millis(r.CreatedAt),
		UpdatedAt:              millis(r.UpdatedAt),
	}
}

func fromUpdateModelStatusRow(r dbgen.UpdateModelStatusRow) modelDTO {
	return modelDTO{
		ID:                     r.ID,
		ModelCode:              r.ModelCode,
		CapabilityType:         r.CapabilityType,
		ContextWindow:          r.ContextWindow,
		DefaultMaxOutputTokens: r.DefaultMaxOutputTokens,
		MaxOutputTokens:        r.MaxOutputTokens,
		Status:                 r.Status,
		CreatedAt:              millis(r.CreatedAt),
		UpdatedAt:              millis(r.UpdatedAt),
	}
}

func fromModels(rows []dbgen.ListAdminModelsRow) []modelDTO {
	out := make([]modelDTO, len(rows))
	for i, r := range rows {
		out[i] = modelDTO{
			ID:                     r.ID,
			ModelCode:              r.ModelCode,
			CapabilityType:         r.CapabilityType,
			ContextWindow:          r.ContextWindow,
			DefaultMaxOutputTokens: r.DefaultMaxOutputTokens,
			MaxOutputTokens:        r.MaxOutputTokens,
			Status:                 r.Status,
			CreatedAt:              millis(r.CreatedAt),
			UpdatedAt:              millis(r.UpdatedAt),
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Model price DTOs
// ---------------------------------------------------------------------------

type modelPriceDTO struct {
	ID                             pgtype.UUID          `json:"id"`
	ModelID                        pgtype.UUID          `json:"model_id"`
	InputPricePer1mCredits         float64              `json:"input_price_per_1m_credits"`  // 积分
	OutputPricePer1mCredits        float64              `json:"output_price_per_1m_credits"` // 积分
	ImagePrices                    []resolutionPriceDTO `json:"image_prices"`
	VideoPrices                    []resolutionPriceDTO `json:"video_prices"`
	AudioTtsPricePer1mCharsCredits float64              `json:"audio_tts_price_per_1m_chars_credits"` // 积分
	AudioSttPricePerMinuteCredits  float64              `json:"audio_stt_price_per_minute_credits"`   // 积分
	CreatedAt                      *int64               `json:"created_at"`
	UpdatedAt                      *int64               `json:"updated_at"`
}

func fromGetModelPrice(r dbgen.GetModelPriceRow) modelPriceDTO {
	return modelPriceDTO{
		ID:                             r.ID,
		ModelID:                        r.ModelID,
		InputPricePer1mCredits:         credits.MicroToCredits(r.InputPricePer1m),
		OutputPricePer1mCredits:        credits.MicroToCredits(r.OutputPricePer1m),
		ImagePrices:                    resolutionPricesMicroToCredits(r.ImagePrices),
		VideoPrices:                    resolutionPricesMicroToCredits(r.VideoPrices),
		AudioTtsPricePer1mCharsCredits: credits.MicroToCredits(r.AudioTtsPricePer1mChars),
		AudioSttPricePerMinuteCredits:  credits.MicroToCredits(r.AudioSttPricePerMinute),
		CreatedAt:                      millis(r.CreatedAt),
		UpdatedAt:                      millis(r.UpdatedAt),
	}
}

func fromUpsertModelPrice(r dbgen.UpsertModelPriceRow) modelPriceDTO {
	return modelPriceDTO{
		ID:                             r.ID,
		ModelID:                        r.ModelID,
		InputPricePer1mCredits:         credits.MicroToCredits(r.InputPricePer1m),
		OutputPricePer1mCredits:        credits.MicroToCredits(r.OutputPricePer1m),
		ImagePrices:                    resolutionPricesMicroToCredits(r.ImagePrices),
		VideoPrices:                    resolutionPricesMicroToCredits(r.VideoPrices),
		AudioTtsPricePer1mCharsCredits: credits.MicroToCredits(r.AudioTtsPricePer1mChars),
		AudioSttPricePerMinuteCredits:  credits.MicroToCredits(r.AudioSttPricePerMinute),
		CreatedAt:                      millis(r.CreatedAt),
		UpdatedAt:                      millis(r.UpdatedAt),
	}
}

// ---------------------------------------------------------------------------
// Tenant model price override DTOs
// ---------------------------------------------------------------------------

type tenantModelPriceDTO struct {
	ID                             pgtype.UUID          `json:"id"`
	TenantID                       string               `json:"tenant_id"`
	ModelID                        pgtype.UUID          `json:"model_id"`
	InputPricePer1mCredits         float64              `json:"input_price_per_1m_credits"`  // 积分
	OutputPricePer1mCredits        float64              `json:"output_price_per_1m_credits"` // 积分
	ImagePrices                    []resolutionPriceDTO `json:"image_prices"`
	VideoPrices                    []resolutionPriceDTO `json:"video_prices"`
	AudioTtsPricePer1mCharsCredits float64              `json:"audio_tts_price_per_1m_chars_credits"` // 积分
	AudioSttPricePerMinuteCredits  float64              `json:"audio_stt_price_per_minute_credits"`   // 积分
	CreatedBy                      pgtype.Text          `json:"created_by"`
	CreatedAt                      *int64               `json:"created_at"`
	UpdatedAt                      *int64               `json:"updated_at"`
	// Extra fields from list query
	ModelCode      string `json:"model_code,omitempty"`
	CapabilityType string `json:"capability_type,omitempty"`
}

func fromGetTenantModelPriceOverride(r dbgen.GetTenantModelPriceOverrideRow) tenantModelPriceDTO {
	return tenantModelPriceDTO{
		ID:                             r.ID,
		TenantID:                       r.TenantID,
		ModelID:                        r.ModelID,
		InputPricePer1mCredits:         credits.MicroToCredits(r.InputPricePer1m),
		OutputPricePer1mCredits:        credits.MicroToCredits(r.OutputPricePer1m),
		ImagePrices:                    resolutionPricesMicroToCredits(r.ImagePrices),
		VideoPrices:                    resolutionPricesMicroToCredits(r.VideoPrices),
		AudioTtsPricePer1mCharsCredits: credits.MicroToCredits(r.AudioTtsPricePer1mChars),
		AudioSttPricePerMinuteCredits:  credits.MicroToCredits(r.AudioSttPricePerMinute),
		CreatedBy:                      r.CreatedBy,
		CreatedAt:                      millis(r.CreatedAt),
		UpdatedAt:                      millis(r.UpdatedAt),
	}
}

func fromUpsertTenantModelPriceOverride(r dbgen.UpsertTenantModelPriceOverrideRow) tenantModelPriceDTO {
	return tenantModelPriceDTO{
		ID:                             r.ID,
		TenantID:                       r.TenantID,
		ModelID:                        r.ModelID,
		InputPricePer1mCredits:         credits.MicroToCredits(r.InputPricePer1m),
		OutputPricePer1mCredits:        credits.MicroToCredits(r.OutputPricePer1m),
		ImagePrices:                    resolutionPricesMicroToCredits(r.ImagePrices),
		VideoPrices:                    resolutionPricesMicroToCredits(r.VideoPrices),
		AudioTtsPricePer1mCharsCredits: credits.MicroToCredits(r.AudioTtsPricePer1mChars),
		AudioSttPricePerMinuteCredits:  credits.MicroToCredits(r.AudioSttPricePerMinute),
		CreatedBy:                      r.CreatedBy,
		CreatedAt:                      millis(r.CreatedAt),
		UpdatedAt:                      millis(r.UpdatedAt),
	}
}

func fromListTenantModelPriceOverride(r dbgen.ListTenantModelPriceOverridesRow) tenantModelPriceDTO {
	return tenantModelPriceDTO{
		ID:                             r.ID,
		TenantID:                       r.TenantID,
		ModelID:                        r.ModelID,
		InputPricePer1mCredits:         credits.MicroToCredits(r.InputPricePer1m),
		OutputPricePer1mCredits:        credits.MicroToCredits(r.OutputPricePer1m),
		ImagePrices:                    resolutionPricesMicroToCredits(r.ImagePrices),
		VideoPrices:                    resolutionPricesMicroToCredits(r.VideoPrices),
		AudioTtsPricePer1mCharsCredits: credits.MicroToCredits(r.AudioTtsPricePer1mChars),
		AudioSttPricePerMinuteCredits:  credits.MicroToCredits(r.AudioSttPricePerMinute),
		CreatedBy:                      r.CreatedBy,
		CreatedAt:                      millis(r.CreatedAt),
		UpdatedAt:                      millis(r.UpdatedAt),
		ModelCode:                      r.ModelCode,
		CapabilityType:                 r.CapabilityType,
	}
}

func fromListTenantModelPriceOverrides(rows []dbgen.ListTenantModelPriceOverridesRow) []tenantModelPriceDTO {
	out := make([]tenantModelPriceDTO, len(rows))
	for i, r := range rows {
		out[i] = fromListTenantModelPriceOverride(r)
	}
	return out
}

// ---------------------------------------------------------------------------
// Tenant user price DTOs
// ---------------------------------------------------------------------------

type tenantUserPriceDTO struct {
	ID                             pgtype.UUID          `json:"id"`
	TenantID                       string               `json:"tenant_id"`
	ModelID                        pgtype.UUID          `json:"model_id"`
	InputPricePer1mCredits         float64              `json:"input_price_per_1m_credits"`  // 积分
	OutputPricePer1mCredits        float64              `json:"output_price_per_1m_credits"` // 积分
	ImagePrices                    []resolutionPriceDTO `json:"image_prices"`
	VideoPrices                    []resolutionPriceDTO `json:"video_prices"`
	AudioTtsPricePer1mCharsCredits float64              `json:"audio_tts_price_per_1m_chars_credits"` // 积分
	AudioSttPricePerMinuteCredits  float64              `json:"audio_stt_price_per_minute_credits"`   // 积分
	CreatedBy                      pgtype.Text          `json:"created_by"`
	CreatedAt                      *int64               `json:"created_at"`
	UpdatedAt                      *int64               `json:"updated_at"`
	// Extra fields from list query
	ModelCode      string `json:"model_code,omitempty"`
	CapabilityType string `json:"capability_type,omitempty"`
}

func fromGetTenantUserPrice(r dbgen.GetTenantUserPriceRow) tenantUserPriceDTO {
	return tenantUserPriceDTO{
		ID:                             r.ID,
		TenantID:                       r.TenantID,
		ModelID:                        r.ModelID,
		InputPricePer1mCredits:         credits.MicroToCredits(r.InputPricePer1m),
		OutputPricePer1mCredits:        credits.MicroToCredits(r.OutputPricePer1m),
		ImagePrices:                    resolutionPricesMicroToCredits(r.ImagePrices),
		VideoPrices:                    resolutionPricesMicroToCredits(r.VideoPrices),
		AudioTtsPricePer1mCharsCredits: credits.MicroToCredits(r.AudioTtsPricePer1mChars),
		AudioSttPricePerMinuteCredits:  credits.MicroToCredits(r.AudioSttPricePerMinute),
		CreatedBy:                      r.CreatedBy,
		CreatedAt:                      millis(r.CreatedAt),
		UpdatedAt:                      millis(r.UpdatedAt),
	}
}

func fromUpsertTenantUserPrice(r dbgen.UpsertTenantUserPriceRow) tenantUserPriceDTO {
	return tenantUserPriceDTO{
		ID:                             r.ID,
		TenantID:                       r.TenantID,
		ModelID:                        r.ModelID,
		InputPricePer1mCredits:         credits.MicroToCredits(r.InputPricePer1m),
		OutputPricePer1mCredits:        credits.MicroToCredits(r.OutputPricePer1m),
		ImagePrices:                    resolutionPricesMicroToCredits(r.ImagePrices),
		VideoPrices:                    resolutionPricesMicroToCredits(r.VideoPrices),
		AudioTtsPricePer1mCharsCredits: credits.MicroToCredits(r.AudioTtsPricePer1mChars),
		AudioSttPricePerMinuteCredits:  credits.MicroToCredits(r.AudioSttPricePerMinute),
		CreatedBy:                      r.CreatedBy,
		CreatedAt:                      millis(r.CreatedAt),
		UpdatedAt:                      millis(r.UpdatedAt),
	}
}

func fromListTenantUserPrice(r dbgen.ListTenantUserPricesRow) tenantUserPriceDTO {
	return tenantUserPriceDTO{
		ID:                             r.ID,
		TenantID:                       r.TenantID,
		ModelID:                        r.ModelID,
		InputPricePer1mCredits:         credits.MicroToCredits(r.InputPricePer1m),
		OutputPricePer1mCredits:        credits.MicroToCredits(r.OutputPricePer1m),
		ImagePrices:                    resolutionPricesMicroToCredits(r.ImagePrices),
		VideoPrices:                    resolutionPricesMicroToCredits(r.VideoPrices),
		AudioTtsPricePer1mCharsCredits: credits.MicroToCredits(r.AudioTtsPricePer1mChars),
		AudioSttPricePerMinuteCredits:  credits.MicroToCredits(r.AudioSttPricePerMinute),
		CreatedBy:                      r.CreatedBy,
		CreatedAt:                      millis(r.CreatedAt),
		UpdatedAt:                      millis(r.UpdatedAt),
		ModelCode:                      r.ModelCode,
		CapabilityType:                 r.CapabilityType,
	}
}

func fromListTenantUserPrices(rows []dbgen.ListTenantUserPricesRow) []tenantUserPriceDTO {
	out := make([]tenantUserPriceDTO, len(rows))
	for i, r := range rows {
		out[i] = fromListTenantUserPrice(r)
	}
	return out
}

// ---------------------------------------------------------------------------
// Model route DTOs
// ---------------------------------------------------------------------------

type modelRouteDTO struct {
	ID                   pgtype.UUID `json:"id"`
	ModelID              pgtype.UUID `json:"model_id"`
	UpstreamDeploymentID pgtype.UUID `json:"upstream_deployment_id"`
	Priority             int32       `json:"priority"`
	Weight               int32       `json:"weight"`
	SupportsStream       bool        `json:"supports_stream"`
	Status               string      `json:"status"`
	CreatedAt            *int64      `json:"created_at"`
	UpdatedAt            *int64      `json:"updated_at"`
}

func fromModelRoute(r dbgen.AiModelRoute) modelRouteDTO {
	return modelRouteDTO{
		ID:                   r.ID,
		ModelID:              r.ModelID,
		UpstreamDeploymentID: r.UpstreamDeploymentID,
		Priority:             r.Priority,
		Weight:               r.Weight,
		SupportsStream:       r.SupportsStream,
		Status:               r.Status,
		CreatedAt:            millis(r.CreatedAt),
		UpdatedAt:            millis(r.UpdatedAt),
	}
}

type listModelRouteDTO struct {
	ID                     pgtype.UUID `json:"id"`
	ModelID                pgtype.UUID `json:"model_id"`
	UpstreamDeploymentID   pgtype.UUID `json:"upstream_deployment_id"`
	Priority               int32       `json:"priority"`
	Weight                 int32       `json:"weight"`
	SupportsStream         bool        `json:"supports_stream"`
	Status                 string      `json:"status"`
	CreatedAt              *int64      `json:"created_at"`
	UpdatedAt              *int64      `json:"updated_at"`
	UpstreamDeploymentName string      `json:"upstream_deployment_name"`
	UpstreamModel          string      `json:"upstream_model"`
	CapabilityType         string      `json:"capability_type"`
	UpstreamProtocol       string      `json:"upstream_protocol"`
	HealthStatus           string      `json:"health_status"`
	CredentialSource       string      `json:"credential_source"`
	EndpointID             pgtype.UUID `json:"endpoint_id"`
	EndpointName           string      `json:"endpoint_name"`
	BaseUrl                string      `json:"base_url"`
	ProviderID             pgtype.UUID `json:"provider_id"`
	ProviderCode           string      `json:"provider_code"`
	ProviderName           string      `json:"provider_name"`
	PoolID                 pgtype.UUID `json:"pool_id"`
	PoolName               string      `json:"pool_name"`
	FixedProviderType      string      `json:"fixed_provider_type"`
}

func fromListModelRoute(r dbgen.ListModelRoutesRow) listModelRouteDTO {
	return listModelRouteDTO{
		ID:                     r.ID,
		ModelID:                r.ModelID,
		UpstreamDeploymentID:   r.UpstreamDeploymentID,
		Priority:               r.Priority,
		Weight:                 r.Weight,
		SupportsStream:         r.SupportsStream,
		Status:                 r.Status,
		CreatedAt:              millis(r.CreatedAt),
		UpdatedAt:              millis(r.UpdatedAt),
		UpstreamDeploymentName: r.UpstreamDeploymentName,
		UpstreamModel:          r.UpstreamModel,
		CapabilityType:         r.CapabilityType,
		UpstreamProtocol:       r.UpstreamProtocol,
		HealthStatus:           r.HealthStatus,
		CredentialSource:       r.CredentialSource,
		EndpointID:             r.EndpointID,
		EndpointName:           r.EndpointName,
		BaseUrl:                r.BaseUrl,
		ProviderID:             r.ProviderID,
		ProviderCode:           r.ProviderCode,
		ProviderName:           r.ProviderName,
		PoolID:                 r.PoolID,
		PoolName:               r.PoolName,
		FixedProviderType:      r.FixedProviderType,
	}
}

func fromListModelRoutes(rows []dbgen.ListModelRoutesRow) []listModelRouteDTO {
	out := make([]listModelRouteDTO, len(rows))
	for i, r := range rows {
		out[i] = fromListModelRoute(r)
	}
	return out
}

// ---------------------------------------------------------------------------
// Upstream deployment DTOs
// ---------------------------------------------------------------------------

type upstreamDeploymentDTO struct {
	ID                 pgtype.UUID     `json:"id"`
	EndpointID         pgtype.UUID     `json:"endpoint_id"`
	CredentialPoolID   pgtype.UUID     `json:"credential_pool_id"`
	UpstreamModel      string          `json:"upstream_model"`
	CapabilityType     string          `json:"capability_type"`
	UpstreamProtocol   string          `json:"upstream_protocol"`
	RequestPath        pgtype.Text     `json:"request_path"`
	UpstreamParameters json.RawMessage `json:"upstream_parameters"`
	Pricing            json.RawMessage `json:"pricing"`
	HealthStatus       string          `json:"health_status"`
	LastHealthCheckAt  *int64          `json:"last_health_check_at"`
	LastHealthError    pgtype.Text     `json:"last_health_error"`
	Status             string          `json:"status"`
	CreatedAt          *int64          `json:"created_at"`
	UpdatedAt          *int64          `json:"updated_at"`
}

func fromAiUpstreamDeployment(r dbgen.AiUpstreamDeployment) upstreamDeploymentDTO {
	return upstreamDeploymentDTO{
		ID:                 r.ID,
		EndpointID:         r.EndpointID,
		CredentialPoolID:   r.CredentialPoolID,
		UpstreamModel:      r.UpstreamModel,
		CapabilityType:     r.CapabilityType,
		UpstreamProtocol:   r.UpstreamProtocol,
		RequestPath:        r.RequestPath,
		UpstreamParameters: rawJSON(r.UpstreamParameters),
		Pricing:            rawJSON(r.Pricing),
		HealthStatus:       r.HealthStatus,
		LastHealthCheckAt:  millis(r.LastHealthCheckAt),
		LastHealthError:    r.LastHealthError,
		Status:             r.Status,
		CreatedAt:          millis(r.CreatedAt),
		UpdatedAt:          millis(r.UpdatedAt),
	}
}

type getUpstreamDeploymentDTO struct {
	ID                 pgtype.UUID     `json:"id"`
	EndpointID         pgtype.UUID     `json:"endpoint_id"`
	CredentialPoolID   pgtype.UUID     `json:"credential_pool_id"`
	UpstreamModel      string          `json:"upstream_model"`
	CapabilityType     string          `json:"capability_type"`
	UpstreamProtocol   string          `json:"upstream_protocol"`
	RequestPath        pgtype.Text     `json:"request_path"`
	UpstreamParameters json.RawMessage `json:"upstream_parameters"`
	Pricing            json.RawMessage `json:"pricing"`
	HealthStatus       string          `json:"health_status"`
	LastHealthCheckAt  *int64          `json:"last_health_check_at"`
	LastHealthError    pgtype.Text     `json:"last_health_error"`
	Status             string          `json:"status"`
	CreatedAt          *int64          `json:"created_at"`
	UpdatedAt          *int64          `json:"updated_at"`
	CredentialSource   string          `json:"credential_source"`
	EndpointName       string          `json:"endpoint_name"`
	BaseUrl            string          `json:"base_url"`
	ExtraHeaders       json.RawMessage `json:"extra_headers"`
	TimeoutMs          int32           `json:"timeout_ms"`
	ProviderID         pgtype.UUID     `json:"provider_id"`
	ProviderCode       string          `json:"provider_code"`
	ProviderName       string          `json:"provider_name"`
	PoolName           string          `json:"pool_name"`
	FixedProviderType  string          `json:"fixed_provider_type"`
	// ApiKeyCiphertext is intentionally omitted from the DTO (sensitive)
}

func fromGetUpstreamDeployment(r dbgen.GetUpstreamDeploymentRow) getUpstreamDeploymentDTO {
	return getUpstreamDeploymentDTO{
		ID:                 r.ID,
		EndpointID:         r.EndpointID,
		CredentialPoolID:   r.CredentialPoolID,
		UpstreamModel:      r.UpstreamModel,
		CapabilityType:     r.CapabilityType,
		UpstreamProtocol:   r.UpstreamProtocol,
		RequestPath:        r.RequestPath,
		UpstreamParameters: rawJSON(r.UpstreamParameters),
		Pricing:            rawJSON(r.Pricing),
		HealthStatus:       r.HealthStatus,
		LastHealthCheckAt:  millis(r.LastHealthCheckAt),
		LastHealthError:    r.LastHealthError,
		Status:             r.Status,
		CreatedAt:          millis(r.CreatedAt),
		UpdatedAt:          millis(r.UpdatedAt),
		CredentialSource:   r.CredentialSource,
		EndpointName:       r.EndpointName,
		BaseUrl:            r.BaseUrl,
		ExtraHeaders:       rawJSON(r.ExtraHeaders),
		TimeoutMs:          r.TimeoutMs,
		ProviderID:         r.ProviderID,
		ProviderCode:       r.ProviderCode,
		ProviderName:       r.ProviderName,
		PoolName:           r.PoolName,
		FixedProviderType:  r.FixedProviderType,
	}
}

type listUpstreamDeploymentDTO struct {
	ID                 pgtype.UUID     `json:"id"`
	EndpointID         pgtype.UUID     `json:"endpoint_id"`
	CredentialPoolID   pgtype.UUID     `json:"credential_pool_id"`
	UpstreamModel      string          `json:"upstream_model"`
	CapabilityType     string          `json:"capability_type"`
	UpstreamProtocol   string          `json:"upstream_protocol"`
	RequestPath        pgtype.Text     `json:"request_path"`
	UpstreamParameters json.RawMessage `json:"upstream_parameters"`
	Pricing            json.RawMessage `json:"pricing"`
	HealthStatus       string          `json:"health_status"`
	LastHealthCheckAt  *int64          `json:"last_health_check_at"`
	LastHealthError    pgtype.Text     `json:"last_health_error"`
	Status             string          `json:"status"`
	CreatedAt          *int64          `json:"created_at"`
	UpdatedAt          *int64          `json:"updated_at"`
	CredentialSource   string          `json:"credential_source"`
	EndpointName       string          `json:"endpoint_name"`
	BaseUrl            string          `json:"base_url"`
	EndpointWeight     int32           `json:"endpoint_weight"`
	ProviderID         pgtype.UUID     `json:"provider_id"`
	ProviderCode       string          `json:"provider_code"`
	ProviderName       string          `json:"provider_name"`
	PoolName           string          `json:"pool_name"`
	FixedProviderType  string          `json:"fixed_provider_type"`
}

func fromListUpstreamDeployment(r dbgen.ListUpstreamDeploymentsRow) listUpstreamDeploymentDTO {
	return listUpstreamDeploymentDTO{
		ID:                 r.ID,
		EndpointID:         r.EndpointID,
		CredentialPoolID:   r.CredentialPoolID,
		UpstreamModel:      r.UpstreamModel,
		CapabilityType:     r.CapabilityType,
		UpstreamProtocol:   r.UpstreamProtocol,
		RequestPath:        r.RequestPath,
		UpstreamParameters: rawJSON(r.UpstreamParameters),
		Pricing:            rawJSON(r.Pricing),
		HealthStatus:       r.HealthStatus,
		LastHealthCheckAt:  millis(r.LastHealthCheckAt),
		LastHealthError:    r.LastHealthError,
		Status:             r.Status,
		CreatedAt:          millis(r.CreatedAt),
		UpdatedAt:          millis(r.UpdatedAt),
		CredentialSource:   r.CredentialSource,
		EndpointName:       r.EndpointName,
		BaseUrl:            r.BaseUrl,
		EndpointWeight:     r.EndpointWeight,
		ProviderID:         r.ProviderID,
		ProviderCode:       r.ProviderCode,
		ProviderName:       r.ProviderName,
		PoolName:           r.PoolName,
		FixedProviderType:  r.FixedProviderType,
	}
}

func fromListUpstreamDeployments(rows []dbgen.ListUpstreamDeploymentsRow) []listUpstreamDeploymentDTO {
	out := make([]listUpstreamDeploymentDTO, len(rows))
	for i, r := range rows {
		out[i] = fromListUpstreamDeployment(r)
	}
	return out
}

// ---------------------------------------------------------------------------
// Tenant model grant DTOs
// ---------------------------------------------------------------------------

type tenantModelGrantDTO struct {
	ID        pgtype.UUID `json:"id"`
	TenantID  string      `json:"tenant_id"`
	ModelID   pgtype.UUID `json:"model_id"`
	Status    string      `json:"status"`
	CreatedBy pgtype.Text `json:"created_by"`
	CreatedAt *int64      `json:"created_at"`
}

func fromAiTenantModelGrant(r dbgen.AiTenantModelGrant) tenantModelGrantDTO {
	return tenantModelGrantDTO{
		ID:        r.ID,
		TenantID:  r.TenantID,
		ModelID:   r.ModelID,
		Status:    r.Status,
		CreatedBy: r.CreatedBy,
		CreatedAt: millis(r.CreatedAt),
	}
}

type listTenantModelGrantDTO struct {
	ID             pgtype.UUID `json:"id"`
	TenantID       string      `json:"tenant_id"`
	ModelID        pgtype.UUID `json:"model_id"`
	Status         string      `json:"status"`
	CreatedBy      pgtype.Text `json:"created_by"`
	CreatedAt      *int64      `json:"created_at"`
	ModelCode      string      `json:"model_code"`
	CapabilityType string      `json:"capability_type"`
}

func fromListTenantModelGrant(r dbgen.ListTenantModelGrantsRow) listTenantModelGrantDTO {
	return listTenantModelGrantDTO{
		ID:             r.ID,
		TenantID:       r.TenantID,
		ModelID:        r.ModelID,
		Status:         r.Status,
		CreatedBy:      r.CreatedBy,
		CreatedAt:      millis(r.CreatedAt),
		ModelCode:      r.ModelCode,
		CapabilityType: r.CapabilityType,
	}
}

func fromListTenantModelGrants(rows []dbgen.ListTenantModelGrantsRow) []listTenantModelGrantDTO {
	out := make([]listTenantModelGrantDTO, len(rows))
	for i, r := range rows {
		out[i] = fromListTenantModelGrant(r)
	}
	return out
}

// ---------------------------------------------------------------------------
// API key DTOs (unified shape — no plaintext key, last_four for display)
// ---------------------------------------------------------------------------

type apiKeyDTO struct {
	ID                   pgtype.UUID     `json:"id"`
	OwnerType            string          `json:"owner_type"`
	TenantID             string          `json:"tenant_id"`
	UserID               pgtype.Text     `json:"user_id"`
	LastFour             pgtype.Text     `json:"last_four"`
	Name                 string          `json:"name"`
	QuotaLimitCredits    *float64        `json:"quota_limit_credits"`    // 积分 (nil=无限制)
	QuotaUsedCredits     float64         `json:"quota_used_credits"`     // 积分
	QuotaReservedCredits float64         `json:"quota_reserved_credits"` // 积分
	AllowedModels        json.RawMessage `json:"allowed_models"`
	Status               string          `json:"status"`
	ExpiresAt            *int64          `json:"expires_at"`
	CreatedBy            pgtype.Text     `json:"created_by"`
	CreatedAt            *int64          `json:"created_at"`
	UpdatedAt            *int64          `json:"updated_at"`
}

type createAPIKeyResponse struct {
	PlaintextKey string    `json:"plaintext_key"`
	Key          apiKeyDTO `json:"key"`
}

type rotateAPIKeyResponse struct {
	PlaintextKey string    `json:"plaintext_key"`
	Key          apiKeyDTO `json:"key"`
}

func apiKeyDTOFromCreate(r dbgen.CreateAPIKeyRow) apiKeyDTO {
	return apiKeyDTO{
		ID:                   r.ID,
		OwnerType:            r.OwnerType,
		TenantID:             r.TenantID,
		UserID:               r.UserID,
		LastFour:             r.LastFour,
		Name:                 r.Name,
		QuotaLimitCredits:    credits.Int8ToCreditsPtr(r.QuotaLimit),
		QuotaUsedCredits:     credits.MicroToCredits(r.QuotaUsed),
		QuotaReservedCredits: credits.MicroToCredits(r.QuotaReserved),
		AllowedModels:        rawJSON(r.AllowedModels),
		Status:               r.Status,
		ExpiresAt:            millis(r.ExpiresAt),
		CreatedBy:            r.CreatedBy,
		CreatedAt:            millis(r.CreatedAt),
		UpdatedAt:            millis(r.UpdatedAt),
	}
}

func apiKeyDTOFromList(r dbgen.ListAPIKeysRow) apiKeyDTO {
	return apiKeyDTO{
		ID:                   r.ID,
		OwnerType:            r.OwnerType,
		TenantID:             r.TenantID,
		UserID:               r.UserID,
		LastFour:             r.LastFour,
		Name:                 r.Name,
		QuotaLimitCredits:    credits.Int8ToCreditsPtr(r.QuotaLimit),
		QuotaUsedCredits:     credits.MicroToCredits(r.QuotaUsed),
		QuotaReservedCredits: credits.MicroToCredits(r.QuotaReserved),
		AllowedModels:        rawJSON(r.AllowedModels),
		Status:               r.Status,
		ExpiresAt:            millis(r.ExpiresAt),
		CreatedBy:            r.CreatedBy,
		CreatedAt:            millis(r.CreatedAt),
		UpdatedAt:            millis(r.UpdatedAt),
	}
}

func apiKeyDTOsFromList(rows []dbgen.ListAPIKeysRow) []apiKeyDTO {
	out := make([]apiKeyDTO, len(rows))
	for i, r := range rows {
		out[i] = apiKeyDTOFromList(r)
	}
	return out
}

func apiKeyDTOFromGetByID(r dbgen.AiApiKey) apiKeyDTO {
	return apiKeyDTO{
		ID:                   r.ID,
		OwnerType:            r.OwnerType,
		TenantID:             r.TenantID,
		UserID:               r.UserID,
		LastFour:             r.LastFour,
		Name:                 r.Name,
		QuotaLimitCredits:    credits.Int8ToCreditsPtr(r.QuotaLimit),
		QuotaUsedCredits:     credits.MicroToCredits(r.QuotaUsed),
		QuotaReservedCredits: credits.MicroToCredits(r.QuotaReserved),
		AllowedModels:        rawJSON(r.AllowedModels),
		Status:               r.Status,
		ExpiresAt:            millis(r.ExpiresAt),
		CreatedBy:            r.CreatedBy,
		CreatedAt:            millis(r.CreatedAt),
		UpdatedAt:            millis(r.UpdatedAt),
	}
}

func apiKeyDTOFromUpdate(r dbgen.UpdateAPIKeyRow) apiKeyDTO {
	return apiKeyDTO{
		ID:                   r.ID,
		OwnerType:            r.OwnerType,
		TenantID:             r.TenantID,
		UserID:               r.UserID,
		LastFour:             r.LastFour,
		Name:                 r.Name,
		QuotaLimitCredits:    credits.Int8ToCreditsPtr(r.QuotaLimit),
		QuotaUsedCredits:     credits.MicroToCredits(r.QuotaUsed),
		QuotaReservedCredits: credits.MicroToCredits(r.QuotaReserved),
		AllowedModels:        rawJSON(r.AllowedModels),
		Status:               r.Status,
		ExpiresAt:            millis(r.ExpiresAt),
		CreatedBy:            r.CreatedBy,
		CreatedAt:            millis(r.CreatedAt),
		UpdatedAt:            millis(r.UpdatedAt),
	}
}

func apiKeyDTOFromUpdateStatus(r dbgen.UpdateAPIKeyStatusRow) apiKeyDTO {
	return apiKeyDTO{
		ID:                   r.ID,
		OwnerType:            r.OwnerType,
		TenantID:             r.TenantID,
		UserID:               r.UserID,
		LastFour:             r.LastFour,
		Name:                 r.Name,
		QuotaLimitCredits:    credits.Int8ToCreditsPtr(r.QuotaLimit),
		QuotaUsedCredits:     credits.MicroToCredits(r.QuotaUsed),
		QuotaReservedCredits: credits.MicroToCredits(r.QuotaReserved),
		AllowedModels:        rawJSON(r.AllowedModels),
		Status:               r.Status,
		ExpiresAt:            millis(r.ExpiresAt),
		CreatedBy:            r.CreatedBy,
		CreatedAt:            millis(r.CreatedAt),
		UpdatedAt:            millis(r.UpdatedAt),
	}
}

func apiKeyDTOFromRotate(r dbgen.RotateAPIKeyRow) apiKeyDTO {
	return apiKeyDTO{
		ID:                   r.ID,
		OwnerType:            r.OwnerType,
		TenantID:             r.TenantID,
		UserID:               r.UserID,
		LastFour:             r.LastFour,
		Name:                 r.Name,
		QuotaLimitCredits:    credits.Int8ToCreditsPtr(r.QuotaLimit),
		QuotaUsedCredits:     credits.MicroToCredits(r.QuotaUsed),
		QuotaReservedCredits: credits.MicroToCredits(r.QuotaReserved),
		AllowedModels:        rawJSON(r.AllowedModels),
		Status:               r.Status,
		ExpiresAt:            millis(r.ExpiresAt),
		CreatedBy:            r.CreatedBy,
		CreatedAt:            millis(r.CreatedAt),
		UpdatedAt:            millis(r.UpdatedAt),
	}
}

// ---------------------------------------------------------------------------
// Usage log DTO
// ---------------------------------------------------------------------------

type usageLogDTO struct {
	ID                   pgtype.UUID `json:"id"`
	RequestID            string      `json:"request_id"`
	TraceID              pgtype.Text `json:"trace_id"`
	ApiKeyID             pgtype.UUID `json:"api_key_id"`
	KeyOwnerType         string      `json:"key_owner_type"`
	AuthMethod           string      `json:"auth_method"`
	RequestSource        string      `json:"request_source"`
	TenantID             string      `json:"tenant_id"`
	UserID               pgtype.Text `json:"user_id"`
	ExternalUserID       pgtype.Text `json:"external_user_id"`
	ModelID              pgtype.UUID `json:"model_id"`
	ModelCode            string      `json:"model_code"`
	CapabilityType       string      `json:"capability_type"`
	ModelRouteID         pgtype.UUID `json:"model_route_id"`
	UpstreamDeploymentID pgtype.UUID `json:"upstream_deployment_id"`
	EndpointID           pgtype.UUID `json:"endpoint_id"`
	ProviderCode         pgtype.Text `json:"provider_code"`
	UpstreamModel        pgtype.Text `json:"upstream_model"`
	ConversationID       pgtype.Text `json:"conversation_id"`
	Stream               bool        `json:"stream"`
	PromptTokens         int32       `json:"prompt_tokens"`
	CompletionTokens     int32       `json:"completion_tokens"`
	TotalTokens          int32       `json:"total_tokens"`
	BillableUnitType     string      `json:"billable_unit_type"`
	BillableUnits        int64       `json:"billable_units"`
	ProviderCredits      float64     `json:"provider_credits"`
	PlatformCredits      float64     `json:"platform_credits"`
	UserCredits          float64     `json:"user_credits"`
	ApiKeyQuotaCredits   float64     `json:"api_key_quota_credits"`
	BillingStatus        string      `json:"billing_status"`
	RequestStatus        string      `json:"request_status"`
	HttpStatus           pgtype.Int4 `json:"http_status"`
	UpstreamStatus       pgtype.Int4 `json:"upstream_status"`
	LatencyMs            pgtype.Int4 `json:"latency_ms"`
	FirstTokenLatencyMs  pgtype.Int4 `json:"first_token_latency_ms"`
	ErrorCode            pgtype.Text `json:"error_code"`
	ErrorMessage         pgtype.Text `json:"error_message"`
	UsageEstimated       bool        `json:"usage_estimated"`
	TokenUsageSource     string      `json:"token_usage_source"`
	CreatedAt            *int64      `json:"created_at"`
}

func fromListUsageLog(r dbgen.ListUsageLogsRow) usageLogDTO {
	return usageLogDTO{
		ID:                   r.ID,
		RequestID:            r.RequestID,
		TraceID:              r.TraceID,
		ApiKeyID:             r.ApiKeyID,
		KeyOwnerType:         r.KeyOwnerType,
		AuthMethod:           r.AuthMethod,
		RequestSource:        r.RequestSource,
		TenantID:             r.TenantID,
		UserID:               r.UserID,
		ExternalUserID:       r.ExternalUserID,
		ModelID:              r.ModelID,
		ModelCode:            r.ModelCode,
		CapabilityType:       r.CapabilityType,
		ModelRouteID:         r.ModelRouteID,
		UpstreamDeploymentID: r.UpstreamDeploymentID,
		EndpointID:           r.EndpointID,
		ProviderCode:         r.ProviderCode,
		UpstreamModel:        r.UpstreamModel,
		ConversationID:       r.ConversationID,
		Stream:               r.Stream,
		PromptTokens:         r.PromptTokens,
		CompletionTokens:     r.CompletionTokens,
		TotalTokens:          r.TotalTokens,
		BillableUnitType:     r.BillableUnitType,
		BillableUnits:        r.BillableUnits,
		ProviderCredits:      credits.MicroToCredits(r.ProviderCost),
		PlatformCredits:      credits.MicroToCredits(r.PlatformCost),
		UserCredits:          credits.MicroToCredits(r.UserCost),
		ApiKeyQuotaCredits:   credits.MicroToCredits(r.ApiKeyQuotaCost),
		BillingStatus:        r.BillingStatus,
		RequestStatus:        r.RequestStatus,
		HttpStatus:           r.HttpStatus,
		UpstreamStatus:       r.UpstreamStatus,
		LatencyMs:            r.LatencyMs,
		FirstTokenLatencyMs:  r.FirstTokenLatencyMs,
		ErrorCode:            r.ErrorCode,
		ErrorMessage:         r.ErrorMessage,
		UsageEstimated:       r.UsageEstimated,
		TokenUsageSource:     r.TokenUsageSource,
		CreatedAt:            millis(r.CreatedAt),
	}
}

func fromListUsageLogs(rows []dbgen.ListUsageLogsRow) []usageLogDTO {
	out := make([]usageLogDTO, len(rows))
	for i, r := range rows {
		out[i] = fromListUsageLog(r)
	}
	return out
}

// ---------------------------------------------------------------------------
// Usage log DTO for tenant (filtered — no upstream/internal fields)
// ---------------------------------------------------------------------------

// billingStatusLabel returns a Chinese label for the given billing status.
func billingStatusLabel(status string) string {
	switch status {
	case "pending":
		return "待确认"
	case "pending_settle":
		return "待结算"
	case "settled":
		return "已结算"
	case "frozen":
		return "已冻结"
	case "confirmed":
		return "已确认"
	case "cancelled":
		return "已取消"
	case "free":
		return "免费"
	default:
		return status
	}
}

type usageLogForTenantDTO struct {
	ID                  pgtype.UUID `json:"id"`
	RequestID           string      `json:"request_id"`
	RequestSource       string      `json:"request_source"`
	TenantID            string      `json:"tenant_id"`
	UserID              pgtype.Text `json:"user_id"`
	ExternalUserID      pgtype.Text `json:"external_user_id"`
	ModelCode           string      `json:"model_code"`
	CapabilityType      string      `json:"capability_type"`
	Stream              bool        `json:"stream"`
	PromptTokens        int32       `json:"prompt_tokens"`
	CompletionTokens    int32       `json:"completion_tokens"`
	TotalTokens         int32       `json:"total_tokens"`
	PlatformCredits     float64     `json:"platform_credits"`
	UserCredits         float64     `json:"user_credits"`
	BillingStatus       string      `json:"billing_status"`
	BillingStatusLabel  string      `json:"billing_status_label"`
	RequestStatus       string      `json:"request_status"`
	HttpStatus          pgtype.Int4 `json:"http_status"`
	LatencyMs           pgtype.Int4 `json:"latency_ms"`
	FirstTokenLatencyMs pgtype.Int4 `json:"first_token_latency_ms"`
	ErrorCode           pgtype.Text `json:"error_code"`
	ErrorMessage        pgtype.Text `json:"error_message"`
	CreatedAt           *int64      `json:"created_at"`
}

func fromListUsageLogForTenant(r dbgen.ListUsageLogsRow) usageLogForTenantDTO {
	return usageLogForTenantDTO{
		ID:                  r.ID,
		RequestID:           r.RequestID,
		RequestSource:       r.RequestSource,
		TenantID:            r.TenantID,
		UserID:              r.UserID,
		ExternalUserID:      r.ExternalUserID,
		ModelCode:           r.ModelCode,
		CapabilityType:      r.CapabilityType,
		Stream:              r.Stream,
		PromptTokens:        r.PromptTokens,
		CompletionTokens:    r.CompletionTokens,
		TotalTokens:         r.TotalTokens,
		PlatformCredits:     credits.MicroToCredits(r.PlatformCost),
		UserCredits:         credits.MicroToCredits(r.UserCost),
		BillingStatus:       r.BillingStatus,
		BillingStatusLabel:  billingStatusLabel(r.BillingStatus),
		RequestStatus:       r.RequestStatus,
		HttpStatus:          r.HttpStatus,
		LatencyMs:           r.LatencyMs,
		FirstTokenLatencyMs: r.FirstTokenLatencyMs,
		ErrorCode:           r.ErrorCode,
		ErrorMessage:        r.ErrorMessage,
		CreatedAt:           millis(r.CreatedAt),
	}
}

func fromListUsageLogsForTenant(rows []dbgen.ListUsageLogsRow) []usageLogForTenantDTO {
	out := make([]usageLogForTenantDTO, len(rows))
	for i, r := range rows {
		out[i] = fromListUsageLogForTenant(r)
	}
	return out
}

// ---------------------------------------------------------------------------
// Dashboard recent errors DTO
// ---------------------------------------------------------------------------

type dashboardRecentErrorDTO struct {
	RequestID     string      `json:"request_id"`
	ModelCode     string      `json:"model_code"`
	RequestStatus string      `json:"request_status"`
	ErrorCode     pgtype.Text `json:"error_code"`
	ErrorMessage  pgtype.Text `json:"error_message"`
	HttpStatus    pgtype.Int4 `json:"http_status"`
	CreatedAt     *int64      `json:"created_at"`
}

func fromListDashboardRecentError(r dbgen.ListDashboardRecentErrorsRow) dashboardRecentErrorDTO {
	return dashboardRecentErrorDTO{
		RequestID:     r.RequestID,
		ModelCode:     r.ModelCode,
		RequestStatus: r.RequestStatus,
		ErrorCode:     r.ErrorCode,
		ErrorMessage:  r.ErrorMessage,
		HttpStatus:    r.HttpStatus,
		CreatedAt:     millis(r.CreatedAt),
	}
}

func fromListDashboardRecentErrors(rows []dbgen.ListDashboardRecentErrorsRow) []dashboardRecentErrorDTO {
	out := make([]dashboardRecentErrorDTO, len(rows))
	for i, r := range rows {
		out[i] = fromListDashboardRecentError(r)
	}
	return out
}

// ---------------------------------------------------------------------------
// Additional provider endpoint DTO converters
// ---------------------------------------------------------------------------

func fromUpdateProviderEndpoint(r dbgen.UpdateProviderEndpointRow) providerEndpointDTO {
	return providerEndpointDTO{
		ID:              r.ID,
		ProviderID:      r.ProviderID,
		Name:            r.Name,
		BaseUrl:         r.BaseUrl,
		ExtraHeaders:    rawJSON(r.ExtraHeaders),
		Weight:          r.Weight,
		TimeoutMs:       r.TimeoutMs,
		DefaultProtocol: r.DefaultProtocol,
		Status:          r.Status,
		CreatedAt:       millis(r.CreatedAt),
		UpdatedAt:       millis(r.UpdatedAt),
	}
}

func fromUpdateProviderEndpointStatus(r dbgen.UpdateProviderEndpointStatusRow) providerEndpointDTO {
	return providerEndpointDTO{
		ID:              r.ID,
		ProviderID:      r.ProviderID,
		Name:            r.Name,
		BaseUrl:         r.BaseUrl,
		ExtraHeaders:    rawJSON(r.ExtraHeaders),
		Weight:          r.Weight,
		TimeoutMs:       r.TimeoutMs,
		DefaultProtocol: r.DefaultProtocol,
		Status:          r.Status,
		CreatedAt:       millis(r.CreatedAt),
		UpdatedAt:       millis(r.UpdatedAt),
	}
}

// ---------------------------------------------------------------------------
// Runtime limit policy DTOs
// ---------------------------------------------------------------------------

type limitPolicyDTO struct {
	ID               pgtype.UUID `json:"id"`
	ScopeType        string      `json:"scope_type"`
	ScopeID          string      `json:"scope_id"`
	CapabilityType   string      `json:"capability_type"`
	ModelCode        pgtype.Text `json:"model_code"`
	RpmLimit         pgtype.Int4 `json:"rpm_limit"`
	TpmLimit         pgtype.Int4 `json:"tpm_limit"`
	ConcurrencyLimit pgtype.Int4 `json:"concurrency_limit"`
	Status           string      `json:"status"`
	CreatedBy        pgtype.Text `json:"created_by"`
	CreatedAt        *int64      `json:"created_at"`
	UpdatedAt        *int64      `json:"updated_at"`
}

func fromLimitPolicy(r dbgen.AiRuntimeLimitPolicy) limitPolicyDTO {
	return limitPolicyDTO{
		ID:               r.ID,
		ScopeType:        r.ScopeType,
		ScopeID:          r.ScopeID,
		CapabilityType:   r.CapabilityType,
		ModelCode:        r.ModelCode,
		RpmLimit:         r.RpmLimit,
		TpmLimit:         r.TpmLimit,
		ConcurrencyLimit: r.ConcurrencyLimit,
		Status:           r.Status,
		CreatedBy:        r.CreatedBy,
		CreatedAt:        millis(r.CreatedAt),
		UpdatedAt:        millis(r.UpdatedAt),
	}
}

func fromLimitPolicies(rows []dbgen.AiRuntimeLimitPolicy) []limitPolicyDTO {
	out := make([]limitPolicyDTO, len(rows))
	for i, r := range rows {
		out[i] = fromLimitPolicy(r)
	}
	return out
}

// ---------------------------------------------------------------------------
// Audit log DTOs
// ---------------------------------------------------------------------------

type auditLogDTO struct {
	ID             pgtype.UUID     `json:"id"`
	Actor          pgtype.Text     `json:"actor"`
	Action         string          `json:"action"`
	ObjectType     pgtype.Text     `json:"object_type"`
	ObjectID       pgtype.Text     `json:"object_id"`
	RequestSummary json.RawMessage `json:"request_summary"`
	Result         string          `json:"result"`
	HttpStatus     pgtype.Int4     `json:"http_status"`
	CreatedAt      *int64          `json:"created_at"`
}

func fromAuditLog(r dbgen.ListAuditLogsRow) auditLogDTO {
	return auditLogDTO{
		ID:             r.ID,
		Actor:          r.Actor,
		Action:         r.Action,
		ObjectType:     r.ObjectType,
		ObjectID:       r.ObjectID,
		RequestSummary: rawJSON(r.RequestSummary),
		Result:         r.Result,
		HttpStatus:     r.HttpStatus,
		CreatedAt:      millis(r.CreatedAt),
	}
}

func fromAuditLogs(rows []dbgen.ListAuditLogsRow) []auditLogDTO {
	out := make([]auditLogDTO, len(rows))
	for i, r := range rows {
		out[i] = fromAuditLog(r)
	}
	return out
}

// ---------------------------------------------------------------------------
// User usage logs by tenant/user DTOs
// ---------------------------------------------------------------------------

type usageLogByUserDTO struct {
	ID               pgtype.UUID `json:"id"`
	RequestID        string      `json:"request_id"`
	TraceID          pgtype.Text `json:"trace_id"`
	TenantID         string      `json:"tenant_id"`
	UserID           pgtype.Text `json:"user_id"`
	RequestSource    string      `json:"request_source"`
	ModelID          pgtype.UUID `json:"model_id"`
	ModelCode        string      `json:"model_code"`
	PromptTokens     int32       `json:"prompt_tokens"`
	CompletionTokens int32       `json:"completion_tokens"`
	TotalTokens      int32       `json:"total_tokens"`
	BillableUnitType string      `json:"billable_unit_type"`
	BillableUnits    int64       `json:"billable_units"`
	UserCredits      float64     `json:"user_credits"`
	RequestStatus    string      `json:"request_status"`
	HttpStatus       pgtype.Int4 `json:"http_status"`
	LatencyMs        pgtype.Int4 `json:"latency_ms"`
	ErrorCode        pgtype.Text `json:"error_code"`
	ErrorMessage     pgtype.Text `json:"error_message"`
	CreatedAt        *int64      `json:"created_at"`
}

func fromListUsageLogByUser(r dbgen.ListUsageLogsByTenantUserRow) usageLogByUserDTO {
	return usageLogByUserDTO{
		ID:               r.ID,
		RequestID:        r.RequestID,
		TraceID:          r.TraceID,
		TenantID:         r.TenantID,
		UserID:           r.UserID,
		RequestSource:    r.RequestSource,
		ModelID:          r.ModelID,
		ModelCode:        r.ModelCode,
		PromptTokens:     r.PromptTokens,
		CompletionTokens: r.CompletionTokens,
		TotalTokens:      r.TotalTokens,
		BillableUnitType: r.BillableUnitType,
		BillableUnits:    r.BillableUnits,
		UserCredits:      credits.MicroToCredits(r.UserCost),
		RequestStatus:    r.RequestStatus,
		HttpStatus:       r.HttpStatus,
		LatencyMs:        r.LatencyMs,
		ErrorCode:        r.ErrorCode,
		ErrorMessage:     r.ErrorMessage,
		CreatedAt:        millis(r.CreatedAt),
	}
}

func fromListUsageLogsByUser(rows []dbgen.ListUsageLogsByTenantUserRow) []usageLogByUserDTO {
	out := make([]usageLogByUserDTO, len(rows))
	for i, r := range rows {
		out[i] = fromListUsageLogByUser(r)
	}
	return out
}

// ---------------------------------------------------------------------------
// User available models DTOs
// ---------------------------------------------------------------------------

type userAvailableModelDTO struct {
	ID                             pgtype.UUID          `json:"id"`
	ModelCode                      string               `json:"model_code"`
	CapabilityType                 string               `json:"capability_type"`
	ContextWindow                  pgtype.Int4          `json:"context_window"`
	DefaultMaxOutputTokens         int32                `json:"default_max_output_tokens"`
	MaxOutputTokens                pgtype.Int4          `json:"max_output_tokens"`
	Status                         string               `json:"status"`
	GrantStatus                    string               `json:"grant_status"`
	GrantedAt                      *int64               `json:"granted_at"`
	InputPricePer1mCredits         float64              `json:"input_price_per_1m_credits"`  // 积分
	OutputPricePer1mCredits        float64              `json:"output_price_per_1m_credits"` // 积分
	ImagePrices                    []resolutionPriceDTO `json:"image_prices"`
	VideoPrices                    []resolutionPriceDTO `json:"video_prices"`
	AudioTtsPricePer1mCharsCredits float64              `json:"audio_tts_price_per_1m_chars_credits"` // 积分
	AudioSttPricePerMinuteCredits  float64              `json:"audio_stt_price_per_minute_credits"`   // 积分
}

func fromListUserAvailableModel(r dbgen.ListUserAvailableModelsRow) userAvailableModelDTO {
	return userAvailableModelDTO{
		ID:                             r.ID,
		ModelCode:                      r.ModelCode,
		CapabilityType:                 r.CapabilityType,
		ContextWindow:                  r.ContextWindow,
		DefaultMaxOutputTokens:         r.DefaultMaxOutputTokens,
		MaxOutputTokens:                r.MaxOutputTokens,
		Status:                         r.Status,
		GrantStatus:                    r.GrantStatus,
		GrantedAt:                      millis(r.GrantedAt),
		InputPricePer1mCredits:         credits.MicroToCredits(r.InputPricePer1m),
		OutputPricePer1mCredits:        credits.MicroToCredits(r.OutputPricePer1m),
		ImagePrices:                    resolutionPricesMicroToCredits(r.ImagePrices),
		VideoPrices:                    resolutionPricesMicroToCredits(r.VideoPrices),
		AudioTtsPricePer1mCharsCredits: credits.MicroToCredits(r.AudioTtsPricePer1mChars),
		AudioSttPricePerMinuteCredits:  credits.MicroToCredits(r.AudioSttPricePerMinute),
	}
}

func fromListUserAvailableModels(rows []dbgen.ListUserAvailableModelsRow) []userAvailableModelDTO {
	out := make([]userAvailableModelDTO, len(rows))
	for i, r := range rows {
		out[i] = fromListUserAvailableModel(r)
	}
	return out
}

// ---------------------------------------------------------------------------
// uuidStr helper — pgtype.UUID already serializes as UUID string via MarshalJSON,
// but for providerDTO we want a plain string field.
// ---------------------------------------------------------------------------

func uuidStr(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	var buf [36]byte
	src := u.Bytes
	const hx = "0123456789abcdef"
	for i, b := range []byte{
		src[0], src[1], src[2], src[3],
	} {
		buf[i*2] = hx[b>>4]
		buf[i*2+1] = hx[b&0xf]
	}
	buf[8] = '-'
	for i, b := range []byte{src[4], src[5]} {
		buf[9+i*2] = hx[b>>4]
		buf[9+i*2+1] = hx[b&0xf]
	}
	buf[13] = '-'
	for i, b := range []byte{src[6], src[7]} {
		buf[14+i*2] = hx[b>>4]
		buf[14+i*2+1] = hx[b&0xf]
	}
	buf[18] = '-'
	for i, b := range []byte{src[8], src[9]} {
		buf[19+i*2] = hx[b>>4]
		buf[19+i*2+1] = hx[b&0xf]
	}
	buf[23] = '-'
	for i, b := range []byte{src[10], src[11], src[12], src[13], src[14], src[15]} {
		buf[24+i*2] = hx[b>>4]
		buf[24+i*2+1] = hx[b&0xf]
	}
	return string(buf[:])
}
