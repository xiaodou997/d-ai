package httpserver

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

	"github.com/jackc/pgx/v5/pgtype"

	dbgen "uni-ai-api/backend/internal/db/gen"
	"uni-ai-api/backend/internal/domain"
	"uni-ai-api/backend/internal/secret"
)

func (s *Server) handleAdminListProviders(w http.ResponseWriter, r *http.Request) {
	rows, err := s.queries.ListProviders(r.Context())
	if err != nil {
		s.writeAdminServerError(w, r, "list providers failed", err)
		return
	}
	writeOK(w, fromProviders(rows))
}

func (s *Server) handleAdminCreateProvider(w http.ResponseWriter, r *http.Request) {
	var req createProviderRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.Code == "" || req.Name == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "code and name are required")
		return
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}

	row, err := s.queries.CreateProvider(r.Context(), dbgen.CreateProviderParams{
		Code:   req.Code,
		Name:   req.Name,
		Config: jsonObjectOrDefault(req.Config),
		Status: req.Status,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromProvider(row))
}

func (s *Server) handleAdminUpdateProvider(w http.ResponseWriter, r *http.Request) {
	providerID, ok := parseUUIDParam(w, r, "providerID")
	if !ok {
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
	if req.Status == "" {
		req.Status = defaultStatus
	}
	row, err := s.queries.UpdateProvider(r.Context(), dbgen.UpdateProviderParams{
		ID:     providerID,
		Code:   req.Code,
		Name:   req.Name,
		Config: jsonObjectOrDefault(req.Config),
		Status: req.Status,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromProvider(row))
}

func (s *Server) handleAdminUpdateProviderStatus(w http.ResponseWriter, r *http.Request) {
	providerID, ok := parseUUIDParam(w, r, "providerID")
	if !ok {
		return
	}
	status, ok := decodeStatusUpdate(w, r)
	if !ok {
		return
	}

	row, err := s.queries.UpdateProviderStatus(r.Context(), dbgen.UpdateProviderStatusParams{
		ID:     providerID,
		Status: status,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromProvider(row))
}

func (s *Server) handleAdminListProviderEndpoints(w http.ResponseWriter, r *http.Request) {
	providerID, ok := parseUUIDParam(w, r, "providerID")
	if !ok {
		return
	}

	rows, err := s.queries.ListProviderEndpoints(r.Context(), providerID)
	if err != nil {
		s.writeAdminServerError(w, r, "list provider endpoints failed", err)
		return
	}
	writeOK(w, fromListProviderEndpoints(rows))
}

func (s *Server) handleAdminCreateProviderEndpoint(w http.ResponseWriter, r *http.Request) {
	providerID, ok := parseUUIDParam(w, r, "providerID")
	if !ok {
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
	if req.Status == "" {
		req.Status = defaultStatus
	}

	ciphertext, err := secret.EncryptProviderKey(s.security.ProviderKeyMaster, req.APIKey)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, err.Error())
		return
	}

	row, err := s.queries.CreateProviderEndpoint(r.Context(), dbgen.CreateProviderEndpointParams{
		ProviderID:       providerID,
		Name:             req.Name,
		BaseUrl:          req.BaseURL,
		ApiKeyCiphertext: ciphertext,
		ExtraHeaders:     jsonObjectOrDefault(req.ExtraHeaders),
		Weight:           int32OrDefault(req.Weight, defaultEndpointWeight),
		TimeoutMs:        int32OrDefault(req.TimeoutMs, defaultTimeoutMs),
		Status:           req.Status,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromCreateProviderEndpoint(row))
}

func (s *Server) handleAdminUpdateProviderEndpoint(w http.ResponseWriter, r *http.Request) {
	providerID, ok := parseUUIDParam(w, r, "providerID")
	if !ok {
		return
	}
	endpointID, ok := parseUUIDParam(w, r, "endpointID")
	if !ok {
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
	if req.Status == "" {
		req.Status = defaultStatus
	}
	current, err := s.queries.GetProviderEndpoint(r.Context(), dbgen.GetProviderEndpointParams{ProviderID: providerID, ID: endpointID})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	ciphertext := current.ApiKeyCiphertext
	if req.APIKey != "" {
		ciphertext, err = secret.EncryptProviderKey(s.security.ProviderKeyMaster, req.APIKey)
		if err != nil {
			writeErr(w, http.StatusBadRequest, BizErrBadRequest, err.Error())
			return
		}
	}
	row, err := s.queries.UpdateProviderEndpoint(r.Context(), dbgen.UpdateProviderEndpointParams{
		ProviderID: providerID, ID: endpointID, Name: req.Name, BaseUrl: req.BaseURL,
		ApiKeyCiphertext: ciphertext, ExtraHeaders: jsonObjectOrDefault(req.ExtraHeaders), Weight: int32OrDefault(req.Weight, defaultEndpointWeight),
		TimeoutMs: int32OrDefault(req.TimeoutMs, defaultTimeoutMs), Status: req.Status,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromUpdateProviderEndpoint(row))
}

func (s *Server) handleAdminUpdateProviderEndpointStatus(w http.ResponseWriter, r *http.Request) {
	providerID, ok := parseUUIDParam(w, r, "providerID")
	if !ok {
		return
	}
	endpointID, ok := parseUUIDParam(w, r, "endpointID")
	if !ok {
		return
	}
	status, ok := decodeStatusUpdate(w, r)
	if !ok {
		return
	}

	row, err := s.queries.UpdateProviderEndpointStatus(r.Context(), dbgen.UpdateProviderEndpointStatusParams{
		ProviderID: providerID,
		ID:         endpointID,
		Status:     status,
	})
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, fromUpdateProviderEndpointStatus(row))
}

func (s *Server) handleAdminCheckUpstreamDeploymentHealth(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) checkUpstreamDeploymentReachable(r *http.Request, deployment dbgen.GetUpstreamDeploymentForHealthCheckRow) error {
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

func healthProbePath(protocol string) string {
	switch protocol {
	case string(domain.ProtocolOpenAIResponses):
		return "/responses"
	case string(domain.ProtocolOpenAIEmbeddings):
		return "/embeddings"
	default:
		return "/chat/completions"
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
