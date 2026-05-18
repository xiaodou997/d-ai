package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	dbgen "uni-ai-api/backend/internal/db/gen"
	"uni-ai-api/backend/internal/domain"
	"uni-ai-api/backend/internal/secret"
)

// ============================================================================
// Model Discovery Handlers
// ============================================================================

// handleAdminFetchEndpointUpstreamModels calls the upstream /v1/models endpoint
// and returns inferred capability + protocol for each model. No DB writes.
func (s *Server) handleAdminFetchEndpointUpstreamModels(w http.ResponseWriter, r *http.Request) {
	providerID, ok := parseUUIDParam(w, r, "providerID")
	if !ok {
		return
	}
	endpointID, ok := parseUUIDParam(w, r, "endpointID")
	if !ok {
		return
	}

	endpoint, err := s.queries.GetProviderEndpoint(r.Context(), dbgen.GetProviderEndpointParams{
		ProviderID: providerID,
		ID:         endpointID,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}

	apiKey, err := secret.DecryptProviderKey(s.security.ProviderKeyMaster, endpoint.ApiKeyCiphertext)
	if err != nil {
		s.logger.Error("model discovery: decrypt api key failed",
			"error", err,
			"endpoint_id", endpointID,
			"request_id", requestIDFromContext(r.Context()))
		writeErr(w, http.StatusInternalServerError, BizErrInternal, "failed to decrypt api key")
		return
	}

	models, fetchErr := s.fetchUpstreamModelList(r.Context(), endpoint.BaseUrl, apiKey, endpoint.DefaultProtocol, endpoint.ExtraHeaders, int(endpoint.TimeoutMs))
	if fetchErr != nil {
		s.logger.Error("model discovery: fetch upstream models failed",
			"error", fetchErr,
			"endpoint_id", endpointID,
			"base_url", endpoint.BaseUrl,
			"default_protocol", endpoint.DefaultProtocol,
			"request_id", requestIDFromContext(r.Context()))
		writeErr(w, http.StatusBadGateway, BizErrUpstreamUnavailable, sanitizeUpstreamFetchError(fetchErr))
		return
	}

	writeOK(w, models)
}

// handleAdminImportEndpointUpstreamModels batch-creates upstream deployments from a
// caller-supplied list. Already-existing (endpoint_id, upstream_model, upstream_protocol)
// tuples are silently skipped (idempotent).
func (s *Server) handleAdminImportEndpointUpstreamModels(w http.ResponseWriter, r *http.Request) {
	providerID, ok := parseUUIDParam(w, r, "providerID")
	if !ok {
		return
	}
	endpointID, ok := parseUUIDParam(w, r, "endpointID")
	if !ok {
		return
	}

	// Verify the endpoint belongs to the provider.
	_, err := s.queries.GetProviderEndpoint(r.Context(), dbgen.GetProviderEndpointParams{
		ProviderID: providerID,
		ID:         endpointID,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}

	var req importUpstreamModelsRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if len(req.Models) == 0 {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "models list is empty")
		return
	}

	var created []upstreamModelImportDTO
	var skipped []string

	for _, m := range req.Models {
		if m.UpstreamModel == "" {
			continue
		}
		name := m.Name
		if name == "" {
			name = m.UpstreamModel
		}
		capType := m.CapabilityType
		if capType == "" {
			capType = defaultCapability
		}
		protocol := m.UpstreamProtocol
		if protocol == "" {
			protocol = defaultProtocol
		}

		tag, insertErr := s.postgres.Exec(r.Context(), `
			INSERT INTO ai_upstream_deployments
			  (endpoint_id, name, upstream_model, capability_type, upstream_protocol, status, health_status)
			VALUES ($1, $2, $3, $4, $5, 'active', 'unknown')
			ON CONFLICT (endpoint_id, upstream_model, upstream_protocol) DO NOTHING`,
			endpointID, name, m.UpstreamModel, capType, protocol,
		)
		if insertErr != nil {
			writeErr(w, http.StatusInternalServerError, BizErrInternal, fmt.Sprintf("insert %s failed: %s", m.UpstreamModel, insertErr.Error()))
			return
		}

		if tag.RowsAffected() == 0 {
			skipped = append(skipped, m.UpstreamModel)
		} else {
			created = append(created, upstreamModelImportDTO{
				UpstreamModel:    m.UpstreamModel,
				Name:             name,
				CapabilityType:   capType,
				UpstreamProtocol: protocol,
			})
		}
	}

	writeOK(w, map[string]any{
		"created": created,
		"skipped": skipped,
	})
}

// ============================================================================
// Upstream model list fetching
// ============================================================================

type discoveredModel struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	CapabilityType   string `json:"capability_type"`
	UpstreamProtocol string `json:"upstream_protocol"`
	Exists           bool   `json:"exists"`
}

