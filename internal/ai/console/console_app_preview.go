package console

import (
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"xiaodou/dai/internal/ai/application"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	coreruntime "xiaodou/dai/internal/ai/core/runtime"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/gateway"
)

func (s *Console) handleConsoleAppPreview(w http.ResponseWriter, r *http.Request) {
	subject, ok := s.consoleRuntimeSubject(w, r)
	if !ok {
		return
	}

	agentID := strings.TrimSpace(chi.URLParam(r, "agentID"))
	if agentID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "agent id is required")
		return
	}

	var req appPreviewRequest
	if !decodeAdminJSON(w, r, &req) {
		return
	}

	agent, err := s.consoleVisibleAgentForPreview(r, subject, agentID)
	if err != nil {
		if err == pgx.ErrNoRows || errors.Is(err, domain.ErrForbidden) {
			writeErr(w, http.StatusForbidden, BizErrForbidden, "app is not authorized")
			return
		}
		writeDBErr(w, err)
		return
	}

	if strings.TrimSpace(agent.GroupID) == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "app's bound group has been deleted, please reassign a group")
		return
	}

	switch agent.AgentType {
	case consoleAgentTypeChat:
		s.handleConsoleChatAppPreview(w, r, subject, agent, req)
	case consoleAgentTypeImageGeneration:
		s.handleConsoleImageGenerationAppPreview(w, r, subject, agent, req)
	case consoleAgentTypeImageEdit:
		s.handleConsoleImageEditAppPreview(w, r, subject, agent, req)
	default:
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "unsupported app type")
	}
}

func (s *Console) consoleVisibleAgentForPreview(r *http.Request, subject *coreidentity.Subject, agentID string) (consoleChatAgentRuntime, error) {
	if found, err := s.consoleVisibleChatAgentByID(r, subject, agentID); err == nil {
		return found, nil
	} else if err != pgx.ErrNoRows {
		return consoleChatAgentRuntime{}, err
	}
	return s.consoleVisibleImageAgentByID(r, subject, agentID)
}

func (s *Console) handleConsoleChatAppPreview(
	w http.ResponseWriter,
	r *http.Request,
	subject *coreidentity.Subject,
	agent consoleChatAgentRuntime,
	req appPreviewRequest,
) {
	input := strings.TrimSpace(req.Input)
	if input == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "input is required")
		return
	}

	cfg := application.ParseRuntimeConfig(application.AppTypeChatAgent, agent.DefaultOptions).Chat
	if cfg == nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "chat app runtime config is invalid")
		return
	}
	if err := validatePreviewAttachments(req.Attachments, *cfg); err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, err.Error())
		return
	}
	resolved, err := resolveConsoleAppPrompt(agent, input, req.Variables)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, err.Error())
		return
	}

	requestMessages := prependConsoleSystemMessage(
		[]consoleChatMessage{{Role: "user", Content: resolved.Input}},
		resolved.Instruction,
	)
	protocol, err := s.chooseConsoleChatProtocol(r, consoleSubjectForApp(subject, agent), agent.ModelCode)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, err.Error())
		return
	}

	body, clientPath, err := buildConsoleProtocolBodyWithStream(protocol, agent.ModelCode, requestMessages, cfg.Temperature(), consoleDefaultMaxTokens, false, req.Attachments)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, err.Error())
		return
	}

	runtimeReq, responseBody, statusCode := s.executeConsoleAppPreviewRuntime(
		r,
		consoleSubjectForPreview(consoleSubjectForApp(subject, agent)),
		domain.CapabilityChat,
		protocol,
		clientPath,
		"application/json",
		body,
	)
	if runtimeReq == nil {
		writeConsolePreviewError(w, statusCode, responseBody)
		return
	}

	writeOK(w, appPreviewResponse{
		Type:      "chat",
		Text:      extractConsoleAssistantTextFromSync(responseBody, protocol),
		Usage:     usageMapFromResult(runtimeReq),
		RequestID: runtimeReq.RequestID,
	})
}

