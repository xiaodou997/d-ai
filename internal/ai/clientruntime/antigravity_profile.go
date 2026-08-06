package clientruntime

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"xiaodou/dai/internal/ai/domain"
)

const (
	AntigravityProfileRevision = "antigravity-cli@1.0.16+dai.1"
	antigravityBaseURL         = "https://cloudcode-pa.googleapis.com"
	antigravityUserAgent       = "antigravity/cli/1.0.16 (aidev_client; os_type=linux; arch=arm64; auth_method=consumer)"
	antigravityGoogleAPIClient = "gl-node/18.18.2 fire/0.8.6 grpc/1.10.x"
)

type antigravityProfileV1016 struct{}

func (antigravityProfileV1016) revision() string {
	return AntigravityProfileRevision
}

func (antigravityProfileV1016) supports(protocol domain.UpstreamProtocol) bool {
	return protocol == domain.ProtocolGeminiGenerate
}

func (antigravityProfileV1016) prepare(in Invocation) (*WireRequest, error) {
	if in.Protocol != domain.ProtocolGeminiGenerate {
		return nil, fmt.Errorf("Antigravity profile requires %q, got %q", domain.ProtocolGeminiGenerate, in.Protocol)
	}
	body, sessionID, err := applyAntigravityContract(in)
	if err != nil {
		return nil, err
	}
	action := "generateContent"
	url := antigravityBaseURL + "/v1internal:" + action
	accept := "application/json"
	if in.Stream {
		action = "streamGenerateContent"
		url = antigravityBaseURL + "/v1internal:" + action + "?alt=sse"
		accept = "text/event-stream"
	}
	clientVersion := firstMetadataString(in.Credential.Metadata, antigravityClientVersionPaths...)
	if clientVersion == "" {
		clientVersion = "1.0.16"
	}
	headers := map[string]string{
		"Authorization":     "Bearer " + in.Credential.AccessToken,
		"Content-Type":      "application/json",
		"Accept":            accept,
		"User-Agent":        antigravityUserAgent,
		"x-client-name":     "antigravity",
		"x-client-version":  clientVersion,
		"x-goog-api-client": antigravityGoogleAPIClient,
	}
	if validHeaderValue(sessionID) {
		headers["x-vscode-sessionid"] = sessionID
	}
	return &WireRequest{
		Method:   http.MethodPost,
		URL:      url,
		Headers:  headers,
		Body:     body,
		Protocol: domain.ProtocolGeminiGenerate,
	}, nil
}

func applyAntigravityContract(in Invocation) ([]byte, string, error) {
	source, inner, err := decodeGeminiEnvelopeSource(in.Body)
	if err != nil {
		return nil, "", err
	}
	delete(inner, "model")
	delete(inner, "safetySettings")
	delete(inner, "safety_settings")

	project := firstMetadataString(in.Credential.Metadata, antigravityProjectPaths...)
	if project == "" {
		return nil, "", fmt.Errorf("Antigravity credential metadata requires a project ID")
	}
	requestID := firstString(source, "requestId")
	if requestID == "" {
		requestID = strings.TrimSpace(in.RequestID)
	}
	if requestID == "" {
		requestID = stableClientID("antigravity-request", in.Credential.ID+":"+in.AffinityKey)
	}
	model := strings.TrimSpace(in.Model)
	if model == "" {
		return nil, "", fmt.Errorf("Antigravity request requires a model")
	}
	userAgent := firstString(source, "userAgent")
	if userAgent == "" {
		userAgent = antigravityUserAgent
	}
	requestType := firstString(source, "requestType")
	switch requestType {
	case "agent", "checkpoint", "endpoint_test":
	default:
		requestType = "agent"
	}
	sessionID := firstMetadataString(in.Credential.Metadata, antigravitySessionPaths...)
	if sessionID == "" {
		sessionSeed := in.AffinityKey
		if sessionSeed == "" {
			sessionSeed = requestID
		}
		sessionID = stableClientID("antigravity-session", in.Credential.ID+":"+sessionSeed)
	}
	envelope := map[string]any{
		"project":     project,
		"requestId":   requestID,
		"request":     inner,
		"model":       model,
		"userAgent":   userAgent,
		"requestType": requestType,
	}
	body, err := json.Marshal(envelope)
	return body, sessionID, err
}

var (
	antigravityProjectPaths = [][]string{
		{"project_id"},
		{"projectId"},
		{"project", "id"},
		{"cloudaicompanionProject"},
		{"cloudaicompanionProject", "id"},
		{"cloudAiCompanionProject"},
		{"cloudAiCompanionProject", "id"},
		{"antigravity", "project_id"},
		{"antigravity", "projectId"},
		{"metadata", "project_id"},
	}
	antigravityClientVersionPaths = [][]string{
		{"client_version"},
		{"clientVersion"},
		{"antigravity", "client_version"},
		{"metadata", "client_version"},
	}
	antigravitySessionPaths = [][]string{
		{"session_id"},
		{"sessionId"},
		{"antigravity", "session_id"},
		{"metadata", "session_id"},
	}
)
