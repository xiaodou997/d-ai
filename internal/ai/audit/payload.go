package audit

import "encoding/json"

// Payload is the structured audit record for a single AI request.
// Fields match the v2 ai_request_payloads schema: only wire-layer info,
// request/response bodies, media refs, and result fields that may not be
// available via JOIN (early-failure requests have no ai_usage_logs row).
//
// Redundant fields that exist in ai_usage_logs (tenant_id, api_key_id,
// capability_type, route_id, upstream_*, prompt/completion tokens,
// latency_ms, first_token_ms) are intentionally absent — callers JOIN
// at query time.
type Payload struct {
	RequestID                   string          `json:"request_id"`
	ClientProtocol              string          `json:"client_protocol"`
	ClientIP                    string          `json:"client_ip"`
	UserAgent                   string          `json:"user_agent"`
	RequestPath                 string          `json:"request_path"`
	AuthMasked                  string          `json:"auth_masked"`
	RequestModel                string          `json:"request_model"`
	MatchedDispatchRuleID       string          `json:"matched_dispatch_rule_id"`
	MatchedDispatchRuleSummary  string          `json:"matched_dispatch_rule_summary"`
	ResolvedLogicalModel        string          `json:"resolved_logical_model"`
	ResolvedProviderFamily      string          `json:"resolved_provider_family"`
	ProtocolConversionEnabled   bool            `json:"protocol_conversion_enabled"`
	SelectedUpstreamProtocol    string          `json:"selected_upstream_protocol"`
	SelectedUpstreamModel       string          `json:"selected_upstream_model"`
	UpstreamModelMappingApplied bool            `json:"upstream_model_mapping_applied"`
	PublicResponseModel         string          `json:"public_response_model"`
	RequestMessages             json.RawMessage `json:"request_messages"`
	RequestParams               json.RawMessage `json:"request_params"`
	ResponseMessage             json.RawMessage `json:"response_message"`
	MediaRefs                   json.RawMessage `json:"media_refs"`
	RequestStatus               string          `json:"request_status"`
	HTTPStatus                  int             `json:"http_status"`
	ErrorCode                   string          `json:"error_code"`

	// InternalErrorDetail / FailedStep: admin-only diagnostics, never exposed
	// to tenant/user self-service usage endpoints. See serving.Request for the
	// field-level rationale.
	InternalErrorDetail string `json:"internal_error_detail"`
	FailedStep          string `json:"failed_step"`

	// AttemptsDetail: admin-only JSON array of every upstream candidate tried
	// during Execute (provider/model/endpoint/outcome/redacted error per
	// attempt). Built by serving.BuildAttemptsDetail; nil when the request
	// never reached Execute.
	AttemptsDetail json.RawMessage `json:"attempts_detail"`
}