func (s *Console) handleConsoleImageGenerationAppPreview(
	w http.ResponseWriter,
	r *http.Request,
	subject *coreidentity.Subject,
	agent consoleChatAgentRuntime,
	req appPreviewRequest,
) {
	prepared, err := s.prepareConsoleImageGeneration(r.Context(), subject, consoleImageGenerateRequest{
		AgentID:        agent.ID,
		Prompt:         strings.TrimSpace(req.Input),
		Variables:      req.Variables,
		ResponseFormat: normalizePreviewResponseFormat(req.ResponseFormat),
		N:              req.N,
	}, false)
	if err != nil {
		writeConsoleImagePrepareErr(w, err)
		return
	}

	replay := prepared.Replay
	replay.Subject = *consoleSubjectForPreview(&replay.Subject)
	runtimeReq, responseBody, statusCode := s.executeConsoleImagePreview(r, replay)
	if runtimeReq == nil {
		writeConsolePreviewError(w, statusCode, responseBody)
		return
	}

	writeOK(w, appPreviewResponse{
		Type:      "image",
		Images:    previewImagesFromBody(responseBody),
		Usage:     usageMapFromResult(runtimeReq),
		RequestID: runtimeReq.RequestID,
	})
}

func (s *Console) handleConsoleImageEditAppPreview(
	w http.ResponseWriter,
	r *http.Request,
	subject *coreidentity.Subject,
	agent consoleChatAgentRuntime,
	req appPreviewRequest,
) {
	editReq := consoleImageGenerateRequest{
		AgentID:        agent.ID,
		Prompt:         strings.TrimSpace(req.Input),
		Variables:      req.Variables,
		ResponseFormat: normalizePreviewResponseFormat(req.ResponseFormat),
		N:              req.N,
	}
	if len(req.Images) > 0 {
		editReq.Images = []consoleImageSource{{ImageURL: strings.TrimSpace(req.Images[0])}}
	}
	prepared, err := s.prepareConsoleImageEditTask(r.Context(), subject, editReq, false)
	if err != nil {
		writeConsoleImagePrepareErr(w, err)
		return
	}

	replay := prepared.Replay
	replay.Subject = *consoleSubjectForPreview(&replay.Subject)
	runtimeReq, responseBody, statusCode := s.executeConsoleImagePreview(r, replay)
	if runtimeReq == nil {
		writeConsolePreviewError(w, statusCode, responseBody)
		return
	}

	writeOK(w, appPreviewResponse{
		Type:      "image",
		Images:    previewImagesFromBody(responseBody),
		Usage:     usageMapFromResult(runtimeReq),
		RequestID: runtimeReq.RequestID,
	})
}

func (s *Console) executeConsoleAppPreviewRuntime(
	r *http.Request,
	subject *coreidentity.Subject,
	capType domain.CapabilityType,
	clientProtocol domain.UpstreamProtocol,
	clientPath string,
	contentType string,
	body []byte,
) (*coreruntime.Result, []byte, int) {
	// Forwards the inbound headers, as this path proxies a live client. Unlike
	// the image preview, the subject is passed through untouched, so the usage
	// log records this as app_preview.
	result := s.gateway.Replay(r.Context(), gateway.ReplayInput{
		Subject:     *subject,
		Capability:  capType,
		Protocol:    clientProtocol,
		ClientPath:  clientPath,
		Body:        body,
		ContentType: contentType,
		Header:      r.Header,
	})
	return result.Request, result.Body, result.StatusCode
}

// executeConsoleImagePreview returns a nil result when the pipeline
// never started, which is the caller's signal that the failure is in the body.
func (s *Console) executeConsoleImagePreview(
	r *http.Request,
	replay gateway.ReplayInput,
) (*coreruntime.Result, []byte, int) {
	// Forwards the inbound headers, as this path proxies a live client.
	replay.Header = r.Header
	result := s.gateway.Replay(r.Context(), replay)
	return result.Request, result.Body, result.StatusCode
}

func consoleSubjectForPreview(subject *coreidentity.Subject) *coreidentity.Subject {
	if subject == nil {
		return nil
	}
	out := *subject
	out.RequestSource = coreidentity.RequestSourceAppPreview
	return &out
}

