package console

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"xiaodou/unihub/ai-service/internal/domain"
	modelsvc "xiaodou/unihub/ai-service/internal/service/model"
)

func (s *Console) handleAdminListModels(w http.ResponseWriter, r *http.Request) {
	models, err := s.modelSvc.List(r.Context())
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, modelsFromDomain(models))
}

func (s *Console) handleAdminCreateModel(w http.ResponseWriter, r *http.Request) {
	var req createModelRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.ModelCode == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "model_code is required")
		return
	}

	model, err := s.modelSvc.Create(r.Context(), req.toModelInput())
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, modelFromDomain(model))
}

func (s *Console) handleAdminUpdateModel(w http.ResponseWriter, r *http.Request) {
	if _, ok := parseUUIDParam(w, r, "modelID"); !ok {
		return
	}
	var req createModelRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.ModelCode == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "model_code is required")
		return
	}
	model, err := s.modelSvc.Update(r.Context(), chi.URLParam(r, "modelID"), req.toModelInput())
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, modelFromDomain(model))
}

func (s *Console) handleAdminUpdateModelStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := parseUUIDParam(w, r, "modelID"); !ok {
		return
	}
	status, ok := decodeStatusUpdate(w, r)
	if !ok {
		return
	}

	model, err := s.modelSvc.UpdateStatus(r.Context(), chi.URLParam(r, "modelID"), status)
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, modelFromDomain(model))
}

func (s *Console) handleAdminListModelRoutes(w http.ResponseWriter, r *http.Request) {
	if _, ok := parseUUIDParam(w, r, "modelID"); !ok {
		return
	}
	items, err := s.modelRouteSvc.List(r.Context(), chi.URLParam(r, "modelID"))
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, modelRouteListFromDomain(items))
}

func (s *Console) handleAdminCreateModelRoute(w http.ResponseWriter, r *http.Request) {
	if _, ok := parseUUIDParam(w, r, "modelID"); !ok {
		return
	}

	var req createModelRouteRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.UpstreamDeploymentID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "upstream_deployment_id is required")
		return
	}
	if _, err := parseUUID(req.UpstreamDeploymentID); err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid upstream_deployment_id")
		return
	}

	route, err := s.modelRouteSvc.Create(r.Context(), req.toRouteInput(chi.URLParam(r, "modelID"), ""))
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, modelRouteFromDomain(route))
}

func (s *Console) handleAdminGetModelRoute(w http.ResponseWriter, r *http.Request) {
	if _, ok := parseUUIDParam(w, r, "routeID"); !ok {
		return
	}
	route, err := s.modelRouteSvc.Get(r.Context(), chi.URLParam(r, "routeID"))
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, modelRouteFromDomain(route))
}

func (s *Console) handleAdminUpdateModelRoute(w http.ResponseWriter, r *http.Request) {
	if _, ok := parseUUIDParam(w, r, "modelID"); !ok {
		return
	}
	if _, ok := parseUUIDParam(w, r, "routeID"); !ok {
		return
	}
	var req createModelRouteRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.UpstreamDeploymentID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "upstream_deployment_id is required")
		return
	}
	if _, err := parseUUID(req.UpstreamDeploymentID); err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid upstream_deployment_id")
		return
	}

	route, err := s.modelRouteSvc.Update(r.Context(), req.toRouteInput(chi.URLParam(r, "modelID"), chi.URLParam(r, "routeID")))
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, modelRouteFromDomain(route))
}

func (s *Console) handleAdminUpdateModelRouteStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := parseUUIDParam(w, r, "modelID"); !ok {
		return
	}
	if _, ok := parseUUIDParam(w, r, "routeID"); !ok {
		return
	}
	status, ok := decodeStatusUpdate(w, r)
	if !ok {
		return
	}

	route, err := s.modelRouteSvc.UpdateStatus(r.Context(), chi.URLParam(r, "modelID"), chi.URLParam(r, "routeID"), status)
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, modelRouteFromDomain(route))
}

func (s *Console) handleAdminDeleteModelRoute(w http.ResponseWriter, r *http.Request) {
	if _, ok := parseUUIDParam(w, r, "modelID"); !ok {
		return
	}
	if _, ok := parseUUIDParam(w, r, "routeID"); !ok {
		return
	}

	if err := s.modelRouteSvc.Delete(r.Context(), chi.URLParam(r, "modelID"), chi.URLParam(r, "routeID")); err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, map[string]string{"status": "deleted"})
}

