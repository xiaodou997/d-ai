package console

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/gateway"
	"xiaodou/dai/internal/ai/workspace"
)

var consoleChatProtocolOrder = []domain.UpstreamProtocol{
	domain.ProtocolOpenAIResponses,
	domain.ProtocolOpenAIChat,
	domain.ProtocolAnthropicMessages,
	domain.ProtocolGeminiGenerate,
}

type consoleChatModelDTO struct {
	GroupID                 string   `json:"group_id"`
	GroupName               string   `json:"group_name"`
	EffectiveUserMultiplier float64  `json:"effective_user_multiplier"`
	BillingGroupLabel       string   `json:"billing_group_label"`
	ModelCode               string   `json:"model_code"`
	CapabilityType          string   `json:"capability_type"`
	DefaultAPIFormat        string   `json:"default_api_format"`
	AvailableAPIFormats     []string `json:"available_api_formats"`
	SupportsStream          bool     `json:"supports_stream"`
	Status                  string   `json:"status"`
}

type consoleChatSessionDTO struct {
	ID                string `json:"id"`
	Title             string `json:"title"`
	ModelCode         string `json:"model_code"`
	GroupID           string `json:"group_id,omitempty"`
	GroupName         string `json:"group_name,omitempty"`
	BillingGroupLabel string `json:"billing_group_label,omitempty"`
	ProviderAPIFormat string `json:"provider_api_format"`
	SelectedRouteID   string `json:"selected_route_id"`
	Status            string `json:"status"`
	CreatedAt         int64  `json:"created_at"`
	UpdatedAt         int64  `json:"updated_at"`
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
	GroupID   string `json:"group_id"`
	Title     string `json:"title"`
}

type consoleChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type streamConsoleChatRequest struct {
	ModelCode string               `json:"model"`
	Messages  []consoleChatMessage `json:"messages"`
}

type consoleChatModelRow struct {
	ModelCode      string
	CapabilityType string
}

type captureResponseWriter struct {
	http.ResponseWriter
	mu         sync.Mutex
	buf        bytes.Buffer
	statusCode int
	onWrite    func()
}

func (w *captureResponseWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	w.buf.Write(p)
	w.mu.Unlock()
	if w.onWrite != nil {
		w.onWrite()
	}
	return w.ResponseWriter.Write(p)
}

func (w *captureResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *captureResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *captureResponseWriter) snapshot() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]byte(nil), w.buf.Bytes()...)
}

const consoleChatStreamPersistInterval = 500 * time.Millisecond

type consoleChatStreamPersistence struct {
	console    *Console
	requestCtx context.Context
	owner      workspace.Owner
	sessionID  string
	messageID  string
	protocol   domain.UpstreamProtocol
	capture    *captureResponseWriter
	updates    chan struct{}
	done       chan struct{}
	wg         sync.WaitGroup
}

func (s *Console) startConsoleChatStreamPersistence(
	requestCtx context.Context,
	owner workspace.Owner,
	sessionID string,
	protocol domain.UpstreamProtocol,
	capture *captureResponseWriter,
) *consoleChatStreamPersistence {
	messageID, err := s.workspaceSvc.CreateChatMessage(requestCtx, owner, sessionID, workspace.ChatMessageWriteInput{
		Role:          workspace.MessageRoleAssistant,
		ClientSurface: workspace.SurfaceFromProtocol(string(protocol)),
		StreamStatus:  workspace.ChatStreamStatusStreaming,
	})
	if err != nil {
		s.logger.Warn("runtime chat: create assistant message failed", zap.Error(err), zap.String("session_id", sessionID))
		return nil
	}
	persistence := &consoleChatStreamPersistence{
		console:    s,
		requestCtx: requestCtx,
		owner:      owner,
		sessionID:  sessionID,
		messageID:  messageID,
		protocol:   protocol,
		capture:    capture,
		updates:    make(chan struct{}, 1),
		done:       make(chan struct{}),
	}
	capture.onWrite = persistence.requestFlush
	persistence.wg.Add(1)
	go persistence.run()
	return persistence
}

