package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	dbgen "uni-ai-api/backend/internal/db/gen"
	"uni-ai-api/backend/internal/secret"
	"uni-ai-api/backend/internal/upstream"
)

type imageGenerationEnvelope struct {
	Model          string `json:"model"`
	Prompt         any    `json:"prompt"`
	N              int32  `json:"n,omitempty"`
	Size           string `json:"size,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
}

func (s *Server) handleImageGenerations(w http.ResponseWriter, r *http.Request) {
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
	var req imageGenerationEnvelope
	if err := decodeImageEnvelope(raw, &req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "invalid_request")
		return
	}
	if req.Model == "" {
		writeOpenAIError(w, http.StatusBadRequest, "Missing required parameter: model.", "invalid_request_error", "missing_required_parameter")
		return
	}
	if req.Prompt == nil {
		writeOpenAIError(w, http.StatusBadRequest, "Missing required parameter: prompt.", "invalid_request_error", "missing_required_parameter")
		return
	}
	if req.N <= 0 {
		req.N = 1
	}
	if !apiKeyAllowsModel(auth.APIKey.AllowedModels, req.Model) {
		writeOpenAIError(w, http.StatusNotFound, "The model does not exist or you do not have access to it.", "invalid_request_error", "model_not_found")
		return
	}
	if quotaExhausted(auth.APIKey.QuotaLimit, auth.APIKey.QuotaUsed, auth.APIKey.QuotaReserved) {
		writeOpenAIError(w, http.StatusPaymentRequired, "You exceeded your current quota.", "insufficient_quota", "insufficient_quota")
		return
	}
	s.logGatewayRequestReceived(r, "image", req.Model, false)

	model, err := s.resolveCallableModelForCapability(r, auth, req.Model, "image")
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeOpenAIError(w, http.StatusNotFound, "The model does not exist or you do not have access to it.", "invalid_request_error", "model_not_found")
			return
		}
		s.logger.Error("resolve image model failed", "error", err, "request_id", requestIDFromContext(r.Context()))
		writeOpenAIError(w, http.StatusInternalServerError, "Failed to resolve model.", "server_error", "server_error")
		return
	}

	routes, err := s.queries.ListRoutesForModel(r.Context(), model.ID)
	if err != nil {
		s.logger.Error("list image routes failed", "error", err, "request_id", requestIDFromContext(r.Context()))
		writeOpenAIError(w, http.StatusInternalServerError, "Failed to resolve model route.", "server_error", "server_error")
		return
	}
	route, ok := s.chooseRoute(r.Context(), routes, "")
	if !ok {
		writeOpenAIError(w, http.StatusServiceUnavailable, "No available route for the requested model.", "server_error", "model_unavailable")
		return
	}
	s.logGatewayRouteSelected(r, "image", req.Model, route)

	runtimeLease, ok := s.acquireRuntimeLimits(w, r, auth, req.Model, "image", estimateImageRateTokens(raw), route)
	if !ok {
		return
	}
	defer s.releaseRuntimeLimits(r.Context(), runtimeLease)

	price, quotaReservation, settlementReservation, ok := s.reserveImageBilling(w, r, auth, model, req.N, req.Size)
	if !ok {
		return
	}
	defer s.releaseAPIKeyQuotaReservation(r.Context(), quotaReservation)

	if route.UpstreamProtocol != upstream.ProtocolOpenAIImagesGenerations {
		s.recordImageUsage(r, imageUsageInput{
			Auth:             auth,
			Model:            model,
			ModelCode:        req.Model,
			Route:            &route,
			ImageCount:       req.N,
			HTTPStatus:       http.StatusBadGateway,
			Latency:          time.Since(start),
			RequestStatus:    "failed",
			ErrorCode:        "unsupported_upstream_protocol",
			ErrorMessage:     "Selected route protocol is not supported for image generations.",
			BillingStatus:    s.cancelChatSettlement(r.Context(), settlementReservation),
			URMTransactionID: settlementTransactionID(settlementReservation),
		})
		writeOpenAIError(w, http.StatusBadGateway, "Selected route protocol is not supported for image generations.", "server_error", "unsupported_upstream_protocol")
		return
	}

	providerKey, err := secret.DecryptProviderKey(s.security.ProviderKeyMaster, route.ApiKeyCiphertext)
	if err != nil {
		s.recordImageUsage(r, imageUsageInput{
			Auth:             auth,
			Model:            model,
			ModelCode:        req.Model,
			Route:            &route,
			ImageCount:       req.N,
			HTTPStatus:       http.StatusInternalServerError,
			Latency:          time.Since(start),
			RequestStatus:    "failed",
			ErrorCode:        "provider_credential_error",
			ErrorMessage:     "Provider credential is not configured correctly.",
			BillingStatus:    s.cancelChatSettlement(r.Context(), settlementReservation),
			URMTransactionID: settlementTransactionID(settlementReservation),
		})
		writeOpenAIError(w, http.StatusInternalServerError, "Provider credential is not configured correctly.", "server_error", "provider_credential_error")
		return
	}

	s.logGatewayUpstreamStarted(r, "image", req.Model, route)
	resp, err := upstream.ForwardOpenAIImageGeneration(r.Context(), s.httpClient, upstream.OpenAIImageRequest{
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
		s.recordImageUsage(r, imageUsageInput{
			Auth:             auth,
			Model:            model,
			ModelCode:        req.Model,
			Route:            &route,
			ImageCount:       req.N,
			HTTPStatus:       http.StatusBadGateway,
			Latency:          time.Since(start),
			RequestStatus:    "failed",
			ErrorCode:        "upstream_error",
			ErrorMessage:     err.Error(),
			BillingStatus:    billingStatus,
			URMTransactionID: settlementTransactionID(settlementReservation),
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
		s.logGatewayUpstreamFailed(r, "image", req.Model, route, resp.StatusCode, resp.Body)
		if shouldCooldownUpstreamStatus(resp.StatusCode) {
			s.markUpstreamDeploymentCooldown(r.Context(), route.UpstreamDeploymentID, "upstream_status_"+http.StatusText(resp.StatusCode))
		}
	}

	costs := s.calculateImageCosts(r.Context(), auth, model, &route, req.N, req.Size, price)
	billingStatus := "not_billed"
	if requestStatus == "success" {
		billingStatus = s.confirmChatSettlement(r.Context(), settlementReservation, costs)
	} else {
		billingStatus = s.cancelChatSettlement(r.Context(), settlementReservation)
	}
	s.recordImageUsage(r, imageUsageInput{
		Auth:             auth,
		Model:            model,
		ModelCode:        req.Model,
		Route:            &route,
		ImageCount:       req.N,
		ImageSize:        req.Size,
		HTTPStatus:       resp.StatusCode,
		UpstreamStatus:   resp.StatusCode,
		Latency:          time.Since(start),
		RequestStatus:    requestStatus,
		ErrorCode:        errorCode,
		ErrorMessage:     errorMessage,
		Costs:            &costs,
		BillingStatus:    billingStatus,
		URMTransactionID: settlementTransactionID(settlementReservation),
	})
	copyUpstreamResponse(w, resp)
}

func decodeImageEnvelope(raw map[string]json.RawMessage, target *imageGenerationEnvelope) error {
	if raw == nil {
		return errors.New("Invalid request body.")
	}
	if value, ok := raw["model"]; ok {
		if err := json.Unmarshal(value, &target.Model); err != nil {
			return errors.New("Invalid parameter: model.")
		}
	}
	if value, ok := raw["prompt"]; ok {
		if err := json.Unmarshal(value, &target.Prompt); err != nil {
			return errors.New("Invalid parameter: prompt.")
		}
	}
	if value, ok := raw["n"]; ok {
		if err := json.Unmarshal(value, &target.N); err != nil {
			return errors.New("Invalid parameter: n.")
		}
	}
	if target.N < 0 {
		return errors.New("Invalid parameter: n.")
	}
	return nil
}

type imageUsageInput struct {
	Auth             RuntimeAuth
	Model            callableModel
	ModelCode        string
	Route            *dbgen.ListRoutesForModelRow
	ImageCount       int32
	ImageSize        string
	HTTPStatus       int
	UpstreamStatus   int
	Latency          time.Duration
	RequestStatus    string
	ErrorCode        string
	ErrorMessage     string
	Costs            *chatCosts
	URMTransactionID string
	BillingStatus    string
}

func (s *Server) recordImageUsage(r *http.Request, input imageUsageInput) {
	usage := upstreamUsage{}
	s.recordChatUsage(r.Context(), usageLogInput{
		Auth:             input.Auth,
		RequestID:        requestIDFromContext(r.Context()),
		TraceID:          requestTraceID(r),
		ExternalUserID:   "",
		ModelCode:        input.ModelCode,
		CapabilityType:   "image",
		ModelID:          input.Model.ID,
		Route:            input.Route,
		Stream:           false,
		HTTPStatus:       input.HTTPStatus,
		UpstreamStatus:   input.UpstreamStatus,
		Latency:          input.Latency,
		RequestStatus:    input.RequestStatus,
		ErrorCode:        input.ErrorCode,
		ErrorMessage:     input.ErrorMessage,
		Usage:            usage,
		BillableUnitType: "image",
		BillableUnits:    int64(input.ImageCount),
		UsageEstimated:   false,
		UsageSource:      "image_count",
		Costs:            input.Costs,
		URMTransactionID: input.URMTransactionID,
		BillingStatus:    input.BillingStatus,
		ImageSize:        input.ImageSize,
	})
}

func (s *Server) reserveImageBilling(w http.ResponseWriter, r *http.Request, auth RuntimeAuth, model callableModel, imageCount int32, imageSize string) (*modelPrice, *quotaReservation, *settlementReservation, bool) {
	price, err := s.getEffectiveModelPrice(r.Context(), auth, model.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeOpenAIError(w, http.StatusPaymentRequired, "Model pricing not configured.", "insufficient_quota", "model_price_not_configured")
			return nil, nil, nil, false
		}
		s.logger.Error("get image model price failed", "error", err, "request_id", requestIDFromContext(r.Context()))
		writeOpenAIError(w, http.StatusInternalServerError, "Quota reservation failed.", "server_error", "quota_reservation_failed")
		return nil, nil, nil, false
	}

	perImagePrice, ok := imagePriceForSize(imageSize, price.ImageSizePrices)
	if !ok {
		writeOpenAIError(w, http.StatusBadRequest, "Unsupported image size for this model.", "invalid_request_error", "unsupported_image_size")
		return nil, nil, nil, false
	}

	quotaCost := imageCost(imageCount, perImagePrice)
	quotaReservation, err := s.reserveAPIKeyQuota(r.Context(), auth, quotaCost)
	if err != nil {
		if errors.Is(err, errInsufficientQuotaReservation) {
			writeOpenAIError(w, http.StatusPaymentRequired, "You exceeded your current quota.", "insufficient_quota", "insufficient_quota")
			return &price, nil, nil, false
		}
		s.logger.Error("reserve image api key quota failed", "error", err, "request_id", requestIDFromContext(r.Context()))
		writeOpenAIError(w, http.StatusInternalServerError, "Quota reservation failed.", "server_error", "quota_reservation_failed")
		return &price, nil, nil, false
	}

	tenantCost := quotaCost
	userCost := int64(0)
	if auth.APIKey.OwnerType == "user" {
		userCost = quotaCost
	}
	settlementReservation, err := s.freezeChatSettlement(r.Context(), auth, requestIDFromContext(r.Context()), tenantCost, userCost)
	if err != nil {
		s.releaseAPIKeyQuotaReservation(r.Context(), quotaReservation)
		if isURMInsufficientBalance(err) {
			writeOpenAIError(w, http.StatusPaymentRequired, "Insufficient quota.", "insufficient_quota", "insufficient_quota")
			return &price, nil, nil, false
		}
		s.logger.Error("freeze image settlement failed", "error", err, "request_id", requestIDFromContext(r.Context()))
		writeOpenAIError(w, http.StatusServiceUnavailable, "Settlement reservation failed.", "server_error", "settlement_reservation_failed")
		return &price, nil, nil, false
	}
	return &price, quotaReservation, settlementReservation, true
}

func (s *Server) calculateImageCosts(ctx context.Context, auth RuntimeAuth, model callableModel, route *dbgen.ListRoutesForModelRow, imageCount int32, imageSize string, price *modelPrice) chatCosts {
	costs := chatCosts{}
	if price != nil {
		perImagePrice, _ := imagePriceForSize(imageSize, price.ImageSizePrices)
		costs.TenantCost = imageCost(imageCount, perImagePrice)
		costs.APIKeyQuotaCost = costs.TenantCost
		if auth.APIKey.OwnerType == "user" {
			costs.UserCost = costs.APIKeyQuotaCost
		}
	}
	if route != nil {
		providerPrice, err := s.queries.GetActiveUpstreamDeploymentCostPrice(ctx, route.UpstreamDeploymentID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			s.logger.Error("get deployment image cost price failed", "error", err)
		}
		if err == nil {
			perImageCost := calculateImageCost(imageSize, providerPrice)
			costs.ProviderCost = providerPrice.RequestCost + imageCost(imageCount, perImageCost)
		}
	}
	return costs
}

func imageCost(count int32, unit int64) int64 {
	if count <= 0 || unit <= 0 {
		return 0
	}
	return int64(count) * unit
}
