package console

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dbgen "xiaodou/unihub/ai-service/internal/db/gen"
	"xiaodou/unihub/ai-service/internal/domain"
	"xiaodou/unihub/ai-service/internal/secret"
	providersvc "xiaodou/unihub/ai-service/internal/service/provider"
)

func (s *Console) handleAdminListProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := s.providerSvc.ListProviders(r.Context())
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, providersFromDomain(providers))
}

func (s *Console) handleAdminCreateProvider(w http.ResponseWriter, r *http.Request) {
	var req createProviderRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.Code == "" || req.Name == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "code and name are required")
		return
	}
	p, err := s.providerSvc.CreateProvider(r.Context(), providersvc.ProviderInput{
		Code:   req.Code,
		Name:   req.Name,
		Config: jsonObjectOrDefault(req.Config),
		Status: req.Status,
	})
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, providerFromDomain(p))
}

func (s *Console) handleAdminUpdateProvider(w http.ResponseWriter, r *http.Request) {
	if _, ok := parseUUIDParam(w, r, "providerID"); !ok {
		return
	}
	var req createProviderRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.Code == "" || req.Name == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "code and name are required")
		return
	}
	p, err := s.providerSvc.UpdateProvider(r.Context(), chi.URLParam(r, "providerID"), providersvc.ProviderInput{
		Code:   req.Code,
		Name:   req.Name,
		Config: jsonObjectOrDefault(req.Config),
		Status: req.Status,
	})
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, providerFromDomain(p))
}

func (s *Console) handleAdminUpdateProviderStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := parseUUIDParam(w, r, "providerID"); !ok {
		return
	}
	status, ok := decodeStatusUpdate(w, r)
	if !ok {
		return
	}
	p, err := s.providerSvc.UpdateProviderStatus(r.Context(), chi.URLParam(r, "providerID"), status)
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, providerFromDomain(p))
}

func (s *Console) handleAdminListProviderEndpoints(w http.ResponseWriter, r *http.Request) {
	if _, ok := parseUUIDParam(w, r, "providerID"); !ok {
		return
	}
	endpoints, err := s.providerSvc.ListEndpoints(r.Context(), chi.URLParam(r, "providerID"))
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, listProviderEndpointsFromDomain(endpoints))
}

func (s *Console) handleAdminCreateProviderEndpoint(w http.ResponseWriter, r *http.Request) {
	if _, ok := parseUUIDParam(w, r, "providerID"); !ok {
		return
	}

	var req createEndpointRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.Name == "" || req.BaseURL == "" || req.APIKey == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "name, base_url and api_key are required")
		return
	}
	ep, err := s.providerSvc.CreateEndpoint(r.Context(), providersvc.CreateEndpointInput{
		ProviderID:      chi.URLParam(r, "providerID"),
		Name:            req.Name,
		BaseURL:         req.BaseURL,
		APIKey:          req.APIKey,
		ExtraHeaders:    jsonObjectOrDefault(req.ExtraHeaders),
		Weight:          req.Weight,
		TimeoutMs:       req.TimeoutMs,
		DefaultProtocol: req.DefaultProtocol,
		PriceBookID:     req.PriceBookID,
		CostMultiplier:  req.CostMultiplier,
		Status:          req.Status,
	})
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, providerEndpointFromDomain(ep))
}

func (s *Console) handleAdminUpdateProviderEndpoint(w http.ResponseWriter, r *http.Request) {
	if _, ok := parseUUIDParam(w, r, "providerID"); !ok {
		return
	}
	if _, ok := parseUUIDParam(w, r, "endpointID"); !ok {
		return
	}
	var req createEndpointRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.Name == "" || req.BaseURL == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "name and base_url are required")
		return
	}
	ep, err := s.providerSvc.UpdateEndpoint(r.Context(), providersvc.UpdateEndpointInput{
		ProviderID:      chi.URLParam(r, "providerID"),
		ID:              chi.URLParam(r, "endpointID"),
		Name:            req.Name,
		BaseURL:         req.BaseURL,
		APIKey:          req.APIKey,
		ExtraHeaders:    jsonObjectOrDefault(req.ExtraHeaders),
		Weight:          req.Weight,
		TimeoutMs:       req.TimeoutMs,
		DefaultProtocol: req.DefaultProtocol,
		PriceBookID:     req.PriceBookID,
		CostMultiplier:  req.CostMultiplier,
		Status:          req.Status,
	})
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, providerEndpointFromDomain(ep))
}