func (p *consoleChatStreamPersistence) requestFlush() {
	select {
	case p.updates <- struct{}{}:
	default:
	}
}

func (p *consoleChatStreamPersistence) run() {
	defer p.wg.Done()
	ticker := time.NewTicker(consoleChatStreamPersistInterval)
	defer ticker.Stop()
	dirty := false
	for {
		select {
		case <-p.updates:
			dirty = true
		case <-ticker.C:
			if dirty {
				p.persistContent()
				dirty = false
			}
		case <-p.done:
			p.persistContent()
			return
		}
	}
}

func (p *consoleChatStreamPersistence) persistContent() {
	content := extractConsoleAssistantText(p.capture.snapshot(), p.protocol)
	if content == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(p.requestCtx), 3*time.Second)
	defer cancel()
	if err := p.console.workspaceSvc.UpdateChatMessageContent(ctx, p.owner, p.messageID, content); err != nil {
		p.console.logger.Warn("runtime chat: persist assistant content failed", zap.Error(err), zap.String("session_id", p.sessionID))
	}
}

func (p *consoleChatStreamPersistence) close(routeID string) {
	close(p.done)
	p.wg.Wait()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(p.requestCtx), 3*time.Second)
	defer cancel()
	streamStatus := workspace.ChatStreamStatusCompleted
	if p.requestCtx.Err() != nil {
		streamStatus = workspace.ChatStreamStatusInterrupted
	}
	if err := p.console.workspaceSvc.UpdateChatMessageRoute(ctx, p.owner, p.messageID, workspace.ChatMessageRouteUpdate{
		ClientSurface: workspace.SurfaceFromProtocol(string(p.protocol)),
		RouteID:       routeID,
		StreamStatus:  streamStatus,
	}); err != nil {
		p.console.logger.Warn("runtime chat: persist assistant route failed", zap.Error(err), zap.String("session_id", p.sessionID))
	}
}

func (s *Console) handleConsoleChatModels(w http.ResponseWriter, r *http.Request) {
	subject, ok := s.consoleRuntimeSubject(w, r)
	if !ok {
		return
	}
	if s.workspaceSvc == nil {
		writeErr(w, http.StatusServiceUnavailable, BizErrInternal, "workspace service is not configured")
		return
	}
	models, err := s.workspaceSvc.ListChatModels(r.Context(), workspace.Owner{
		Scope:    subject.Scope,
		TenantID: subject.TenantID,
		UserID:   subject.UserID,
	})
	if err != nil {
		s.logger.Error("runtime chat models: list workspace models failed",
			consoleSubjectLogFields(r, subject, zap.Error(err))...,
		)
		writeDBErr(w, err)
		return
	}
	out := make([]consoleChatModelDTO, 0, len(models))
	for _, model := range models {
		out = append(out, workspaceChatModelToConsoleDTO(model))
	}
	writeOK(w, out)
}

func (s *Console) handleConsoleChatListSessions(w http.ResponseWriter, r *http.Request) {
	subject, ok := s.consoleRuntimeSubject(w, r)
	if !ok {
		return
	}
	if s.workspaceSvc == nil {
		writeErr(w, http.StatusServiceUnavailable, BizErrInternal, "workspace service is not configured")
		return
	}
	sessions, err := s.workspaceSvc.ListChatSessions(r.Context(), workspace.Owner{
		Scope:    subject.Scope,
		TenantID: subject.TenantID,
		UserID:   subject.UserID,
	}, 100)
	if err != nil {
		writeDBErr(w, err)
		return
	}
	out := make([]consoleChatSessionDTO, 0, len(sessions))
	for _, session := range sessions {
		out = append(out, workspaceChatSessionToConsoleDTO(session))
	}
	writeOK(w, out)
}

