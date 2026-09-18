package postgres

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/serving"
)

const usageClientUserAgentMaxLen = 512

type apiKeyCacheInvalidator interface {
	DelByID(context.Context, string) error
}
type usageBiller interface {
	Calculate(context.Context, *serving.Request) (domain.BillingResult, error)
}

// Deprecated constructor name for internal callers; there is only one writer.
func NewUsageLogger(pool *pgxpool.Pool, biller usageBiller) *RequestStore {
	return NewRequestStore(pool, biller, nil)
}
func (s *RequestStore) WithLogger(logger *zap.Logger) *RequestStore {
	if logger != nil {
		s.logger = logger
	}
	return s
}

type usageMetadata struct {
	RequestID                          string         `json:"request_id"`
	TraceID                            pgtype.Text    `json:"trace_id"`
	ApiKeyID                           pgtype.UUID    `json:"api_key_id"`
	KeyOwnerType                       string         `json:"key_owner_type"`
	AuthMethod                         string         `json:"auth_method"`
	RequestSource                      string         `json:"request_source"`
	TenantID                           string         `json:"tenant_id"`
	UserID                             pgtype.Text    `json:"user_id"`
	ClientUserAgent                    string         `json:"client_user_agent"`
	ExternalUserID                     pgtype.Text    `json:"external_user_id"`
	GroupID                            pgtype.UUID    `json:"group_id"`
	GroupNameSnapshot                  string         `json:"group_name_snapshot"`
	GroupDefaultUserMultiplierSnapshot pgtype.Numeric `json:"group_default_user_multiplier_snapshot"`
	UserMultiplierOverrideSnapshot     pgtype.Numeric `json:"user_multiplier_override_snapshot"`
	EffectiveUserMultiplierSnapshot    pgtype.Numeric `json:"effective_user_multiplier_snapshot"`
	BillingGroupLabelSnapshot          string         `json:"billing_group_label_snapshot"`
	ModelCode                          string         `json:"model_code"`
	RequestedModel                     string         `json:"requested_model"`
	MatchedDispatchRuleID              pgtype.UUID    `json:"matched_dispatch_rule_id"`
	MatchedDispatchRuleSummary         pgtype.Text    `json:"matched_dispatch_rule_summary"`
	ResolvedLogicalModel               pgtype.Text    `json:"resolved_logical_model"`
	ResolvedProviderFamily             pgtype.Text    `json:"resolved_provider_family"`
	CapabilityType                     string         `json:"capability_type"`
	GroupTargetID                      pgtype.UUID    `json:"group_target_id"`
	UpstreamAccountID                  pgtype.UUID    `json:"upstream_account_id"`
	EndpointID                         pgtype.UUID    `json:"endpoint_id"`
	CredentialPoolID                   pgtype.UUID    `json:"credential_pool_id"`
	OauthCredentialID                  pgtype.UUID    `json:"oauth_credential_id"`
	ProviderCode                       pgtype.Text    `json:"provider_code"`
	UpstreamModel                      pgtype.Text    `json:"upstream_model"`
	ProviderFormat                     pgtype.Text    `json:"provider_format"`
	ConversationID                     pgtype.Text    `json:"conversation_id"`
	Stream                             bool           `json:"stream"`
	PromptTokens                       int32          `json:"prompt_tokens"`
	CompletionTokens                   int32          `json:"completion_tokens"`
	CacheWriteTokens                   int32          `json:"cache_write_tokens"`
	CacheReadTokens                    int32          `json:"cache_read_tokens"`
	ReasoningTokens                    int32          `json:"reasoning_tokens"`
	ReasoningEffort                    pgtype.Text    `json:"reasoning_effort"`
	TotalTokens                        int32          `json:"total_tokens"`
	BillableUnitType                   string         `json:"billable_unit_type"`
	BillableUnits                      int64          `json:"billable_units"`
	CatalogBase                        int64          `json:"catalog_base"`
	TenantPayable                      int64          `json:"tenant_payable"`
	RetailBase                         int64          `json:"retail_base"`
	UserPayable                        int64          `json:"user_payable"`
	UserCharged                        int64          `json:"user_charged"`
	ApiKeyQuotaCost                    int64          `json:"api_key_quota_cost"`
	ServiceTier                        string         `json:"service_tier"`
	BillingBreakdown                   []byte         `json:"billing_breakdown"`
	BillingStatus                      string         `json:"billing_status"`
	RequestStatus                      string         `json:"request_status"`
	HttpStatus                         pgtype.Int4    `json:"http_status"`
	UpstreamStatus                     pgtype.Int4    `json:"upstream_status"`
	LatencyMs                          pgtype.Int4    `json:"latency_ms"`
	FirstTokenLatencyMs                pgtype.Int4    `json:"first_token_latency_ms"`
	RequestTotalMs                     pgtype.Int4    `json:"request_total_ms"`
	RequestSetupMs                     pgtype.Int4    `json:"request_setup_ms"`
	FirstResponseByteMs                pgtype.Int4    `json:"first_response_byte_ms"`
	ResponseTailMs                     pgtype.Int4    `json:"response_tail_ms"`
	FinalAttemptHeaderMs               pgtype.Int4    `json:"final_attempt_header_ms"`
	FinalAttemptTotalMs                pgtype.Int4    `json:"final_attempt_total_ms"`
	ErrorCode                          pgtype.Text    `json:"error_code"`
	ErrorMessage                       pgtype.Text    `json:"error_message"`
	UsageEstimated                     bool           `json:"usage_estimated"`
	TokenUsageSource                   string         `json:"token_usage_source"`
	ProviderTerminalState              string         `json:"provider_terminal_state"`
	ClientDeliveryState                string         `json:"client_delivery_state"`
	CancellationOrigin                 string         `json:"cancellation_origin"`
	BillingReason                      string         `json:"billing_reason"`
	ResponseSummaryState               string         `json:"response_summary_state"`
	AttemptsCount                      int32          `json:"attempts_count"`
	FinalRouteID                       pgtype.UUID    `json:"final_route_id"`
	ClientProtocol                     string         `json:"client_protocol"`
	Resolution                         pgtype.Text    `json:"resolution"`
	ProtocolConversionEnabled          bool           `json:"protocol_conversion_enabled"`
	UpstreamModelMappingApplied        bool           `json:"upstream_model_mapping_applied"`
	PublicResponseModel                pgtype.Text    `json:"public_response_model"`
	BillingSource                      string         `json:"billing_source"`
	SubscriptionID                     pgtype.UUID    `json:"subscription_id"`
}