func validatePreviewAttachments(items []runAttachment, cfg application.ChatRuntimeConfig) error {
	if !cfg.AllowAttachments {
		if len(items) > 0 {
			return errors.New("attachments are not allowed for this app")
		}
		return nil
	}
	if len(items) > cfg.MaxAttachments() {
		return errors.New("too many attachments")
	}
	for _, item := range items {
		u := strings.TrimSpace(item.URL)
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			return errors.New("attachments must use http(s) direct URLs")
		}
	}
	return nil
}

func buildConsoleProtocolBodyWithStream(
	protocol domain.UpstreamProtocol,
	modelCode string,
	messages []consoleChatMessage,
	temp float64,
	maxTokens int,
	stream bool,
	attachments []runAttachment,
) ([]byte, string, error) {
	if len(attachments) == 0 {
		return buildConsoleProtocolBodyWithMessages(protocol, modelCode, messages, temp, maxTokens, stream)
	}
	if protocol != domain.ProtocolOpenAIChat {
		return nil, "", errors.New("attachments preview currently requires an OpenAI chat route")
	}
	body := map[string]any{
		"model":       modelCode,
		"messages":    buildConsoleChatMessagesWithAttachments(messages, attachments),
		"stream":      stream,
		"temperature": temp,
		"max_tokens":  maxTokens,
	}
	raw, err := json.Marshal(body)
	return raw, "/v1/chat/completions", err
}

func buildConsoleProtocolBodyWithMessages(
	protocol domain.UpstreamProtocol,
	modelCode string,
	messages []consoleChatMessage,
	temp float64,
	maxTokens int,
	stream bool,
) ([]byte, string, error) {
	switch protocol {
	case domain.ProtocolOpenAIChat:
		body := map[string]any{
			"model":       modelCode,
			"messages":    messages,
			"stream":      stream,
			"temperature": temp,
			"max_tokens":  maxTokens,
		}
		raw, err := json.Marshal(body)
		return raw, "/v1/chat/completions", err
	case domain.ProtocolOpenAIResponses:
		body := map[string]any{
			"model":             modelCode,
			"input":             openAIResponsesInput(messages),
			"stream":            stream,
			"temperature":       temp,
			"max_output_tokens": maxTokens,
		}
		raw, err := json.Marshal(body)
		return raw, "/v1/responses", err
	case domain.ProtocolAnthropicMessages:
		body := map[string]any{
			"model":       modelCode,
			"messages":    anthropicMessages(messages),
			"stream":      stream,
			"temperature": temp,
			"max_tokens":  maxTokens,
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
			"generationConfig": map[string]any{"temperature": temp, "maxOutputTokens": maxTokens},
		}
		raw, err := json.Marshal(body)
		if stream {
			return raw, "/v1beta/models/{model}:streamGenerateContent", err
		}
		return raw, "/v1beta/models/{model}:generateContent", err
	default:
		return nil, "", errors.New("unsupported protocol")
	}
}

func buildConsoleChatMessagesWithAttachments(messages []consoleChatMessage, attachments []runAttachment) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for idx, msg := range messages {
		if idx == len(messages)-1 && msg.Role == "user" {
			out = append(out, map[string]any{
				"role":    msg.Role,
				"content": runChatUserContent(msg.Content, attachments),
			})
			continue
		}
		out = append(out, map[string]any{
			"role":    msg.Role,
			"content": msg.Content,
		})
	}
	return out
}

func runChatUserContent(input string, attachments []runAttachment) any {
	if len(attachments) == 0 {
		return input
	}
	parts := make([]map[string]any, 0, len(attachments)+1)
	parts = append(parts, map[string]any{"type": "text", "text": input})
	for _, item := range attachments {
		kind := strings.TrimSpace(item.Type)
		if kind == "" && (strings.HasPrefix(strings.ToLower(item.MIMEType), "image/") || looksLikeImageURL(item.URL)) {
			kind = "image"
		}
		switch kind {
		case "image":
			parts = append(parts, map[string]any{
				"type": "image_url",
				"image_url": map[string]any{
					"url": item.URL,
				},
			})
		default:
			file := map[string]any{"file_url": item.URL}
			if strings.TrimSpace(item.Name) != "" {
				file["filename"] = strings.TrimSpace(item.Name)
			}
			parts = append(parts, map[string]any{
				"type": "file",
				"file": file,
			})
		}
	}
	return parts
}

