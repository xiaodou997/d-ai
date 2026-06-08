package console

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
//   - API 边界统一使用「积分」；配置型字段使用整数积分，结果型字段使用小数积分。
//     后端在 DTO 层做 micro-credit 与 credit 的单位转换。
//   - 前端只看到「积分」，不再需要感知微积分。
//   - usage DTO、价格 DTO、配额 DTO 都通过 internal/credits 在边界转换单位。

import (
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"xiaodou/unihub/ai-service/internal/credits"
	dbgen "xiaodou/unihub/ai-service/internal/db/gen"
)

// ---------------------------------------------------------------------------
// Price DTO helpers
// ---------------------------------------------------------------------------

func validateCreditAmount(name string, value int64) string {
	if value < 0 {
		return fmt.Sprintf("%s must be a non-negative credit value", name)
	}
	if value > maxCreditsPerField {
		return fmt.Sprintf("%s exceeds maximum allowed value (%d credits)", name, maxCreditsPerField)
	}
	return ""
}

func validateOptionalCreditAmount(name string, value *int64) string {
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
	PriceBookID     string          `json:"price_book_id"`
	CostMultiplier  *float64        `json:"cost_multiplier"`
	Status          string          `json:"status"`
	CreatedAt       *int64          `json:"created_at"`
	UpdatedAt       *int64          `json:"updated_at"`
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
	PriceBookID     string          `json:"price_book_id"`
	CostMultiplier  *float64        `json:"cost_multiplier"`
	Status          string          `json:"status"`
	CreatedAt       *int64          `json:"created_at"`
	UpdatedAt       *int64          `json:"updated_at"`
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
	PriceBookID        pgtype.UUID     `json:"price_book_id"`
	CostMultiplier     float64         `json:"cost_multiplier"`
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
		PriceBookID:        r.PriceBookID,
		CostMultiplier:     numericFloatVal(r.CostMultiplier),
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
	PriceBookID        pgtype.UUID     `json:"price_book_id"`
	CostMultiplier     float64         `json:"cost_multiplier"`
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
		PriceBookID:        r.PriceBookID,
		CostMultiplier:     numericFloatVal(r.CostMultiplier),
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
	PriceBookID        pgtype.UUID     `json:"price_book_id"`
	CostMultiplier     float64         `json:"cost_multiplier"`
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
		PriceBookID:        r.PriceBookID,
		CostMultiplier:     numericFloatVal(r.CostMultiplier),
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
	QuotaLimitCredits    *int64          `json:"quota_limit_credits"`    // 积分 (nil=无限制)
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

// ---------------------------------------------------------------------------
// Additional provider endpoint DTO converters
// ---------------------------------------------------------------------------

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
	ID                     pgtype.UUID `json:"id"`
	ModelCode              string      `json:"model_code"`
	CapabilityType         string      `json:"capability_type"`
	ContextWindow          pgtype.Int4 `json:"context_window"`
	DefaultMaxOutputTokens int32       `json:"default_max_output_tokens"`
	MaxOutputTokens        pgtype.Int4 `json:"max_output_tokens"`
	Status                 string      `json:"status"`
	GrantStatus            string      `json:"grant_status"`
	GrantedAt              *int64      `json:"granted_at"`
}

func fromListUserAvailableModel(r dbgen.ListUserAvailableModelsRow) userAvailableModelDTO {
	return userAvailableModelDTO{
		ID:                     r.ID,
		ModelCode:              r.ModelCode,
		CapabilityType:         r.CapabilityType,
		ContextWindow:          r.ContextWindow,
		DefaultMaxOutputTokens: r.DefaultMaxOutputTokens,
		MaxOutputTokens:        r.MaxOutputTokens,
		Status:                 r.Status,
		GrantStatus:            r.GrantStatus,
		GrantedAt:              millis(r.GrantedAt),
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