func usageUserMultiplierOverrideSnapshot(billing domain.BillingResult) pgtype.Numeric {
	if billing.UserMultiplierOverride != nil {
		return floatPtrToNumeric(billing.UserMultiplierOverride)
	}
	return pgtype.Numeric{}
}

// ============================================================================
// DB writes
// ============================================================================

func buildUsageLogParams(req *serving.Request, billing domain.BillingResult) usageMetadata {
	c := req.Candidate
	subject := req.RuntimeSubject()
	usage := req.TokenUsage

	attempted := len(req.Attempts) > 0
	groupTargetUUID := pgtype.UUID{}
	apiKeyUUID := pgtype.UUID{}
	if subject != nil && subject.APIKeyID != "" {
		apiKeyUUID = mustParseUUID(subject.APIKeyID)
	}

	// 直连路由分别记录账号与具体请求端点；池路由记录凭证池与实际 OAuth 凭证。
	accountUUID := pgtype.UUID{}
	endpointUUID := pgtype.UUID{}
	poolUUID := pgtype.UUID{}
	var credUUID pgtype.UUID
	if attempted {
		groupTargetUUID = mustParseUUID(c.RouteID)
		accountUUID = mustParseUUID(c.AccountID)
		endpointUUID = mustParseUUID(c.EndpointID)
		poolUUID = mustParseUUID(c.PoolID)
	}
	if attempted && req.SelectedCredential != nil {
		credUUID = mustParseUUID(req.SelectedCredential.ID)
	}

	// Pool routes use PoolUpstreamModel; direct routes use UpstreamModel.
	upstreamModel, providerCode, providerFormat := "", "", ""
	resolvedProviderFamily := ""
	protocolConversionEnabled := false
	upstreamModelMappingApplied := false
	if attempted {
		upstreamModel = c.UpstreamModel
		if c.IsPoolRoute() {
			upstreamModel = c.PoolUpstreamModel
		}
		providerCode = c.ProviderCode
		providerFormat = string(c.Protocol)
		resolvedProviderFamily = req.ResolvedProviderFamily
		protocolConversionEnabled = req.ProtocolConversionEnabled
		upstreamModelMappingApplied = req.UpstreamModelMappingApplied
	}
	requestTotalMs, hasRequestTotalMs := req.RequestTotalMs()
	requestSetupMs, hasRequestSetupMs := req.RequestSetupMs()
	firstResponseByteMs, hasFirstResponseByteMs := req.FirstResponseByteDurationMs()
	responseTailMs, hasResponseTailMs := req.ResponseTailMs()
	finalAttemptHeaderMs, hasFinalAttemptHeaderMs := req.FinalAttemptHeaderMs()
	finalAttemptTotalMs, hasFinalAttemptTotalMs := req.FinalAttemptTotalMs()

	params := usageMetadata{
		RequestID:                          req.RequestID,
		TraceID:                            nullableText(req.TraceID),
		ApiKeyID:                           apiKeyUUID,
		KeyOwnerType:                       string(runtimeSubjectOwnerType(subject)),
		AuthMethod:                         string(runtimeSubjectAuthMethod(subject)),
		RequestSource:                      string(runtimeSubjectRequestSource(subject)),
		TenantID:                           runtimeSubjectTenantID(subject),
		UserID:                             nullableText(runtimeSubjectUserID(subject)),
		ClientUserAgent:                    usageClientUserAgent(req),
		GroupID:                            nullableUUID(c.GroupID),
		GroupNameSnapshot:                  billing.GroupNameSnapshot,
		GroupDefaultUserMultiplierSnapshot: floatToNumeric(billing.GroupDefaultUserMultiplier),
		UserMultiplierOverrideSnapshot:     usageUserMultiplierOverrideSnapshot(billing),
		EffectiveUserMultiplierSnapshot:    floatToNumeric(billing.EffectiveUserMultiplier),
		BillingGroupLabelSnapshot:          billing.BillingGroupLabel,
		ModelCode:                          req.ModelCode,
		RequestedModel:                     req.PublicModel(),
		MatchedDispatchRuleID:              nullableUUID(req.MatchedDispatchRuleID),
		MatchedDispatchRuleSummary:         nullableText(req.MatchedDispatchRuleSummary),
		ResolvedLogicalModel:               nullableText(req.ResolvedLogicalModel),
		ResolvedProviderFamily:             nullableText(resolvedProviderFamily),
		CapabilityType:                     string(req.CapabilityType),
		GroupTargetID:                      groupTargetUUID,
		UpstreamAccountID:                  accountUUID,
		EndpointID:                         endpointUUID,
		CredentialPoolID:                   poolUUID,
		OauthCredentialID:                  credUUID,
		ProviderCode:                       nullableText(providerCode),
		UpstreamModel:                      nullableText(upstreamModel),
		ProviderFormat:                     nullableText(providerFormat),
		Stream:                             req.IsStream,
		PromptTokens:                       int32(usage.PromptTokens),
		CompletionTokens:                   int32(usage.CompletionTokens),
		CacheWriteTokens:                   int32(usage.CacheWriteTokens),
		CacheReadTokens:                    int32(usage.CacheReadTokens),
		ReasoningTokens:                    int32(usage.ReasoningTokens),
		ReasoningEffort:                    nullableText(req.ReasoningEffort),
		TotalTokens:                        int32(usage.TotalTokens()),
		BillableUnitType:                   billing.BillableUnitType,
		BillableUnits:                      billing.BillableUnits,
		CatalogBase:                        billing.CatalogBaseMicro,
		TenantPayable:                      billing.TenantPayableMicro,
		RetailBase:                         billing.RetailBaseMicro,
		UserPayable:                        billing.UserPayableMicro,
		UserCharged:                        billing.UserChargedMicro,
		ApiKeyQuotaCost:                    billing.APIKeyQuotaCostMicro,
		ServiceTier:                        string(billing.ServiceTier),
		BillingBreakdown:                   billing.BillingBreakdownJSON,
		BillingStatus:                      billingStatus(req),
		RequestStatus:                      string(req.RequestStatus),
		HttpStatus:                         nullableInt4(req.HTTPStatus),
		UpstreamStatus:                     nullableInt4(req.UpstreamStatus),
		LatencyMs:                          nullableInt4(req.LatencyMs),
		FirstTokenLatencyMs:                nullableInt4(req.FirstTokenMs),
		RequestTotalMs:                     nullableInt4WithValid(requestTotalMs, hasRequestTotalMs),
		RequestSetupMs:                     nullableInt4WithValid(requestSetupMs, hasRequestSetupMs),
		FirstResponseByteMs:                nullableInt4WithValid(firstResponseByteMs, hasFirstResponseByteMs),
		ResponseTailMs:                     nullableInt4WithValid(responseTailMs, hasResponseTailMs),
		FinalAttemptHeaderMs:               nullableInt4WithValid(finalAttemptHeaderMs, hasFinalAttemptHeaderMs),
		FinalAttemptTotalMs:                nullableInt4WithValid(finalAttemptTotalMs, hasFinalAttemptTotalMs),
		ErrorCode:                          nullableText(req.ErrorCode),
		ErrorMessage:                       nullableText(req.ErrorMessage),
		UsageEstimated:                     req.TokenCountSource == domain.TokenUsageSourceEstimated || req.TokenCountSource == domain.TokenUsageSourceMixed,
		TokenUsageSource:                   tokenCountSource(req.TokenCountSource),
		ProviderTerminalState:              string(req.ProviderTerminalState),
		ClientDeliveryState:                string(req.ClientDeliveryState),
		CancellationOrigin:                 string(req.CancellationOrigin),
		BillingReason:                      req.BillingReason,
		ResponseSummaryState:               string(req.ResponseSummaryState),
		AttemptsCount:                      int32(len(req.Attempts)),
		FinalRouteID:                       groupTargetUUID,
		ClientProtocol:                     string(req.ClientProtocol),
		Resolution:                         nullableText(resolution(req)),
		ProtocolConversionEnabled:          protocolConversionEnabled,
		UpstreamModelMappingApplied:        upstreamModelMappingApplied,
		PublicResponseModel:                nullableText(req.PublicModel()),
		// 计费来源快照：SubscriptionGateStep 在管线里判定后写入 req。空串
		// 兜底为 payg，防未经 gate 的路径违反 CHECK IN ('payg','subscription')。
		BillingSource:  usageBillingSource(req.BillingSource),
		SubscriptionID: nullableUUID(req.SubscriptionID),
	}

	return params
}

