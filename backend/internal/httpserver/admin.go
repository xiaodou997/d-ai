package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"uni-ai-api/backend/internal/apikey"
	dbgen "uni-ai-api/backend/internal/db/gen"
	"uni-ai-api/backend/internal/secret"
	"uni-ai-api/backend/internal/upstream"
)

const (
	defaultProtocol       = "openai_chat_completions"
	defaultCapability     = "chat"
	defaultStatus         = "active"
	defaultEndpointWeight = int32(100)
	defaultTimeoutMs      = int32(30000)
)

type adminErrorResponse struct {
	Error string `json:"error"`
}

type adminActorContextKey struct{}

type createProviderRequest struct {
	Code         string          `json:"code"`
	Name         string          `json:"name"`
	ProviderType string          `json:"provider_type"`
	ProtocolType string          `json:"protocol_type"`
	IsCustom     bool            `json:"is_custom"`
	Config       json.RawMessage `json:"config"`
	Status       string          `json:"status"`
}

type updateStatusRequest struct {
	Status string `json:"status"`
}

type createEndpointRequest struct {
	Name              string          `json:"name"`
	BaseURL           string          `json:"base_url"`
	ProtocolType      string          `json:"protocol_type"`
	APIKey            string          `json:"api_key"`
	ExtraHeaders      json.RawMessage `json:"extra_headers"`
	CustomPath        *string         `json:"custom_path"`
	ProtocolOverrides json.RawMessage `json:"protocol_overrides"`
	Weight            *int32          `json:"weight"`
	TimeoutMs         *int32          `json:"timeout_ms"`
	Status            string          `json:"status"`
}

type createModelRequest struct {
	ModelCode              string `json:"model_code"`
	DisplayName            string `json:"display_name"`
	CapabilityType         string `json:"capability_type"`
	ContextWindow          *int32 `json:"context_window"`
	DefaultMaxOutputTokens *int32 `json:"default_max_output_tokens"`
	MaxOutputTokens        *int32 `json:"max_output_tokens"`
	Status                 string `json:"status"`
}

type createModelPriceRequest struct {
	PlatformInputPricePer1M  int64          `json:"platform_input_price_per_1m"`
	PlatformOutputPricePer1M int64          `json:"platform_output_price_per_1m"`
	PlatformImagePrice       int64          `json:"platform_image_price"`
	TenantInputPricePer1M    int64          `json:"tenant_input_price_per_1m"`
	TenantOutputPricePer1M   int64          `json:"tenant_output_price_per_1m"`
	TenantImagePrice         int64          `json:"tenant_image_price"`
	EffectiveFrom            adminTimestamp `json:"effective_from"`
	Status                   string         `json:"status"`
}

type createProviderModelPriceRequest struct {
	EndpointID         string         `json:"endpoint_id"`
	UpstreamModel      string         `json:"upstream_model"`
	CapabilityType     string         `json:"capability_type"`
	Currency           string         `json:"currency"`
	InputCostPer1M     int64          `json:"input_cost_per_1m"`
	OutputCostPer1M    int64          `json:"output_cost_per_1m"`
	RequestCost        int64          `json:"request_cost"`
	ImageCost          int64          `json:"image_cost"`
	VideoCostPerSecond int64          `json:"video_cost_per_second"`
	EffectiveFrom      adminTimestamp `json:"effective_from"`
	Status             string         `json:"status"`
}

func validateModelPriceCredits(req createModelPriceRequest) string {
	fields := map[string]int64{
		"platform_input_price_per_1m":  req.PlatformInputPricePer1M,
		"platform_output_price_per_1m": req.PlatformOutputPricePer1M,
		"platform_image_price":         req.PlatformImagePrice,
		"tenant_input_price_per_1m":    req.TenantInputPricePer1M,
		"tenant_output_price_per_1m":   req.TenantOutputPricePer1M,
		"tenant_image_price":           req.TenantImagePrice,
	}
	for name, value := range fields {
		if value < 0 {
			return fmt.Sprintf("%s must be a non-negative integer credit value", name)
		}
	}
	return ""
}

func validateProviderModelPriceCredits(req createProviderModelPriceRequest) string {
	fields := map[string]int64{
		"input_cost_per_1m":     req.InputCostPer1M,
		"output_cost_per_1m":    req.OutputCostPer1M,
		"request_cost":          req.RequestCost,
		"image_cost":            req.ImageCost,
		"video_cost_per_second": req.VideoCostPerSecond,
	}
	for name, value := range fields {
		if value < 0 {
			return fmt.Sprintf("%s must be a non-negative integer credit value", name)
		}
	}
	return ""
}

type createDeploymentRequest struct {
	EndpointID         string          `json:"endpoint_id"`
	UpstreamModel      string          `json:"upstream_model"`
	CapabilityType     string          `json:"capability_type"`
	UpstreamProtocol   string          `json:"upstream_protocol"`
	UpstreamParameters json.RawMessage `json:"upstream_parameters"`
	Priority           *int32          `json:"priority"`
	Weight             *int32          `json:"weight"`
	SupportsStream     *bool           `json:"supports_stream"`
	Status             string          `json:"status"`
}

type grantModelToTenantRequest struct {
	ModelID   string `json:"model_id"`
	Status    string `json:"status"`
	CreatedBy string `json:"created_by"`
}

type createTenantAPIKeyRequest struct {
	Name          string          `json:"name"`
	QuotaLimit    *int64          `json:"quota_limit"`
	AllowedModels []string        `json:"allowed_models"`
	Status        string          `json:"status"`
	ExpiresAt     *adminTimestamp `json:"expires_at"`
	CreatedBy     string          `json:"created_by"`
}

