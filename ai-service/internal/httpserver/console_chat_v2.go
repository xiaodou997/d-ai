package httpserver

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"

	"xiaodou/uni-ai-api/internal/domain"
)

var consoleChatProtocolOrder = []domain.UpstreamProtocol{
	domain.ProtocolOpenAIResponses,
	domain.ProtocolOpenAIChat,
	domain.ProtocolAnthropicMessages,
	domain.ProtocolGeminiGenerate,
}

type consoleChatModelV2DTO struct {
	ModelCode          string   `json:"model_code"`
	CapabilityType     string   `json:"capability_type"`
	DefaultProtocol    string   `json:"default_protocol"`
	AvailableProtocols []string `json:"available_protocols"`
	SupportsStream     bool     `json:"supports_stream"`
	ContextWindow      *int32   `json:"context_window,omitempty"`
	MaxOutputTokens    *int32   `json:"max_output_tokens,omitempty"`
	Status             string   `json:"status"`
}

type consoleChatSessionDTO struct {
	ID               string `json:"id"`
	Title            string `json:"title"`
	ModelCode        string `json:"model_code"`
	SelectedProtocol string `json:"selected_protocol"`
	SelectedRouteID  string `json:"selected_route_id"`
	Status           string `json:"status"`
	CreatedAt        int64  `json:"created_at"`
	UpdatedAt        int64  `json:"updated_at"`
}

type consoleChatMessageDTO struct {
	ID        string         `json:"id"`
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	Protocol  string         `json:"protocol,omitempty"`
	RouteID   string         `json:"route_id,omitempty"`
	Usage     map[string]any `json:"usage,omitempty"`
	Error     map[string]any `json:"error,omitempty"`
	CreatedAt int64          `json:"created_at"`
}

type consoleChatSessionDetailDTO struct {
	Session  consoleChatSessionDTO   `json:"session"`
	Messages []consoleChatMessageDTO `json:"messages"`
}

type createConsoleChatSessionRequest struct {
	ModelCode string `json:"model_code"`
	Title     string `json:"title"`
}

type consoleChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type consoleChatOptions struct {
	Temperature *float64 `json:"temperature"`
	MaxTokens   *int     `json:"max_tokens"`
}

type streamConsoleChatRequest struct {
	ModelCode      string               `json:"model"`
	ProtocolPolicy string               `json:"protocol_policy"`
	Protocol       string               `json:"protocol"`
	Messages       []consoleChatMessage `json:"messages"`
	Options        consoleChatOptions   `json:"options"`
}

type consoleChatModelRow struct {
	ID                     pgtype.UUID
	ModelCode              string
	CapabilityType         string
	ContextWindow          pgtype.Int4
	DefaultMaxOutputTokens int32
	MaxOutputTokens        pgtype.Int4
}

type captureResponseWriter struct {
	http.ResponseWriter
	buf bytes.Buffer
}

func (w *captureResponseWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	return w.ResponseWriter.Write(p)
}

func (w *captureResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Server) handleConsoleChatModelsV2(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.consoleRuntimeIdentity(w, r)
	if !ok {
		return
	}

	models, err := s.consoleGrantedChatModels(r, identity)
	if err != nil {
		s.logger.Error("console chat models v2: list grants failed", zap.Error(err))
		writeDBErr(w, err)
		return
	}

	out := make([]consoleChatModelV2DTO, 0, len(models))
	for _, model := range models {
		protocols, err := s.availableConsoleChatProtocols(r, model.ID)
		if err != nil {
			s.logger.Error("console chat models v2: route lookup failed", zap.Error(err))
			writeDBErr(w, err)
			return
		}
		if len(protocols) == 0 {
			continue
		}
		dto := consoleChatModelV2DTO{
			ModelCode:          model.ModelCode,
			CapabilityType:     model.CapabilityType,
			DefaultProtocol:    string(protocols[0]),
			AvailableProtocols: protocolStrings(protocols),
			SupportsStream:     true,
			Status:             "available",
		}
		if model.ContextWindow.Valid {
			v := model.ContextWindow.Int32
			dto.ContextWindow = &v
		}
		if model.MaxOutputTokens.Valid {
			v := model.MaxOutputTokens.Int32
			dto.MaxOutputTokens = &v
		} else {
			v := model.DefaultMaxOutputTokens
			dto.MaxOutputTokens = &v
		}
		out = append(out, dto)
	}
	writeOK(w, out)
}

