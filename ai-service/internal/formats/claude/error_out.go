package claude

import "encoding/json"

// ErrorBody is the Anthropic-style error envelope returned to clients
// hitting /v1/messages or /v1/messages/count_tokens.
type ErrorBody struct {
	Type  string     `json:"type"` // always "error"
	Error ErrorInner `json:"error"`
}

type ErrorInner struct {
	Type    string `json:"type"` // "invalid_request_error" | "authentication_error" | ...
	Message string `json:"message"`
}

// MarshalError returns the JSON bytes for an Anthropic error response.
// errType maps the gateway's internal error code to one of Anthropic's
// canonical error types.
func MarshalError(errType, message string) ([]byte, error) {
	return json.Marshal(ErrorBody{
		Type: "error",
		Error: ErrorInner{
			Type:    mapErrorType(errType),
			Message: message,
		},
	})
}

// mapErrorType normalises arbitrary internal codes to Anthropic's documented
// set of error types. Unknown codes pass through unchanged so callers can
// still introspect gateway-specific failures.
func mapErrorType(code string) string {
	switch code {
	case "invalid_api_key", "missing_api_key", "invalid_token":
		return "authentication_error"
	case "model_not_authorized", "forbidden":
		return "permission_error"
	case "rate_limit_exceeded":
		return "rate_limit_error"
	case "quota_exceeded", "insufficient_balance", "quota_reserve_failed":
		return "billing_error"
	case "no_available_route", "no_credential":
		return "overloaded_error"
	case "upstream_error", "upstream_http_error":
		return "api_error"
	case "internal_error", "server_error":
		return "api_error"
	case "":
		return "invalid_request_error"
	default:
		if code == "invalid_request_error" || code == "authentication_error" ||
			code == "permission_error" || code == "rate_limit_error" ||
			code == "api_error" || code == "overloaded_error" {
			return code
		}
		return "invalid_request_error"
	}
}