type importUpstreamModelsRequest struct {
	Models []importModelItem `json:"models"`
}

type importModelItem struct {
	UpstreamModel    string `json:"upstream_model"`
	Name             string `json:"name"`
	CapabilityType   string `json:"capability_type"`
	UpstreamProtocol string `json:"upstream_protocol"`
}

type upstreamModelImportDTO struct {
	UpstreamModel    string `json:"upstream_model"`
	Name             string `json:"name"`
	CapabilityType   string `json:"capability_type"`
	UpstreamProtocol string `json:"upstream_protocol"`
}

func (s *Server) fetchUpstreamModelList(ctx context.Context, baseURL, apiKey, defaultProtocol string, extraHeaders []byte, timeoutMs int) ([]discoveredModel, error) {
	timeout := time.Duration(timeoutMs) * time.Millisecond
	if timeout <= 0 || timeout > 30*time.Second {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	listURL, authHeader, authValue := modelListURLAndAuth(baseURL, apiKey, defaultProtocol)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if authHeader != "" {
		req.Header.Set(authHeader, authValue)
	}
	if defaultProtocol == string(domain.EndpointProtocolAnthropic) {
		req.Header.Set("anthropic-version", "2023-06-01")
	}
	if err := applyAdminExtraHeaders(req.Header, extraHeaders); err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &upstreamFetchError{
			Status: resp.StatusCode,
			URL:    listURL,
			Body:   truncate(string(body), 500),
		}
	}

	return parseModelListResponse(body, defaultProtocol)
}

func modelListURLAndAuth(baseURL, apiKey, defaultProtocol string) (listURL, authHeader, authValue string) {
	base := strings.TrimRight(baseURL, "/")
	switch defaultProtocol {
	case string(domain.EndpointProtocolAnthropic):
		return base + "/v1/models", "x-api-key", apiKey
	case string(domain.EndpointProtocolGemini):
		return base + "/v1beta/models", "x-goog-api-key", apiKey
	default:
		return base + "/models", "Authorization", "Bearer " + apiKey
	}
}

// parseModelListResponse handles both OpenAI-style {data:[{id}]} and
// Anthropic/Gemini {data:[{id,display_name}]} / {models:[{name}]} formats.
func parseModelListResponse(body []byte, defaultProtocol string) ([]discoveredModel, error) {
	if defaultProtocol == string(domain.EndpointProtocolGemini) {
		return parseGeminiModelList(body)
	}

	var envelope struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
			Name        string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("parse model list: %w", err)
	}

	out := make([]discoveredModel, 0, len(envelope.Data))
	for _, m := range envelope.Data {
		id := m.ID
		if id == "" {
			continue
		}
		displayName := m.DisplayName
		if displayName == "" {
			displayName = id
		}
		cap, proto := inferCapabilityAndProtocol(id, defaultProtocol)
		out = append(out, discoveredModel{
			ID:               id,
			Name:             displayName,
			CapabilityType:   cap,
			UpstreamProtocol: proto,
		})
	}
	return out, nil
}