func looksLikeImageURL(rawURL string) bool {
	path := strings.TrimSpace(rawURL)
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	path = strings.ToLower(path)
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".webp", ".gif", ".bmp", ".avif"} {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

func extractConsoleAssistantTextFromSync(raw []byte, protocol domain.UpstreamProtocol) string {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return strings.TrimSpace(string(raw))
	}
	switch protocol {
	case domain.ProtocolOpenAIResponses:
		if output, ok := payload["output"].([]any); ok {
			var builder strings.Builder
			for _, item := range output {
				obj, ok := item.(map[string]any)
				if !ok {
					continue
				}
				content, _ := obj["content"].([]any)
				for _, part := range content {
					partObj, ok := part.(map[string]any)
					if !ok {
						continue
					}
					partType, _ := partObj["type"].(string)
					if partType == "output_text" || partType == "text" {
						if text, ok := partObj["text"].(string); ok {
							builder.WriteString(text)
						}
					}
				}
			}
			return strings.TrimSpace(builder.String())
		}
	case domain.ProtocolAnthropicMessages:
		if content, ok := payload["content"].([]any); ok {
			var builder strings.Builder
			for _, item := range content {
				obj, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if text, ok := obj["text"].(string); ok {
					builder.WriteString(text)
				}
			}
			return strings.TrimSpace(builder.String())
		}
	case domain.ProtocolGeminiGenerate:
		return geminiTextDelta(payload)
	default:
		if choices, ok := payload["choices"].([]any); ok && len(choices) > 0 {
			if first, ok := choices[0].(map[string]any); ok {
				if message, ok := first["message"].(map[string]any); ok {
					if text, ok := message["content"].(string); ok {
						return strings.TrimSpace(text)
					}
				}
				if text, ok := first["text"].(string); ok {
					return strings.TrimSpace(text)
				}
			}
		}
	}
	return ""
}

func previewImagesFromBody(raw []byte) []map[string]any {
	var payload struct {
		Data []map[string]any `json:"data"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return []map[string]any{}
	}
	return payload.Data
}

func usageMapFromResult(result *coreruntime.Result) map[string]any {
	if result == nil || len(result.Usage) == 0 {
		return nil
	}
	hasUsage := false
	for _, value := range result.Usage {
		switch typed := value.(type) {
		case int:
			hasUsage = hasUsage || typed != 0
		case int64:
			hasUsage = hasUsage || typed != 0
		case float64:
			hasUsage = hasUsage || typed != 0
		case string:
			hasUsage = hasUsage || strings.TrimSpace(typed) != ""
		}
	}
	if !hasUsage {
		return nil
	}
	out := make(map[string]any, len(result.Usage))
	for key, value := range result.Usage {
		out[key] = value
	}
	return out
}

func normalizePreviewResponseFormat(value string) string {
	if strings.TrimSpace(value) == "url" {
		return "url"
	}
	return "b64_json"
}

func copyRequestHeaders(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func writeConsolePreviewError(w http.ResponseWriter, statusCode int, body []byte) {
	message := strings.TrimSpace(string(body))
	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &payload) == nil {
		if strings.TrimSpace(payload.Error.Message) != "" {
			message = strings.TrimSpace(payload.Error.Message)
		} else if strings.TrimSpace(payload.Message) != "" {
			message = strings.TrimSpace(payload.Message)
		}
	} else if mediaType, _, err := mime.ParseMediaType(w.Header().Get("Content-Type")); err == nil && strings.EqualFold(mediaType, "application/json") {
		if json.Unmarshal(body, &payload) == nil {
			if strings.TrimSpace(payload.Error.Message) != "" {
				message = strings.TrimSpace(payload.Error.Message)
			} else if strings.TrimSpace(payload.Message) != "" {
				message = strings.TrimSpace(payload.Message)
			}
		}
	}
	if message == "" {
		message = "preview failed"
	}
	if statusCode <= 0 {
		statusCode = http.StatusBadGateway
	}
	writeErr(w, statusCode, BizErrInternal, message)
}
