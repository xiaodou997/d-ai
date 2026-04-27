package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"

	dbgen "uni-ai-api/backend/internal/db/gen"
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

func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	auth, ok := runtimeAuthFromContext(r.Context())
	if !ok {
		writeOpenAIError(w, http.StatusUnauthorized, "Invalid API key.", "invalid_api_key", "invalid_api_key")
		return
	}

	models, err := s.callableModels(r, auth)
	if err != nil {
		s.logger.Error("list models failed", "error", err, "request_id", requestIDFromContext(r.Context()))
		writeOpenAIError(w, http.StatusInternalServerError, "Failed to list models.", "server_error", "server_error")
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

	writeJSON(w, http.StatusOK, modelsResponse{
		Object: "list",
		Data:   items,
	})
}

func (s *Server) callableModels(r *http.Request, auth RuntimeAuth) ([]string, error) {
	allowed, err := allowedModelSet(auth.APIKey.AllowedModels)
	if err != nil {
		return nil, err
	}

	var modelCodes []string
	switch auth.APIKey.OwnerType {
	case "tenant":
		rows, err := s.queries.ListModelsForTenant(r.Context(), auth.APIKey.TenantID)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			modelCodes = appendIfAllowed(modelCodes, row.ModelCode, allowed)
		}
		return modelCodes, nil
	case "user":
		if !auth.APIKey.UserID.Valid {
			return nil, errors.New("user api key missing user_id")
		}
		rows, err := s.queries.ListModelsForUser(r.Context(), dbgen.ListModelsForUserParams{
			TenantID: auth.APIKey.TenantID,
			UserID:   auth.APIKey.UserID.String,
		})
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			modelCodes = appendIfAllowed(modelCodes, row.ModelCode, allowed)
		}
		return modelCodes, nil
	default:
		return nil, errors.New("unsupported api key owner_type")
	}
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