func (s *Server) handleConsoleChatListSessionsV2(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.consoleRuntimeIdentity(w, r)
	if !ok {
		return
	}
	rows, err := s.postgres.Query(r.Context(), `
		SELECT id::text, title, model_code, COALESCE(selected_protocol, ''), COALESCE(selected_route_id::text, ''), status,
		       EXTRACT(EPOCH FROM created_at)::bigint, EXTRACT(EPOCH FROM updated_at)::bigint
		FROM ai_console_sessions
		WHERE tenant_id = $1
		  AND owner_type = $2
		  AND COALESCE(user_id, '') = $3
		  AND status <> 'deleted'
		ORDER BY updated_at DESC
		LIMIT 100`,
		identity.TenantID, string(identity.OwnerType), identity.UserID)
	if err != nil {
		writeDBErr(w, err)
		return
	}
	defer rows.Close()
	sessions := make([]consoleChatSessionDTO, 0)
	for rows.Next() {
		var item consoleChatSessionDTO
		if err := rows.Scan(&item.ID, &item.Title, &item.ModelCode, &item.SelectedProtocol, &item.SelectedRouteID, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
			writeDBErr(w, err)
			return
		}
		sessions = append(sessions, item)
	}
	if err := rows.Err(); err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, sessions)
}

func (s *Server) handleConsoleChatCreateSessionV2(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.consoleRuntimeIdentity(w, r)
	if !ok {
		return
	}
	var req createConsoleChatSessionRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	req.ModelCode = strings.TrimSpace(req.ModelCode)
	if req.ModelCode == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "model_code is required")
		return
	}
	if req.Title == "" {
		req.Title = "新对话"
	}
	row := s.postgres.QueryRow(r.Context(), `
		INSERT INTO ai_console_sessions (tenant_id, user_id, owner_type, title, model_code, status)
		VALUES ($1, NULLIF($2, ''), $3, $4, $5, 'active')
		RETURNING id::text, title, model_code, COALESCE(selected_protocol, ''), COALESCE(selected_route_id::text, ''), status,
		          EXTRACT(EPOCH FROM created_at)::bigint, EXTRACT(EPOCH FROM updated_at)::bigint`,
		identity.TenantID, identity.UserID, string(identity.OwnerType), req.Title, req.ModelCode)
	var dto consoleChatSessionDTO
	if err := row.Scan(&dto.ID, &dto.Title, &dto.ModelCode, &dto.SelectedProtocol, &dto.SelectedRouteID, &dto.Status, &dto.CreatedAt, &dto.UpdatedAt); err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, dto)
}

func (s *Server) handleConsoleChatGetSessionV2(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.consoleRuntimeIdentity(w, r)
	if !ok {
		return
	}
	sessionID := chi.URLParam(r, "sessionID")
	session, ok := s.loadConsoleChatSession(w, r, identity, sessionID)
	if !ok {
		return
	}
	msgs, err := s.listConsoleChatMessages(r, sessionID)
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, consoleChatSessionDetailDTO{Session: session, Messages: msgs})
}

func (s *Server) handleConsoleChatDeleteSessionV2(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.consoleRuntimeIdentity(w, r)
	if !ok {
		return
	}
	sessionID := chi.URLParam(r, "sessionID")
	tag, err := s.postgres.Exec(r.Context(), `
		UPDATE ai_console_sessions
		SET status = 'deleted', updated_at = now()
		WHERE id = $1 AND tenant_id = $2 AND owner_type = $3 AND COALESCE(user_id, '') = $4`,
		sessionID, identity.TenantID, string(identity.OwnerType), identity.UserID)
	if err != nil {
		writeDBErr(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, BizErrNotFound, "session not found")
		return
	}
	writeOK(w, nil)
}

