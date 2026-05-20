package gemini

import (
	"encoding/json"
	"fmt"
	"strings"
)

// gemini_cli and antigravity providers reach Google's internal CodeAssist API
// at https://cloudcode-pa.googleapis.com/v1internal:<action>, NOT the public
// generativelanguage endpoint. The body is wrapped in an envelope carrying the
// OAuth project_id.

const (
	cliUserAgent      = "GeminiCLI/0.1.5 (linux; x64)"
	antigravityUA     = "google-api-nodejs-client/8.0.0"
	antigravityClient = "antigravity"
)

// CLIAction selects between sync and streaming Code Assist endpoints.
type CLIAction int

const (
	CLISyncGenerate CLIAction = iota
	CLIStreamGenerate
)

func (a CLIAction) String() string {
	if a == CLIStreamGenerate {
		return "streamGenerateContent"
	}
	return "generateContent"
}

// BuildCLIURL constructs the v1internal URL for gemini_cli / antigravity.
// Stream requests append ?alt=sse so the upstream returns SSE chunks.
func BuildCLIURL(baseURL string, action CLIAction) string {
	url := strings.TrimRight(baseURL, "/") + "/v1internal:" + action.String()
	if action == CLIStreamGenerate {
		url += "?alt=sse"
	}
	return url
}

// WrapCLIRequest wraps a raw Gemini generateContent body in the CodeAssist
// envelope used by gemini_cli OAuth credentials:
//
//	{
//	  "model":   "<model>",
//	  "project": "<project_id from credential auth_metadata>",
//	  "request": { ...original body without "model" and safety settings... }
//	}
func WrapCLIRequest(body []byte, model, projectID string) ([]byte, error) {
	inner, err := decodeInnerRequest(body)
	if err != nil {
		return nil, err
	}
	envelope := map[string]any{
		"model":   model,
		"project": projectID,
		"request": inner,
	}
	return json.Marshal(envelope)
}

// WrapAntigravityRequest builds the Antigravity-flavoured envelope, which
// additionally carries a requestId, userAgent and requestType field. The
// `tools`, `systemInstruction`, `thinkingConfig` keys are stripped from the
// inner request (matching Aether's "safe envelope" rules).
func WrapAntigravityRequest(body []byte, model, projectID, requestID string) ([]byte, error) {
	inner, err := decodeInnerRequest(body)
	if err != nil {
		return nil, err
	}
	stripAntigravityKeys(inner)
	envelope := map[string]any{
		"project":     projectID,
		"requestId":   requestID,
		"request":     inner,
		"model":       model,
		"userAgent":   antigravityUA,
		"requestType": antigravityClient,
	}
	return json.Marshal(envelope)
}

func decodeInnerRequest(body []byte) (map[string]any, error) {
	if len(body) == 0 {
		return map[string]any{}, nil
	}
	var inner map[string]any
	if err := json.Unmarshal(body, &inner); err != nil {
		return nil, fmt.Errorf("decode gemini body: %w", err)
	}
	if inner == nil {
		inner = map[string]any{}
	}
	// "model" lives in the envelope, not the inner request.
	delete(inner, "model")
	delete(inner, "safetySettings")
	delete(inner, "safety_settings")
	return inner, nil
}

func stripAntigravityKeys(inner map[string]any) {
	for _, k := range []string{
		"systemInstruction", "system_instruction",
		"tools", "toolConfig", "tool_config",
		"thinkingConfig", "thinking_config",
		"imageConfig", "image_config",
		"functionCall", "function_call",
		"functionResponse", "function_response",
	} {
		delete(inner, k)
	}
}

// ProjectIDFromMetadata extracts the OAuth project_id from the credential's
// auth_metadata JSON. Gemini CLI imports stash it as "project_id" (snake) or
// "projectId" (camel) depending on the source tool.
func ProjectIDFromMetadata(meta map[string]any) string {
	if meta == nil {
		return ""
	}
	for _, k := range []string{"project_id", "projectId", "project"} {
		if v, ok := meta[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// CLIUserAgent returns the user-agent header value matching the genuine
// gemini-cli client. Antigravity uses a different one (see envelope).
func CLIUserAgent() string { return cliUserAgent }
