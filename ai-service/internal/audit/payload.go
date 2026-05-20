package audit

import "encoding/json"

// Payload is the structured audit record for a single AI request.
// Populated by the serving pipeline and written asynchronously via Worker.
type Payload struct {
	RequestID        string
	TenantID         string
	APIKeyID         string // UUID string
	CapabilityType   string
	ClientProtocol   string
	ClientIP         string
	UserAgent        string
	RequestPath      string
	AuthMasked       string
	RequestModel     string
	RequestMessages  json.RawMessage
	RequestParams    json.RawMessage
	RouteID          string
	UpstreamProvider string
	UpstreamModel    string
	UpstreamEndpoint string
	ResponseMessage  json.RawMessage
	ResponseModel    string
	PromptTokens     int
	CompletionTokens int
	RequestStatus    string
	HTTPStatus       int
	ErrorCode        string
	LatencyMs        int
	FirstTokenMs     int
}
