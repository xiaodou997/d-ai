package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	dbgen "uni-ai-api/backend/internal/db/gen"
	"uni-ai-api/backend/internal/secret"
	"uni-ai-api/backend/internal/upstream"
)

type embeddingsEnvelope struct {
	Model string          `json:"model"`
	Input json.RawMessage `json:"input"`
}

func (s *Server) handleEmbeddings(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	auth, ok := runtimeAuthFromContext(r.Context())
	if !ok {
		writeOpenAIError(w, http.StatusUnauthorized, "Invalid API key.", "invalid_api_key", "invalid_api_key")
		return
	}

	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "Invalid request body.", "invalid_request_error", "invalid_json")
		return
	}
	var req embeddingsEnvelope
	if err := decodeEmbeddingsEnvelope(raw, &req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "invalid_request")
		return
	}
	if req.Model == "" {
		writeOpenAIError(w, http.StatusBadRequest, "Missing required parameter: model.", "invalid_request_error", "missing_required_parameter")
		return
	}
	if len(req.Input) == 0 {
		writeOpenAIError(w, http.StatusBadRequest, "Missing required parameter: input.", "invalid_request_error", "missing_required_parameter")
		return
	}
	if !apiKeyAllowsModel(auth.APIKey.AllowedModels, req.Model) {
		writeOpenAIError(w, http.StatusNotFound, "The model does not exist or you do not have access to it.", "invalid_request_error", "model_not_found")
		return
	}
	if quotaExhausted(auth.APIKey.QuotaLimit, auth.APIKey.QuotaUsed, auth.APIKey.QuotaReserved) {
		writeOpenAIError(w, http.StatusPaymentRequired, "You exceeded your current quota.", "insufficient_quota", "insufficient_quota")
		return
	}
	s.logGatewayRequestReceived(r, "embedding", req.Model, false)

	model, err := s.resolveCallableModelForCapability(r, auth, req.Model, "embedding")
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeOpenAIError(w, http.StatusNotFound, "The model does not exist or you do not have access to it.", "invalid_request_error", "model_not_found")
			return
		}
		s.logger.Error("resolve embedding model failed", "error", err, "request_id", requestIDFromContext(r.Context()))
		writeOpenAIError(w, http.StatusInternalServerError, "Failed to resolve model.", "server_error", "server_error")
		return
	}

	routes, err := s.queries.ListRoutesForModel(r.Context(), model.ID)
	if err != nil {
		s.logger.Error("list embedding routes failed", "error", err, "request_id", requestIDFromContext(r.Context()))
		writeOpenAIError(w, http.StatusInternalServerError, "Failed to resolve model route.", "server_error", "server_error")
		return
	}
	route, ok := s.chooseRoute(r.Context(), routes, "")
	if !ok {
		writeOpenAIError(w, http.StatusServiceUnavailable, "No available route for the requested model.", "server_error", "model_unavailable")
		return
	}
	s.logGatewayRouteSelected(r, "embedding", req.Model, route)

	estimatedUsage := ensureUsageTotal(upstreamUsage{PromptTokens: estimateEmbeddingInputTokens(raw)})
	runtimeLease, ok := s.acquireRuntimeLimits(w, r, auth, req.Model, "embedding", estimatedUsage.PromptTokens, route)
	if !ok {
		return
	}
	defer s.releaseRuntimeLimits(r.Context(), runtimeLease)

	_, quotaReservation, settlementReservation, ok := s.reserveTokenBilling(w, r, auth, model, estimatedUsage)
	if !ok {
		return
	}
	defer s.releaseAPIKeyQuotaReservation(r.Context(), quotaReservation)

	if route.UpstreamProtocol != upstream.ProtocolOpenAIEmbeddings {
		s.recordEmbeddingUsage(r, embeddingUsageInput{
			start:            start,
			auth:             auth,
			raw:              raw,
			model:            model,
			modelCode:        req.Model,
			route:            &route,
			httpStatus:       http.StatusBadGateway,
			requestStatus:    "failed",
			errorCode:        "unsupported_upstream_protocol",
			errorMessage:     "Selected route protocol is not supported for embeddings.",
			billingStatus:    s.cancelChatSettlement(r.Context(), settlementReservation),
			urmTransactionID: settlementTransactionID(settlementReservation),
		})
		writeOpenAIError(w, http.StatusBadGateway, "Selected route protocol is not supported for embeddings.", "server_error", "unsupported_upstream_protocol")
		return
	}

	providerKey, err := secret.DecryptProviderKey(s.security.ProviderKeyMaster, route.ApiKeyCiphertext)
	if err != nil {
		s.recordEmbeddingUsage(r, embeddingUsageInput{
			start:            start,
			auth:             auth,
			raw:              raw,
			model:            model,
			modelCode:        req.Model,
			route:            &route,
			httpStatus:       http.StatusInternalServerError,
			requestStatus:    "failed",
			errorCode:        "provider_credential_error",
			errorMessage:     "Provider credential is not configured correctly.",
			billingStatus:    s.cancelChatSettlement(r.Context(), settlementReservation),
			urmTransactionID: settlementTransactionID(settlementReservation),
		})
		writeOpenAIError(w, http.StatusInternalServerError, "Provider credential is not configured correctly.", "server_error", "provider_credential_error")
		return
	}

	s.logGatewayUpstreamStarted(r, "embedding", req.Model, route)
	resp, err := upstream.ForwardOpenAIEmbeddings(r.Context(), s.httpClient, upstream.OpenAIEmbeddingsRequest{
		BaseURL:            route.BaseUrl,
		RequestPath:        optionalText(route.RequestPath),
		APIKey:             providerKey,
		UpstreamModel:      route.UpstreamModel,
		ExtraHeaders:       route.ExtraHeaders,
		UpstreamParameters: route.UpstreamParameters,
		Timeout:            time.Duration(route.TimeoutMs) * time.Millisecond,
		Body:               raw,
	})
	if err != nil {
		s.markUpstreamDeploymentCooldown(r.Context(), route.UpstreamDeploymentID, "upstream_request_failed")
		billingStatus := s.cancelChatSettlement(r.Context(), settlementReservation)
		s.recordEmbeddingUsage(r, embeddingUsageInput{
			start: start, auth: auth, raw: raw, model: model, modelCode: req.Model, route: &route,
			httpStatus: http.StatusBadGateway, requestStatus: "failed", errorCode: "upstream_error", errorMessage: err.Error(),
			billingStatus: billingStatus, urmTransactionID: settlementTransactionID(settlementReservation),
		})
		writeOpenAIError(w, http.StatusBadGateway, "Failed to call upstream provider.", "server_error", "upstream_error")
		return
	}

	requestStatus := "success"
	errorCode := ""
	errorMessage := ""
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		requestStatus = "failed"
		errorCode = "upstream_error"
		errorMessage = string(resp.Body)
		s.logGatewayUpstreamFailed(r, "embedding", req.Model, route, resp.StatusCode, resp.Body)
		if shouldCooldownUpstreamStatus(resp.StatusCode) {
			s.markUpstreamDeploymentCooldown(r.Context(), route.UpstreamDeploymentID, "upstream_status_"+http.StatusText(resp.StatusCode))
		}
	}
	usage := ensureUsageTotal(parseOpenAIEmbeddingsUsage(resp))
	usageEstimated := false
	usageSource := "upstream"
	if requestStatus == "success" && !usageHasTokens(usage) {
		usage = estimatedUsage
		usageEstimated = usageHasTokens(usage)
		if usageEstimated {
			usageSource = "estimated_length"
		}
	}
	costs := s.calculateChatCosts(r.Context(), usageLogInput{
		Auth: auth, ModelID: model.ID, Route: &route, CapabilityType: "embedding", RequestStatus: requestStatus, Usage: usage,
	})
	billingStatus := "not_billed"
	if requestStatus == "success" {
		billingStatus = s.confirmChatSettlement(r.Context(), settlementReservation, costs)
	} else {
		billingStatus = s.cancelChatSettlement(r.Context(), settlementReservation)
	}
	s.recordEmbeddingUsage(r, embeddingUsageInput{
		start: start, auth: auth, raw: raw, model: model, modelCode: req.Model, route: &route,
		httpStatus: resp.StatusCode, upstreamStatus: resp.StatusCode, requestStatus: requestStatus, errorCode: errorCode, errorMessage: errorMessage,
		usage: usage, usageEstimated: usageEstimated, usageSource: usageSource, costs: &costs, billingStatus: billingStatus,
		urmTransactionID: settlementTransactionID(settlementReservation),
	})
	copyUpstreamResponse(w, resp)
}