func (s *Console) handleConsoleChatCreateSession(w http.ResponseWriter, r *http.Request) {
	subject, ok := s.consoleRuntimeSubject(w, r)
	if !ok {
		return
	}
	if s.workspaceSvc == nil {
		writeErr(w, http.StatusServiceUnavailable, BizErrInternal, "workspace service is not configured")
		return
	}
	var req createConsoleChatSessionRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	session, err := s.workspaceSvc.CreateChatSession(r.Context(), workspace.Owner{
		Scope:    subject.Scope,
		TenantID: subject.TenantID,
		UserID:   subject.UserID,
	}, workspace.CreateChatSessionInput{
		ModelCode: req.ModelCode,
		GroupID:   req.GroupID,
		Title:     req.Title,
	})
	if err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, workspaceChatSessionToConsoleDTO(session))
}

func (s *Console) handleConsoleChatGetSession(w http.ResponseWriter, r *http.Request) {
	subject, ok := s.consoleRuntimeSubject(w, r)
	if !ok {
		return
	}
	sessionID := chi.URLParam(r, "sessionID")
	session, ok := s.loadConsoleChatSession(w, r, subject, sessionID)
	if !ok {
		return
	}
	msgs, err := s.listConsoleChatMessages(r, subject, sessionID)
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeOK(w, consoleChatSessionDetailDTO{Session: session, Messages: msgs})
}

func (s *Console) handleConsoleChatDeleteSession(w http.ResponseWriter, r *http.Request) {
	subject, ok := s.consoleRuntimeSubject(w, r)
	if !ok {
		return
	}
	if s.workspaceSvc == nil {
		writeErr(w, http.StatusServiceUnavailable, BizErrInternal, "workspace service is not configured")
		return
	}
	sessionID := chi.URLParam(r, "sessionID")
	if err := s.workspaceSvc.DeleteChatSession(r.Context(), workspace.Owner{
		Scope:    subject.Scope,
		TenantID: subject.TenantID,
		UserID:   subject.UserID,
	}, sessionID); err != nil {
		s.writeServiceErr(w, r, err)
		return
	}
	writeOK(w, nil)
}

func (s *Console) handleConsoleChatStream(w http.ResponseWriter, r *http.Request) {
	subject, ok := s.consoleRuntimeSubject(w, r)
	if !ok {
		return
	}
	sessionID := chi.URLParam(r, "sessionID")
	owner := workspace.Owner{Scope: subject.Scope, TenantID: subject.TenantID, UserID: subject.UserID}

	var req streamConsoleChatRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}
	if len(req.Messages) == 0 {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "messages are required")
		return
	}

	session, ok := s.loadConsoleChatSession(w, r, subject, sessionID)
	if !ok {
		return
	}
	sessionSubject := consoleSubjectForSession(subject, session.GroupID)
	req.ModelCode = strings.TrimSpace(req.ModelCode)
	modelCode := req.ModelCode
	requestMessages := req.Messages
	maxTokens := consoleDefaultMaxTokens
	if modelCode == "" {
		modelCode = session.ModelCode
	}
	if modelCode == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "model is required")
		return
	}
	if _, err := s.consoleGrantedChatModelByCode(r, sessionSubject, modelCode); err != nil {
		if err == pgx.ErrNoRows {
			writeErr(w, http.StatusForbidden, BizErrForbidden, "model is not authorized")
			return
		}
		writeDBErr(w, err)
		return
	}

	protocol, err := s.chooseConsoleChatProtocol(r, sessionSubject, modelCode)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, err.Error())
		return
	}

	body, clientPath, err := buildConsoleProtocolBody(protocol, modelCode, requestMessages, maxTokens)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, err.Error())
		return
	}

	userText := lastUserText(req.Messages)
	if userText != "" {
		if _, err := s.workspaceSvc.CreateChatMessage(r.Context(), owner, sessionID, workspace.ChatMessageWriteInput{Role: workspace.MessageRoleUser, Content: userText}); err != nil {
			s.logger.Warn("runtime chat: persist user message failed", zap.Error(err), zap.String("session_id", sessionID))
		}
	}
	_, _ = s.postgres.Exec(r.Context(), `
		UPDATE ai_workspace_threads
		SET target_model_code = $2,
		    selected_surface = $3,
		    title = CASE WHEN title = '新对话' AND $4 <> '' THEN left($4, 80) ELSE title END,
		    updated_at = now()
		WHERE id = $1`,
		sessionID, modelCode, workspace.SurfaceFromProtocol(string(protocol)), userText)

	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "text/event-stream")

	capture := &captureResponseWriter{ResponseWriter: w}
	persistence := s.startConsoleChatStreamPersistence(r.Context(), owner, sessionID, protocol, capture)
	runtimeResult := s.gateway.ExecuteRuntime(capture, r, domain.CapabilityChat, gateway.RuntimeOverride{
		ClientProtocol: protocol,
		ClientPath:     clientPath,
	}, sessionSubject, false)

	routeID := runtimeResult.RouteID
	if persistence != nil {
		persistence.close(routeID)
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 3*time.Second)
	defer cancel()
	if err := s.workspaceSvc.UpdateChatSessionRoute(ctx, owner, sessionID, workspace.SurfaceFromProtocol(string(protocol)), routeID); err != nil {
		s.logger.Warn("runtime chat: persist session route failed", zap.Error(err), zap.String("session_id", sessionID))
	}
}