// maxCreditsPerField is the maximum allowed value for a single price field in credits.
// This prevents accidental input of absurdly large values (e.g. entering micro-credits
// as credits). 1,000,000 积分 = 10,000 元人民币 should be more than enough.
const maxCreditsPerField int64 = 1_000_000

// ===========================================================================
// request → service input + domain → wire DTO mappers (model + routes)
// ===========================================================================

func (req createModelRequest) toModelInput() modelsvc.ModelInput {
	return modelsvc.ModelInput{
		ModelCode:              req.ModelCode,
		CapabilityType:         req.CapabilityType,
		ContextWindow:          req.ContextWindow,
		DefaultMaxOutputTokens: req.DefaultMaxOutputTokens,
		MaxOutputTokens:        req.MaxOutputTokens,
		Status:                 req.Status,
	}
}

func (req createModelRouteRequest) toRouteInput(modelID, routeID string) modelsvc.RouteInput {
	return modelsvc.RouteInput{
		ModelID:              modelID,
		RouteID:              routeID,
		UpstreamDeploymentID: req.UpstreamDeploymentID,
		Priority:             req.Priority,
		Weight:               req.Weight,
		SupportsStream:       req.SupportsStream,
		Status:               req.Status,
	}
}

func modelFromDomain(m domain.ManagedModel) modelDTO {
	return modelDTO{
		ID:                     pgUUIDFromString(m.ID),
		ModelCode:              m.ModelCode,
		CapabilityType:         m.CapabilityType,
		ContextWindow:          int32PtrToPg(m.ContextWindow),
		DefaultMaxOutputTokens: m.DefaultMaxOutputTokens,
		MaxOutputTokens:        int32PtrToPg(m.MaxOutputTokens),
		Status:                 m.Status,
		CreatedAt:              timeToMillisPtr(m.CreatedAt),
		UpdatedAt:              timeToMillisPtr(m.UpdatedAt),
	}
}

func modelsFromDomain(models []domain.ManagedModel) []modelDTO {
	out := make([]modelDTO, len(models))
	for i, m := range models {
		out[i] = modelFromDomain(m)
	}
	return out
}

func modelRouteFromDomain(rt domain.ModelRoute) modelRouteDTO {
	return modelRouteDTO{
		ID:                   pgUUIDFromString(rt.ID),
		ModelID:              pgUUIDFromString(rt.ModelID),
		UpstreamDeploymentID: pgUUIDFromString(rt.UpstreamDeploymentID),
		Priority:             rt.Priority,
		Weight:               rt.Weight,
		SupportsStream:       rt.SupportsStream,
		Status:               rt.Status,
		CreatedAt:            timeToMillisPtr(rt.CreatedAt),
		UpdatedAt:            timeToMillisPtr(rt.UpdatedAt),
	}
}

func modelRouteListFromDomain(items []domain.ModelRouteListItem) []listModelRouteDTO {
	out := make([]listModelRouteDTO, len(items))
	for i, it := range items {
		out[i] = listModelRouteDTO{
			ID:                     pgUUIDFromString(it.ID),
			ModelID:                pgUUIDFromString(it.ModelID),
			UpstreamDeploymentID:   pgUUIDFromString(it.UpstreamDeploymentID),
			Priority:               it.Priority,
			Weight:                 it.Weight,
			SupportsStream:         it.SupportsStream,
			Status:                 it.Status,
			CreatedAt:              timeToMillisPtr(it.CreatedAt),
			UpdatedAt:              timeToMillisPtr(it.UpdatedAt),
			UpstreamDeploymentName: it.UpstreamDeploymentName,
			UpstreamModel:          it.UpstreamModel,
			CapabilityType:         it.CapabilityType,
			UpstreamProtocol:       it.UpstreamProtocol,
			HealthStatus:           it.HealthStatus,
			CredentialSource:       it.CredentialSource,
			EndpointID:             pgUUIDFromString(it.EndpointID),
			EndpointName:           it.EndpointName,
			BaseUrl:                it.BaseURL,
			ProviderID:             pgUUIDFromString(it.ProviderID),
			ProviderCode:           it.ProviderCode,
			ProviderName:           it.ProviderName,
			PoolID:                 pgUUIDFromString(it.PoolID),
			PoolName:               it.PoolName,
			FixedProviderType:      it.FixedProviderType,
		}
	}
	return out
}