func usageClientUserAgent(req *serving.Request) string {
	if req == nil || req.Envelope == nil || req.Envelope.R == nil {
		return ""
	}
	ua := req.Envelope.R.Header.Get("User-Agent")
	if len(ua) > usageClientUserAgentMaxLen {
		return ua[:usageClientUserAgentMaxLen]
	}
	return ua
}

// unattemptedBilling zeroes a request that never reached an upstream. Requests
// that did reach an upstream are normally billed for authoritative usage, while
// client/gateway cancellations before a provider terminal event use
// voidBilling below.
func unattemptedBilling(source domain.BillingResult) domain.BillingResult {
	source.CatalogBaseMicro = 0
	source.TenantPayableMicro = 0
	source.RetailBaseMicro = 0
	source.UserPayableMicro = 0
	source.UserChargedMicro = 0
	source.APIKeyQuotaCostMicro = 0
	source.BillableUnits = 0
	source.BillingBreakdownJSON = []byte(`{"reason":"upstream_not_attempted"}`)
	return source
}

// voidBilling keeps the observed token/cost breakdown for audit, but removes
// every customer-facing payable amount and billable unit. This is used for a
// client/gateway cancellation before the provider emitted a terminal event.
func voidBilling(source domain.BillingResult, reason string) domain.BillingResult {
	source.CatalogBaseMicro = 0
	source.TenantPayableMicro = 0
	source.RetailBaseMicro = 0
	source.UserPayableMicro = 0
	source.UserChargedMicro = 0
	source.APIKeyQuotaCostMicro = 0
	source.BillableUnits = 0
	source.BillingBreakdownJSON = annotateBillingBreakdown(source.BillingBreakdownJSON, reason)
	return source
}