func (s *Server) handleConsoleChatStreamV2(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.consoleRuntimeIdentity(w, r)
	if !ok {
		return
	}
	sessionID := chi.URLParam(r, "sessionID")
	if _, ok := s.loadConsoleChatSession(w, r, identity, sessionID); !ok {
		return
	}

	var req streamConsoleChatRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	req.ModelCode = strings.TrimSpace(req.ModelCode)
	if req.ModelCode == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "model is required")
		return
	}
	if len(req.Messages) == 0 {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "messages are required")
		return
	}

	model, err := s.consoleGrantedChatModelByCode(r, identity, req.ModelCode)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeErr(w, http.StatusForbidden, BizErrForbidden, "model is not authorized")
			return
		}
		writeDBErr(w, err)
		return
	}
	protocol, err := s.chooseConsoleChatProtocol(r, model.ID, req.ProtocolPolicy, req.Protocol)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, err.Error())
		return
	}

	body, clientPath, err := buildConsoleProtocolBody(protocol, req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, err.Error())
		return
	}

	userText := lastUserText(req.Messages)
	if userText != "" {
		_ = s.insertConsoleChatMessage(r, sessionID, "user", userText, "", "", nil, nil)
	}
	_, _ = s.postgres.Exec(r.Context(), `
		UPDATE ai_console_sessions
		SET model_code = $2,
		    selected_protocol = $3,
		    title = CASE WHEN title = '新对话' AND $4 <> '' THEN left($4, 80) ELSE title END,
		    updated_at = now()
		WHERE id = $1`,
		sessionID, req.ModelCode, string(protocol), userText)

	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "text/event-stream")

	capture := &captureResponseWriter{ResponseWriter: w}
	runtimeReq := s.serveRuntime(capture, r, domain.CapabilityChat, runtimeOverride{
		clientProtocol: protocol,
		clientPath:     clientPath,
	}, identity, false)

	routeID := ""
	if runtimeReq != nil && runtimeReq.Candidate != nil {
		routeID = runtimeReq.Candidate.RouteID
		_ = s.updateConsoleChatSessionRoute(r, sessionID, protocol, routeID)
	}
	assistantText := extractConsoleAssistantText(capture.buf.Bytes(), protocol)
	if assistantText != "" {
		_ = s.insertConsoleChatMessage(r, sessionID, "assistant", assistantText, string(protocol), routeID, nil, nil)
	}
}

func (s *Server) consoleGrantedChatModels(r *http.Request, identity *domain.RuntimeIdentity) ([]consoleChatModelRow, error) {
	switch identity.OwnerType {
	case domain.OwnerTenant:
		return s.queryConsoleChatModels(r, `
			SELECT m.id, m.model_code, m.capability_type, m.context_window, m.default_max_output_tokens, m.max_output_tokens
			FROM ai_tenant_model_grants g
			JOIN ai_models m ON m.id = g.model_id
			WHERE g.tenant_id = $1 AND g.status = 'active' AND m.status = 'active' AND m.capability_type = 'chat'
			ORDER BY m.model_code ASC`, identity.TenantID)
	case domain.OwnerUser:
		return s.queryConsoleChatModels(r, `
			SELECT m.id, m.model_code, m.capability_type, m.context_window, m.default_max_output_tokens, m.max_output_tokens
			FROM ai_user_model_grants ug
			JOIN ai_tenant_model_grants tg ON tg.model_id = ug.model_id AND tg.tenant_id = ug.tenant_id
			JOIN ai_models m ON m.id = ug.model_id
			WHERE ug.tenant_id = $1 AND ug.user_id = $2
			  AND ug.status <> 'disabled' AND tg.status = 'active' AND m.status = 'active' AND m.capability_type = 'chat'
			ORDER BY m.model_code ASC`, identity.TenantID, identity.UserID)
	default:
		return nil, fmt.Errorf("forbidden")
	}
}