type createTenantAPIKeyResponse struct {
	APIKey string                      `json:"api_key"`
	Key    dbgen.CreateTenantAPIKeyRow `json:"key"`
}

type createUserAPIKeyResponse struct {
	APIKey string                    `json:"api_key"`
	Key    dbgen.CreateUserAPIKeyRow `json:"key"`
}

func (s *Server) adminAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.validLocalAdminToken(r) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), adminActorContextKey{}, "local_admin")))
			return
		}
		if actor, ok := s.validURMAdminToken(r); ok {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), adminActorContextKey{}, actor)))
			return
		}

		if s.security.AdminToken == "" && s.urmClient == nil {
			next.ServeHTTP(w, r)
			return
		}

		writeAdminError(w, http.StatusUnauthorized, "invalid admin credential")
	})
}

func (s *Server) validLocalAdminToken(r *http.Request) bool {
	return s.security.AdminToken != "" && r.Header.Get("X-Admin-Token") == s.security.AdminToken
}

func (s *Server) validURMAdminToken(r *http.Request) (string, bool) {
	if s.urmClient == nil {
		return "", false
	}

	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		return "", false
	}

	userInfo, err := s.urmClient.UserInfo(r.Context(), token)
	if err != nil {
		s.logger.Warn("validate urm admin token failed",
			"error", err,
			"request_id", requestIDFromContext(r.Context()),
		)
		return "", false
	}

	if userInfo.UserType == 1 || userInfo.UserType == 2 {
		actor := userInfo.Subject
		if actor == "" {
			actor = userInfo.Username
		}
		if actor == "" {
			actor = "urm_admin"
		}
		return actor, true
	}
	return "", false
}