func (s *Console) handleAdminUpdateProviderEndpointStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := parseUUIDParam(w, r, "providerID"); !ok {
		return
	}
	if _, ok := parseUUIDParam(w, r, "endpointID"); !ok {
		return
	}
	status, ok := decodeStatusUpdate(w, r)
	if !ok {
		return
	}
	ep, err := s.providerSvc.UpdateEndpointStatus(r.Context(), chi.URLParam(r, "providerID"), chi.URLParam(r, "endpointID"), status)
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, providerEndpointFromDomain(ep))
}

func (s *Console) handleAdminDeleteProviderEndpoint(w http.ResponseWriter, r *http.Request) {
	if _, ok := parseUUIDParam(w, r, "providerID"); !ok {
		return
	}
	if _, ok := parseUUIDParam(w, r, "endpointID"); !ok {
		return
	}
	if err := s.providerSvc.DeleteEndpoint(r.Context(), chi.URLParam(r, "providerID"), chi.URLParam(r, "endpointID")); err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, map[string]string{"status": "deleted"})
}

// ---- provider domain → wire DTO ----

func providerFromDomain(p domain.Provider) providerDTO {
	return providerDTO{
		ID:        p.ID,
		Code:      p.Code,
		Name:      p.Name,
		Config:    rawJSON(p.Config),
		Status:    p.Status,
		CreatedAt: timeToMillisPtr(p.CreatedAt),
		UpdatedAt: timeToMillisPtr(p.UpdatedAt),
	}
}

func providersFromDomain(providers []domain.Provider) []providerDTO {
	out := make([]providerDTO, len(providers))
	for i, p := range providers {
		out[i] = providerFromDomain(p)
	}
	return out
}

func providerEndpointFromDomain(e domain.ProviderEndpoint) providerEndpointDTO {
	return providerEndpointDTO{
		ID:              pgUUIDFromString(e.ID),
		ProviderID:      pgUUIDFromString(e.ProviderID),
		Name:            e.Name,
		BaseUrl:         e.BaseURL,
		ExtraHeaders:    rawJSON(e.ExtraHeaders),
		Weight:          e.Weight,
		TimeoutMs:       e.TimeoutMs,
		DefaultProtocol: e.DefaultProtocol,
		PriceBookID:     e.PriceBookID,
		CostMultiplier:  e.CostMultiplier,
		Status:          e.Status,
		CreatedAt:       timeToMillisPtr(e.CreatedAt),
		UpdatedAt:       timeToMillisPtr(e.UpdatedAt),
	}
}

func listProviderEndpointsFromDomain(endpoints []domain.ProviderEndpoint) []listProviderEndpointDTO {
	out := make([]listProviderEndpointDTO, len(endpoints))
	for i, e := range endpoints {
		out[i] = listProviderEndpointDTO{
			ID:              pgUUIDFromString(e.ID),
			ProviderID:      pgUUIDFromString(e.ProviderID),
			Name:            e.Name,
			BaseUrl:         e.BaseURL,
			ExtraHeaders:    rawJSON(e.ExtraHeaders),
			Weight:          e.Weight,
			TimeoutMs:       e.TimeoutMs,
			DefaultProtocol: e.DefaultProtocol,
			PriceBookID:     e.PriceBookID,
			CostMultiplier:  e.CostMultiplier,
			Status:          e.Status,
			CreatedAt:       timeToMillisPtr(e.CreatedAt),
			UpdatedAt:       timeToMillisPtr(e.UpdatedAt),
		}
	}
	return out
}

