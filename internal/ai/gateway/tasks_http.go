package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"github.com/go-chi/chi/v5"

	"xiaodou/dai/internal/ai/asynctask"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/formats"
	"xiaodou/dai/internal/ai/imageedit"
)

type decodedTaskSubmission struct {
	wireType    string
	body        []byte
	contentType string
	metadata    json.RawMessage
	webhookURL  string
}

type taskCaller struct {
	Subject coreidentity.Subject
}

func (s *Gateway) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	if s.asyncTasks == nil {
		writeOpenAIError(w, http.StatusServiceUnavailable, "Async task service is not configured.", "service_unavailable", "service_unavailable")
		return
	}
	s.withTaskAuth(w, r, s.handleCreateTaskAuthorized)
}

func (s *Gateway) handleCreateTaskAuthorized(w http.ResponseWriter, r *http.Request, caller taskCaller) {
	decoded, err := decodeTaskSubmission(w, r)
	if err != nil {
		s.writeTaskHTTPError(w, err)
		return
	}
	taskType, err := resolveTaskType(decoded.wireType)
	if err != nil {
		s.writeTaskHTTPError(w, err)
		return
	}
	responseWireType, ok := wireTaskType(taskType)
	if !ok {
		s.writeTaskHTTPError(w, asynctask.Errorf(http.StatusBadRequest, "unsupported_task_type", "task type is not public"))
		return
	}
	created, err := s.asyncTasks.Submit(r.Context(), asynctask.SubmitRequest{
		Subject:        caller.Subject,
		Type:           taskType,
		Body:           decoded.body,
		ContentType:    decoded.contentType,
		Metadata:       decoded.metadata,
		WebhookURL:     decoded.webhookURL,
		IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key")),
	})
	if err != nil {
		s.writeTaskHTTPError(w, err)
		return
	}
	view, err := s.asyncTasks.Get(r.Context(), caller.Subject, created.ID)
	if err != nil {
		s.writeTaskHTTPError(w, err)
		return
	}
	response := taskCreateResponseFromView(view, responseWireType)
	// A freshly inserted task is reported as pending deterministically, even if a
	// worker has already claimed it between insert and this read. An idempotent
	// duplicate instead reports its true current status: the original task may
	// have finished long ago, and claiming it is still pending would be a lie.
	if !created.Duplicate {
		response.Status = "pending"
	}
	writeJSON(w, http.StatusAccepted, response)
}

func (s *Gateway) handleGetTask(w http.ResponseWriter, r *http.Request) {
	if s.asyncTasks == nil {
		writeOpenAIError(w, http.StatusServiceUnavailable, "Async task service is not configured.", "service_unavailable", "service_unavailable")
		return
	}
	s.withTaskAuth(w, r, s.handleGetTaskAuthorized)
}

func (s *Gateway) handleListTasks(w http.ResponseWriter, r *http.Request) {
	if s.asyncTasks == nil {
		writeOpenAIError(w, http.StatusServiceUnavailable, "Async task service is not configured.", "service_unavailable", "service_unavailable")
		return
	}
	s.withTaskAuth(w, r, s.handleListTasksAuthorized)
}

func (s *Gateway) handleCancelTask(w http.ResponseWriter, r *http.Request) {
	if s.asyncTasks == nil {
		writeOpenAIError(w, http.StatusServiceUnavailable, "Async task service is not configured.", "service_unavailable", "service_unavailable")
		return
	}
	s.withTaskAuth(w, r, s.handleCancelTaskAuthorized)
}

