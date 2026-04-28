package httpserver

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dbgen "uni-ai-api/backend/internal/db/gen"
	"uni-ai-api/backend/internal/secret"
	"uni-ai-api/backend/internal/upstream"
)

type chatCompletionEnvelope struct {
	Model    string            `json:"model"`
	Messages []json.RawMessage `json:"messages"`
	Stream   bool              `json:"stream,omitempty"`
}

type callableModel struct {
	ID                     pgtype.UUID
	DefaultMaxOutputTokens int32
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
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
	var req chatCompletionEnvelope
	if err := decodeChatEnvelope(raw, &req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "invalid_request")
		return
	}
	if req.Model == "" {
		writeOpenAIError(w, http.StatusBadRequest, "Missing required parameter: model.", "invalid_request_error", "missing_required_parameter")
		return
	}
	if len(req.Messages) == 0 {
		writeOpenAIError(w, http.StatusBadRequest, "Missing required parameter: messages.", "invalid_request_error", "missing_required_parameter")
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

	// Resolve model - only need tenant grant, no user grant required
	model, err := s.resolveCallableModel(r, auth, req.Model)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeOpenAIError(w, http.StatusNotFound, "The model does not exist or you do not have access to it.", "invalid_request_error", "model_not_found")
			return
		}
		s.logger.Error("resolve model failed", "error", err, "request_id", requestIDFromContext(r.Context()))
		writeOpenAIError(w, http.StatusInternalServerError, "Failed to resolve model.", "server_error", "server_error")
		return
	}
	conversationID := chatConversationID(r, raw)

	// List routes instead of deployments
	routes, err := s.queries.ListRoutesForModel(r.Context(), model.ID)
	if err != nil {
		s.logger.Error("list routes failed", "error", err, "request_id", requestIDFromContext(r.Context()))
		writeOpenAIError(w, http.StatusInternalServerError, "Failed to resolve model route.", "server_error", "server_error")
		return
	}
	if len(routes) == 0 {
		writeOpenAIError(w, http.StatusServiceUnavailable, "No available route for the requested model.", "server_error", "model_unavailable")
		return
	}
	if req.Stream {
		routes = filterStreamingRoutes(routes)
		if len(routes) == 0 {
			writeOpenAIError(w, http.StatusServiceUnavailable, "No streaming route for the requested model.", "server_error", "model_unavailable")
			return
		}
	}

	route, ok := s.chooseRoute(r.Context(), routes, conversationStickyKey(auth, req.Model, conversationID))
	if !ok {
		writeOpenAIError(w, http.StatusServiceUnavailable, "No available route for the requested model.", "server_error", "model_unavailable")
		return
	}
	runtimeLease, ok := s.acquireRuntimeLimits(w, r, auth, req.Model, "chat", estimateChatRateTokens(raw, model.DefaultMaxOutputTokens), route)
	if !ok {
		return
	}
	defer s.releaseRuntimeLimits(r.Context(), runtimeLease)

	reservation, ok := s.reserveEstimatedChatQuota(w, r, auth, model, raw)
	if !ok {
		return
	}
	defer s.releaseAPIKeyQuotaReservation(r.Context(), reservation)

	if route.UpstreamProtocol != upstream.ProtocolOpenAIChatCompletions {
		s.recordChatUsage(r.Context(), usageLogInput{
			Auth:           auth,
			RequestID:      requestIDFromContext(r.Context()),
			TraceID:        requestTraceID(r),
			ExternalUserID: externalUserID(raw),
			ConversationID: conversationID,
			ModelCode:      req.Model,
			ModelID:        model.ID,
			Route:          &route,
			Stream:         req.Stream,
			HTTPStatus:     http.StatusBadGateway,
			Latency:        time.Since(start),
			RequestStatus:  "failed",
			ErrorCode:      "unsupported_upstream_protocol",
			ErrorMessage:   "Selected route protocol is not supported for chat completions.",
		})
		writeOpenAIError(w, http.StatusBadGateway, "Selected route protocol is not supported for chat completions.", "server_error", "unsupported_upstream_protocol")
		return
	}

	providerKey, err := secret.DecryptProviderKey(s.security.ProviderKeyMaster, route.ApiKeyCiphertext)
	if err != nil {
		s.logger.Error("decrypt provider key failed", "error", err, "request_id", requestIDFromContext(r.Context()))
		s.recordChatUsage(r.Context(), usageLogInput{
			Auth:           auth,
			RequestID:      requestIDFromContext(r.Context()),
			TraceID:        requestTraceID(r),
			ExternalUserID: externalUserID(raw),
			ConversationID: conversationID,
			ModelCode:      req.Model,
			ModelID:        model.ID,
			Route:          &route,
			Stream:         req.Stream,
			HTTPStatus:     http.StatusInternalServerError,
			Latency:        time.Since(start),
			RequestStatus:  "failed",
			ErrorCode:      "provider_credential_error",
			ErrorMessage:   "Provider credential is not configured correctly.",
		})
		writeOpenAIError(w, http.StatusInternalServerError, "Provider credential is not configured correctly.", "server_error", "provider_credential_error")
		return
	}

	price, settlementReservation, ok := s.freezeEstimatedChatSettlement(w, r, auth, model, raw)
	if !ok {
		return
	}

	if req.Stream {
		s.forwardStreamingChat(w, r, streamChatInput{
			Start:          start,
			Auth:           auth,
			Raw:            raw,
			Envelope:       req,
			Model:          model,
			Route:          route,
			ProviderKey:    providerKey,
			Reservation:    settlementReservation,
			Price:          price,
			ConversationID: conversationID,
		})
		return
	}

	resp, err := upstream.ForwardOpenAIChatCompletions(r.Context(), s.httpClient, upstream.OpenAIChatRequest{
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
		s.logger.Error("forward chat completion failed", "error", err, "request_id", requestIDFromContext(r.Context()))
		s.markUpstreamDeploymentCooldown(r.Context(), route.UpstreamDeploymentID, "upstream_request_failed")
		billingStatus := s.cancelChatSettlement(r.Context(), settlementReservation)
		s.recordChatUsage(r.Context(), usageLogInput{
			Auth:             auth,
			RequestID:        requestIDFromContext(r.Context()),
			TraceID:          requestTraceID(r),
			ExternalUserID:   externalUserID(raw),
			ConversationID:   conversationID,
			ModelCode:        req.Model,
			ModelID:          model.ID,
			Route:            &route,
			Stream:           req.Stream,
			HTTPStatus:       http.StatusBadGateway,
			Latency:          time.Since(start),
			RequestStatus:    "failed",
			ErrorCode:        "upstream_error",
			ErrorMessage:     err.Error(),
			URMTransactionID: settlementTransactionID(settlementReservation),
			BillingStatus:    billingStatus,
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
		if shouldCooldownUpstreamStatus(resp.StatusCode) {
			s.markUpstreamDeploymentCooldown(r.Context(), route.UpstreamDeploymentID, "upstream_status_"+http.StatusText(resp.StatusCode))
		}
	}
	usage := ensureUsageTotal(parseOpenAIUsage(resp))
	usageEstimated := false
	usageSource := "upstream"
	if requestStatus == "success" && !usageHasTokens(usage) {
		usage = estimateNonStreamChatUsage(raw, resp)
		usageEstimated = usageHasTokens(usage)
		if usageEstimated {
			usageSource = "estimated_length"
		}
	}
	costs := s.calculateChatCosts(r.Context(), usageLogInput{
		Auth: auth, ModelID: model.ID, Route: &route, RequestStatus: requestStatus, Usage: usage,
	})
	billingStatus := "not_billed"
	if requestStatus == "success" {
		billingStatus = s.confirmChatSettlement(r.Context(), settlementReservation, costs)
	} else {
		billingStatus = s.cancelChatSettlement(r.Context(), settlementReservation)
	}
	s.recordChatUsage(r.Context(), usageLogInput{
		Auth:             auth,
		RequestID:        requestIDFromContext(r.Context()),
		TraceID:          requestTraceID(r),
		ExternalUserID:   externalUserID(raw),
		ConversationID:   conversationID,
		ModelCode:        req.Model,
		ModelID:          model.ID,
		Route:            &route,
		Stream:           req.Stream,
		HTTPStatus:       resp.StatusCode,
		UpstreamStatus:   resp.StatusCode,
		Latency:          time.Since(start),
		RequestStatus:    requestStatus,
		ErrorCode:        errorCode,
		ErrorMessage:     errorMessage,
		Usage:            usage,
		UsageEstimated:   usageEstimated,
		UsageSource:      usageSource,
		Costs:            &costs,
		URMTransactionID: settlementTransactionID(settlementReservation),
		BillingStatus:    billingStatus,
	})
	copyUpstreamResponse(w, resp)
}

func decodeChatEnvelope(raw map[string]json.RawMessage, target *chatCompletionEnvelope) error {
	if raw == nil {
		return errors.New("Invalid request body.")
	}
	if value, ok := raw["model"]; ok {
		if err := json.Unmarshal(value, &target.Model); err != nil {
			return errors.New("Invalid parameter: model.")
		}
	}
	if value, ok := raw["messages"]; ok {
		if err := json.Unmarshal(value, &target.Messages); err != nil {
			return errors.New("Invalid parameter: messages.")
		}
	}
	if value, ok := raw["stream"]; ok {
		if err := json.Unmarshal(value, &target.Stream); err != nil {
			return errors.New("Invalid parameter: stream.")
		}
	}
	return nil
}

type streamChatInput struct {
	Start          time.Time
	Auth           RuntimeAuth
	Raw            map[string]json.RawMessage
	Envelope       chatCompletionEnvelope
	Model          callableModel
	Route          dbgen.ListRoutesForModelRow
	ProviderKey    string
	Reservation    *settlementReservation
	Price          *modelPrice
	ConversationID string
}

func (s *Server) forwardStreamingChat(w http.ResponseWriter, r *http.Request, input streamChatInput) {
	resp, err := upstream.ForwardOpenAIChatCompletionsStream(r.Context(), s.httpClient, upstream.OpenAIChatRequest{
		BaseURL:            input.Route.BaseUrl,
		RequestPath:        optionalText(input.Route.RequestPath),
		APIKey:             input.ProviderKey,
		UpstreamModel:      input.Route.UpstreamModel,
		ExtraHeaders:       input.Route.ExtraHeaders,
		UpstreamParameters: input.Route.UpstreamParameters,
		Timeout:            time.Duration(input.Route.TimeoutMs) * time.Millisecond,
		Body:               input.Raw,
	})
	if err != nil {
		s.logger.Error("forward streaming chat completion failed", "error", err, "request_id", requestIDFromContext(r.Context()))
		s.markUpstreamDeploymentCooldown(r.Context(), input.Route.UpstreamDeploymentID, "upstream_request_failed")
		billingStatus := s.cancelChatSettlement(r.Context(), input.Reservation)
		s.recordChatUsage(r.Context(), usageLogInput{
			Auth:             input.Auth,
			RequestID:        requestIDFromContext(r.Context()),
			TraceID:          requestTraceID(r),
			ExternalUserID:   externalUserID(input.Raw),
			ConversationID:   input.ConversationID,
			ModelCode:        input.Envelope.Model,
			ModelID:          input.Model.ID,
			Route:            &input.Route,
			Stream:           true,
			HTTPStatus:       http.StatusBadGateway,
			Latency:          time.Since(input.Start),
			RequestStatus:    "failed",
			ErrorCode:        "upstream_error",
			ErrorMessage:     err.Error(),
			URMTransactionID: settlementTransactionID(input.Reservation),
			BillingStatus:    billingStatus,
		})
		writeOpenAIError(w, http.StatusBadGateway, "Failed to call upstream provider.", "server_error", "upstream_error")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		if shouldCooldownUpstreamStatus(resp.StatusCode) {
			s.markUpstreamDeploymentCooldown(r.Context(), input.Route.UpstreamDeploymentID, "upstream_status_"+http.StatusText(resp.StatusCode))
		}
		billingStatus := s.cancelChatSettlement(r.Context(), input.Reservation)
		s.recordChatUsage(r.Context(), usageLogInput{
			Auth:             input.Auth,
			RequestID:        requestIDFromContext(r.Context()),
			TraceID:          requestTraceID(r),
			ExternalUserID:   externalUserID(input.Raw),
			ConversationID:   input.ConversationID,
			ModelCode:        input.Envelope.Model,
			ModelID:          input.Model.ID,
			Route:            &input.Route,
			Stream:           true,
			HTTPStatus:       resp.StatusCode,
			UpstreamStatus:   resp.StatusCode,
			Latency:          time.Since(input.Start),
			RequestStatus:    "failed",
			ErrorCode:        "upstream_error",
			ErrorMessage:     string(body),
			URMTransactionID: settlementTransactionID(input.Reservation),
			BillingStatus:    billingStatus,
		})
		copyUpstreamResponse(w, &upstream.Response{StatusCode: resp.StatusCode, Header: resp.Header, Body: body})
		return
	}

	for key, values := range resp.Header {
		if isHopByHopHeader(key) {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(resp.StatusCode)

	flusher, _ := w.(http.Flusher)
	parser := newSSEUsageParser()
	buffer := make([]byte, 32*1024)
	firstTokenLatency := time.Duration(0)
	status := "success"
	errorCode := ""
	errorMessage := ""
	for {
		n, readErr := resp.Body.Read(buffer)
		if n > 0 {
			chunk := buffer[:n]
			if firstTokenLatency == 0 {
				firstTokenLatency = time.Since(input.Start)
			}
			parser.Write(chunk)
			if _, err := w.Write(chunk); err != nil {
				status = "failed"
				errorCode = "client_stream_closed"
				errorMessage = err.Error()
				break
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				status = "failed"
				errorCode = "upstream_stream_error"
				errorMessage = readErr.Error()
			}
			break
		}
	}
	parser.Close()

	usage := ensureUsageTotal(parser.Usage())
	usageEstimated := false
	usageSource := "upstream"
	if status == "success" && !usageHasTokens(usage) {
		usage = upstreamUsage{
			PromptTokens:     estimateJSONTokens(input.Raw["messages"]),
			CompletionTokens: estimateTextTokens(parser.CompletionText()),
		}
		usage = ensureUsageTotal(usage)
		usageEstimated = usageHasTokens(usage)
		if usageEstimated {
			usageSource = "estimated_length"
		}
	}
	costs := s.calculateChatCosts(r.Context(), usageLogInput{
		Auth: input.Auth, ModelID: input.Model.ID, Route: &input.Route, RequestStatus: status, Usage: usage,
	})
	billingStatus := "not_billed"
	if status == "success" {
		billingStatus = s.confirmChatSettlement(r.Context(), input.Reservation, costs)
	} else if usage.TotalTokens > 0 {
		billingStatus = s.confirmChatSettlement(r.Context(), input.Reservation, costs)
	} else {
		billingStatus = s.cancelChatSettlement(r.Context(), input.Reservation)
	}
	s.recordChatUsage(r.Context(), usageLogInput{
		Auth:              input.Auth,
		RequestID:         requestIDFromContext(r.Context()),
		TraceID:           requestTraceID(r),
		ExternalUserID:    externalUserID(input.Raw),
		ConversationID:    input.ConversationID,
		ModelCode:         input.Envelope.Model,
		ModelID:           input.Model.ID,
		Route:             &input.Route,
		Stream:            true,
		HTTPStatus:        resp.StatusCode,
		UpstreamStatus:    resp.StatusCode,
		Latency:           time.Since(input.Start),
		RequestStatus:     status,
		ErrorCode:         errorCode,
		ErrorMessage:      errorMessage,
		Usage:             usage,
		UsageEstimated:    usageEstimated,
		UsageSource:       usageSource,
		Costs:             &costs,
		URMTransactionID:  settlementTransactionID(input.Reservation),
		BillingStatus:     billingStatus,
		FirstTokenLatency: firstTokenLatency,
	})
}

type sseUsageParser struct {
	buffer     bytes.Buffer
	completion strings.Builder
	usage      upstreamUsage
}

func newSSEUsageParser() *sseUsageParser {
	return &sseUsageParser{}
}

func (p *sseUsageParser) Write(chunk []byte) {
	_, _ = p.buffer.Write(chunk)
	p.parse(false)
}

func (p *sseUsageParser) Close() {
	p.parse(true)
}

func (p *sseUsageParser) Usage() upstreamUsage {
	return p.usage
}

func (p *sseUsageParser) CompletionText() string {
	return p.completion.String()
}

func (p *sseUsageParser) parse(final bool) {
	data := p.buffer.Bytes()
	parseBytes := data
	remaining := []byte(nil)
	if !final {
		lastNewline := bytes.LastIndexByte(data, '\n')
		if lastNewline < 0 {
			return
		}
		parseBytes = data[:lastNewline+1]
		remaining = append([]byte(nil), data[lastNewline+1:]...)
	}
	scanner := bufio.NewScanner(bytes.NewReader(parseBytes))
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		p.parseLine(line)
	}
	p.buffer.Reset()
	if !final && len(remaining) > 0 {
		_, _ = p.buffer.Write(remaining)
	}
}

func (p *sseUsageParser) parseLine(line string) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "data:") {
		return
	}
	payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if payload == "" || payload == "[DONE]" {
		return
	}
	var body struct {
		Usage struct {
			PromptTokens     int32 `json:"prompt_tokens"`
			CompletionTokens int32 `json:"completion_tokens"`
			TotalTokens      int32 `json:"total_tokens"`
		} `json:"usage"`
		Choices []struct {
			Delta struct {
				Content any `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(payload), &body); err != nil {
		return
	}
	for _, choice := range body.Choices {
		if text := flattenText(choice.Delta.Content); text != "" {
			p.completion.WriteString(text)
		}
	}
	if body.Usage.PromptTokens == 0 && body.Usage.CompletionTokens == 0 && body.Usage.TotalTokens == 0 {
		return
	}
	p.usage = upstreamUsage{
		PromptTokens:     body.Usage.PromptTokens,
		CompletionTokens: body.Usage.CompletionTokens,
		TotalTokens:      body.Usage.TotalTokens,
	}
}

// resolveCallableModel - only checks tenant grant, no user grant required
func (s *Server) resolveCallableModel(r *http.Request, auth RuntimeAuth, modelCode string) (callableModel, error) {
	return s.resolveCallableModelForCapability(r, auth, modelCode, "chat")
}

func (s *Server) resolveCallableModelForCapability(r *http.Request, auth RuntimeAuth, modelCode string, capabilityType string) (callableModel, error) {
	// Only tenant grant is required, user grant is no longer needed
	row, err := s.queries.GetTenantModel(r.Context(), dbgen.GetTenantModelParams{
		TenantID:       auth.APIKey.TenantID,
		ModelCode:      modelCode,
		CapabilityType: capabilityType,
	})
	return callableModel{ID: row.ID, DefaultMaxOutputTokens: row.DefaultMaxOutputTokens}, err
}

func (s *Server) reserveEstimatedChatQuota(w http.ResponseWriter, r *http.Request, auth RuntimeAuth, model callableModel, raw map[string]json.RawMessage) (*quotaReservation, bool) {
	price, err := s.getEffectiveModelPrice(r.Context(), auth, model.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeOpenAIError(w, http.StatusPaymentRequired, "Model pricing not configured.", "insufficient_quota", "model_price_not_configured")
			return nil, false
		}
		s.logger.Error("get model price for quota reservation failed", "error", err, "request_id", requestIDFromContext(r.Context()))
		writeOpenAIError(w, http.StatusInternalServerError, "Quota reservation failed.", "server_error", "quota_reservation_failed")
		return nil, false
	}

	estimated := estimateChatQuotaCost(raw, model.DefaultMaxOutputTokens, price)
	reservation, err := s.reserveAPIKeyQuota(r.Context(), auth, estimated)
	if err != nil {
		if errors.Is(err, errInsufficientQuotaReservation) {
			writeOpenAIError(w, http.StatusPaymentRequired, "You exceeded your current quota.", "insufficient_quota", "insufficient_quota")
			return nil, false
		}
		s.logger.Error("reserve api key quota failed", "error", err, "request_id", requestIDFromContext(r.Context()))
		writeOpenAIError(w, http.StatusInternalServerError, "Quota reservation failed.", "server_error", "quota_reservation_failed")
		return nil, false
	}
	return reservation, true
}

func (s *Server) freezeEstimatedChatSettlement(w http.ResponseWriter, r *http.Request, auth RuntimeAuth, model callableModel, raw map[string]json.RawMessage) (*modelPrice, *settlementReservation, bool) {
	price, err := s.getEffectiveModelPrice(r.Context(), auth, model.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeOpenAIError(w, http.StatusPaymentRequired, "Model pricing not configured.", "insufficient_quota", "model_price_not_configured")
			return nil, nil, false
		}
		s.logger.Error("get model price for settlement failed", "error", err, "request_id", requestIDFromContext(r.Context()))
		writeOpenAIError(w, http.StatusInternalServerError, "Settlement reservation failed.", "server_error", "settlement_reservation_failed")
		return nil, nil, false
	}

	estimated := estimatedSettlementCosts(raw, model, price, auth)
	reservation, err := s.freezeChatSettlement(r.Context(), auth, requestIDFromContext(r.Context()), estimated.PlatformCost, estimated.UserCost)
	if err != nil {
		if isURMInsufficientBalance(err) {
			s.recordChatUsage(r.Context(), usageLogInput{
				Auth:           auth,
				RequestID:      requestIDFromContext(r.Context()),
				TraceID:        requestTraceID(r),
				ExternalUserID: externalUserID(raw),
				ModelCode:      rawString(raw["model"]),
				ModelID:        model.ID,
				HTTPStatus:     http.StatusPaymentRequired,
				Latency:        0,
				RequestStatus:  "rejected",
				ErrorCode:      "insufficient_quota",
				ErrorMessage:   "URM balance is insufficient.",
				BillingStatus:  "rejected",
			})
			writeOpenAIError(w, http.StatusPaymentRequired, "Insufficient quota.", "insufficient_quota", "insufficient_quota")
			return &price, nil, false
		}
		s.logger.Error("freeze urm settlement failed", "error", err, "request_id", requestIDFromContext(r.Context()))
		writeOpenAIError(w, http.StatusServiceUnavailable, "Settlement reservation failed.", "server_error", "settlement_reservation_failed")
		return &price, nil, false
	}
	return &price, reservation, true
}

func chatConversationID(r *http.Request, raw map[string]json.RawMessage) string {
	if value := strings.TrimSpace(r.Header.Get("X-Conversation-Id")); value != "" {
		return value
	}
	if raw == nil {
		return ""
	}
	metadataRaw, ok := raw["metadata"]
	if !ok {
		return ""
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(metadataRaw, &metadata); err != nil {
		return ""
	}
	return rawString(metadata["conversation_id"])
}

func rawString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return value
}

func settlementTransactionID(reservation *settlementReservation) string {
	if reservation == nil {
		return ""
	}
	return reservation.transactionID
}

func apiKeyAllowsModel(raw []byte, model string) bool {
	allowed, err := allowedModelSet(raw)
	if err != nil {
		return false
	}
	if allowed == nil {
		return true
	}
	_, ok := allowed[model]
	return ok
}

func quotaExhausted(limit pgtype.Int8, used int64, reserved int64) bool {
	if !limit.Valid {
		return false
	}
	return used+reserved >= limit.Int64
}

func optionalText(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func copyUpstreamResponse(w http.ResponseWriter, resp *upstream.Response) {
	for key, values := range resp.Header {
		if isHopByHopHeader(key) {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(resp.Body)
}

func isHopByHopHeader(header string) bool {
	switch http.CanonicalHeaderKey(header) {
	case "Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade":
		return true
	default:
		return false
	}
}