func bearerToken(header string) string {
	if header == "" {
		return ""
	}
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

func (s *Server) handleAdminListProviders(w http.ResponseWriter, r *http.Request) {
	rows, err := s.queries.ListProviders(r.Context())
	if err != nil {
		s.writeAdminServerError(w, r, "list providers failed", err)
		return
	}
	writeAdminJSON(w, http.StatusOK, rows)
}

func (s *Server) handleAdminCreateProvider(w http.ResponseWriter, r *http.Request) {
	var req createProviderRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.Code == "" || req.Name == "" {
		writeAdminError(w, http.StatusBadRequest, "code and name are required")
		return
	}
	if req.ProviderType == "" {
		req.ProviderType = req.Code
	}
	if req.ProtocolType == "" {
		req.ProtocolType = defaultProtocol
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}

	row, err := s.queries.CreateProvider(r.Context(), dbgen.CreateProviderParams{
		Code:         req.Code,
		Name:         req.Name,
		ProviderType: req.ProviderType,
		ProtocolType: req.ProtocolType,
		IsCustom:     req.IsCustom,
		Config:       jsonObjectOrDefault(req.Config),
		Status:       req.Status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusCreated, row)
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
		writeAdminError(w, http.StatusBadRequest, "code and name are required")
		return
	}
	if req.ProviderType == "" {
		req.ProviderType = req.Code
	}
	if req.ProtocolType == "" {
		req.ProtocolType = defaultProtocol
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}
	row, err := s.queries.UpdateProvider(r.Context(), dbgen.UpdateProviderParams{
		ID: providerID, Code: req.Code, Name: req.Name, ProviderType: req.ProviderType, ProtocolType: req.ProtocolType,
		IsCustom: req.IsCustom, Config: jsonObjectOrDefault(req.Config), Status: req.Status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
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
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
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
	writeAdminJSON(w, http.StatusOK, rows)
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
		writeAdminError(w, http.StatusBadRequest, "name, base_url and api_key are required")
		return
	}
	if req.ProtocolType == "" {
		req.ProtocolType = defaultProtocol
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}

	ciphertext, err := secret.EncryptProviderKey(s.security.ProviderKeyMaster, req.APIKey)
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, err.Error())
		return
	}

	row, err := s.queries.CreateProviderEndpoint(r.Context(), dbgen.CreateProviderEndpointParams{
		ProviderID:        providerID,
		Name:              req.Name,
		BaseUrl:           req.BaseURL,
		ProtocolType:      req.ProtocolType,
		ApiKeyCiphertext:  ciphertext,
		ExtraHeaders:      jsonObjectOrDefault(req.ExtraHeaders),
		CustomPath:        optionalTextParam(req.CustomPath),
		ProtocolOverrides: jsonObjectOrDefault(req.ProtocolOverrides),
		Weight:            int32OrDefault(req.Weight, defaultEndpointWeight),
		TimeoutMs:         int32OrDefault(req.TimeoutMs, defaultTimeoutMs),
		Status:            req.Status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusCreated, row)
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
		writeAdminError(w, http.StatusBadRequest, "name and base_url are required")
		return
	}
	if req.ProtocolType == "" {
		req.ProtocolType = defaultProtocol
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}
	current, err := s.queries.GetProviderEndpoint(r.Context(), dbgen.GetProviderEndpointParams{ProviderID: providerID, ID: endpointID})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	ciphertext := current.ApiKeyCiphertext
	if req.APIKey != "" {
		ciphertext, err = secret.EncryptProviderKey(s.security.ProviderKeyMaster, req.APIKey)
		if err != nil {
			writeAdminError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	row, err := s.queries.UpdateProviderEndpoint(r.Context(), dbgen.UpdateProviderEndpointParams{
		ProviderID: providerID, ID: endpointID, Name: req.Name, BaseUrl: req.BaseURL, ProtocolType: req.ProtocolType,
		ApiKeyCiphertext: ciphertext, ExtraHeaders: jsonObjectOrDefault(req.ExtraHeaders), CustomPath: optionalTextParam(req.CustomPath),
		ProtocolOverrides: jsonObjectOrDefault(req.ProtocolOverrides), Weight: int32OrDefault(req.Weight, defaultEndpointWeight),
		TimeoutMs: int32OrDefault(req.TimeoutMs, defaultTimeoutMs), Status: req.Status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
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
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
}

func (s *Server) handleAdminCheckProviderEndpointHealth(w http.ResponseWriter, r *http.Request) {
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
		writeAdminDBError(w, err)
		return
	}

	status := "healthy"
	checkErr := s.checkProviderEndpointReachable(r, endpointID, endpoint)
	if checkErr != nil {
		status = "unhealthy"
		if errors.Is(checkErr, pgx.ErrNoRows) {
			status = "unknown"
			checkErr = errors.New("endpoint has no active low-cost deployment to probe")
		}
		s.recordEndpointHealthFailure(r.Context(), endpointID, checkErr.Error())
	}

	row, err := s.queries.UpdateProviderEndpointHealth(r.Context(), dbgen.UpdateProviderEndpointHealthParams{
		ProviderID:   providerID,
		ID:           endpointID,
		HealthStatus: status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}

	response := map[string]any{
		"endpoint": row,
		"status":   status,
	}
	if checkErr != nil {
		response["error"] = checkErr.Error()
	}
	writeAdminJSON(w, http.StatusOK, response)
}

func (s *Server) recordEndpointHealthFailure(ctx context.Context, endpointID pgtype.UUID, reason string) {
	if s.redis == nil || reason == "" {
		return
	}
	key := "uni_ai_api:endpoint:" + endpointID.String() + ":health_error"
	if err := s.redis.Set(ctx, key, reason, 24*time.Hour).Err(); err != nil {
		s.logger.Error("record endpoint health failure failed", "error", err, "endpoint_id", endpointID.String())
	}
}

func (s *Server) checkProviderEndpointReachable(r *http.Request, endpointID pgtype.UUID, endpoint dbgen.GetProviderEndpointRow) error {
	deployment, err := s.queries.GetFirstActiveProbeDeploymentForEndpoint(r.Context(), endpointID)
	if err != nil {
		return err
	}
	path, body, err := healthProbeRequest(endpoint.ProtocolOverrides, deployment.UpstreamModel, deployment.UpstreamProtocol)
	if err != nil {
		return err
	}
	url, err := upstream.BuildEndpointURL(endpoint.BaseUrl, optionalText(endpoint.CustomPath), path)
	if err != nil {
		return err
	}
	ctx := r.Context()
	timeout := time.Duration(endpoint.TimeoutMs) * time.Millisecond
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
	if providerKey, err := secret.DecryptProviderKey(s.security.ProviderKeyMaster, endpoint.ApiKeyCiphertext); err == nil && providerKey != "" {
		req.Header.Set("Authorization", "Bearer "+providerKey)
	}
	if err := applyAdminExtraHeaders(req.Header, endpoint.ExtraHeaders); err != nil {
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

func healthProbeRequest(overrides []byte, upstreamModel string, protocol string) (string, []byte, error) {
	if len(overrides) > 0 {
		var wrapper struct {
			HealthProbe json.RawMessage `json:"health_probe"`
		}
		if err := json.Unmarshal(overrides, &wrapper); err != nil {
			return "", nil, fmt.Errorf("parse protocol_overrides: %w", err)
		}
		if len(wrapper.HealthProbe) > 0 {
			var body map[string]json.RawMessage
			if err := json.Unmarshal(wrapper.HealthProbe, &body); err != nil {
				return "", nil, fmt.Errorf("parse health_probe: %w", err)
			}
			model, err := json.Marshal(upstreamModel)
			if err != nil {
				return "", nil, err
			}
			if _, ok := body["model"]; !ok {
				body["model"] = model
			}
			encoded, err := json.Marshal(body)
			return healthProbePath(protocol), encoded, err
		}
	}
	switch protocol {
	case upstream.ProtocolOpenAIChatCompletions:
		body, err := json.Marshal(map[string]any{
			"model":      upstreamModel,
			"messages":   []map[string]string{{"role": "user", "content": "ping"}},
			"max_tokens": 1,
		})
		return "/chat/completions", body, err
	case upstream.ProtocolOpenAIResponses:
		body, err := json.Marshal(map[string]any{
			"model":             upstreamModel,
			"input":             "ping",
			"max_output_tokens": 1,
		})
		return "/responses", body, err
	case upstream.ProtocolOpenAIEmbeddings:
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
	case upstream.ProtocolOpenAIResponses:
		return "/responses"
	case upstream.ProtocolOpenAIEmbeddings:
		return "/embeddings"
	default:
		return "/chat/completions"
	}
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

func (s *Server) handleAdminListModels(w http.ResponseWriter, r *http.Request) {
	rows, err := s.queries.ListAdminModels(r.Context())
	if err != nil {
		s.writeAdminServerError(w, r, "list models failed", err)
		return
	}
	writeAdminJSON(w, http.StatusOK, rows)
}

func (s *Server) handleAdminCreateModel(w http.ResponseWriter, r *http.Request) {
	var req createModelRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.ModelCode == "" || req.DisplayName == "" {
		writeAdminError(w, http.StatusBadRequest, "model_code and display_name are required")
		return
	}
	if req.CapabilityType == "" {
		req.CapabilityType = defaultCapability
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}

	row, err := s.queries.CreateModel(r.Context(), dbgen.CreateModelParams{
		ModelCode:              req.ModelCode,
		DisplayName:            req.DisplayName,
		CapabilityType:         req.CapabilityType,
		ContextWindow:          optionalInt4(req.ContextWindow),
		DefaultMaxOutputTokens: int32OrDefault(req.DefaultMaxOutputTokens, 2048),
		MaxOutputTokens:        optionalInt4(req.MaxOutputTokens),
		Status:                 req.Status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusCreated, row)
}

func (s *Server) handleAdminUpdateModel(w http.ResponseWriter, r *http.Request) {
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}
	var req createModelRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.ModelCode == "" || req.DisplayName == "" {
		writeAdminError(w, http.StatusBadRequest, "model_code and display_name are required")
		return
	}
	if req.CapabilityType == "" {
		req.CapabilityType = defaultCapability
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}
	row, err := s.queries.UpdateModel(r.Context(), dbgen.UpdateModelParams{
		ID: modelID, ModelCode: req.ModelCode, DisplayName: req.DisplayName, CapabilityType: req.CapabilityType,
		ContextWindow: optionalInt4(req.ContextWindow), DefaultMaxOutputTokens: int32OrDefault(req.DefaultMaxOutputTokens, 2048),
		MaxOutputTokens: optionalInt4(req.MaxOutputTokens), Status: req.Status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
}

func (s *Server) handleAdminUpdateModelStatus(w http.ResponseWriter, r *http.Request) {
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}
	status, ok := decodeStatusUpdate(w, r)
	if !ok {
		return
	}

	row, err := s.queries.UpdateModelStatus(r.Context(), dbgen.UpdateModelStatusParams{
		ID:     modelID,
		Status: status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
}

func (s *Server) handleAdminListModelPrices(w http.ResponseWriter, r *http.Request) {
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}

	rows, err := s.queries.ListModelPrices(r.Context(), modelID)
	if err != nil {
		s.writeAdminServerError(w, r, "list model prices failed", err)
		return
	}
	writeAdminJSON(w, http.StatusOK, rows)
}

func (s *Server) handleAdminCreateModelPrice(w http.ResponseWriter, r *http.Request) {
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}

	var req createModelPriceRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}
	if message := validateModelPriceCredits(req); message != "" {
		writeAdminError(w, http.StatusBadRequest, message)
		return
	}
	effectiveFrom, err := parseEffectiveFrom(req.EffectiveFrom)
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid effective_from")
		return
	}

	row, err := s.queries.CreateModelPrice(r.Context(), dbgen.CreateModelPriceParams{
		ModelID:                  modelID,
		PlatformInputPricePer1m:  req.PlatformInputPricePer1M,
		PlatformOutputPricePer1m: req.PlatformOutputPricePer1M,
		PlatformImagePrice:       req.PlatformImagePrice,
		TenantInputPricePer1m:    req.TenantInputPricePer1M,
		TenantOutputPricePer1m:   req.TenantOutputPricePer1M,
		TenantImagePrice:         req.TenantImagePrice,
		EffectiveFrom:            effectiveFrom,
		Status:                   req.Status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusCreated, row)
}

func (s *Server) handleAdminUpdateModelPrice(w http.ResponseWriter, r *http.Request) {
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}
	priceID, ok := parseUUIDParam(w, r, "priceID")
	if !ok {
		return
	}
	var req createModelPriceRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}
	if message := validateModelPriceCredits(req); message != "" {
		writeAdminError(w, http.StatusBadRequest, message)
		return
	}
	effectiveFrom, err := parseEffectiveFrom(req.EffectiveFrom)
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid effective_from")
		return
	}
	row, err := s.queries.UpdateModelPrice(r.Context(), dbgen.UpdateModelPriceParams{
		ModelID: modelID, ID: priceID, PlatformInputPricePer1m: req.PlatformInputPricePer1M, PlatformOutputPricePer1m: req.PlatformOutputPricePer1M,
		PlatformImagePrice: req.PlatformImagePrice, TenantInputPricePer1m: req.TenantInputPricePer1M, TenantOutputPricePer1m: req.TenantOutputPricePer1M,
		TenantImagePrice: req.TenantImagePrice, EffectiveFrom: effectiveFrom, Status: req.Status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
}

func (s *Server) handleAdminUpdateModelPriceStatus(w http.ResponseWriter, r *http.Request) {
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}
	priceID, ok := parseUUIDParam(w, r, "priceID")
	if !ok {
		return
	}
	status, ok := decodeStatusUpdate(w, r)
	if !ok {
		return
	}

	row, err := s.queries.UpdateModelPriceStatus(r.Context(), dbgen.UpdateModelPriceStatusParams{
		ModelID: modelID,
		ID:      priceID,
		Status:  status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
}

func (s *Server) handleAdminListModelDeployments(w http.ResponseWriter, r *http.Request) {
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}

	rows, err := s.queries.ListModelDeployments(r.Context(), modelID)
	if err != nil {
		s.writeAdminServerError(w, r, "list model deployments failed", err)
		return
	}
	writeAdminJSON(w, http.StatusOK, rows)
}

func (s *Server) handleAdminCreateModelDeployment(w http.ResponseWriter, r *http.Request) {
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}

	var req createDeploymentRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.EndpointID == "" || req.UpstreamModel == "" {
		writeAdminError(w, http.StatusBadRequest, "endpoint_id and upstream_model are required")
		return
	}
	endpointID, err := parseUUID(req.EndpointID)
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid endpoint_id")
		return
	}
	if req.CapabilityType == "" {
		req.CapabilityType = defaultCapability
	}
	if req.UpstreamProtocol == "" {
		req.UpstreamProtocol = defaultProtocol
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}

	row, err := s.queries.CreateModelDeployment(r.Context(), dbgen.CreateModelDeploymentParams{
		ModelID:            modelID,
		EndpointID:         endpointID,
		UpstreamModel:      req.UpstreamModel,
		CapabilityType:     req.CapabilityType,
		UpstreamProtocol:   req.UpstreamProtocol,
		UpstreamParameters: jsonObjectOrDefault(req.UpstreamParameters),
		Priority:           int32OrDefault(req.Priority, 100),
		Weight:             int32OrDefault(req.Weight, 100),
		SupportsStream:     boolOrDefault(req.SupportsStream, true),
		Status:             req.Status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusCreated, row)
}

func (s *Server) handleAdminUpdateModelDeployment(w http.ResponseWriter, r *http.Request) {
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}
	deploymentID, ok := parseUUIDParam(w, r, "deploymentID")
	if !ok {
		return
	}
	var req createDeploymentRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.EndpointID == "" || req.UpstreamModel == "" {
		writeAdminError(w, http.StatusBadRequest, "endpoint_id and upstream_model are required")
		return
	}
	endpointID, err := parseUUID(req.EndpointID)
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid endpoint_id")
		return
	}
	if req.CapabilityType == "" {
		req.CapabilityType = defaultCapability
	}
	if req.UpstreamProtocol == "" {
		req.UpstreamProtocol = defaultProtocol
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}
	row, err := s.queries.UpdateModelDeployment(r.Context(), dbgen.UpdateModelDeploymentParams{
		ModelID: modelID, ID: deploymentID, EndpointID: endpointID, UpstreamModel: req.UpstreamModel, CapabilityType: req.CapabilityType,
		UpstreamProtocol: req.UpstreamProtocol, UpstreamParameters: jsonObjectOrDefault(req.UpstreamParameters), Priority: int32OrDefault(req.Priority, 100),
		Weight: int32OrDefault(req.Weight, 100), SupportsStream: boolOrDefault(req.SupportsStream, true), Status: req.Status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
}

func (s *Server) handleAdminUpdateModelDeploymentStatus(w http.ResponseWriter, r *http.Request) {
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}
	deploymentID, ok := parseUUIDParam(w, r, "deploymentID")
	if !ok {
		return
	}
	status, ok := decodeStatusUpdate(w, r)
	if !ok {
		return
	}

	row, err := s.queries.UpdateModelDeploymentStatus(r.Context(), dbgen.UpdateModelDeploymentStatusParams{
		ModelID: modelID,
		ID:      deploymentID,
		Status:  status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
}

func (s *Server) handleAdminListProviderModelPrices(w http.ResponseWriter, r *http.Request) {
	providerID, ok := parseUUIDParam(w, r, "providerID")
	if !ok {
		return
	}

	rows, err := s.queries.ListProviderModelPrices(r.Context(), providerID)
	if err != nil {
		s.writeAdminServerError(w, r, "list provider model prices failed", err)
		return
	}
	writeAdminJSON(w, http.StatusOK, rows)
}

func (s *Server) handleAdminCreateProviderModelPrice(w http.ResponseWriter, r *http.Request) {
	providerID, ok := parseUUIDParam(w, r, "providerID")
	if !ok {
		return
	}

	var req createProviderModelPriceRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.UpstreamModel == "" {
		writeAdminError(w, http.StatusBadRequest, "upstream_model is required")
		return
	}
	if req.CapabilityType == "" {
		req.CapabilityType = defaultCapability
	}
	if req.Currency == "" {
		req.Currency = "CNY_CREDITS"
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}
	if message := validateProviderModelPriceCredits(req); message != "" {
		writeAdminError(w, http.StatusBadRequest, message)
		return
	}
	endpointID, err := optionalUUIDString(req.EndpointID)
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid endpoint_id")
		return
	}
	effectiveFrom, err := parseEffectiveFrom(req.EffectiveFrom)
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid effective_from")
		return
	}

	row, err := s.queries.CreateProviderModelPrice(r.Context(), dbgen.CreateProviderModelPriceParams{
		ProviderID:         providerID,
		EndpointID:         endpointID,
		UpstreamModel:      req.UpstreamModel,
		CapabilityType:     req.CapabilityType,
		Currency:           req.Currency,
		InputCostPer1m:     req.InputCostPer1M,
		OutputCostPer1m:    req.OutputCostPer1M,
		RequestCost:        req.RequestCost,
		ImageCost:          req.ImageCost,
		VideoCostPerSecond: req.VideoCostPerSecond,
		EffectiveFrom:      effectiveFrom,
		Status:             req.Status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusCreated, row)
}

func (s *Server) handleAdminUpdateProviderModelPrice(w http.ResponseWriter, r *http.Request) {
	providerID, ok := parseUUIDParam(w, r, "providerID")
	if !ok {
		return
	}
	priceID, ok := parseUUIDParam(w, r, "priceID")
	if !ok {
		return
	}
	var req createProviderModelPriceRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.UpstreamModel == "" {
		writeAdminError(w, http.StatusBadRequest, "upstream_model is required")
		return
	}
	if req.CapabilityType == "" {
		req.CapabilityType = defaultCapability
	}
	if req.Currency == "" {
		req.Currency = "CNY_CREDITS"
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}
	if message := validateProviderModelPriceCredits(req); message != "" {
		writeAdminError(w, http.StatusBadRequest, message)
		return
	}
	endpointID, err := optionalUUIDString(req.EndpointID)
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid endpoint_id")
		return
	}
	effectiveFrom, err := parseEffectiveFrom(req.EffectiveFrom)
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid effective_from")
		return
	}
	row, err := s.queries.UpdateProviderModelPrice(r.Context(), dbgen.UpdateProviderModelPriceParams{
		ProviderID: providerID, ID: priceID, EndpointID: endpointID, UpstreamModel: req.UpstreamModel, CapabilityType: req.CapabilityType,
		Currency: req.Currency, InputCostPer1m: req.InputCostPer1M, OutputCostPer1m: req.OutputCostPer1M, RequestCost: req.RequestCost,
		ImageCost: req.ImageCost, VideoCostPerSecond: req.VideoCostPerSecond, EffectiveFrom: effectiveFrom, Status: req.Status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
}

func (s *Server) handleAdminUpdateProviderModelPriceStatus(w http.ResponseWriter, r *http.Request) {
	providerID, ok := parseUUIDParam(w, r, "providerID")
	if !ok {
		return
	}
	priceID, ok := parseUUIDParam(w, r, "priceID")
	if !ok {
		return
	}
	status, ok := decodeStatusUpdate(w, r)
	if !ok {
		return
	}

	row, err := s.queries.UpdateProviderModelPriceStatus(r.Context(), dbgen.UpdateProviderModelPriceStatusParams{
		ProviderID: providerID,
		ID:         priceID,
		Status:     status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
}

func (s *Server) handleAdminListTenantModelGrants(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeAdminError(w, http.StatusBadRequest, "tenantID is required")
		return
	}

	rows, err := s.queries.ListTenantModelGrants(r.Context(), tenantID)
	if err != nil {
		s.writeAdminServerError(w, r, "list tenant model grants failed", err)
		return
	}
	writeAdminJSON(w, http.StatusOK, rows)
}

func (s *Server) handleAdminGrantModelToTenant(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeAdminError(w, http.StatusBadRequest, "tenantID is required")
		return
	}

	var req grantModelToTenantRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.ModelID == "" {
		writeAdminError(w, http.StatusBadRequest, "model_id is required")
		return
	}
	modelID, err := parseUUID(req.ModelID)
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid model_id")
		return
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}

	row, err := s.queries.GrantModelToTenant(r.Context(), dbgen.GrantModelToTenantParams{
		TenantID:  tenantID,
		ModelID:   modelID,
		Status:    req.Status,
		CreatedBy: optionalTextValue(req.CreatedBy),
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusCreated, row)
}

func (s *Server) handleAdminUpdateTenantModelGrantStatus(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeAdminError(w, http.StatusBadRequest, "tenantID is required")
		return
	}
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}
	status, ok := decodeStatusUpdate(w, r)
	if !ok {
		return
	}

	row, err := s.queries.UpdateTenantModelGrantStatus(r.Context(), dbgen.UpdateTenantModelGrantStatusParams{
		TenantID: tenantID,
		ModelID:  modelID,
		Status:   status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
}

func (s *Server) handleAdminListUserModelGrants(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := tenantUserParams(w, r)
	if !ok {
		return
	}

	rows, err := s.queries.ListUserModelGrants(r.Context(), dbgen.ListUserModelGrantsParams{
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		s.writeAdminServerError(w, r, "list user model grants failed", err)
		return
	}
	writeAdminJSON(w, http.StatusOK, rows)
}

func (s *Server) handleAdminGrantModelToUser(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := tenantUserParams(w, r)
	if !ok {
		return
	}

	var req grantModelToTenantRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.ModelID == "" {
		writeAdminError(w, http.StatusBadRequest, "model_id is required")
		return
	}
	modelID, err := parseUUID(req.ModelID)
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid model_id")
		return
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}

	row, err := s.queries.GrantModelToUser(r.Context(), dbgen.GrantModelToUserParams{
		TenantID:  tenantID,
		UserID:    userID,
		ModelID:   modelID,
		Status:    req.Status,
		CreatedBy: optionalTextValue(req.CreatedBy),
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusCreated, row)
}

func (s *Server) handleAdminUpdateUserModelGrantStatus(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := tenantUserParams(w, r)
	if !ok {
		return
	}
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}
	status, ok := decodeStatusUpdate(w, r)
	if !ok {
		return
	}

	row, err := s.queries.UpdateUserModelGrantStatus(r.Context(), dbgen.UpdateUserModelGrantStatusParams{
		TenantID: tenantID,
		UserID:   userID,
		ModelID:  modelID,
		Status:   status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
}

func (s *Server) handleAdminListTenantAPIKeys(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeAdminError(w, http.StatusBadRequest, "tenantID is required")
		return
	}

	rows, err := s.queries.ListTenantAPIKeys(r.Context(), tenantID)
	if err != nil {
		s.writeAdminServerError(w, r, "list tenant api keys failed", err)
		return
	}
	writeAdminJSON(w, http.StatusOK, rows)
}

func (s *Server) handleAdminCreateTenantAPIKey(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeAdminError(w, http.StatusBadRequest, "tenantID is required")
		return
	}

	var req createTenantAPIKeyRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeAdminError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}

	key, err := apikey.Generate()
	if err != nil {
		s.writeAdminServerError(w, r, "generate api key failed", err)
		return
	}
	allowedModels, err := json.Marshal(req.AllowedModels)
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid allowed_models")
		return
	}
	expiresAt, err := optionalTime(req.ExpiresAt)
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid expires_at")
		return
	}

	row, err := s.queries.CreateTenantAPIKey(r.Context(), dbgen.CreateTenantAPIKeyParams{
		TenantID:      tenantID,
		KeyHash:       apikey.Hash(key),
		KeyPrefix:     apikey.PrefixForDisplay(key),
		Name:          req.Name,
		QuotaLimit:    optionalInt8(req.QuotaLimit),
		AllowedModels: allowedModels,
		Status:        req.Status,
		ExpiresAt:     expiresAt,
		CreatedBy:     optionalTextValue(req.CreatedBy),
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusCreated, createTenantAPIKeyResponse{
		APIKey: key,
		Key:    row,
	})
}

func (s *Server) handleAdminUpdateTenantAPIKeyStatus(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeAdminError(w, http.StatusBadRequest, "tenantID is required")
		return
	}
	apiKeyID, ok := parseUUIDParam(w, r, "apiKeyID")
	if !ok {
		return
	}
	status, ok := decodeStatusUpdate(w, r)
	if !ok {
		return
	}

	row, err := s.queries.UpdateTenantAPIKeyStatus(r.Context(), dbgen.UpdateTenantAPIKeyStatusParams{
		TenantID: tenantID,
		ID:       apiKeyID,
		Status:   status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
}

func (s *Server) handleAdminUpdateTenantAPIKey(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeAdminError(w, http.StatusBadRequest, "tenantID is required")
		return
	}
	apiKeyID, ok := parseUUIDParam(w, r, "apiKeyID")
	if !ok {
		return
	}
	var req createTenantAPIKeyRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeAdminError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}
	allowedModels, err := json.Marshal(req.AllowedModels)
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid allowed_models")
		return
	}
	expiresAt, err := optionalTime(req.ExpiresAt)
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid expires_at")
		return
	}
	row, err := s.queries.UpdateTenantAPIKey(r.Context(), dbgen.UpdateTenantAPIKeyParams{
		TenantID: tenantID, ID: apiKeyID, Name: req.Name, QuotaLimit: optionalInt8(req.QuotaLimit), AllowedModels: allowedModels,
		Status: req.Status, ExpiresAt: expiresAt,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
}

func (s *Server) handleAdminListUserAPIKeys(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := tenantUserParams(w, r)
	if !ok {
		return
	}

	rows, err := s.queries.ListUserAPIKeys(r.Context(), dbgen.ListUserAPIKeysParams{
		TenantID: tenantID,
		UserID:   optionalTextValue(userID),
	})
	if err != nil {
		s.writeAdminServerError(w, r, "list user api keys failed", err)
		return
	}
	writeAdminJSON(w, http.StatusOK, rows)
}

func (s *Server) handleAdminCreateUserAPIKey(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := tenantUserParams(w, r)
	if !ok {
		return
	}

	var req createTenantAPIKeyRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeAdminError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}

	key, err := apikey.Generate()
	if err != nil {
		s.writeAdminServerError(w, r, "generate api key failed", err)
		return
	}
	allowedModels, err := json.Marshal(req.AllowedModels)
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid allowed_models")
		return
	}
	expiresAt, err := optionalTime(req.ExpiresAt)
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid expires_at")
		return
	}

	row, err := s.queries.CreateUserAPIKey(r.Context(), dbgen.CreateUserAPIKeyParams{
		TenantID:      tenantID,
		UserID:        optionalTextValue(userID),
		KeyHash:       apikey.Hash(key),
		KeyPrefix:     apikey.PrefixForDisplay(key),
		Name:          req.Name,
		QuotaLimit:    optionalInt8(req.QuotaLimit),
		AllowedModels: allowedModels,
		Status:        req.Status,
		ExpiresAt:     expiresAt,
		CreatedBy:     optionalTextValue(req.CreatedBy),
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusCreated, createUserAPIKeyResponse{
		APIKey: key,
		Key:    row,
	})
}