func (s *Server) consoleGrantedChatModelByCode(r *http.Request, identity *domain.RuntimeIdentity, modelCode string) (consoleChatModelRow, error) {
	models, err := s.consoleGrantedChatModels(r, identity)
	if err != nil {
		return consoleChatModelRow{}, err
	}
	for _, model := range models {
		if model.ModelCode == modelCode {
			return model, nil
		}
	}
	return consoleChatModelRow{}, pgx.ErrNoRows
}

func (s *Server) queryConsoleChatModels(r *http.Request, query string, args ...any) ([]consoleChatModelRow, error) {
	rows, err := s.postgres.Query(r.Context(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]consoleChatModelRow, 0)
	for rows.Next() {
		var item consoleChatModelRow
		if err := rows.Scan(&item.ID, &item.ModelCode, &item.CapabilityType, &item.ContextWindow, &item.DefaultMaxOutputTokens, &item.MaxOutputTokens); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Server) availableConsoleChatProtocols(r *http.Request, modelID pgtype.UUID) ([]domain.UpstreamProtocol, error) {
	out := make([]domain.UpstreamProtocol, 0, len(consoleChatProtocolOrder))
	for _, protocol := range consoleChatProtocolOrder {
		ok, err := s.modelHasConsoleProtocolRoute(r, modelID, protocol)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, protocol)
		}
	}
	return out, nil
}

func (s *Server) modelHasConsoleProtocolRoute(r *http.Request, modelID pgtype.UUID, protocol domain.UpstreamProtocol) (bool, error) {
	routable, err := s.routeSelector.ModelsWithProtocolRoute(r.Context(), []pgtype.UUID{modelID}, protocol, true)
	if err != nil {
		return false, err
	}
	return routable[modelID], nil
}

func (s *Server) chooseConsoleChatProtocol(r *http.Request, modelID pgtype.UUID, policy string, requested string) (domain.UpstreamProtocol, error) {
	if policy == "" {
		policy = "auto"
	}
	if policy == "manual" {
		protocol := domain.UpstreamProtocol(requested)
		if !consoleChatProtocolAllowed(protocol) {
			return "", fmt.Errorf("unsupported protocol")
		}
		ok, err := s.modelHasConsoleProtocolRoute(r, modelID, protocol)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", fmt.Errorf("selected protocol is not available for this model")
		}
		return protocol, nil
	}
	protocols, err := s.availableConsoleChatProtocols(r, modelID)
	if err != nil {
		return "", err
	}
	if len(protocols) == 0 {
		return "", fmt.Errorf("no streamable protocol route available for this model")
	}
	return protocols[0], nil
}

func consoleChatProtocolAllowed(protocol domain.UpstreamProtocol) bool {
	for _, item := range consoleChatProtocolOrder {
		if item == protocol {
			return true
		}
	}
	return false
}

func protocolStrings(protocols []domain.UpstreamProtocol) []string {
	out := make([]string, 0, len(protocols))
	for _, protocol := range protocols {
		out = append(out, string(protocol))
	}
	return out
}