func (s *Console) handleAdminCheckUpstreamDeploymentHealth(w http.ResponseWriter, r *http.Request) {
	deploymentID, ok := parseUUIDParam(w, r, "deploymentID")
	if !ok {
		return
	}

	deployment, err := s.queries.GetUpstreamDeploymentForHealthCheck(r.Context(), deploymentID)
	if err != nil {
		writeDBErr(w, err)
		return
	}
	if !upstreamDeploymentHealthProbeSupported(deployment.UpstreamProtocol) {
		writeErr(w, http.StatusUnprocessableEntity, BizErrBadRequest, "deployment protocol does not support active health probes")
		return
	}

	status := "healthy"
	lastHealthError := pgtype.Text{}
	checkErr := s.checkUpstreamDeploymentReachable(r, deployment)
	if checkErr != nil {
		status = "unhealthy"
		lastHealthError = pgtype.Text{String: checkErr.Error(), Valid: true}
	}

	row, err := s.queries.UpdateUpstreamDeploymentHealth(r.Context(), dbgen.UpdateUpstreamDeploymentHealthParams{
		ID:              deploymentID,
		HealthStatus:    status,
		LastHealthError: lastHealthError,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}

	response := map[string]any{
		"deployment": fromAiUpstreamDeployment(row),
		"status":     status,
	}
	if checkErr != nil {
		response["error"] = checkErr.Error()
	}
	writeOK(w, response)
}

func upstreamDeploymentHealthProbeSupported(protocol string) bool {
	return protocol == string(domain.ProtocolOpenAIChat) ||
		protocol == string(domain.ProtocolOpenAIResponses) ||
		protocol == string(domain.ProtocolOpenAIEmbeddings)
}

func (s *Console) checkUpstreamDeploymentReachable(r *http.Request, deployment dbgen.GetUpstreamDeploymentForHealthCheckRow) error {
	defaultPath, body, err := healthProbeRequest(deployment.UpstreamModel, deployment.UpstreamProtocol)
	if err != nil {
		return err
	}
	url, err := buildProbeURL(deployment.BaseUrl, optionalText(deployment.RequestPath), defaultPath)
	if err != nil {
		return err
	}
	ctx := r.Context()
	timeout := time.Duration(deployment.TimeoutMs) * time.Millisecond
	if timeout <= 0 || timeout > 10*time.Second {
		timeout = 10 * time.Second
	}
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if providerKey, err := secret.DecryptProviderKey(s.security.ProviderKeyMaster, deployment.ApiKeyCiphertext); err == nil && providerKey != "" {
		req.Header.Set("Authorization", "Bearer "+providerKey)
	}
	if err := applyAdminExtraHeaders(req.Header, deployment.ExtraHeaders); err != nil {
		return err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New("endpoint returned " + strconv.Itoa(resp.StatusCode))
	}
	return nil
}

func healthProbeRequest(upstreamModel string, protocol string) (string, []byte, error) {
	switch protocol {
	case string(domain.ProtocolOpenAIChat):
		body, err := json.Marshal(map[string]any{
			"model":      upstreamModel,
			"messages":   []map[string]string{{"role": "user", "content": "ping"}},
			"max_tokens": 1,
		})
		return "/chat/completions", body, err
	case string(domain.ProtocolOpenAIResponses):
		body, err := json.Marshal(map[string]any{
			"model":             upstreamModel,
			"input":             "ping",
			"max_output_tokens": 1,
		})
		return "/responses", body, err
	case string(domain.ProtocolOpenAIEmbeddings):
		body, err := json.Marshal(map[string]any{
			"model": upstreamModel,
			"input": "ping",
		})
		return "/embeddings", body, err
	default:
		return "", nil, errors.New("active deployment uses unsupported probe protocol")
	}
}

func buildProbeURL(baseURL, customPath, defaultPath string) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil {
		return "", fmt.Errorf("parse upstream base url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("invalid upstream base url")
	}
	path := defaultPath
	if customPath != "" {
		path = customPath
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + path
	return parsed.String(), nil
}

func applyAdminExtraHeaders(headers http.Header, raw []byte) error {
	if len(raw) == 0 {
		return nil
	}
	var values map[string]string
	if err := json.Unmarshal(raw, &values); err == nil {
		for key, value := range values {
			headers.Set(key, value)
		}
		return nil
	}
	var anyValues map[string]any
	if err := json.Unmarshal(raw, &anyValues); err != nil {
		return fmt.Errorf("parse extra_headers: %w", err)
	}
	for key, value := range anyValues {
		headers.Set(key, fmt.Sprint(value))
	}
	return nil
}