// consoleGrantedChatModels 返回调用者在 Web 运行层聊天中可选的 chat 模型。
// 模型来源与运行时路由同源：可访问分组的 active 上游显式模型绑定，
// 价格表只用于确认该模型具备售价。
func (s *Console) consoleGrantedChatModels(r *http.Request, subject *coreidentity.Subject) ([]consoleChatModelRow, error) {
	groups, err := s.grantChecker.AccessibleGroupIDsForSubject(r.Context(), subject)
	if err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return []consoleChatModelRow{}, nil
	}
	models, err := s.queryConsoleChatModels(r, `
		SELECT DISTINCT um.model_code, um.capability_type
		FROM ai_groups g
		JOIN ai_group_targets gt
		  ON gt.group_id = g.id
		 AND gt.status = 'active'
		JOIN ai_upstream_models um
		  ON um.upstream_kind = gt.target_kind
		 AND um.upstream_id = gt.target_id
		 AND um.status = 'active'
		JOIN ai_price_book_entries e
		  ON e.price_book_id = g.retail_price_book_id
		 AND e.model_code = um.model_code
		 AND e.capability_type = um.capability_type
		LEFT JOIN ai_upstream_accounts a
		  ON gt.target_kind = 'direct_upstream' AND a.id = gt.target_id
		LEFT JOIN ai_credential_pools cp
		  ON gt.target_kind = 'oauth_pool' AND cp.id = gt.target_id
		WHERE g.id = ANY($1::uuid[])
		  AND g.status = 'active'
		  AND um.capability_type = 'chat'
		  AND (
		    (gt.target_kind = 'direct_upstream' AND a.status = 'active')
		    OR
		    (gt.target_kind = 'oauth_pool' AND cp.status = 'active')
		  )
		ORDER BY um.model_code ASC`, groups)
	if err != nil {
		return nil, err
	}
	// key 级 allowed_models 收窄，与 runtime resolver 的最终候选资格保持一致。
	if len(subject.AllowedModels) > 0 {
		allowed := make(map[string]struct{}, len(subject.AllowedModels))
		for _, m := range subject.AllowedModels {
			allowed[m] = struct{}{}
		}
		filtered := models[:0]
		for _, m := range models {
			if _, ok := allowed[m.ModelCode]; ok {
				filtered = append(filtered, m)
			}
		}
		models = filtered
	}
	return models, nil
}

