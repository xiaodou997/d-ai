package console

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"xiaodou/dai/internal/ai/asynctask"
	"xiaodou/dai/internal/ai/audit"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	coreruntime "xiaodou/dai/internal/ai/core/runtime"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/gateway"
	"xiaodou/dai/internal/ai/imageassets"
	"xiaodou/dai/internal/ai/imageedit"
)

const (
	consoleImageGenerationTaskType           = "console.images.generation"
	consoleImageEditTaskType                 = "console.images.edit"
	defaultConsoleImageTaskMaxUploadBodySize = 64 << 20
)

// consoleImageReplayer is the consumer-owned seam around gateway.Replay. It
// keeps handler tests independent from a live runtime pipeline.
type consoleImageReplayer interface {
	Replay(context.Context, gateway.ReplayInput) gateway.ReplayResult
}

type consoleImageTaskResolver interface {
	prepareConsoleImageTask(
		context.Context,
		coreidentity.Subject,
		[]byte,
		string,
		string,
	) (consoleImageResolution, error)
	resolveConsoleImageTask(
		context.Context,
		coreidentity.Subject,
		consoleImageTaskInputPayload,
		string,
	) (gateway.ReplayInput, error)
}

type consoleImageTaskAssets interface {
	DeleteTaskAssets(string) (int, error)
}

type consoleImageTaskHandler struct {
	resolver  consoleImageTaskResolver
	operation string
	replayer  consoleImageReplayer
	assets    consoleImageTaskAssets
}

// RegisterImageTaskHandlers binds the console surface before the composition
// root starts the engine. Console construction itself launches no goroutines.
func (s *Console) RegisterImageTaskHandlers(engine *asynctask.Engine) {
	if s == nil || engine == nil {
		panic("console: async task engine is required")
	}
	if s.asyncTasks != nil {
		panic("console: image task handlers registered twice")
	}
	s.asyncTasks = engine
	engine.Register(consoleImageGenerationTaskType, &consoleImageTaskHandler{
		resolver: s, operation: "generation", replayer: s.gateway, assets: s.imageAssets,
	}, asynctask.Options{MaxAttempts: 1, TTL: s.asyncTaskConfig.Retention})
	engine.Register(consoleImageEditTaskType, &consoleImageTaskHandler{
		resolver: s, operation: "edit", replayer: s.gateway, assets: s.imageAssets,
	}, asynctask.Options{MaxAttempts: 1, TTL: s.asyncTaskConfig.Retention})
}

func (h *consoleImageTaskHandler) OnExpire(_ context.Context, task asynctask.Task) error {
	if h == nil || h.assets == nil {
		return nil
	}
	_, err := h.assets.DeleteTaskAssets(task.ID)
	return err
}

func (h *consoleImageTaskHandler) Prepare(ctx context.Context, sub asynctask.Submission) (asynctask.Prepared, error) {
	if h == nil || h.resolver == nil {
		return asynctask.Prepared{}, errors.New("console image task handler is not configured")
	}
	resolved, err := h.resolver.prepareConsoleImageTask(ctx, sub.Subject, sub.Body, sub.ContentType, h.operation)
	if err != nil {
		return asynctask.Prepared{}, err
	}
	return asynctask.Prepared{Input: resolved.Input, ModelCode: resolved.ModelCode}, nil
}

func (h *consoleImageTaskHandler) Execute(ctx context.Context, task asynctask.Task) (asynctask.Result, error) {
	if h == nil || h.resolver == nil || h.replayer == nil {
		return asynctask.Result{}, errors.New("console image task handler is not configured")
	}
	var input consoleImageTaskInputPayload
	if err := json.Unmarshal(task.Input, &input); err != nil {
		return asynctask.Result{}, fmt.Errorf("decode console image task input: %w", err)
	}
	if input.Operation == "" {
		input.Operation = h.operation
	}
	if input.Operation != h.operation {
		return asynctask.Result{}, fmt.Errorf("task type %q contains operation %q", task.Type, input.Operation)
	}
	replay, err := h.resolver.resolveConsoleImageTask(ctx, task.Subject, input, h.operation)
	if err != nil {
		return asynctask.Result{
			Status: domain.TaskFailed,
			Failure: &asynctask.Failure{
				Code:           "task_resolution_failed",
				Message:        "the image task can no longer be resolved",
				InternalDetail: err.Error(),
				Step:           "resolve",
			},
		}, nil
	}
	replay.RequestID = task.RequestID
	replay.ExecutionMode = coreruntime.ExecutionModeAsync
	result := h.replayer.Replay(ctx, replay)
	if err := ctx.Err(); err != nil {
		return asynctask.Result{}, err
	}
	return consoleImageReplayTaskResult(result), nil
}

