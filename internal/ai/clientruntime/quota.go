package clientruntime

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/upstreamquota"
)

// These read-only subscription endpoints follow the native clients. Quota
// responses are reduced to numeric facts; identities and raw bodies stay local.
func (p codexProfileV01441) prepareQuota(in Inspection) (*WireRequest, error) {
	req, err := p.prepareInspection(in)
	if err != nil {
		return nil, err
	}
	req.URL = "https://chatgpt.com/backend-api/wham/usage"
	delete(req.Headers, "If-None-Match")
	return req, nil
}

func (claudeProfileV21220) prepareQuota(in Inspection) (*WireRequest, error) {
	return &WireRequest{Method: http.MethodGet, URL: claudeBaseURL + "/api/oauth/usage", Protocol: domain.ProtocolAnthropicMessages,
		Headers: map[string]string{"Authorization": "Bearer " + in.Credential.AccessToken, "Accept": "application/json", "User-Agent": claudeUserAgent, "anthropic-beta": "oauth-2025-04-20"}}, nil
}

func (geminiCLIProfileV015) prepareQuota(in Inspection) (*WireRequest, error) {
	project := firstMetadataString(in.Credential.Metadata, geminiProjectPaths...)
	if project == "" {
		return nil, fmt.Errorf("gemini quota inspection requires a project ID")
	}
	body, _ := json.Marshal(map[string]string{"project": project})
	return &WireRequest{Method: http.MethodPost, URL: geminiCLIBaseURL + "/v1internal:retrieveUserQuota", Protocol: domain.ProtocolGeminiGenerate,
		Headers: map[string]string{"Authorization": "Bearer " + in.Credential.AccessToken, "Content-Type": "application/json", "Accept": "application/json", "User-Agent": geminiCLIUserAgent}, Body: body}, nil
}

func (antigravityProfileV1016) prepareQuota(in Inspection) (*WireRequest, error) {
	project := firstMetadataString(in.Credential.Metadata, antigravityProjectPaths...)
	if project == "" {
		return nil, fmt.Errorf("antigravity quota inspection requires a project ID")
	}
	body, _ := json.Marshal(map[string]string{"project": project})
	return &WireRequest{Method: http.MethodPost, URL: antigravityBaseURL + "/v1internal:fetchAvailableModels", Protocol: domain.ProtocolGeminiGenerate,
		Headers: map[string]string{"Authorization": "Bearer " + in.Credential.AccessToken, "Content-Type": "application/json", "Accept": "application/json", "User-Agent": antigravityUserAgent, "x-client-name": "antigravity", "x-goog-api-client": antigravityGoogleAPIClient}, Body: body}, nil
}

func (codexProfileV01441) decodeQuota(body []byte, now time.Time) ([]domain.CredentialQuota, error) {
	return upstreamquota.CodexUsage(body, now)
}
func (claudeProfileV21220) decodeQuota(body []byte, now time.Time) ([]domain.CredentialQuota, error) {
	return upstreamquota.ClaudeUsage(body, now)
}
func (geminiCLIProfileV015) decodeQuota(body []byte, now time.Time) ([]domain.CredentialQuota, error) {
	return upstreamquota.GeminiUsage(body, now)
}
func (antigravityProfileV1016) decodeQuota(body []byte, now time.Time) ([]domain.CredentialQuota, error) {
	return upstreamquota.AntigravityUsage(body, now)
}