func (s *Server) handleAdminUpdateUserAPIKeyStatus(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := tenantUserParams(w, r)
	if !ok {
		return
	}
	apiKeyID, ok := parseUUIDParam(w, r, "apiKeyID")
	if !ok {
		return
	}
	status, ok := decodeStatusUpdate(w, r)
	if !ok {
		return
	}

	row, err := s.queries.UpdateUserAPIKeyStatus(r.Context(), dbgen.UpdateUserAPIKeyStatusParams{
		TenantID: tenantID,
		UserID:   optionalTextValue(userID),
		ID:       apiKeyID,
		Status:   status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
}

func (s *Server) handleAdminUpdateUserAPIKey(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := tenantUserParams(w, r)
	if !ok {
		return
	}
	apiKeyID, ok := parseUUIDParam(w, r, "apiKeyID")
	if !ok {
		return
	}
	var req createTenantAPIKeyRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeAdminError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}
	allowedModels, err := json.Marshal(req.AllowedModels)
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid allowed_models")
		return
	}
	expiresAt, err := optionalTime(req.ExpiresAt)
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid expires_at")
		return
	}
	row, err := s.queries.UpdateUserAPIKey(r.Context(), dbgen.UpdateUserAPIKeyParams{
		TenantID: tenantID, UserID: optionalTextValue(userID), ID: apiKeyID, Name: req.Name, QuotaLimit: optionalInt8(req.QuotaLimit),
		AllowedModels: allowedModels, Status: req.Status, ExpiresAt: expiresAt,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
}

func (s *Server) handleAdminListUsageLogs(w http.ResponseWriter, r *http.Request) {
	limit := int32(100)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || parsed <= 0 {
			writeAdminError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		if parsed > 500 {
			parsed = 500
		}
		limit = int32(parsed)
	}

	rows, err := s.queries.ListUsageLogs(r.Context(), dbgen.ListUsageLogsParams{
		TenantID:      optionalTextValue(r.URL.Query().Get("tenant_id")),
		UserID:        optionalTextValue(r.URL.Query().Get("user_id")),
		ModelCode:     optionalTextValue(r.URL.Query().Get("model_code")),
		RequestStatus: optionalTextValue(r.URL.Query().Get("request_status")),
		Limit:         limit,
	})
	if err != nil {
		s.writeAdminServerError(w, r, "list usage logs failed", err)
		return
	}
	writeAdminJSON(w, http.StatusOK, rows)
}

func decodeAdminJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

func decodeStatusUpdate(w http.ResponseWriter, r *http.Request) (string, bool) {
	var req updateStatusRequest
	if !decodeAdminJSON(w, r, &req) {
		return "", false
	}
	status := strings.TrimSpace(req.Status)
	switch status {
	case "active", "inactive", "disabled":
		return status, true
	default:
		writeAdminError(w, http.StatusBadRequest, "status must be active, inactive or disabled")
		return "", false
	}
}

func tenantUserParams(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeAdminError(w, http.StatusBadRequest, "tenantID is required")
		return "", "", false
	}
	userID := chi.URLParam(r, "userID")
	if userID == "" {
		writeAdminError(w, http.StatusBadRequest, "userID is required")
		return "", "", false
	}
	return tenantID, userID, true
}