func (s *Console) consoleGrantedChatModelByCode(r *http.Request, subject *coreidentity.Subject, modelCode string) (consoleChatModelRow, error) {
	models, err := s.consoleGrantedChatModels(r, subject)
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

func (s *Console) queryConsoleChatModels(r *http.Request, query string, args ...any) ([]consoleChatModelRow, error) {
	rows, err := s.postgres.Query(r.Context(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]consoleChatModelRow, 0)
	for rows.Next() {
		var item consoleChatModelRow
		if err := rows.Scan(&item.ModelCode, &item.CapabilityType); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Console) availableConsoleChatProtocols(r *http.Request, subject *coreidentity.Subject, modelCode string) ([]domain.UpstreamProtocol, error) {
	groups, err := s.consoleAccessibleChatGroups(r, subject)
	if err != nil {
		return nil, err
	}
	out := make([]domain.UpstreamProtocol, 0, len(consoleChatProtocolOrder))
	for _, protocol := range consoleChatProtocolOrder {
		ok, err := s.modelHasConsoleProtocolRoute(r, modelCode, groups, protocol)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, protocol)
		}
	}
	return out, nil
}

func (s *Console) consoleAccessibleChatGroups(r *http.Request, subject *coreidentity.Subject) ([]string, error) {
	return s.grantChecker.AccessibleGroupIDsForSubject(r.Context(), subject)
}

func consoleSubjectForSession(subject *coreidentity.Subject, groupID string) *coreidentity.Subject {
	if subject == nil {
		return nil
	}
	out := *subject
	groupID = strings.TrimSpace(groupID)
	if groupID != "" {
		out.GroupID = groupID
	}
	return &out
}

func (s *Console) modelHasConsoleProtocolRoute(r *http.Request, modelCode string, groups []string, protocol domain.UpstreamProtocol) (bool, error) {
	return s.routeInspector.ModelSupportsClientProtocolInGroups(r.Context(), modelCode, domain.CapabilityChat, groups, protocol, true, false)
}

func (s *Console) chooseConsoleChatProtocol(r *http.Request, subject *coreidentity.Subject, modelCode string) (domain.UpstreamProtocol, error) {
	protocols, err := s.availableConsoleChatProtocols(r, subject, modelCode)
	if err != nil {
		return "", err
	}
	if len(protocols) == 0 {
		return "", fmt.Errorf("no streamable protocol route available for this model")
	}
	return protocols[0], nil
}

func workspaceChatModelToConsoleDTO(model workspace.ChatModel) consoleChatModelDTO {
	return consoleChatModelDTO{
		GroupID:                 model.GroupID,
		GroupName:               model.GroupName,
		EffectiveUserMultiplier: model.EffectiveUserMultiplier,
		BillingGroupLabel:       model.BillingGroupLabel,
		ModelCode:               model.ModelCode,
		CapabilityType:          model.CapabilityType,
		DefaultAPIFormat:        model.DefaultProtocol,
		AvailableAPIFormats:     append([]string{}, model.AvailableProtocols...),
		SupportsStream:          model.SupportsStream,
		Status:                  model.Status,
	}
}