func buildConsoleProtocolBody(protocol domain.UpstreamProtocol, req streamConsoleChatRequest) ([]byte, string, error) {
	maxTokens := 2048
	if req.Options.MaxTokens != nil && *req.Options.MaxTokens > 0 {
		maxTokens = *req.Options.MaxTokens
	}
	temp := 0.7
	if req.Options.Temperature != nil {
		temp = *req.Options.Temperature
	}
	switch protocol {
	case domain.ProtocolOpenAIChat:
		body := map[string]any{
			"model":       req.ModelCode,
			"messages":    req.Messages,
			"stream":      true,
			"temperature": temp,
			"max_tokens":  maxTokens,
		}
		raw, err := json.Marshal(body)
		return raw, "/v1/chat/completions", err
	case domain.ProtocolOpenAIResponses:
		body := map[string]any{
			"model":             req.ModelCode,
			"input":             openAIResponsesInput(req.Messages),
			"stream":            true,
			"temperature":       temp,
			"max_output_tokens": maxTokens,
		}
		raw, err := json.Marshal(body)
		return raw, "/v1/responses", err
	case domain.ProtocolAnthropicMessages:
		body := map[string]any{
			"model":       req.ModelCode,
			"messages":    anthropicMessages(req.Messages),
			"stream":      true,
			"temperature": temp,
			"max_tokens":  maxTokens,
		}
		if system := systemPrompt(req.Messages); system != "" {
			body["system"] = system
		}
		raw, err := json.Marshal(body)
		return raw, "/v1/messages", err
	case domain.ProtocolGeminiGenerate:
		body := map[string]any{
			"model":            req.ModelCode,
			"contents":         geminiContents(req.Messages),
			"generationConfig": map[string]any{"temperature": temp, "maxOutputTokens": maxTokens},
		}
		raw, err := json.Marshal(body)
		return raw, "/v1beta/models/{model}:streamGenerateContent", err
	default:
		return nil, "", fmt.Errorf("unsupported protocol")
	}
}

func openAIResponsesInput(messages []consoleChatMessage) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		role := msg.Role
		if role == "assistant" {
			role = "assistant"
		} else if role != "system" {
			role = "user"
		}
		out = append(out, map[string]any{
			"role": role,
			"content": []map[string]any{{
				"type": "input_text",
				"text": msg.Content,
			}},
		})
	}
	return out
}

func anthropicMessages(messages []consoleChatMessage) []map[string]string {
	out := make([]map[string]string, 0, len(messages))
	for _, msg := range messages {
		if msg.Role == "system" {
			continue
		}
		role := msg.Role
		if role != "assistant" {
			role = "user"
		}
		out = append(out, map[string]string{"role": role, "content": msg.Content})
	}
	return out
}

func systemPrompt(messages []consoleChatMessage) string {
	parts := make([]string, 0)
	for _, msg := range messages {
		if msg.Role != "system" {
			continue
		}
		if text := strings.TrimSpace(msg.Content); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n")
}

func geminiContents(messages []consoleChatMessage) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		role := "user"
		if msg.Role == "assistant" {
			role = "model"
		}
		out = append(out, map[string]any{
			"role":  role,
			"parts": []map[string]string{{"text": msg.Content}},
		})
	}
	return out
}

func lastUserText(messages []consoleChatMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return strings.TrimSpace(messages[i].Content)
		}
	}
	return ""
}

func (s *Server) loadConsoleChatSession(w http.ResponseWriter, r *http.Request, identity *domain.RuntimeIdentity, sessionID string) (consoleChatSessionDTO, bool) {
	var dto consoleChatSessionDTO
	err := s.postgres.QueryRow(r.Context(), `
		SELECT id::text, title, model_code, COALESCE(selected_protocol, ''), COALESCE(selected_route_id::text, ''), status,
		       EXTRACT(EPOCH FROM created_at)::bigint, EXTRACT(EPOCH FROM updated_at)::bigint
		FROM ai_console_sessions
		WHERE id = $1 AND tenant_id = $2 AND owner_type = $3 AND COALESCE(user_id, '') = $4 AND status <> 'deleted'`,
		sessionID, identity.TenantID, string(identity.OwnerType), identity.UserID,
	).Scan(&dto.ID, &dto.Title, &dto.ModelCode, &dto.SelectedProtocol, &dto.SelectedRouteID, &dto.Status, &dto.CreatedAt, &dto.UpdatedAt)
	if err != nil {
		writeDBErr(w, err)
		return dto, false
	}
	return dto, true
}

