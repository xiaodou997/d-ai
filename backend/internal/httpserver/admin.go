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
type adminContextKey struct{}

type adminRole string

const (
	adminRolePlatform adminRole = "platform"
	adminRoleTenant   adminRole = "tenant"
	adminRoleUser     adminRole = "user"
)

type adminContext struct {
	Actor    string    `json:"actor"`
	Role     adminRole `json:"role"`
	TenantID string    `json:"tenant_id"`
	UserID   string    `json:"user_id"`
	UserType int       `json:"user_type"`
}

type createProviderRequest struct {
	Code   string          `json:"code"`
	Name   string          `json:"name"`
	Config json.RawMessage `json:"config"`
	Status string          `json:"status"`
}

type updateStatusRequest struct {
	Status string `json:"status"`
}

type createEndpointRequest struct {
	Name         string          `json:"name"`
	BaseURL      string          `json:"base_url"`
	APIKey       string          `json:"api_key"`
	ExtraHeaders json.RawMessage `json:"extra_headers"`
	Weight       *int32          `json:"weight"`
	TimeoutMs    *int32          `json:"timeout_ms"`
	Status       string          `json:"status"`
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

type modelPriceRequest struct {
	InputPricePer1M          int64           `json:"input_price_per_1m"`
	OutputPricePer1M         int64           `json:"output_price_per_1m"`
	ImageSizePrices          json.RawMessage `json:"image_size_prices"`
	VideoPricePerSecond      int64           `json:"video_price_per_second"`
	AudioTTSPricePer1MChars  int64           `json:"audio_tts_price_per_1m_chars"`
	AudioSTTPricePerMinute   int64           `json:"audio_stt_price_per_minute"`
}

type createUpstreamDeploymentCostPriceRequest struct {
	CapabilityType     string          `json:"capability_type"`
	Currency           string          `json:"currency"`
	InputCostPer1M     int64           `json:"input_cost_per_1m"`
	OutputCostPer1M    int64           `json:"output_cost_per_1m"`
	RequestCost        int64           `json:"request_cost"`
	ImageCost          int64           `json:"image_cost"`
	ImageSizePrices    json.RawMessage `json:"image_size_prices"`
	VideoCostPerSecond int64           `json:"video_cost_per_second"`
	EffectiveFrom      adminTimestamp  `json:"effective_from"`
	Status             string          `json:"status"`
}

func validateModelPriceCredits(req modelPriceRequest) string {
	fields := map[string]int64{
		"input_price_per_1m":           req.InputPricePer1M,
		"output_price_per_1m":          req.OutputPricePer1M,
		"video_price_per_second":       req.VideoPricePerSecond,
		"audio_tts_price_per_1m_chars": req.AudioTTSPricePer1MChars,
		"audio_stt_price_per_minute":   req.AudioSTTPricePerMinute,
	}
	for name, value := range fields {
		if value < 0 {
			return fmt.Sprintf("%s must be a non-negative integer credit value", name)
		}
	}
	return ""
}

func validateUpstreamDeploymentCostPriceCredits(req createUpstreamDeploymentCostPriceRequest) string {
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

type createUpstreamDeploymentRequest struct {
	EndpointID         string          `json:"endpoint_id"`
	Name               string          `json:"name"`
	UpstreamModel      string          `json:"upstream_model"`
	CapabilityType     string          `json:"capability_type"`
	UpstreamProtocol   string          `json:"upstream_protocol"`
	RequestPath        *string         `json:"request_path"`
	UpstreamParameters json.RawMessage `json:"upstream_parameters"`
	Tags               json.RawMessage `json:"tags"`
	Status             string          `json:"status"`
}

type createModelRouteRequest struct {
	UpstreamDeploymentID string `json:"upstream_deployment_id"`
	Priority             *int32 `json:"priority"`
	Weight               *int32 `json:"weight"`
	SupportsStream       *bool  `json:"supports_stream"`
	Status               string `json:"status"`
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
			ctx := adminContext{
				Actor:    "local_admin",
				Role:     adminRolePlatform,
				UserType: 1,
			}
			next.ServeHTTP(w, r.WithContext(withAdminContext(r.Context(), ctx)))
			return
		}
		if ctx, ok := s.validURMAdminToken(r); ok {
			next.ServeHTTP(w, r.WithContext(withAdminContext(r.Context(), ctx)))
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

func (s *Server) validURMAdminToken(r *http.Request) (adminContext, bool) {
	if s.urmClient == nil {
		return adminContext{}, false
	}

	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		return adminContext{}, false
	}

	userInfo, err := s.urmClient.UserInfo(r.Context(), token)
	if err != nil {
		s.logger.Warn("validate urm admin token failed",
			"error", err,
			"request_id", requestIDFromContext(r.Context()),
		)
		return adminContext{}, false
	}

	role, ok := adminRoleForUserType(userInfo.UserType)
	if !ok {
		return adminContext{}, false
	}
	actor := userInfo.Subject
	if actor == "" {
		actor = userInfo.Username
	}
	if actor == "" {
		actor = "urm_admin"
	}
	return adminContext{
		Actor:    actor,
		Role:     role,
		TenantID: userInfo.TenantID,
		UserID:   userInfo.Subject,
		UserType: userInfo.UserType,
	}, true
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

func adminRoleForUserType(userType int) (adminRole, bool) {
	switch userType {
	case 1:
		return adminRolePlatform, true
	case 2:
		return adminRoleTenant, true
	case 3:
		return adminRoleUser, true
	default:
		return "", false
	}
}

func withAdminContext(ctx context.Context, admin adminContext) context.Context {
	ctx = context.WithValue(ctx, adminContextKey{}, admin)
	ctx = context.WithValue(ctx, adminActorContextKey{}, admin.Actor)
	return ctx
}

func adminFromContext(ctx context.Context) (adminContext, bool) {
	admin, ok := ctx.Value(adminContextKey{}).(adminContext)
	return admin, ok
}

func (s *Server) adminScope(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		admin, ok := adminFromContext(r.Context())
		if !ok || admin.Role == "" || admin.Role == adminRolePlatform {
			next.ServeHTTP(w, r)
			return
		}
		if adminRequestAllowed(admin, r.Method, r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		writeAdminError(w, http.StatusForbidden, "forbidden")
	})
}

func adminRequestAllowed(admin adminContext, method string, path string) bool {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 || parts[0] != "admin" {
		return false
	}
	if method == http.MethodGet && (parts[1] == "usage-logs" || parts[1] == "usage-summary" || parts[1] == "usage-unit-summary" || parts[1] == "dashboard") {
		return true
	}
	if len(parts) >= 3 && parts[1] == "tenants" {
		return tenantScopedAdminRequestAllowed(admin, method, parts[2], parts[3:])
	}
	return false
}

func tenantScopedAdminRequestAllowed(admin adminContext, method string, tenantID string, rest []string) bool {
	if tenantID == "" || tenantID != admin.TenantID {
		return false
	}
	switch admin.Role {
	case adminRoleTenant:
		return tenantAdminRequestAllowed(method, rest)
	case adminRoleUser:
		return userAdminRequestAllowed(admin, method, rest)
	default:
		return false
	}
}

func tenantAdminRequestAllowed(method string, rest []string) bool {
	if len(rest) == 1 && rest[0] == "model-grants" {
		return method == http.MethodGet
	}
	if len(rest) >= 1 && rest[0] == "api-keys" {
		return method == http.MethodGet || method == http.MethodPost || method == http.MethodPatch
	}
	if len(rest) >= 3 && rest[0] == "users" {
		switch rest[2] {
		case "model-grants":
			return method == http.MethodGet || method == http.MethodPost || method == http.MethodPatch
		case "api-keys":
			return method == http.MethodGet || method == http.MethodPost || method == http.MethodPatch
		default:
			return false
		}
	}
	return false
}

func userAdminRequestAllowed(admin adminContext, method string, rest []string) bool {
	if len(rest) < 3 || rest[0] != "users" || rest[1] != admin.UserID {
		return false
	}
	switch rest[2] {
	case "model-grants":
		return method == http.MethodGet
	case "api-keys":
		return method == http.MethodGet || method == http.MethodPost || method == http.MethodPatch
	default:
		return false
	}
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
	if req.Status == "" {
		req.Status = defaultStatus
	}

	ciphertext, err := secret.EncryptProviderKey(s.security.ProviderKeyMaster, req.APIKey)
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, err.Error())
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
		ProviderID: providerID, ID: endpointID, Name: req.Name, BaseUrl: req.BaseURL,
		ApiKeyCiphertext: ciphertext, ExtraHeaders: jsonObjectOrDefault(req.ExtraHeaders), Weight: int32OrDefault(req.Weight, defaultEndpointWeight),
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

func (s *Server) handleAdminCheckUpstreamDeploymentHealth(w http.ResponseWriter, r *http.Request) {
	deploymentID, ok := parseUUIDParam(w, r, "deploymentID")
	if !ok {
		return
	}

	deployment, err := s.queries.GetUpstreamDeploymentForHealthCheck(r.Context(), deploymentID)
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	if !upstreamDeploymentHealthProbeSupported(deployment.UpstreamProtocol) {
		writeAdminError(w, http.StatusUnprocessableEntity, "deployment protocol does not support active health probes")
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
		writeAdminDBError(w, err)
		return
	}

	response := map[string]any{
		"deployment": row,
		"status":     status,
	}
	if checkErr != nil {
		response["error"] = checkErr.Error()
	}
	writeAdminJSON(w, http.StatusOK, response)
}

func upstreamDeploymentHealthProbeSupported(protocol string) bool {
	return protocol == upstream.ProtocolOpenAIChatCompletions ||
		protocol == upstream.ProtocolOpenAIResponses ||
		protocol == upstream.ProtocolOpenAIEmbeddings
}

func (s *Server) checkUpstreamDeploymentReachable(r *http.Request, deployment dbgen.GetUpstreamDeploymentForHealthCheckRow) error {
	defaultPath, body, err := healthProbeRequest(deployment.UpstreamModel, deployment.UpstreamProtocol)
	if err != nil {
		return err
	}
	url, err := upstream.BuildEndpointURL(deployment.BaseUrl, optionalText(deployment.RequestPath), defaultPath)
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

func (s *Server) handleAdminGetModelPrice(w http.ResponseWriter, r *http.Request) {
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}
	row, err := s.queries.GetModelPrice(r.Context(), modelID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeAdminJSON(w, http.StatusOK, nil)
			return
		}
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
}

func (s *Server) handleAdminUpsertModelPrice(w http.ResponseWriter, r *http.Request) {
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}
	var req modelPriceRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if message := validateModelPriceCredits(req); message != "" {
		writeAdminError(w, http.StatusBadRequest, message)
		return
	}
	imageSizePrices := jsonObjectOrDefault(req.ImageSizePrices)
	row, err := s.queries.UpsertModelPrice(r.Context(), dbgen.UpsertModelPriceParams{
		ModelID:                modelID,
		InputPricePer1m:        req.InputPricePer1M,
		OutputPricePer1m:       req.OutputPricePer1M,
		ImageSizePrices:        imageSizePrices,
		VideoPricePerSecond:    req.VideoPricePerSecond,
		AudioTtsPricePer1mChars: req.AudioTTSPricePer1MChars,
		AudioSttPricePerMinute: req.AudioSTTPricePerMinute,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
}

// ============================================================================
// Tenant Model Price Override Handlers
// ============================================================================

func (s *Server) handleAdminListTenantModelPriceOverrides(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	rows, err := s.queries.ListTenantModelPriceOverrides(r.Context(), tenantID)
	if err != nil {
		s.writeAdminServerError(w, r, "list tenant model price overrides failed", err)
		return
	}
	writeAdminJSON(w, http.StatusOK, rows)
}

func (s *Server) handleAdminGetTenantModelPriceOverride(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}
	row, err := s.queries.GetTenantModelPriceOverride(r.Context(), dbgen.GetTenantModelPriceOverrideParams{
		TenantID: tenantID,
		ModelID:  modelID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeAdminJSON(w, http.StatusOK, nil)
			return
		}
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
}

func (s *Server) handleAdminUpsertTenantModelPriceOverride(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}
	var req modelPriceRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if message := validateModelPriceCredits(req); message != "" {
		writeAdminError(w, http.StatusBadRequest, message)
		return
	}
	imageSizePrices := jsonObjectOrDefault(req.ImageSizePrices)
	adminCtx, _ := adminFromContext(r.Context())
	row, err := s.queries.UpsertTenantModelPriceOverride(r.Context(), dbgen.UpsertTenantModelPriceOverrideParams{
		TenantID:                tenantID,
		ModelID:                 modelID,
		InputPricePer1m:         req.InputPricePer1M,
		OutputPricePer1m:        req.OutputPricePer1M,
		ImageSizePrices:         imageSizePrices,
		VideoPricePerSecond:     req.VideoPricePerSecond,
		AudioTtsPricePer1mChars: req.AudioTTSPricePer1MChars,
		AudioSttPricePerMinute:  req.AudioSTTPricePerMinute,
		CreatedBy:               optionalTextString(adminCtx.Actor),
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
}

func (s *Server) handleAdminDeleteTenantModelPriceOverride(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenantID")
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}
	err := s.queries.DeleteTenantModelPriceOverride(r.Context(), dbgen.DeleteTenantModelPriceOverrideParams{
		TenantID: tenantID,
		ModelID:  modelID,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ============================================================================
// Upstream Deployment Handlers
// ============================================================================

func (s *Server) handleAdminListUpstreamDeployments(w http.ResponseWriter, r *http.Request) {
	// endpoint_id is optional - if not provided, list all deployments
	endpointIDParam := r.URL.Query().Get("endpoint_id")

	var rows []dbgen.ListUpstreamDeploymentsRow
	var err error

	if endpointIDParam != "" {
		endpointID, parseErr := parseUUID(endpointIDParam)
		if parseErr != nil {
			writeAdminError(w, http.StatusBadRequest, "invalid endpoint_id")
			return
		}
		rows, err = s.queries.ListUpstreamDeployments(r.Context(), endpointID)
	} else {
		// List all deployments using direct query since no endpoint_id filter needed
		query := `
			SELECT
			  ud.id,
			  ud.endpoint_id,
			  ud.name,
			  ud.upstream_model,
			  ud.capability_type,
			  ud.upstream_protocol,
			  ud.request_path,
			  ud.upstream_parameters,
			  ud.tags,
			  ud.health_status,
			  ud.last_health_check_at,
			  ud.last_health_error,
			  ud.status,
			  ud.created_at,
			  ud.updated_at,
			  e.name AS endpoint_name,
			  e.base_url,
			  e.weight AS endpoint_weight,
			  p.id AS provider_id,
			  p.code AS provider_code,
			  p.name AS provider_name
			FROM ai_upstream_deployments ud
			JOIN ai_provider_endpoints e ON e.id = ud.endpoint_id
			JOIN ai_providers p ON p.id = e.provider_id
			ORDER BY ud.name ASC`
		dbRows, queryErr := s.postgres.Query(r.Context(), query)
		if queryErr != nil {
			err = queryErr
		} else {
			defer dbRows.Close()
			for dbRows.Next() {
				var row dbgen.ListUpstreamDeploymentsRow
				if scanErr := dbRows.Scan(
					&row.ID,
					&row.EndpointID,
					&row.Name,
					&row.UpstreamModel,
					&row.CapabilityType,
					&row.UpstreamProtocol,
					&row.RequestPath,
					&row.UpstreamParameters,
					&row.Tags,
					&row.HealthStatus,
					&row.LastHealthCheckAt,
					&row.LastHealthError,
					&row.Status,
					&row.CreatedAt,
					&row.UpdatedAt,
					&row.EndpointName,
					&row.BaseUrl,
					&row.EndpointWeight,
					&row.ProviderID,
					&row.ProviderCode,
					&row.ProviderName,
				); scanErr != nil {
					err = scanErr
					break
				}
				rows = append(rows, row)
			}
			if closeErr := dbRows.Err(); closeErr != nil {
				err = closeErr
			}
		}
	}

	if err != nil {
		s.writeAdminServerError(w, r, "list upstream deployments failed", err)
		return
	}
	writeAdminJSON(w, http.StatusOK, rows)
}

func (s *Server) handleAdminCreateUpstreamDeployment(w http.ResponseWriter, r *http.Request) {
	var req createUpstreamDeploymentRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.EndpointID == "" || req.UpstreamModel == "" || req.Name == "" {
		writeAdminError(w, http.StatusBadRequest, "endpoint_id, upstream_model and name are required")
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

	row, err := s.queries.CreateUpstreamDeployment(r.Context(), dbgen.CreateUpstreamDeploymentParams{
		EndpointID:         endpointID,
		Name:               req.Name,
		UpstreamModel:      req.UpstreamModel,
		CapabilityType:     req.CapabilityType,
		UpstreamProtocol:   req.UpstreamProtocol,
		RequestPath:        optionalTextParam(req.RequestPath),
		UpstreamParameters: jsonObjectOrDefault(req.UpstreamParameters),
		Tags:               jsonObjectOrDefault(req.Tags),
		Status:             req.Status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusCreated, row)
}

func (s *Server) handleAdminGetUpstreamDeployment(w http.ResponseWriter, r *http.Request) {
	deploymentID, ok := parseUUIDParam(w, r, "deploymentID")
	if !ok {
		return
	}

	row, err := s.queries.GetUpstreamDeployment(r.Context(), deploymentID)
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
}

func (s *Server) handleAdminUpdateUpstreamDeployment(w http.ResponseWriter, r *http.Request) {
	deploymentID, ok := parseUUIDParam(w, r, "deploymentID")
	if !ok {
		return
	}
	var req createUpstreamDeploymentRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.UpstreamModel == "" || req.Name == "" {
		writeAdminError(w, http.StatusBadRequest, "upstream_model and name are required")
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

	row, err := s.queries.UpdateUpstreamDeployment(r.Context(), dbgen.UpdateUpstreamDeploymentParams{
		ID:                 deploymentID,
		Name:               req.Name,
		UpstreamModel:      req.UpstreamModel,
		CapabilityType:     req.CapabilityType,
		UpstreamProtocol:   req.UpstreamProtocol,
		RequestPath:        optionalTextParam(req.RequestPath),
		UpstreamParameters: jsonObjectOrDefault(req.UpstreamParameters),
		Tags:               jsonObjectOrDefault(req.Tags),
		Status:             req.Status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
}

func (s *Server) handleAdminUpdateUpstreamDeploymentStatus(w http.ResponseWriter, r *http.Request) {
	deploymentID, ok := parseUUIDParam(w, r, "deploymentID")
	if !ok {
		return
	}
	status, ok := decodeStatusUpdate(w, r)
	if !ok {
		return
	}

	row, err := s.queries.UpdateUpstreamDeploymentStatus(r.Context(), dbgen.UpdateUpstreamDeploymentStatusParams{
		ID:     deploymentID,
		Status: status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
}

// ============================================================================
// Model Route Handlers
// ============================================================================

func (s *Server) handleAdminListModelRoutes(w http.ResponseWriter, r *http.Request) {
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}

	rows, err := s.queries.ListModelRoutes(r.Context(), modelID)
	if err != nil {
		s.writeAdminServerError(w, r, "list model routes failed", err)
		return
	}
	writeAdminJSON(w, http.StatusOK, rows)
}

func (s *Server) handleAdminCreateModelRoute(w http.ResponseWriter, r *http.Request) {
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}

	var req createModelRouteRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.UpstreamDeploymentID == "" {
		writeAdminError(w, http.StatusBadRequest, "upstream_deployment_id is required")
		return
	}
	upstreamDeploymentID, err := parseUUID(req.UpstreamDeploymentID)
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid upstream_deployment_id")
		return
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}

	row, err := s.queries.CreateModelRoute(r.Context(), dbgen.CreateModelRouteParams{
		ModelID:              modelID,
		UpstreamDeploymentID: upstreamDeploymentID,
		Priority:             int32OrDefault(req.Priority, 100),
		Weight:               int32OrDefault(req.Weight, 100),
		SupportsStream:       boolOrDefault(req.SupportsStream, true),
		Status:               req.Status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusCreated, row)
}

func (s *Server) handleAdminGetModelRoute(w http.ResponseWriter, r *http.Request) {
	routeID, ok := parseUUIDParam(w, r, "routeID")
	if !ok {
		return
	}

	row, err := s.queries.GetModelRoute(r.Context(), routeID)
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
}

func (s *Server) handleAdminUpdateModelRoute(w http.ResponseWriter, r *http.Request) {
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}
	routeID, ok := parseUUIDParam(w, r, "routeID")
	if !ok {
		return
	}
	var req createModelRouteRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if req.UpstreamDeploymentID == "" {
		writeAdminError(w, http.StatusBadRequest, "upstream_deployment_id is required")
		return
	}
	upstreamDeploymentID, err := parseUUID(req.UpstreamDeploymentID)
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid upstream_deployment_id")
		return
	}
	if req.Status == "" {
		req.Status = defaultStatus
	}

	row, err := s.queries.UpdateModelRoute(r.Context(), dbgen.UpdateModelRouteParams{
		ModelID:              modelID,
		ID:                   routeID,
		UpstreamDeploymentID: upstreamDeploymentID,
		Priority:             int32OrDefault(req.Priority, 100),
		Weight:               int32OrDefault(req.Weight, 100),
		SupportsStream:       boolOrDefault(req.SupportsStream, true),
		Status:               req.Status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
}

func (s *Server) handleAdminUpdateModelRouteStatus(w http.ResponseWriter, r *http.Request) {
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}
	routeID, ok := parseUUIDParam(w, r, "routeID")
	if !ok {
		return
	}
	status, ok := decodeStatusUpdate(w, r)
	if !ok {
		return
	}

	row, err := s.queries.UpdateModelRouteStatus(r.Context(), dbgen.UpdateModelRouteStatusParams{
		ModelID: modelID,
		ID:      routeID,
		Status:  status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
}

func (s *Server) handleAdminDeleteModelRoute(w http.ResponseWriter, r *http.Request) {
	modelID, ok := parseUUIDParam(w, r, "modelID")
	if !ok {
		return
	}
	routeID, ok := parseUUIDParam(w, r, "routeID")
	if !ok {
		return
	}

	// Delete route using direct SQL since no generated function exists
	result, err := s.postgres.Exec(r.Context(),
		"DELETE FROM ai_model_routes WHERE model_id = $1 AND id = $2",
		modelID, routeID)
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	if result.RowsAffected() == 0 {
		writeAdminError(w, http.StatusNotFound, "route not found")
		return
	}
	writeAdminJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ============================================================================
// Upstream Deployment Cost Price Handlers
// ============================================================================

func (s *Server) handleAdminListUpstreamDeploymentCostPrices(w http.ResponseWriter, r *http.Request) {
	deploymentID, ok := parseUUIDParam(w, r, "deploymentID")
	if !ok {
		return
	}

	rows, err := s.queries.ListUpstreamDeploymentCostPrices(r.Context(), deploymentID)
	if err != nil {
		s.writeAdminServerError(w, r, "list upstream deployment cost prices failed", err)
		return
	}
	writeAdminJSON(w, http.StatusOK, rows)
}

func (s *Server) handleAdminCreateUpstreamDeploymentCostPrice(w http.ResponseWriter, r *http.Request) {
	deploymentID, ok := parseUUIDParam(w, r, "deploymentID")
	if !ok {
		return
	}

	var req createUpstreamDeploymentCostPriceRequest
	if !decodeAdminJSON(w, r, &req) {
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
	if message := validateUpstreamDeploymentCostPriceCredits(req); message != "" {
		writeAdminError(w, http.StatusBadRequest, message)
		return
	}
	effectiveFrom, err := parseEffectiveFrom(req.EffectiveFrom)
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid effective_from")
		return
	}

	row, err := s.queries.CreateUpstreamDeploymentCostPrice(r.Context(), dbgen.CreateUpstreamDeploymentCostPriceParams{
		UpstreamDeploymentID: deploymentID,
		CapabilityType:       req.CapabilityType,
		Currency:             req.Currency,
		InputCostPer1m:       req.InputCostPer1M,
		OutputCostPer1m:      req.OutputCostPer1M,
		RequestCost:          req.RequestCost,
		ImageCost:            req.ImageCost,
		ImageSizePrices:      jsonObjectOrDefault(req.ImageSizePrices),
		VideoCostPerSecond:   req.VideoCostPerSecond,
		EffectiveFrom:        effectiveFrom,
		Status:               req.Status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusCreated, row)
}

func (s *Server) handleAdminUpdateUpstreamDeploymentCostPrice(w http.ResponseWriter, r *http.Request) {
	deploymentID, ok := parseUUIDParam(w, r, "deploymentID")
	if !ok {
		return
	}
	priceID, ok := parseUUIDParam(w, r, "priceID")
	if !ok {
		return
	}
	var req createUpstreamDeploymentCostPriceRequest
	if !decodeAdminJSON(w, r, &req) {
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
	if message := validateUpstreamDeploymentCostPriceCredits(req); message != "" {
		writeAdminError(w, http.StatusBadRequest, message)
		return
	}
	effectiveFrom, err := parseEffectiveFrom(req.EffectiveFrom)
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid effective_from")
		return
	}

	row, err := s.queries.UpdateUpstreamDeploymentCostPrice(r.Context(), dbgen.UpdateUpstreamDeploymentCostPriceParams{
		UpstreamDeploymentID: deploymentID,
		ID:                   priceID,
		CapabilityType:       req.CapabilityType,
		Currency:             req.Currency,
		InputCostPer1m:       req.InputCostPer1M,
		OutputCostPer1m:      req.OutputCostPer1M,
		RequestCost:          req.RequestCost,
		ImageCost:            req.ImageCost,
		ImageSizePrices:      jsonObjectOrDefault(req.ImageSizePrices),
		VideoCostPerSecond:   req.VideoCostPerSecond,
		EffectiveFrom:        effectiveFrom,
		Status:               req.Status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
}

func (s *Server) handleAdminUpdateUpstreamDeploymentCostPriceStatus(w http.ResponseWriter, r *http.Request) {
	deploymentID, ok := parseUUIDParam(w, r, "deploymentID")
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

	row, err := s.queries.UpdateUpstreamDeploymentCostPriceStatus(r.Context(), dbgen.UpdateUpstreamDeploymentCostPriceStatusParams{
		UpstreamDeploymentID: deploymentID,
		ID:                   priceID,
		Status:               status,
	})
	if err != nil {
		writeAdminDBError(w, err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
}

// ============================================================================
// Tenant Model Grant Handlers
// ============================================================================

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

// ============================================================================
// Tenant API Key Handlers
// ============================================================================

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

	filters, ok := scopedUsageFilters(w, r)
	if !ok {
		return
	}
	rows, err := s.queries.ListUsageLogs(r.Context(), dbgen.ListUsageLogsParams{
		TenantID: filters.tenantID,
		Limit:    limit,
		Offset:   0,
	})
	if err != nil {
		s.writeAdminServerError(w, r, "list usage logs failed", err)
		return
	}
	writeAdminJSON(w, http.StatusOK, rows)
}

func (s *Server) handleAdminListUsageSummary(w http.ResponseWriter, r *http.Request) {
	filters, ok := scopedUsageFilters(w, r)
	if !ok {
		return
	}
	rows, err := s.queries.ListUsageSummary(r.Context(), dbgen.ListUsageSummaryParams{
		TenantID:      optionalTextValue(filters.tenantID),
		UserID:        optionalTextValue(filters.userID),
		ModelCode:     optionalTextValue(filters.modelCode),
		RequestStatus: optionalTextValue(filters.requestStatus),
	})
	if err != nil {
		s.writeAdminServerError(w, r, "list usage summary failed", err)
		return
	}
	writeAdminJSON(w, http.StatusOK, rows)
}

func (s *Server) handleAdminListUsageUnitSummary(w http.ResponseWriter, r *http.Request) {
	filters, ok := scopedUsageFilters(w, r)
	if !ok {
		return
	}
	rows, err := s.queries.ListUsageUnitSummary(r.Context(), dbgen.ListUsageUnitSummaryParams{
		TenantID:      optionalTextValue(filters.tenantID),
		UserID:        optionalTextValue(filters.userID),
		ModelCode:     optionalTextValue(filters.modelCode),
		RequestStatus: optionalTextValue(filters.requestStatus),
	})
	if err != nil {
		s.writeAdminServerError(w, r, "list usage unit summary failed", err)
		return
	}
	writeAdminJSON(w, http.StatusOK, rows)
}

func (s *Server) handleAdminDashboardSummary(w http.ResponseWriter, r *http.Request) {
	params, ok := scopedDashboardParams(w, r)
	if !ok {
		return
	}
	row, err := s.queries.GetDashboardSummary(r.Context(), dbgen.GetDashboardSummaryParams{
		TenantID: optionalTextValue(params.tenantID),
		UserID:   optionalTextValue(params.userID),
		Since:    params.since,
	})
	if err != nil {
		s.writeAdminServerError(w, r, "get dashboard summary failed", err)
		return
	}
	writeAdminJSON(w, http.StatusOK, row)
}

func (s *Server) handleAdminDashboardTopModels(w http.ResponseWriter, r *http.Request) {
	params, ok := scopedDashboardParams(w, r)
	if !ok {
		return
	}
	limit, ok := parseAdminLimit(w, r, 10, 50)
	if !ok {
		return
	}
	rows, err := s.queries.ListDashboardTopModels(r.Context(), dbgen.ListDashboardTopModelsParams{
		TenantID: optionalTextValue(params.tenantID),
		UserID:   optionalTextValue(params.userID),
		Since:    params.since,
		Limit:    limit,
	})
	if err != nil {
		s.writeAdminServerError(w, r, "list dashboard top models failed", err)
		return
	}
	writeAdminJSON(w, http.StatusOK, rows)
}

func (s *Server) handleAdminDashboardTopTenants(w http.ResponseWriter, r *http.Request) {
	params, ok := scopedDashboardParams(w, r)
	if !ok {
		return
	}
	limit, ok := parseAdminLimit(w, r, 10, 50)
	if !ok {
		return
	}
	rows, err := s.queries.ListDashboardTopTenants(r.Context(), dbgen.ListDashboardTopTenantsParams{
		TenantID: optionalTextValue(params.tenantID),
		UserID:   optionalTextValue(params.userID),
		Since:    params.since,
		Limit:    limit,
	})
	if err != nil {
		s.writeAdminServerError(w, r, "list dashboard top tenants failed", err)
		return
	}
	writeAdminJSON(w, http.StatusOK, rows)
}

func (s *Server) handleAdminDashboardRecentErrors(w http.ResponseWriter, r *http.Request) {
	params, ok := scopedDashboardParams(w, r)
	if !ok {
		return
	}
	limit, ok := parseAdminLimit(w, r, 10, 50)
	if !ok {
		return
	}
	rows, err := s.queries.ListDashboardRecentErrors(r.Context(), dbgen.ListDashboardRecentErrorsParams{
		TenantID: optionalTextValue(params.tenantID),
		UserID:   optionalTextValue(params.userID),
		Since:    params.since,
		Limit:    limit,
	})
	if err != nil {
		s.writeAdminServerError(w, r, "list dashboard recent errors failed", err)
		return
	}
	writeAdminJSON(w, http.StatusOK, rows)
}

type usageFilters struct {
	tenantID      string
	userID        string
	modelCode     string
	requestStatus string
}

type dashboardParams struct {
	tenantID string
	userID   string
	since    pgtype.Timestamptz
}

func scopedDashboardParams(w http.ResponseWriter, r *http.Request) (dashboardParams, bool) {
	filters, ok := scopedUsageFilters(w, r)
	if !ok {
		return dashboardParams{}, false
	}
	since, ok := parseDashboardSince(w, r)
	if !ok {
		return dashboardParams{}, false
	}
	return dashboardParams{
		tenantID: filters.tenantID,
		userID:   filters.userID,
		since:    since,
	}, true
}

func parseDashboardSince(w http.ResponseWriter, r *http.Request) (pgtype.Timestamptz, bool) {
	raw := r.URL.Query().Get("days")
	if raw == "" {
		raw = "1"
	}
	days, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || days < 0 || days > 3650 {
		writeAdminError(w, http.StatusBadRequest, "invalid days")
		return pgtype.Timestamptz{}, false
	}
	if days == 0 {
		return pgtype.Timestamptz{}, true
	}
	return pgtype.Timestamptz{Time: time.Now().UTC().AddDate(0, 0, -int(days)), Valid: true}, true
}

func parseAdminLimit(w http.ResponseWriter, r *http.Request, defaultLimit int32, maxLimit int32) (int32, bool) {
	limit := defaultLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || parsed <= 0 {
			writeAdminError(w, http.StatusBadRequest, "invalid limit")
			return 0, false
		}
		if parsed > int64(maxLimit) {
			parsed = int64(maxLimit)
		}
		limit = int32(parsed)
	}
	return limit, true
}

func scopedUsageFilters(w http.ResponseWriter, r *http.Request) (usageFilters, bool) {
	filters := usageFilters{
		tenantID:      r.URL.Query().Get("tenant_id"),
		userID:        r.URL.Query().Get("user_id"),
		modelCode:     r.URL.Query().Get("model_code"),
		requestStatus: r.URL.Query().Get("request_status"),
	}
	admin, ok := adminFromContext(r.Context())
	if !ok {
		return filters, true
	}
	switch admin.Role {
	case adminRolePlatform, "":
		return filters, true
	case adminRoleTenant:
		if admin.TenantID == "" {
			writeAdminError(w, http.StatusForbidden, "tenant scope is required")
			return usageFilters{}, false
		}
		if filters.tenantID != "" && filters.tenantID != admin.TenantID {
			writeAdminError(w, http.StatusForbidden, "forbidden")
			return usageFilters{}, false
		}
		filters.tenantID = admin.TenantID
	case adminRoleUser:
		if admin.TenantID == "" || admin.UserID == "" {
			writeAdminError(w, http.StatusForbidden, "user scope is required")
			return usageFilters{}, false
		}
		if filters.tenantID != "" && filters.tenantID != admin.TenantID {
			writeAdminError(w, http.StatusForbidden, "forbidden")
			return usageFilters{}, false
		}
		if filters.userID != "" && filters.userID != admin.UserID {
			writeAdminError(w, http.StatusForbidden, "forbidden")
			return usageFilters{}, false
		}
		filters.tenantID = admin.TenantID
		filters.userID = admin.UserID
	default:
		writeAdminError(w, http.StatusForbidden, "forbidden")
		return usageFilters{}, false
	}
	return filters, true
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
