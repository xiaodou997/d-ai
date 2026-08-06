package clientruntime

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"xiaodou/dai/internal/ai/domain"
)

const (
	GeminiCLIProfileRevision = "gemini-cli@0.1.5+dai.1"
	geminiCLIBaseURL         = "https://cloudcode-pa.googleapis.com"
	geminiCLIUserAgent       = "GeminiCLI/0.1.5 (Windows; AMD64)"
)

type geminiCLIProfileV015 struct{}

func (geminiCLIProfileV015) revision() string {
	return GeminiCLIProfileRevision
}

func (geminiCLIProfileV015) supports(protocol domain.UpstreamProtocol) bool {
	return protocol == domain.ProtocolGeminiGenerate
}

func (geminiCLIProfileV015) prepare(in Invocation) (*WireRequest, error) {
	if in.Protocol != domain.ProtocolGeminiGenerate {
		return nil, fmt.Errorf("gemini CLI profile requires %q, got %q", domain.ProtocolGeminiGenerate, in.Protocol)
	}
	body, err := applyGeminiCLIContract(in)
	if err != nil {
		return nil, err
	}
	action := "generateContent"
	url := geminiCLIBaseURL + "/v1internal:" + action
	accept := "application/json"
	if in.Stream {
		action = "streamGenerateContent"
		url = geminiCLIBaseURL + "/v1internal:" + action + "?alt=sse"
		accept = "text/event-stream"
	}
	return &WireRequest{
		Method: http.MethodPost,
		URL:    url,
		Headers: map[string]string{
			"Authorization": "Bearer " + in.Credential.AccessToken,
			"Content-Type":  "application/json",
			"Accept":        accept,
			"User-Agent":    geminiCLIUserAgent,
		},
		Body:     body,
		Protocol: domain.ProtocolGeminiGenerate,
	}, nil
}

func applyGeminiCLIContract(in Invocation) ([]byte, error) {
	source, inner, err := decodeGeminiEnvelopeSource(in.Body)
	if err != nil {
		return nil, err
	}
	delete(inner, "model")
	delete(inner, "stream")
	if sessionID := firstMetadataString(in.Credential.Metadata, geminiSessionPaths...); sessionID != "" {
		if _, exists := inner["session_id"]; !exists {
			if _, exists := inner["sessionId"]; !exists {
				inner["session_id"] = sessionID
			}
		}
	}

	promptID := firstString(source, "user_prompt_id", "userPromptId")
	if promptID == "" {
		promptID = strings.TrimSpace(in.RequestID)
	}
	if promptID == "" {
		promptID = stableClientID("gemini-cli-prompt", in.Credential.ID+":"+in.AffinityKey)
	}
	project := firstString(source, "project")
	if project == "" {
		project = firstMetadataString(in.Credential.Metadata, geminiProjectPaths...)
	}
	model := strings.TrimSpace(in.Model)
	if model == "" {
		return nil, fmt.Errorf("gemini CLI request requires a model")
	}
	envelope := map[string]any{
		"model":          model,
		"user_prompt_id": promptID,
		"request":        inner,
	}
	if project != "" {
		envelope["project"] = project
	}
	return json.Marshal(envelope)
}

func decodeGeminiEnvelopeSource(body []byte) (map[string]any, map[string]any, error) {
	var source map[string]any
	if err := json.Unmarshal(body, &source); err != nil {
		return nil, nil, fmt.Errorf("decode Gemini request: %w", err)
	}
	if source == nil {
		return nil, nil, fmt.Errorf("Gemini request body must be an object")
	}
	inner := source
	if existing, ok := source["request"].(map[string]any); ok {
		if _, hasContents := existing["contents"]; hasContents {
			inner = cloneAnyMap(existing)
		}
	} else {
		inner = cloneAnyMap(source)
	}
	if _, hasContents := inner["contents"]; !hasContents {
		return nil, nil, fmt.Errorf("Gemini request requires contents")
	}
	return source, inner, nil
}

var (
	geminiProjectPaths = [][]string{
		{"project"},
		{"project_id"},
		{"projectId"},
		{"project", "id"},
		{"cloudaicompanionProject"},
		{"cloudaicompanionProject", "id"},
		{"gemini_cli", "project_id"},
		{"geminiCli", "projectId"},
		{"metadata", "project_id"},
	}
	geminiSessionPaths = [][]string{
		{"session_id"},
		{"sessionId"},
		{"gemini_cli", "session_id"},
		{"geminiCli", "sessionId"},
		{"metadata", "session_id"},
	}
)

func stableClientID(kind, seed string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("dai:"+kind+":v1:"+seed)).String()
}
