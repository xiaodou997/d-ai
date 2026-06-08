package console

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"xiaodou/unihub/ai-service/internal/serving"
)

// handleAdminGetRouteWeights returns the scorer weights for a given scope.
//
//	GET /api/v1/route-weights/{scope}
func (s *Console) handleAdminGetRouteWeights(w http.ResponseWriter, r *http.Request) {
	scope := chi.URLParam(r, "scope")
	if scope == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "scope path param is required")
		return
	}
	weights := s.routeWeightsStore.Get(r.Context(), scope)
	writeOK(w, map[string]any{
		"scope":   scope,
		"weights": weights,
	})
}

// handleAdminPutRouteWeights upserts the scorer weights for a given scope.
//
//	PUT /api/v1/route-weights/{scope}
func (s *Console) handleAdminPutRouteWeights(w http.ResponseWriter, r *http.Request) {
	scope := chi.URLParam(r, "scope")
	if scope == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "scope path param is required")
		return
	}

	var req struct {
		Weights serving.ScoreWeights `json:"weights"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, err.Error())
		return
	}

	if err := s.routeWeightsStore.Upsert(r.Context(), scope, req.Weights); err != nil {
		writeErr(w, http.StatusInternalServerError, BizErrInternal, err.Error())
		return
	}
	writeOK(w, map[string]any{
		"scope":   scope,
		"weights": req.Weights,
	})
}