func (s *Gateway) handleCancelTaskAuthorized(w http.ResponseWriter, r *http.Request, caller taskCaller) {
	taskID := strings.TrimSpace(chi.URLParam(r, "taskID"))
	if taskID == "" {
		s.writeTaskHTTPError(w, asynctask.Errorf(http.StatusBadRequest, "task_id_required", "task id is required"))
		return
	}
	current, err := s.asyncTasks.Get(r.Context(), caller.Subject, taskID)
	if err != nil {
		s.writeTaskHTTPError(w, err)
		return
	}
	wireType, ok := wireTaskType(current.Type)
	if !ok {
		s.writeTaskHTTPError(w, asynctask.ErrNotFound)
		return
	}
	cancelled, err := s.asyncTasks.Cancel(r.Context(), caller.Subject, taskID)
	if err != nil {
		s.writeTaskHTTPError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, taskGetResponseFromView(cancelled, wireType))
}

func (s *Gateway) handleListTasksAuthorized(w http.ResponseWriter, r *http.Request, caller taskCaller) {
	filter, err := decodeTaskListFilter(r)
	if err != nil {
		s.writeTaskHTTPError(w, err)
		return
	}
	page, err := s.asyncTasks.List(r.Context(), caller.Subject, filter)
	if err != nil {
		s.writeTaskHTTPError(w, err)
		return
	}
	data := make([]taskGetResponse, 0, len(page.Data))
	for _, view := range page.Data {
		wireType, ok := wireTaskType(view.Type)
		if !ok {
			continue
		}
		data = append(data, taskGetResponseFromView(view, wireType))
	}
	writeJSON(w, http.StatusOK, taskListResponse{Object: "list", Data: data, HasMore: page.HasMore})
}

func (s *Gateway) handleGetTaskAuthorized(w http.ResponseWriter, r *http.Request, caller taskCaller) {
	taskID := strings.TrimSpace(chi.URLParam(r, "taskID"))
	if taskID == "" {
		s.writeTaskHTTPError(w, asynctask.Errorf(http.StatusBadRequest, "task_id_required", "task id is required"))
		return
	}
	view, err := s.asyncTasks.Get(r.Context(), caller.Subject, taskID)
	if err != nil {
		s.writeTaskHTTPError(w, err)
		return
	}
	wireType, ok := wireTaskType(view.Type)
	if !ok {
		s.writeTaskHTTPError(w, asynctask.ErrNotFound)
		return
	}
	writeJSON(w, http.StatusOK, taskGetResponseFromView(view, wireType))
}

func (s *Gateway) withTaskAuth(
	w http.ResponseWriter,
	r *http.Request,
	next func(http.ResponseWriter, *http.Request, taskCaller),
) {
	s.runtimeAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, ok := runtimeAuthFromContext(r.Context())
		if !ok {
			writeOpenAIError(w, http.StatusUnauthorized, "Invalid API key.", "invalid_api_key", "invalid_api_key")
			return
		}
		next(w, r, taskCaller{Subject: auth.Subject})
	})).ServeHTTP(w, r)
}

