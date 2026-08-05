package gateway

import (
	"net/http"
	"strings"

	"go.uber.org/zap"
	"xiaodou/dai/internal/ai/domain"
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
	Type        string `json:"type"` // "model"
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at"`
}

func (s *Gateway) handleListModels(w http.ResponseWriter, r *http.Request) {
	auth, ok := runtimeAuthFromContext(r.Context())
	if !ok {
		WriteRuntimeErrorByProtocol(w, clientProtoFromRequest(r),
			http.StatusUnauthorized, "Invalid API key.", "invalid_api_key")
		return
	}

	models, err := s.callableModels(r, auth)
	if err != nil {
		s.logger.Error("list models failed",
			gatewayLogFields(r.Context(), auth.Subject.TenantID, auth.Subject.UserID, zap.Error(err))...,
		)
		WriteRuntimeErrorByProtocol(w, clientProtoFromRequest(r),
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
			ID:     m,
			Object: "model",
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

// callableModels returns the caller-visible callable model directory. Tenant
// owners see tenant-visible groups; end users see tenant-default-visible groups
// plus their explicit group exceptions.
func (s *Gateway) callableModels(r *http.Request, auth RuntimeAuth) ([]string, error) {
	allowed := allowedModelSetFromSlice(auth.Subject.AllowedModels)

	rows, err := s.postgres.Query(r.Context(), `
		SELECT DISTINCT um.model_code
		FROM ai_group_targets gt
		JOIN ai_groups g
		  ON g.id = gt.group_id AND g.status = 'active'
		JOIN ai_upstream_models um
		  ON um.upstream_kind = gt.target_kind
		 AND um.upstream_id = gt.target_id
		 AND um.status = 'active'
		JOIN ai_price_book_entries e
		  ON e.price_book_id = g.retail_price_book_id
		 AND e.model_code = um.model_code
		 AND e.capability_type = um.capability_type
		LEFT JOIN ai_user_groups ug
		  ON ug.group_id = g.id
		 AND ug.tenant_id = $1
		 AND ug.user_id = $2
		LEFT JOIN ai_upstream_accounts a
		  ON gt.target_kind = 'direct_upstream' AND a.id = gt.target_id
		LEFT JOIN ai_credential_pools cp
		  ON gt.target_kind = 'oauth_pool' AND cp.id = gt.target_id
		WHERE gt.status = 'active'
		  AND g.tenant_id = $1
		  AND ($2::text = '' OR g.user_default_visible OR ug.id IS NOT NULL)
		  AND (
		    (gt.target_kind = 'direct_upstream' AND a.status = 'active')
		    OR
		    (gt.target_kind = 'oauth_pool' AND cp.status = 'active')
		  )
		  AND (
		    COALESCE(a.tenant_access_mode, cp.tenant_access_mode) = 'public'
		    OR EXISTS (
		      SELECT 1 FROM ai_upstream_resource_tenant_policies rg
		      WHERE rg.resource_kind = gt.target_kind AND rg.resource_id = gt.target_id AND rg.tenant_id = $1
		        AND rg.access_granted
		    )
		  )
		ORDER BY um.model_code ASC
	`, auth.Subject.TenantID, auth.Subject.UserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	modelCodes := make([]string, 0)
	for rows.Next() {
		var modelCode string
		if err := rows.Scan(&modelCode); err != nil {
			return nil, err
		}
		modelCodes = appendIfAllowed(modelCodes, modelCode, allowed)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return modelCodes, nil
}

func allowedModelSetFromSlice(models []string) map[string]struct{} {
	if len(models) == 0 {
		return nil
	}

	set := make(map[string]struct{}, len(models))
	for _, model := range models {
		set[model] = struct{}{}
	}
	return set
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