func parseUUIDParam(w http.ResponseWriter, r *http.Request, name string) (pgtype.UUID, bool) {
	value := chi.URLParam(r, name)
	id, err := parseUUID(value)
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid "+name)
		return pgtype.UUID{}, false
	}
	return id, true
}

func parseUUID(value string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if value == "" {
		return id, errors.New("uuid is required")
	}
	if err := id.Scan(value); err != nil {
		return id, err
	}
	return id, nil
}

func optionalUUIDString(value string) (pgtype.UUID, error) {
	if value == "" {
		return pgtype.UUID{}, nil
	}
	return parseUUID(value)
}

func jsonObjectOrDefault(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return []byte("{}")
	}
	return raw
}

func optionalTextParam(value *string) pgtype.Text {
	if value == nil || *value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func optionalTextValue(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func optionalInt4(value *int32) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *value, Valid: true}
}

func optionalInt8(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

type adminTimestamp struct {
	Time  time.Time
	Valid bool
}

func (t *adminTimestamp) UnmarshalJSON(b []byte) error {
	raw := strings.TrimSpace(string(b))
	if raw == "" || raw == "null" || raw == `""` {
		t.Valid = false
		t.Time = time.Time{}
		return nil
	}
	if strings.HasPrefix(raw, `"`) {
		var value string
		if err := json.Unmarshal(b, &value); err != nil {
			return err
		}
		if strings.TrimSpace(value) == "" {
			t.Valid = false
			t.Time = time.Time{}
			return nil
		}
		if millis, err := strconv.ParseInt(value, 10, 64); err == nil {
			t.Time = time.UnixMilli(millis).UTC()
			t.Valid = true
			return nil
		}
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return err
		}
		t.Time = parsed.UTC()
		t.Valid = true
		return nil
	}

	var millis int64
	if err := json.Unmarshal(b, &millis); err != nil {
		return err
	}
	if millis == 0 {
		t.Valid = false
		t.Time = time.Time{}
		return nil
	}
	t.Time = time.UnixMilli(millis).UTC()
	t.Valid = true
	return nil
}