func parseGeminiModelList(body []byte) ([]discoveredModel, error) {
	var envelope struct {
		Models []struct {
			Name        string   `json:"name"`
			DisplayName string   `json:"displayName"`
			SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("parse gemini model list: %w", err)
	}

	out := make([]discoveredModel, 0, len(envelope.Models))
	for _, m := range envelope.Models {
		// Gemini model names are like "models/gemini-1.5-pro", strip prefix.
		id := strings.TrimPrefix(m.Name, "models/")
		if id == "" {
			continue
		}
		displayName := m.DisplayName
		if displayName == "" {
			displayName = id
		}
		cap, proto := inferCapabilityAndProtocol(id, string(domain.EndpointProtocolGemini))
		out = append(out, discoveredModel{
			ID:               id,
			Name:             displayName,
			CapabilityType:   cap,
			UpstreamProtocol: proto,
		})
	}
	return out, nil
}

// inferCapabilityAndProtocol returns the best-guess capability and upstream
// protocol for a given model ID and endpoint default_protocol.
func inferCapabilityAndProtocol(modelID, endpointProtocol string) (capabilityType, upstreamProtocol string) {
	lower := strings.ToLower(modelID)

	switch {
	case containsAny(lower, "text-embedding-", "embedding-", "-embed", "embed-"):
		return string(domain.CapabilityEmbedding), string(domain.ProtocolOpenAIEmbeddings)

	case containsAny(lower, "dall-e-", "gpt-image-", "stable-diffusion", "sdxl", "image-generation"):
		return string(domain.CapabilityImage), string(domain.ProtocolOpenAIImages)

	case containsAny(lower, "whisper-", "-transcription", "-asr"):
		return string(domain.CapabilityAudioSTT), string(domain.ProtocolOpenAIChat)

	case containsAny(lower, "tts-", "-tts", "text-to-speech"):
		return string(domain.CapabilityAudioTTS), string(domain.ProtocolOpenAIChat)

	case containsAny(lower, "rerank", "re-rank"):
		return string(domain.CapabilityRerank), string(domain.ProtocolOpenAIChat)

	case strings.HasPrefix(lower, "claude-"):
		if endpointProtocol == string(domain.EndpointProtocolAnthropic) {
			return string(domain.CapabilityChat), string(domain.ProtocolAnthropicMessages)
		}
		return string(domain.CapabilityChat), string(domain.ProtocolOpenAIChat)

	case containsAny(lower, "gemini-", "gemma-"):
		return string(domain.CapabilityChat), string(domain.ProtocolGeminiGenerate)

	default:
		return string(domain.CapabilityChat), string(domain.ProtocolOpenAIChat)
	}
}

func containsAny(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

// upstreamFetchError carries the raw upstream failure detail for logging while
// allowing the handler to render a sanitized message to the client.
type upstreamFetchError struct {
	Status int
	URL    string
	Body   string
}

func (e *upstreamFetchError) Error() string {
	return fmt.Sprintf("upstream %s returned %d: %s", e.URL, e.Status, e.Body)
}

// sanitizeUpstreamFetchError returns a short, user-safe message that hides raw
// upstream response bodies (which may include vendor-specific HTML, tunnel
// errors, internal hostnames, etc.). The full detail still goes to the log.
func sanitizeUpstreamFetchError(err error) string {
	var ufe *upstreamFetchError
	if errors.As(err, &ufe) {
		switch {
		case ufe.Status == http.StatusUnauthorized || ufe.Status == http.StatusForbidden:
			return "upstream rejected api key (check provider api_key)"
		case ufe.Status == http.StatusNotFound:
			return "upstream /models endpoint not found (check base_url and default_protocol)"
		case ufe.Status >= 500:
			return fmt.Sprintf("upstream unavailable (HTTP %d)", ufe.Status)
		default:
			return fmt.Sprintf("upstream returned HTTP %d", ufe.Status)
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "upstream request timed out"
	}
	return "failed to reach upstream"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