func (s *Server) listConsoleChatMessages(r *http.Request, sessionID string) ([]consoleChatMessageDTO, error) {
	rows, err := s.postgres.Query(r.Context(), `
		SELECT id::text, role, content, COALESCE(protocol, ''), COALESCE(route_id::text, ''), usage_json, error_json,
		       EXTRACT(EPOCH FROM created_at)::bigint
		FROM ai_console_messages
		WHERE session_id = $1
		ORDER BY created_at ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]consoleChatMessageDTO, 0)
	for rows.Next() {
		var item consoleChatMessageDTO
		var usageRaw, errorRaw []byte
		if err := rows.Scan(&item.ID, &item.Role, &item.Content, &item.Protocol, &item.RouteID, &usageRaw, &errorRaw, &item.CreatedAt); err != nil {
			return nil, err
		}
		if len(usageRaw) > 0 {
			_ = json.Unmarshal(usageRaw, &item.Usage)
		}
		if len(errorRaw) > 0 {
			_ = json.Unmarshal(errorRaw, &item.Error)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Server) insertConsoleChatMessage(r *http.Request, sessionID string, role string, content string, protocol string, routeID string, usage any, errorInfo any) error {
	usageRaw, _ := json.Marshal(usage)
	errorRaw, _ := json.Marshal(errorInfo)
	if usage == nil {
		usageRaw = []byte("{}")
	}
	if errorInfo == nil {
		errorRaw = []byte("{}")
	}
	var route any
	if routeID != "" {
		route = routeID
	}
	_, err := s.postgres.Exec(r.Context(), `
		INSERT INTO ai_console_messages (session_id, role, content, protocol, route_id, usage_json, error_json)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7)`,
		sessionID, role, content, protocol, route, usageRaw, errorRaw)
	return err
}

func (s *Server) updateConsoleChatSessionRoute(r *http.Request, sessionID string, protocol domain.UpstreamProtocol, routeID string) error {
	var route any
	if routeID != "" {
		route = routeID
	}
	_, err := s.postgres.Exec(r.Context(), `
		UPDATE ai_console_sessions
		SET selected_protocol = $2, selected_route_id = $3, updated_at = now()
		WHERE id = $1`,
		sessionID, string(protocol), route)
	return err
}

func extractConsoleAssistantText(raw []byte, protocol domain.UpstreamProtocol) string {
	var b strings.Builder
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	eventType := ""
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		if delta := consoleDeltaFromData([]byte(data), eventType, protocol); delta != "" {
			b.WriteString(delta)
		}
		eventType = ""
	}
	return b.String()
}

func consoleDeltaFromData(data []byte, eventType string, protocol domain.UpstreamProtocol) string {
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return ""
	}
	switch protocol {
	case domain.ProtocolAnthropicMessages:
		if delta, ok := obj["delta"].(map[string]any); ok {
			if text, ok := delta["text"].(string); ok {
				return text
			}
		}
	case domain.ProtocolGeminiGenerate:
		return geminiTextDelta(obj)
	case domain.ProtocolOpenAIResponses:
		if delta, ok := obj["delta"].(string); ok {
			return delta
		}
		if text, ok := obj["text"].(string); ok && strings.Contains(eventType, "delta") {
			return text
		}
	default:
		if choices, ok := obj["choices"].([]any); ok && len(choices) > 0 {
			if first, ok := choices[0].(map[string]any); ok {
				if delta, ok := first["delta"].(map[string]any); ok {
					if text, ok := delta["content"].(string); ok {
						return text
					}
				}
				if text, ok := first["text"].(string); ok {
					return text
				}
			}
		}
	}
	return ""
}

func geminiTextDelta(obj map[string]any) string {
	candidates, ok := obj["candidates"].([]any)
	if !ok || len(candidates) == 0 {
		return ""
	}
	first, ok := candidates[0].(map[string]any)
	if !ok {
		return ""
	}
	content, ok := first["content"].(map[string]any)
	if !ok {
		return ""
	}
	parts, ok := content["parts"].([]any)
	if !ok {
		return ""
	}
	var b strings.Builder
	for _, part := range parts {
		if p, ok := part.(map[string]any); ok {
			if text, ok := p["text"].(string); ok {
				b.WriteString(text)
			}
		}
	}
	return b.String()
}