func decodeEmbeddingsEnvelope(raw map[string]json.RawMessage, target *embeddingsEnvelope) error {
	if raw == nil {
		return errors.New("Invalid request body.")
	}
	if value, ok := raw["model"]; ok {
		if err := json.Unmarshal(value, &target.Model); err != nil {
			return errors.New("Invalid parameter: model.")
		}
	}
	if value, ok := raw["input"]; ok {
		target.Input = value
	}
	return nil
}

func parseOpenAIEmbeddingsUsage(resp *upstream.Response) upstreamUsage {
	if resp == nil || len(resp.Body) == 0 || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return upstreamUsage{}
	}
	var body struct {
		Usage struct {
			PromptTokens int32 `json:"prompt_tokens"`
			TotalTokens  int32 `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return upstreamUsage{}
	}
	return upstreamUsage{PromptTokens: body.Usage.PromptTokens, TotalTokens: body.Usage.TotalTokens}
}

func estimateEmbeddingInputTokens(raw map[string]json.RawMessage) int32 {
	if raw == nil {
		return 0
	}
	if input, ok := raw["input"]; ok {
		return estimateJSONTokens(input)
	}
	return 0
}

type embeddingUsageInput struct {
	start            time.Time
	auth             RuntimeAuth
	raw              map[string]json.RawMessage
	model            callableModel
	modelCode        string
	route            *dbgen.ListRoutesForModelRow
	httpStatus       int
	upstreamStatus   int
	requestStatus    string
	errorCode        string
	errorMessage     string
	usage            upstreamUsage
	usageEstimated   bool
	usageSource      string
	costs            *chatCosts
	billingStatus    string
	urmTransactionID string
}

func (s *Server) recordEmbeddingUsage(r *http.Request, input embeddingUsageInput) {
	usage := input.usage
	s.recordChatUsage(r.Context(), usageLogInput{
		Auth:             input.auth,
		RequestID:        requestIDFromContext(r.Context()),
		TraceID:          requestTraceID(r),
		ExternalUserID:   externalUserID(input.raw),
		ModelCode:        input.modelCode,
		CapabilityType:   "embedding",
		ModelID:          input.model.ID,
		Route:            input.route,
		Stream:           false,
		HTTPStatus:       input.httpStatus,
		UpstreamStatus:   input.upstreamStatus,
		Latency:          time.Since(input.start),
		RequestStatus:    input.requestStatus,
		ErrorCode:        input.errorCode,
		ErrorMessage:     input.errorMessage,
		Usage:            usage,
		BillableUnitType: "input_token",
		BillableUnits:    int64(usage.PromptTokens),
		UsageEstimated:   input.usageEstimated,
		UsageSource:      input.usageSource,
		Costs:            input.costs,
		URMTransactionID: input.urmTransactionID,
		BillingStatus:    input.billingStatus,
	})
}