func consoleImageReplayTaskResult(result gateway.ReplayResult) asynctask.Result {
	output := buildConsoleImageTaskResultPayload(result.Body, nil, "", false)
	if output == nil && len(result.Body) > 0 {
		output = json.RawMessage(result.Body)
	}
	if result.Request == nil {
		message := gateway.ExtractResponseErrorMessage(result.Body)
		if message == "" {
			message = "runtime image request failed"
		}
		return asynctask.Result{
			Status: domain.TaskFailed,
			Output: output,
			Failure: &asynctask.Failure{
				Code:           "runtime_failed",
				Message:        message,
				InternalDetail: string(result.Body),
				Step:           "runtime",
			},
		}
	}
	if result.Request.RequestStatus == string(domain.RequestSuccess) && result.StatusCode < http.StatusBadRequest {
		return asynctask.Result{
			Status:       domain.TaskCompleted,
			Output:       output,
			CallerCharge: gateway.ReplayCallerChargeMicro(result.Request),
		}
	}
	message := gateway.ExtractResponseErrorMessage(result.Body)
	if message == "" {
		message = result.Request.ErrorMessage
	}
	return asynctask.Result{
		Status:       domain.TaskFailed,
		Output:       output,
		CallerCharge: gateway.ReplayCallerChargeMicro(result.Request),
		Failure: &asynctask.Failure{
			Code:           result.Request.ErrorCode,
			Message:        message,
			InternalDetail: result.Request.InternalErrorDetail,
			Step:           result.Request.FailedStep,
		},
	}
}

func (s *Console) consoleImageTaskMaxUploadBodySize() int64 {
	if s == nil || s.asyncTaskConfig.MaxUploadBodySize <= 0 {
		return defaultConsoleImageTaskMaxUploadBodySize
	}
	return s.asyncTaskConfig.MaxUploadBodySize
}

func (s *Console) handleConsoleImageCreateTask(w http.ResponseWriter, r *http.Request) {
	subject, ok := s.consoleRuntimeSubject(w, r)
	if !ok {
		return
	}
	if s.asyncTasks == nil {
		writeErr(w, http.StatusServiceUnavailable, BizErrInternal, "async task service is not configured")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.consoleImageTaskMaxUploadBodySize())
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid request body or body exceeds configured limit")
		return
	}
	taskType, err := consoleImageSubmissionTaskType(body, r.Header.Get("Content-Type"))
	if err != nil {
		writeConsoleImageTaskErr(w, err)
		return
	}
	created, err := s.asyncTasks.Submit(r.Context(), asynctask.SubmitRequest{
		Subject:     *subject,
		Type:        taskType,
		Body:        body,
		ContentType: r.Header.Get("Content-Type"),
	})
	if err != nil {
		writeConsoleImageTaskErr(w, err)
		return
	}
	writeOK(w, consoleImageTaskCreateResponse{TaskID: created.ID, Status: string(domain.TaskPending)})
}

func consoleImageSubmissionTaskType(body []byte, contentType string) (string, error) {
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if mediaType == imageedit.TransportMultipart {
		return consoleImageEditTaskType, nil
	}
	var head struct {
		Operation string `json:"operation"`
	}
	// Full validation belongs to Handler.Prepare. If this lightweight decode
	// fails, route to generation so Prepare returns the canonical body error.
	if err := json.Unmarshal(body, &head); err != nil {
		return consoleImageGenerationTaskType, nil
	}
	switch strings.TrimSpace(head.Operation) {
	case "", "generation":
		return consoleImageGenerationTaskType, nil
	case "edit":
		return consoleImageEditTaskType, nil
	default:
		return "", domain.NewValidationError("operation", "operation must be generation or edit")
	}
}

func (s *Console) handleConsoleImageGetTask(w http.ResponseWriter, r *http.Request) {
	subject, ok := s.consoleRuntimeSubject(w, r)
	if !ok {
		return
	}
	taskID := strings.TrimSpace(chi.URLParam(r, "taskID"))
	if taskID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "task_id is required")
		return
	}
	task, err := s.getConsoleImageTask(r.Context(), subject, taskID)
	if err != nil {
		writeConsoleImageTaskErr(w, err)
		return
	}
	writeOK(w, task)
}

