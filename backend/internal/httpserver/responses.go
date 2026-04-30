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

	dbgen "uni-ai-api/backend/internal/db/gen"
	"uni-ai-api/backend/internal/secret"
	"uni-ai-api/backend/internal/upstream"
)

type responsesEnvelope struct {
	Model  string          `json:"model"`
	Input  json.RawMessage `json:"input"`
	Stream bool            `json:"stream,omitempty"`
}

func (s *Server) handleResponses(w http.ResponseWriter, r *http.Request) {
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
	var req responsesEnvelope
	if err := decodeResponsesEnvelope(raw, &req); err != nil {
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
	s.logGatewayRequestReceived(r, "chat", req.Model, req.Stream)

	model, err := s.resolveCallableModelForCapability(r, auth, req.Model, "chat")
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeOpenAIError(w, http.StatusNotFound, "The model does not exist or you do not have access to it.", "invalid_request_error", "model_not_found")
			return
		}
		s.logger.Error("resolve responses model failed", "error", err, "request_id", requestIDFromContext(r.Context()))
		writeOpenAIError(w, http.StatusInternalServerError, "Failed to resolve model.", "server_error", "server_error")
		return
	}

	routes, err := s.queries.ListRoutesForModel(r.Context(), model.ID)
	if err != nil {
		s.logger.Error("list responses routes failed", "error", err, "request_id", requestIDFromContext(r.Context()))
		writeOpenAIError(w, http.StatusInternalServerError, "Failed to resolve model route.", "server_error", "server_error")
		return
	}
	if req.Stream {
		routes = filterStreamingRoutes(routes)
	}
	route, ok := s.chooseRoute(r.Context(), routes, "")
	if !ok {
		writeOpenAIError(w, http.StatusServiceUnavailable, "No available route for the requested model.", "server_error", "model_unavailable")
		return
	}
	s.logGatewayRouteSelected(r, "chat", req.Model, route)

	estimate := estimateResponsesUsage(raw, model.DefaultMaxOutputTokens)
	runtimeLease, ok := s.acquireRuntimeLimits(w, r, auth, req.Model, "chat", estimate.TotalTokens, route)
	if !ok {
		return
	}
	defer s.releaseRuntimeLimits(r.Context(), runtimeLease)

	price, quotaReservation, settlementReservation, ok := s.reserveTokenBilling(w, r, auth, model, estimate)
	if !ok {
		return
	}
	defer s.releaseAPIKeyQuotaReservation(r.Context(), quotaReservation)

	if route.UpstreamProtocol != upstream.ProtocolOpenAIResponses {
		s.recordChatUsage(r.Context(), usageLogInput{
			Auth:             auth,
			RequestID:        requestIDFromContext(r.Context()),
			TraceID:          requestTraceID(r),
			ExternalUserID:   externalUserID(raw),
			ModelCode:        req.Model,
			CapabilityType:   "chat",
			ModelID:          model.ID,
			Route:            &route,
			Stream:           req.Stream,
			HTTPStatus:       http.StatusBadGateway,
			Latency:          time.Since(start),
			RequestStatus:    "failed",
			ErrorCode:        "unsupported_upstream_protocol",
			ErrorMessage:     "Selected route protocol is not supported for responses.",
			BillingStatus:    s.cancelChatSettlement(r.Context(), settlementReservation),
			URMTransactionID: settlementTransactionID(settlementReservation),
		})
		writeOpenAIError(w, http.StatusBadGateway, "Selected route protocol is not supported for responses.", "server_error", "unsupported_upstream_protocol")
		return
	}

	providerKey, err := secret.DecryptProviderKey(s.security.ProviderKeyMaster, route.ApiKeyCiphertext)
	if err != nil {
		s.recordChatUsage(r.Context(), usageLogInput{
			Auth:             auth,
			RequestID:        requestIDFromContext(r.Context()),
			TraceID:          requestTraceID(r),
			ExternalUserID:   externalUserID(raw),
			ModelCode:        req.Model,
			CapabilityType:   "chat",
			ModelID:          model.ID,
			Route:            &route,
			Stream:           req.Stream,
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

	input := responsesForwardInput{start: start, auth: auth, raw: raw, req: req, model: model, route: route, providerKey: providerKey, price: price, quota: quotaReservation, settlement: settlementReservation}
	if req.Stream {
		s.logGatewayUpstreamStarted(r, "chat", req.Model, route)
		s.forwardStreamingResponses(w, r, input)
		return
	}
	s.forwardNonStreamingResponses(w, r, input)
}

type responsesForwardInput struct {
	start       time.Time
	auth        RuntimeAuth
	raw         map[string]json.RawMessage
	req         responsesEnvelope
	model       callableModel
	route       dbgen.ListRoutesForModelRow
	providerKey string
	price       *modelPrice
	quota       *quotaReservation
	settlement  *settlementReservation
}

func (s *Server) forwardNonStreamingResponses(w http.ResponseWriter, r *http.Request, input responsesForwardInput) {
	s.logGatewayUpstreamStarted(r, "chat", input.req.Model, input.route)
	resp, err := upstream.ForwardOpenAIResponses(r.Context(), s.httpClient, upstream.OpenAIResponsesRequest{
		BaseURL:            input.route.BaseUrl,
		RequestPath:        optionalText(input.route.RequestPath),
		APIKey:             input.providerKey,
		UpstreamModel:      input.route.UpstreamModel,
		ExtraHeaders:       input.route.ExtraHeaders,
		UpstreamParameters: input.route.UpstreamParameters,
		Timeout:            time.Duration(input.route.TimeoutMs) * time.Millisecond,
		Body:               input.raw,
	})
	if err != nil {
		s.markUpstreamDeploymentCooldown(r.Context(), input.route.UpstreamDeploymentID, "upstream_request_failed")
		billingStatus := s.cancelChatSettlement(r.Context(), input.settlement)
		s.recordTokenUsage(r, input, http.StatusBadGateway, 0, "failed", "upstream_error", err.Error(), upstreamUsage{}, false, "upstream", billingStatus)
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
		s.logGatewayUpstreamFailed(r, "chat", input.req.Model, input.route, resp.StatusCode, resp.Body)
		if shouldCooldownUpstreamStatus(resp.StatusCode) {
			s.markUpstreamDeploymentCooldown(r.Context(), input.route.UpstreamDeploymentID, "upstream_status_"+http.StatusText(resp.StatusCode))
		}
	}
	usage := ensureUsageTotal(parseOpenAIResponsesUsage(resp))
	estimated := false
	source := "upstream"
	if requestStatus == "success" && !usageHasTokens(usage) {
		usage = estimateResponsesUsageFromBody(input.raw, resp)
		estimated = usageHasTokens(usage)
		if estimated {
			source = "estimated_length"
		}
	}
	costs := s.calculateChatCosts(r.Context(), usageLogInput{
		Auth: input.auth, ModelID: input.model.ID, Route: &input.route, CapabilityType: "chat", RequestStatus: requestStatus, Usage: usage,
	})
	billingStatus := "not_billed"
	if requestStatus == "success" {
		billingStatus = s.confirmChatSettlement(r.Context(), input.settlement, costs)
	} else {
		billingStatus = s.cancelChatSettlement(r.Context(), input.settlement)
	}
	s.recordTokenUsageWithCosts(r, input, resp.StatusCode, resp.StatusCode, requestStatus, errorCode, errorMessage, usage, estimated, source, billingStatus, &costs, 0)
	copyUpstreamResponse(w, resp)
}

func (s *Server) forwardStreamingResponses(w http.ResponseWriter, r *http.Request, input responsesForwardInput) {
	resp, err := upstream.ForwardOpenAIResponsesStream(r.Context(), s.httpClient, upstream.OpenAIResponsesRequest{
		BaseURL:            input.route.BaseUrl,
		RequestPath:        optionalText(input.route.RequestPath),
		APIKey:             input.providerKey,
		UpstreamModel:      input.route.UpstreamModel,
		ExtraHeaders:       input.route.ExtraHeaders,
		UpstreamParameters: input.route.UpstreamParameters,
		Timeout:            time.Duration(input.route.TimeoutMs) * time.Millisecond,
		Body:               input.raw,
	})
	if err != nil {
		s.markUpstreamDeploymentCooldown(r.Context(), input.route.UpstreamDeploymentID, "upstream_request_failed")
		billingStatus := s.cancelChatSettlement(r.Context(), input.settlement)
		s.recordTokenUsage(r, input, http.StatusBadGateway, 0, "failed", "upstream_error", err.Error(), upstreamUsage{}, false, "upstream", billingStatus)
		writeOpenAIError(w, http.StatusBadGateway, "Failed to call upstream provider.", "server_error", "upstream_error")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		s.logGatewayUpstreamFailed(r, "chat", input.req.Model, input.route, resp.StatusCode, body)
		if shouldCooldownUpstreamStatus(resp.StatusCode) {
			s.markUpstreamDeploymentCooldown(r.Context(), input.route.UpstreamDeploymentID, "upstream_status_"+http.StatusText(resp.StatusCode))
		}
		billingStatus := s.cancelChatSettlement(r.Context(), input.settlement)
		s.recordTokenUsage(r, input, resp.StatusCode, resp.StatusCode, "failed", "upstream_error", string(body), upstreamUsage{}, false, "upstream", billingStatus)
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
	parser := newResponsesSSEUsageParser()
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
				firstTokenLatency = time.Since(input.start)
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
	estimated := false
	source := "upstream"
	if status == "success" && !usageHasTokens(usage) {
		usage = ensureUsageTotal(upstreamUsage{
			PromptTokens:     estimateResponsesInputTokens(input.raw),
			CompletionTokens: estimateTextTokens(parser.OutputText()),
		})
		estimated = usageHasTokens(usage)
		if estimated {
			source = "estimated_length"
		}
	}
	costs := s.calculateChatCosts(r.Context(), usageLogInput{
		Auth: input.auth, ModelID: input.model.ID, Route: &input.route, CapabilityType: "chat", RequestStatus: status, Usage: usage,
	})
	billingStatus := "not_billed"
	if status == "success" || usage.TotalTokens > 0 {
		billingStatus = s.confirmChatSettlement(r.Context(), input.settlement, costs)
	} else {
		billingStatus = s.cancelChatSettlement(r.Context(), input.settlement)
	}
	s.recordTokenUsageWithCosts(r, input, resp.StatusCode, resp.StatusCode, status, errorCode, errorMessage, usage, estimated, source, billingStatus, &costs, firstTokenLatency)
}

func decodeResponsesEnvelope(raw map[string]json.RawMessage, target *responsesEnvelope) error {
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
	if value, ok := raw["stream"]; ok {
		if err := json.Unmarshal(value, &target.Stream); err != nil {
			return errors.New("Invalid parameter: stream.")
		}
	}
	return nil
}

func parseOpenAIResponsesUsage(resp *upstream.Response) upstreamUsage {
	if resp == nil || len(resp.Body) == 0 || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return upstreamUsage{}
	}
	var body struct {
		Usage struct {
			InputTokens  int32 `json:"input_tokens"`
			OutputTokens int32 `json:"output_tokens"`
			TotalTokens  int32 `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return upstreamUsage{}
	}
	return upstreamUsage{PromptTokens: body.Usage.InputTokens, CompletionTokens: body.Usage.OutputTokens, TotalTokens: body.Usage.TotalTokens}
}

func estimateResponsesUsage(raw map[string]json.RawMessage, defaultMaxOutputTokens int32) upstreamUsage {
	return ensureUsageTotal(upstreamUsage{
		PromptTokens:     estimateResponsesInputTokens(raw),
		CompletionTokens: requestedOutputTokens(raw, defaultMaxOutputTokens),
	})
}

func estimateResponsesUsageFromBody(raw map[string]json.RawMessage, resp *upstream.Response) upstreamUsage {
	usage := upstreamUsage{PromptTokens: estimateResponsesInputTokens(raw)}
	if resp != nil && len(resp.Body) > 0 && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var body struct {
			OutputText string `json:"output_text"`
			Output     any    `json:"output"`
		}
		if err := json.Unmarshal(resp.Body, &body); err == nil {
			text := body.OutputText
			if text == "" {
				text = flattenText(body.Output)
			}
			usage.CompletionTokens = estimateTextTokens(text)
		}
	}
	return ensureUsageTotal(usage)
}

func estimateResponsesInputTokens(raw map[string]json.RawMessage) int32 {
	if raw == nil {
		return 0
	}
	if input, ok := raw["input"]; ok {
		return estimateJSONTokens(input)
	}
	return 0
}

func (s *Server) reserveTokenBilling(w http.ResponseWriter, r *http.Request, auth RuntimeAuth, model callableModel, usage upstreamUsage) (*modelPrice, *quotaReservation, *settlementReservation, bool) {
	price, err := s.getEffectiveModelPrice(r.Context(), auth, model.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeOpenAIError(w, http.StatusPaymentRequired, "Model pricing not configured.", "insufficient_quota", "model_price_not_configured")
			return nil, nil, nil, false
		}
		s.logger.Error("get model price for token billing failed", "error", err, "request_id", requestIDFromContext(r.Context()))
		writeOpenAIError(w, http.StatusInternalServerError, "Quota reservation failed.", "server_error", "quota_reservation_failed")
		return nil, nil, nil, false
	}
	quotaCost := tokenCost(usage.PromptTokens, price.InputPricePer1m) + tokenCost(usage.CompletionTokens, price.OutputPricePer1m)
	quotaReservation, err := s.reserveAPIKeyQuota(r.Context(), auth, quotaCost)
	if err != nil {
		if errors.Is(err, errInsufficientQuotaReservation) {
			writeOpenAIError(w, http.StatusPaymentRequired, "You exceeded your current quota.", "insufficient_quota", "insufficient_quota")
			return &price, nil, nil, false
		}
		s.logger.Error("reserve api key quota failed", "error", err, "request_id", requestIDFromContext(r.Context()))
		writeOpenAIError(w, http.StatusInternalServerError, "Quota reservation failed.", "server_error", "quota_reservation_failed")
		return &price, nil, nil, false
	}
	// Tenant cost (what tenant pays to platform)
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
		s.logger.Error("freeze token settlement failed", "error", err, "request_id", requestIDFromContext(r.Context()))
		writeOpenAIError(w, http.StatusServiceUnavailable, "Settlement reservation failed.", "server_error", "settlement_reservation_failed")
		return &price, nil, nil, false
	}
	return &price, quotaReservation, settlementReservation, true
}

func (s *Server) recordTokenUsage(r *http.Request, input responsesForwardInput, httpStatus int, upstreamStatus int, status string, errorCode string, errorMessage string, usage upstreamUsage, estimated bool, source string, billingStatus string) {
	s.recordTokenUsageWithCosts(r, input, httpStatus, upstreamStatus, status, errorCode, errorMessage, usage, estimated, source, billingStatus, nil, 0)
}

func (s *Server) recordTokenUsageWithCosts(r *http.Request, input responsesForwardInput, httpStatus int, upstreamStatus int, status string, errorCode string, errorMessage string, usage upstreamUsage, estimated bool, source string, billingStatus string, costs *chatCosts, firstTokenLatency time.Duration) {
	s.recordChatUsage(r.Context(), usageLogInput{
		Auth:              input.auth,
		RequestID:         requestIDFromContext(r.Context()),
		TraceID:           requestTraceID(r),
		ExternalUserID:    externalUserID(input.raw),
		ModelCode:         input.req.Model,
		CapabilityType:    "chat",
		ModelID:           input.model.ID,
		Route:             &input.route,
		Stream:            input.req.Stream,
		HTTPStatus:        httpStatus,
		UpstreamStatus:    upstreamStatus,
		Latency:           time.Since(input.start),
		RequestStatus:     status,
		ErrorCode:         errorCode,
		ErrorMessage:      errorMessage,
		Usage:             usage,
		BillableUnitType:  "token",
		BillableUnits:     int64(usage.TotalTokens),
		UsageEstimated:    estimated,
		UsageSource:       source,
		Costs:             costs,
		URMTransactionID:  settlementTransactionID(input.settlement),
		BillingStatus:     billingStatus,
		FirstTokenLatency: firstTokenLatency,
	})
}

type responsesSSEUsageParser struct {
	buffer bytes.Buffer
	output strings.Builder
	usage  upstreamUsage
}

func newResponsesSSEUsageParser() *responsesSSEUsageParser {
	return &responsesSSEUsageParser{}
}

func (p *responsesSSEUsageParser) Write(chunk []byte) {
	_, _ = p.buffer.Write(chunk)
	p.parse(false)
}

func (p *responsesSSEUsageParser) Close() {
	p.parse(true)
}

func (p *responsesSSEUsageParser) Usage() upstreamUsage {
	return p.usage
}

func (p *responsesSSEUsageParser) OutputText() string {
	return p.output.String()
}

func (p *responsesSSEUsageParser) parse(final bool) {
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
		p.parseLine(scanner.Text())
	}
	p.buffer.Reset()
	if !final && len(remaining) > 0 {
		_, _ = p.buffer.Write(remaining)
	}
}

func (p *responsesSSEUsageParser) parseLine(line string) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "data:") {
		return
	}
	payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if payload == "" || payload == "[DONE]" {
		return
	}
	var event struct {
		Type  string `json:"type"`
		Delta string `json:"delta"`
		Usage struct {
			InputTokens  int32 `json:"input_tokens"`
			OutputTokens int32 `json:"output_tokens"`
			TotalTokens  int32 `json:"total_tokens"`
		} `json:"usage"`
		Response struct {
			Usage struct {
				InputTokens  int32 `json:"input_tokens"`
				OutputTokens int32 `json:"output_tokens"`
				TotalTokens  int32 `json:"total_tokens"`
			} `json:"usage"`
		} `json:"response"`
	}
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return
	}
	if event.Type == "response.output_text.delta" && event.Delta != "" {
		p.output.WriteString(event.Delta)
	}
	usage := upstreamUsage{PromptTokens: event.Usage.InputTokens, CompletionTokens: event.Usage.OutputTokens, TotalTokens: event.Usage.TotalTokens}
	if !usageHasTokens(usage) {
		usage = upstreamUsage{PromptTokens: event.Response.Usage.InputTokens, CompletionTokens: event.Response.Usage.OutputTokens, TotalTokens: event.Response.Usage.TotalTokens}
	}
	if usageHasTokens(usage) {
		p.usage = usage
	}
}