func buildConsoleProtocolBody(protocol domain.UpstreamProtocol, modelCode string, messages []consoleChatMessage, maxTokens int) ([]byte, string, error) {
	switch protocol {
	case domain.ProtocolOpenAIChat:
		body := map[string]any{
			"model":      modelCode,
			"messages":   messages,
			"stream":     true,
			"max_tokens": maxTokens,
		}
		raw, err := json.Marshal(body)
		return raw, "/v1/chat/completions", err
	case domain.ProtocolOpenAIResponses:
		body := map[string]any{
			"model":             modelCode,
			"input":             openAIResponsesInput(messages),
			"stream":            true,
			"max_output_tokens": maxTokens,
		}
		raw, err := json.Marshal(body)
		return raw, "/v1/responses", err
	case domain.ProtocolAnthropicMessages:
		body := map[string]any{
			"model":      modelCode,
			"messages":   anthropicMessages(messages),
			"stream":     true,
			"max_tokens": maxTokens,
		}
		if system := systemPrompt(messages); system != "" {
			body["system"] = system
		}
		raw, err := json.Marshal(body)
		return raw, "/v1/messages", err
	case domain.ProtocolGeminiGenerate:
		body := map[string]any{
			"model":            modelCode,
			"contents":         geminiContents(messages),
			"generationConfig": map[string]any{"maxOutputTokens": maxTokens},
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

func replaceLastUserText(messages []consoleChatMessage, content string) []consoleChatMessage {
	out := append([]consoleChatMessage(nil), messages...)
	for index := len(out) - 1; index >= 0; index-- {
		if out[index].Role == "user" {
			out[index].Content = content
			break
		}
	}
	return out
}

func (s *Console) loadConsoleChatSession(w http.ResponseWriter, r *http.Request, subject *coreidentity.Subject, sessionID string) (consoleChatSessionDTO, bool) {
	if s.workspaceSvc == nil {
		writeErr(w, http.StatusServiceUnavailable, BizErrInternal, "workspace service is not configured")
		return consoleChatSessionDTO{}, false
	}
	session, err := s.workspaceSvc.GetChatSession(r.Context(), workspace.Owner{
		Scope:    subject.Scope,
		TenantID: subject.TenantID,
		UserID:   subject.UserID,
	}, sessionID)
	if err != nil {
		writeDBErr(w, err)
		return consoleChatSessionDTO{}, false
	}
	dto := workspaceChatSessionToConsoleDTO(session)
	return dto, true
}

func prependConsoleSystemMessage(messages []consoleChatMessage, systemText string) []consoleChatMessage {
	systemText = strings.TrimSpace(systemText)
	if systemText == "" {
		return messages
	}
	out := make([]consoleChatMessage, 0, len(messages)+1)
	out = append(out, consoleChatMessage{Role: "system", Content: systemText})
	out = append(out, messages...)
	return out
}

// Console defaults for direct-model chat sessions. Max output is not a
// user-facing control — the model decides — but a sane cap is still required by
// upstreams such as Anthropic Messages, so we keep one here.
const (
	consoleDefaultMaxTokens = 2048
)

func effectiveConsoleMaxTokens(req *int) int {
	if req != nil && *req > 0 {
		return *req
	}
	return consoleDefaultMaxTokens
}

func numberFromAny(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func intFromAny(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case float32:
		return int(v), true
	case json.Number:
		i, err := v.Int64()
		return int(i), err == nil
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(v))
		return i, err == nil
	default:
		return 0, false
	}
}

func (s *Console) listConsoleChatMessages(r *http.Request, subject *coreidentity.Subject, sessionID string) ([]consoleChatMessageDTO, error) {
	if s.workspaceSvc == nil {
		return nil, fmt.Errorf("workspace service is not configured")
	}
	messages, err := s.workspaceSvc.ListChatMessages(r.Context(), workspace.Owner{
		Scope:    subject.Scope,
		TenantID: subject.TenantID,
		UserID:   subject.UserID,
	}, sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]consoleChatMessageDTO, 0, len(messages))
	for _, message := range messages {
		out = append(out, workspaceChatMessageToConsoleDTO(message))
	}
	return out, nil
}

func workspaceChatSessionToConsoleDTO(session workspace.ChatSession) consoleChatSessionDTO {
	return consoleChatSessionDTO{
		ID:                session.ID,
		Title:             session.Title,
		ModelCode:         session.ModelCode,
		GroupID:           session.GroupID,
		GroupName:         session.GroupName,
		BillingGroupLabel: session.BillingGroupLabel,
		ProviderAPIFormat: session.SelectedProtocol,
		SelectedRouteID:   session.SelectedRouteID,
		Status:            session.Status,
		CreatedAt:         session.CreatedAt.UnixMilli(),
		UpdatedAt:         session.UpdatedAt.UnixMilli(),
	}
}

func workspaceChatMessageToConsoleDTO(message workspace.ChatMessage) consoleChatMessageDTO {
	return consoleChatMessageDTO{
		ID:        message.ID,
		Role:      message.Role,
		Content:   message.Content,
		Protocol:  message.Protocol,
		RouteID:   message.RouteID,
		Usage:     message.Usage,
		Error:     message.Error,
		CreatedAt: message.CreatedAt.UnixMilli(),
	}
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