func (s *Console) handleConsoleImageCancelTask(w http.ResponseWriter, r *http.Request) {
	subject, ok := s.consoleRuntimeSubject(w, r)
	if !ok {
		return
	}
	taskID := strings.TrimSpace(chi.URLParam(r, "taskID"))
	if taskID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "task_id is required")
		return
	}
	view, err := s.getConsoleImageTaskView(r.Context(), *subject, taskID)
	if err != nil {
		writeConsoleImageTaskErr(w, err)
		return
	}
	if view.Status == domain.TaskPending || view.Status == domain.TaskRunning {
		view, err = s.asyncTasks.Cancel(r.Context(), *subject, taskID)
		if err != nil {
			var taskErr *asynctask.Error
			if !errors.As(err, &taskErr) || taskErr.Code != "task_not_cancellable" {
				writeConsoleImageTaskErr(w, err)
				return
			}
			view, err = s.getConsoleImageTaskView(r.Context(), *subject, taskID)
			if err != nil {
				writeConsoleImageTaskErr(w, err)
				return
			}
		}
	}
	dto := consoleImageTaskDTOFromView(view)
	s.refreshConsoleImageAssetURLs(r.Context(), dto.Assets)
	writeOK(w, dto)
}

func (s *Console) handleConsoleImageDeleteTask(w http.ResponseWriter, r *http.Request) {
	subject, ok := s.consoleRuntimeSubject(w, r)
	if !ok {
		return
	}
	taskID := strings.TrimSpace(chi.URLParam(r, "taskID"))
	if taskID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "task_id is required")
		return
	}
	if _, err := s.getConsoleImageTaskView(r.Context(), *subject, taskID); err != nil {
		writeConsoleImageTaskErr(w, err)
		return
	}
	if err := s.asyncTasks.DeleteTerminal(r.Context(), *subject, taskID); err != nil {
		writeConsoleImageTaskErr(w, err)
		return
	}
	writeOK(w, struct {
		Deleted bool `json:"deleted"`
	}{Deleted: true})
}

func (s *Console) getConsoleImageTask(ctx context.Context, subject *coreidentity.Subject, taskID string) (consoleImageTaskDTO, error) {
	if subject == nil {
		return consoleImageTaskDTO{}, asynctask.ErrNotFound
	}
	view, err := s.getConsoleImageTaskView(ctx, *subject, taskID)
	if err != nil {
		return consoleImageTaskDTO{}, err
	}
	dto := consoleImageTaskDTOFromView(view)
	s.refreshConsoleImageAssetURLs(ctx, dto.Assets)
	return dto, nil
}

func (s *Console) getConsoleImageTaskView(ctx context.Context, subject coreidentity.Subject, taskID string) (asynctask.TaskView, error) {
	if s == nil || s.asyncTasks == nil {
		return asynctask.TaskView{}, errors.New("async task service is not configured")
	}
	view, err := s.asyncTasks.Get(ctx, subject, taskID)
	if err != nil {
		return asynctask.TaskView{}, err
	}
	if view.Type != consoleImageGenerationTaskType && view.Type != consoleImageEditTaskType {
		return asynctask.TaskView{}, asynctask.ErrNotFound
	}
	return view, nil
}

func consoleImageTaskDTOFromView(view asynctask.TaskView) consoleImageTaskDTO {
	dto := consoleImageTaskDTO{}
	dto.ID = view.ID
	dto.Operation = imageTaskOperation(view.Type)
	dto.ModelCode = view.ModelCode
	dto.Status = string(view.Status)
	dto.StoragePolicy = "auto"
	dto.RawImageRetained = false
	dto.CallerChargeUSD = domain.MicroToUSD(view.CallerCharge)
	dto.ErrorMessage = view.ErrorMessage
	dto.CreatedAt = view.CreatedAt.UnixMilli()
	if view.CompletedAt != nil {
		completedAt := view.CompletedAt.UnixMilli()
		dto.CompletedAt = &completedAt
	}
	fillConsoleImageJobInput(&dto.consoleImageJobDTO, view.Input)
	fillConsoleImageJobSummary(&dto.consoleImageJobDTO, view.Output)
	dto.Assets = extractConsoleImageTaskAssets(view.Output)
	return dto
}

func imageTaskOperation(taskType string) string {
	if taskType == consoleImageEditTaskType {
		return "edit"
	}
	return "generation"
}

func writeConsoleImageTaskErr(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	if errors.Is(err, asynctask.ErrNotFound) {
		writeErr(w, http.StatusNotFound, BizErrNotFound, "not found")
		return
	}
	var taskErr *asynctask.Error
	if errors.As(err, &taskErr) {
		bizCode := BizErrInternal
		switch taskErr.Status {
		case http.StatusBadRequest, http.StatusTooManyRequests:
			bizCode = BizErrBadRequest
		case http.StatusUnauthorized, http.StatusForbidden:
			bizCode = BizErrForbidden
		case http.StatusNotFound:
			bizCode = BizErrNotFound
		case http.StatusConflict:
			bizCode = BizErrConflict
		}
		writeErr(w, taskErr.Status, bizCode, taskErr.Message)
		return
	}
	writeConsoleImagePrepareErr(w, err)
}

