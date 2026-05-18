package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"

	"uni-ai-api/backend/internal/domain"
)

type modelsResponse struct {
	Object string      `json:"object"`
	Data   []modelItem `json:"data"`
}

type modelItem struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
}

// Anthropic /v1/models response shape.
type anthropicModelsResponse struct {
	Data    []anthropicModelItem `json:"data"`
	HasMore bool                 `json:"has_more"`
	FirstID string               `json:"first_id,omitempty"`
	LastID  string               `json:"last_id,omitempty"`
}

type anthropicModelItem struct {
	Type        string `json:"type"`         // "model"
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at"`
}

func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	auth, ok := runtimeAuthFromContext(r.Context())
	if !ok {
		writeRuntimeErrorByProtocol(w, clientProtoFromRequest(r),
			http.StatusUnauthorized, "Invalid API key.", "invalid_api_key")
		return
	}

	models, err := s.callableModels(r, auth)
	if err != nil {
		s.logger.Error("list models failed", "error", err, "request_id", requestIDFromContext(r.Context()))
		writeRuntimeErrorByProtocol(w, clientProtoFromRequest(r),
			http.StatusInternalServerError, "Failed to list models.", "server_error")
		return
	}

	if clientProtoFromRequest(r) == domain.ProtocolAnthropicMessages {
		writeJSON(w, http.StatusOK, buildAnthropicModelsResponse(models))
		return
	}

	items := make([]modelItem, 0, len(models))
	for _, m := range models {
		items = append(items, modelItem{
			ID:      m,
			Object:  "model",
			OwnedBy: "uni-ai-api",
		})
	}
	writeJSON(w, http.StatusOK, modelsResponse{Object: "list", Data: items})
}

// clientProtoFromRequest infers the wire protocol the caller expects on
// endpoints that don't carry a body (GET /v1/models, errors, etc.) by sniffing
// the User-Agent. Anthropic SDKs and Claude Code identify themselves; the
// `anthropic-version` header is also a strong signal.
func clientProtoFromRequest(r *http.Request) domain.UpstreamProtocol {
	if r.Header.Get("anthropic-version") != "" {
		return domain.ProtocolAnthropicMessages
	}
	ua := strings.ToLower(r.Header.Get("User-Agent"))
	if strings.Contains(ua, "anthropic") || strings.Contains(ua, "claude") {
		return domain.ProtocolAnthropicMessages
	}
	return domain.ProtocolOpenAIChat
}

func buildAnthropicModelsResponse(codes []string) anthropicModelsResponse {
	items := make([]anthropicModelItem, 0, len(codes))
	for _, c := range codes {
		items = append(items, anthropicModelItem{
			Type:        "model",
			ID:          c,
			DisplayName: c,
		})
	}
	resp := anthropicModelsResponse{Data: items}
	if len(items) > 0 {
		resp.FirstID = items[0].ID
		resp.LastID = items[len(items)-1].ID
	}
	return resp
}

// callableModels - only check tenant grant, no user grant required
func (s *Server) callableModels(r *http.Request, auth RuntimeAuth) ([]string, error) {
	allowed, err := allowedModelSet(auth.APIKey.AllowedModels)
	if err != nil {
		return nil, err
	}

	// Only tenant grant is required, user grant is no longer needed
	rows, err := s.queries.ListModelsForTenant(r.Context(), auth.APIKey.TenantID)
	if err != nil {
		return nil, err
	}
	modelCodes := make([]string, 0, len(rows))
	for _, row := range rows {
		modelCodes = appendIfAllowed(modelCodes, row.ModelCode, allowed)
	}
	return modelCodes, nil
}

func allowedModelSet(raw []byte) (map[string]struct{}, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	var models []string
	if err := json.Unmarshal(raw, &models); err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return nil, nil
	}

	set := make(map[string]struct{}, len(models))
	for _, model := range models {
		set[model] = struct{}{}
	}
	return set, nil
}

func appendIfAllowed(dst []string, model string, allowed map[string]struct{}) []string {
	if allowed == nil {
		return append(dst, model)
	}
	if _, ok := allowed[model]; ok {
		return append(dst, model)
	}
	return dst
}