func decodeTaskSubmission(w http.ResponseWriter, r *http.Request) (decodedTaskSubmission, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImageRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return decodedTaskSubmission{}, asynctask.Errorf(http.StatusBadRequest, "invalid_body",
			"invalid request body or body exceeds configured limit")
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return decodedTaskSubmission{}, asynctask.Errorf(http.StatusBadRequest, "invalid_content_type",
			"a valid Content-Type is required")
	}
	switch mediaType {
	case imageedit.TransportJSON:
		var envelope createTaskEnvelope
		decoder := json.NewDecoder(bytes.NewReader(body))
		if err := decoder.Decode(&envelope); err != nil {
			return decodedTaskSubmission{}, asynctask.Errorf(http.StatusBadRequest, "invalid_body", "invalid JSON request body")
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			return decodedTaskSubmission{}, asynctask.Errorf(http.StatusBadRequest, "invalid_body", "request body must contain one JSON object")
		}
		input := bytes.TrimSpace(envelope.Input)
		if len(input) == 0 || input[0] != '{' || !json.Valid(input) {
			return decodedTaskSubmission{}, asynctask.Errorf(http.StatusBadRequest, "invalid_task_input", "input must be a JSON object")
		}
		metadata, err := normalizeTaskMetadata(envelope.Metadata)
		if err != nil {
			return decodedTaskSubmission{}, err
		}
		return decodedTaskSubmission{
			wireType: strings.TrimSpace(envelope.Type), body: input,
			contentType: imageedit.TransportJSON, metadata: metadata,
			webhookURL: strings.TrimSpace(envelope.WebhookURL),
		}, nil
	case imageedit.TransportMultipart:
		fields, err := formats.MultipartScalarFields(body, r.Header.Get("Content-Type"), 1<<20)
		if err != nil {
			return decodedTaskSubmission{}, asynctask.Errorf(http.StatusBadRequest, "invalid_body", "%s", err.Error())
		}
		metadata, err := normalizeTaskMetadata(json.RawMessage(strings.TrimSpace(fields["metadata"])))
		if err != nil {
			return decodedTaskSubmission{}, err
		}
		return decodedTaskSubmission{
			wireType: strings.TrimSpace(fields["type"]), body: body,
			contentType: r.Header.Get("Content-Type"), metadata: metadata,
			webhookURL: strings.TrimSpace(fields["webhook_url"]),
		}, nil
	default:
		return decodedTaskSubmission{}, asynctask.Errorf(http.StatusBadRequest, "invalid_content_type",
			"application/json or multipart/form-data is required")
	}
}

func decodeTaskListFilter(r *http.Request) (asynctask.ListFilter, error) {
	query := r.URL.Query()
	types, err := publicTaskRegistryTypes(query.Get("type"))
	if err != nil {
		return asynctask.ListFilter{}, err
	}
	var status domain.TaskStatus
	if raw := strings.TrimSpace(query.Get("status")); raw != "" {
		status = domain.TaskStatus(raw)
		switch status {
		case domain.TaskPending, domain.TaskRunning, domain.TaskCompleted, domain.TaskFailed, domain.TaskCancelled:
		default:
			return asynctask.ListFilter{}, asynctask.Errorf(http.StatusBadRequest, "invalid_task_status",
				"status %q is not supported", raw)
		}
	}
	limit := 20
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			return asynctask.ListFilter{}, asynctask.Errorf(http.StatusBadRequest, "invalid_limit",
				"limit must be an integer from 1 to 100")
		}
		limit = parsed
	}
	return asynctask.ListFilter{
		Types: types, Status: status, Limit: limit,
		StartingAfter: strings.TrimSpace(query.Get("starting_after")),
	}, nil
}

func normalizeTaskMetadata(raw json.RawMessage) (json.RawMessage, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return json.RawMessage(`{}`), nil
	}
	if raw[0] != '{' || !json.Valid(raw) {
		return nil, asynctask.Errorf(http.StatusBadRequest, "invalid_metadata", "metadata must be a JSON object")
	}
	return raw, nil
}

func (s *Gateway) writeTaskHTTPError(w http.ResponseWriter, err error) {
	if errors.Is(err, asynctask.ErrNotFound) {
		writeOpenAIError(w, http.StatusNotFound, "Task not found.", "invalid_request_error", "task_not_found")
		return
	}
	var taskErr *asynctask.Error
	if errors.As(err, &taskErr) {
		errorType := "invalid_request_error"
		if taskErr.Status >= http.StatusInternalServerError {
			errorType = "server_error"
		}
		writeOpenAIError(w, taskErr.Status, taskErr.Message, errorType, taskErr.Code)
		return
	}
	// 兜底分支：非 asynctask.Error 的错误对客户端一律脱敏成 internal_error，
	// 因此必须在此留痕，否则任务提交失败在服务端不留任何线索。
	if s != nil && s.logger != nil {
		s.logger.Error("task request failed with unmapped error", zap.Error(err))
	}
	writeOpenAIError(w, http.StatusInternalServerError, "Internal server error.", "server_error", "internal_error")
}