type consoleImageTaskCreateResponse struct {
	TaskID string `json:"task_id"`
	Status string `json:"status"`
}

type consoleImageTaskDTO struct {
	consoleImageJobDTO
}

type consoleImageSummary struct {
	ImageCount  int `json:"image_count"`
	InlineCount int `json:"inline_count"`
	URLCount    int `json:"url_count"`
	Items       []struct {
		RevisedPrompt string `json:"revised_prompt"`
	} `json:"items"`
}

type consoleImageTaskInputPayload struct {
	Operation         string               `json:"operation"`
	Model             string               `json:"model"`
	GroupID           string               `json:"group_id"`
	Prompt            string               `json:"prompt"`
	N                 int                  `json:"n"`
	Images            []consoleImageSource `json:"images"`
	Mask              *consoleImageSource  `json:"mask"`
	Size              string               `json:"size"`
	ResponseFormat    string               `json:"response_format"`
	Background        string               `json:"background"`
	InputFidelity     string               `json:"input_fidelity"`
	Moderation        string               `json:"moderation"`
	OutputFormat      string               `json:"output_format"`
	OutputCompression *int                 `json:"output_compression"`
	User              string               `json:"user"`
	EditRequest       json.RawMessage      `json:"edit_request"`
}

func fillConsoleImageJobInput(dto *consoleImageJobDTO, raw []byte) {
	if len(raw) == 0 {
		return
	}
	var payload struct {
		Operation      string `json:"operation"`
		GroupID        string `json:"group_id"`
		Prompt         string `json:"prompt"`
		N              int    `json:"n"`
		Size           string `json:"size"`
		Quality        string `json:"quality"`
		Style          string `json:"style"`
		ResponseFormat string `json:"response_format"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return
	}
	if payload.Operation != "" {
		dto.Operation = payload.Operation
	}
	dto.GroupID = strings.TrimSpace(payload.GroupID)
	dto.RetryPrompt = strings.TrimSpace(payload.Prompt)
	dto.Prompt = strings.TrimSpace(payload.Prompt)
	if payload.N > 0 {
		dto.RequestedOutputCount = payload.N
	} else {
		dto.RequestedOutputCount = domain.DefaultImageOutputCount
	}
	dto.Size = payload.Size
	dto.Quality = payload.Quality
	dto.Style = payload.Style
	dto.ResponseFormat = payload.ResponseFormat
}

func fillConsoleImageJobSummary(dto *consoleImageJobDTO, raw []byte) {
	if len(raw) == 0 {
		return
	}
	var summary consoleImageSummary
	if err := json.Unmarshal(raw, &summary); err != nil {
		return
	}
	var payload struct {
		StoragePolicy    string `json:"storage_policy"`
		RawImageRetained *bool  `json:"raw_image_retained"`
	}
	_ = json.Unmarshal(raw, &payload)
	dto.ImageCount = summary.ImageCount
	dto.InlineCount = summary.InlineCount
	dto.URLCount = summary.URLCount
	if strings.TrimSpace(payload.StoragePolicy) != "" {
		dto.StoragePolicy = strings.TrimSpace(payload.StoragePolicy)
	}
	if payload.RawImageRetained != nil {
		dto.RawImageRetained = *payload.RawImageRetained
	}
	revised := make([]string, 0, len(summary.Items))
	for _, item := range summary.Items {
		if strings.TrimSpace(item.RevisedPrompt) == "" {
			continue
		}
		revised = append(revised, item.RevisedPrompt)
	}
	dto.RevisedPrompts = revised
}

func buildConsoleImageTaskResultPayload(responseBody []byte, stored []imageassets.StoredAsset, storagePolicy string, rawImageRetained bool) json.RawMessage {
	if len(responseBody) == 0 && len(stored) == 0 {
		return nil
	}
	result := map[string]any{}
	if len(responseBody) > 0 {
		summary := audit.SummarizeImagesResponse(responseBody)
		if len(summary) > 0 {
			_ = json.Unmarshal(summary, &result)
		}
	}
	assets := consoleStoredImageAssetsToDTO(stored)
	if len(assets) == 0 {
		assets = extractConsoleImageTaskAssets(responseBody)
	}
	if len(assets) > 0 {
		result["assets"] = assets
	}
	if strings.TrimSpace(storagePolicy) != "" {
		result["storage_policy"] = storagePolicy
		result["raw_image_retained"] = rawImageRetained
	}
	if len(result) == 0 {
		return nil
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return nil
	}
	return raw
}