func annotateBillingBreakdown(raw []byte, reason string) []byte {
	var value map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		value = map[string]any{}
	}
	value["billing_status"] = string(domain.BillingVoid)
	if reason == "" {
		reason = "customer_charge_void"
	}
	value["billing_reason"] = reason
	out, err := json.Marshal(value)
	if err != nil {
		return []byte(`{"billing_status":"void"}`)
	}
	return out
}

// tokenCountSource normalises the source label; defaults to "upstream" when empty
// (e.g. failed requests where Execute never reached the estimation path).
func tokenCountSource(s string) string {
	switch s {
	case domain.TokenUsageSourceEstimated, domain.TokenUsageSourceMixed, domain.TokenUsageSourceUpstream, domain.TokenUsageSourceMissing:
		return s
	default:
		return domain.TokenUsageSourceUpstream
	}
}

// resolution returns the request resolution for image/video billing audit,
// or empty string for token-based requests.
func resolution(req *serving.Request) string {
	if req.TokenUsage.ImageResolution != "" {
		return req.TokenUsage.ImageResolution
	}
	return req.TokenUsage.VideoResolution
}

// usageBillingSource coalesces an empty gate decision to "payg" so the usage
// log never violates the billing_source CHECK constraint.
func usageBillingSource(src string) string {
	if src == "" {
		return "payg"
	}
	return src
}

// billingStatus is the settlement state at insert time. A billable request is
// written "pending" and the outbox consumer promotes it to "settled" once the
// balance has actually moved. A customer-charge void is terminal and never
// enters the outbox.
func billingStatus(req *serving.Request) string {
	if req.BillingStatus != "" {
		return string(req.BillingStatus)
	}
	if req.BillingResult.TenantPayableMicro == 0 && req.BillingResult.UserChargedMicro == 0 {
		return string(domain.BillingFree)
	}
	return string(domain.BillingPending)
}