func optionalTime(value *adminTimestamp) (pgtype.Timestamptz, error) {
	if value == nil || !value.Valid {
		return pgtype.Timestamptz{}, nil
	}
	return pgtype.Timestamptz{Time: value.Time, Valid: true}, nil
}

func parseEffectiveFrom(value adminTimestamp) (pgtype.Timestamptz, error) {
	if !value.Valid {
		return pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}, nil
	}
	return pgtype.Timestamptz{Time: value.Time, Valid: true}, nil
}

func int32OrDefault(value *int32, defaultValue int32) int32 {
	if value == nil {
		return defaultValue
	}
	return *value
}

func boolOrDefault(value *bool, defaultValue bool) bool {
	if value == nil {
		return defaultValue
	}
	return *value
}

func writeAdminError(w http.ResponseWriter, status int, message string) {
	writeAdminJSON(w, status, adminErrorResponse{Error: message})
}

func writeAdminDBError(w http.ResponseWriter, err error) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			writeAdminError(w, http.StatusConflict, pgErr.Message)
			return
		case "23503", "23514", "22P02":
			writeAdminError(w, http.StatusBadRequest, pgErr.Message)
			return
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		writeAdminError(w, http.StatusNotFound, "not found")
		return
	}
	writeAdminError(w, http.StatusInternalServerError, "database error")
}

func (s *Server) writeAdminServerError(w http.ResponseWriter, r *http.Request, message string, err error) {
	s.logger.Error(message, "error", err, "request_id", requestIDFromContext(r.Context()))
	writeAdminError(w, http.StatusInternalServerError, "server error")
}
