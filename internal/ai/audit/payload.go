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
	RequestID                   string
	ClientProtocol              string
	ClientIP                    string
	UserAgent                   string
	RequestPath                 string
	AuthMasked                  string
	RequestModel                string
	MatchedDispatchRuleID       string
	MatchedDispatchRuleSummary  string
	ResolvedLogicalModel        string
	ResolvedProviderFamily      string
	ProtocolConversionEnabled   bool
	SelectedUpstreamProtocol    string
	SelectedUpstreamModel       string
	UpstreamModelMappingApplied bool
	PublicResponseModel         string
	RequestMessages             json.RawMessage
	RequestParams               json.RawMessage
	ResponseMessage             json.RawMessage
	MediaRefs                   json.RawMessage
	RequestStatus               string
	HTTPStatus                  int
	ErrorCode                   string

	// InternalErrorDetail / FailedStep: admin-only diagnostics, never exposed
	// to tenant/user self-service usage endpoints. See serving.Request for the
	// field-level rationale.
	InternalErrorDetail string
	FailedStep          string

	// AttemptsDetail: admin-only JSON array of every upstream candidate tried
	// during Execute (provider/model/endpoint/outcome/redacted error per
	// attempt). Built by serving.BuildAttemptsDetail; nil when the request
	// never reached Execute.
	AttemptsDetail json.RawMessage
}
