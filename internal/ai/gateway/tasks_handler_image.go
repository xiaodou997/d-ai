package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"

	"xiaodou/dai/internal/ai/asynctask"
	coreruntime "xiaodou/dai/internal/ai/core/runtime"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/formats"
	"xiaodou/dai/internal/ai/imageedit"
	"xiaodou/dai/internal/ai/serving"
)

type taskAdmission interface {
	Admit(context.Context, serving.AdmissionInput) error
}

type taskReplayer interface {
	Replay(context.Context, ReplayInput) ReplayResult
}

type imageTaskInput struct {
	Body        json.RawMessage `json:"body"`
	ContentType string          `json:"content_type"`
}

type imageTaskHandler struct {
	operation string
	admission taskAdmission
	replayer  taskReplayer
}

// RegisterTaskHandlers binds every public async capability before the
// composition root starts the engine.
func (s *Gateway) RegisterTaskHandlers(engine *asynctask.Engine) {
	if s == nil || engine == nil {
		panic("gateway: async task engine is required")
	}
	if s.asyncTasks != nil && s.asyncTasks != engine {
		panic("gateway: a different async task engine is already configured")
	}
	s.asyncTasks = engine
	engine.Register(apiImageGenerationTaskType, &imageTaskHandler{
		operation: "generation", admission: s.taskAdmission, replayer: s,
	}, asynctask.Options{MaxAttempts: 1})
	engine.Register(apiImageEditTaskType, &imageTaskHandler{
		operation: "edit", admission: s.taskAdmission, replayer: s,
	}, asynctask.Options{MaxAttempts: 1})
	engine.Register(apiChatCompletionTaskType, &chatTaskHandler{
		admission: s.taskAdmission, replayer: s,
	}, asynctask.Options{MaxAttempts: 1})
}

func (h *imageTaskHandler) Prepare(ctx context.Context, sub asynctask.Submission) (asynctask.Prepared, error) {
	clientPath, err := h.clientPath()
	if err != nil {
		return asynctask.Prepared{}, err
	}
	mediaType, _, mediaErr := mime.ParseMediaType(sub.ContentType)
	if mediaErr != nil {
		return asynctask.Prepared{}, asynctask.Errorf(http.StatusBadRequest, "invalid_content_type",
			"a valid Content-Type is required")
	}
	if h.operation == "generation" && mediaType != imageedit.TransportJSON {
		return asynctask.Prepared{}, asynctask.Errorf(http.StatusBadRequest, "invalid_content_type",
			"application/json is required for image generation")
	}
	if h.operation == "edit" && mediaType != imageedit.TransportJSON && mediaType != imageedit.TransportMultipart {
		return asynctask.Prepared{}, asynctask.Errorf(http.StatusBadRequest, "invalid_content_type",
			"application/json or multipart/form-data is required for image edits")
	}
	if err := validateOpenAIImageInputLimits(sub.Body, sub.ContentType); err != nil {
		return asynctask.Prepared{}, imageTaskPrepareError(err)
	}
	body, contentType, err := normalizeOpenAIImageRuntimeRequest(sub.Body, sub.ContentType, clientPath)
	if err != nil {
		return asynctask.Prepared{}, imageTaskPrepareError(err)
	}
	meta, err := formats.ParseRequestMeta(body, contentType)
	if err != nil {
		return asynctask.Prepared{}, asynctask.Errorf(http.StatusBadRequest, "invalid_body",
			"invalid request body: %v", err)
	}
	if meta.Model == "" {
		return asynctask.Prepared{}, asynctask.Errorf(http.StatusBadRequest, "missing_required_parameter",
			"missing required parameter: model")
	}
	if h.admission != nil {
		if err := h.admission.Admit(ctx, serving.AdmissionInput{
			Subject:        sub.Subject,
			ModelCode:      meta.Model,
			RequestedModel: meta.Model,
			CapabilityType: domain.CapabilityImage,
			ClientProtocol: domain.ProtocolOpenAIImages,
		}); err != nil {
			return asynctask.Prepared{}, imageTaskPrepareError(err)
		}
	}
	persisted, err := json.Marshal(imageTaskInput{
		Body:        json.RawMessage(body),
		ContentType: contentType,
	})
	if err != nil {
		return asynctask.Prepared{}, fmt.Errorf("marshal image task input: %w", err)
	}
	return asynctask.Prepared{Input: persisted, ModelCode: meta.Model}, nil
}

func (h *imageTaskHandler) Execute(ctx context.Context, task asynctask.Task) (asynctask.Result, error) {
	if h == nil || h.replayer == nil {
		return asynctask.Result{}, errors.New("image task handler is not configured")
	}
	clientPath, err := h.clientPath()
	if err != nil {
		return asynctask.Result{}, err
	}
	var input imageTaskInput
	if err := json.Unmarshal(task.Input, &input); err != nil {
		return asynctask.Result{}, fmt.Errorf("decode image task input: %w", err)
	}
	meta, err := formats.ParseRequestMeta(input.Body, input.ContentType)
	if err != nil {
		return asynctask.Result{}, fmt.Errorf("decode persisted image request: %w", err)
	}
	replayed := h.replayer.Replay(ctx, ReplayInput{
		Subject:        task.Subject,
		ExecutionMode:  coreruntime.ExecutionModeAsync,
		Capability:     domain.CapabilityImage,
		Protocol:       domain.ProtocolOpenAIImages,
		ClientPath:     clientPath,
		Body:           input.Body,
		ContentType:    input.ContentType,
		RequestID:      task.RequestID,
		StreamExpected: meta.Stream,
	})
	if err := ctx.Err(); err != nil {
		return asynctask.Result{}, err
	}
	return imageReplayTaskResult(replayed), nil
}

func imageReplayTaskResult(result ReplayResult) asynctask.Result {
	var output json.RawMessage
	if len(result.Body) > 0 {
		output = json.RawMessage(result.Body)
	}
	if result.Request == nil {
		message := ExtractResponseErrorMessage(result.Body)
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
			CallerCharge: ReplayCallerChargeMicro(result.Request),
		}
	}
	code := result.Request.ErrorCode
	if code == "" {
		code = "runtime_failed"
	}
	message := ExtractResponseErrorMessage(result.Body)
	if message == "" {
		message = result.Request.ErrorMessage
	}
	if message == "" {
		message = "runtime image request failed"
	}
	return asynctask.Result{
		Status:       domain.TaskFailed,
		Output:       output,
		CallerCharge: ReplayCallerChargeMicro(result.Request),
		Failure: &asynctask.Failure{
			Code:           code,
			Message:        message,
			InternalDetail: result.Request.InternalErrorDetail,
			Step:           result.Request.FailedStep,
		},
	}
}

func (h *imageTaskHandler) clientPath() (string, error) {
	if h == nil {
		return "", errors.New("image task handler is not configured")
	}
	return imageTaskClientPath(h.operation)
}

func imageTaskClientPath(operation string) (string, error) {
	switch operation {
	case "generation":
		return "/v1/images/generations", nil
	case "edit":
		return "/v1/images/edits", nil
	default:
		return "", fmt.Errorf("unsupported image task operation %q", operation)
	}
}

func imageTaskPrepareError(err error) error {
	var apiErr *serving.APIError
	if errors.As(err, &apiErr) {
		return asynctask.Errorf(apiErr.Status, apiErr.Code, "%s", apiErr.Message)
	}
	return asynctask.Errorf(http.StatusBadRequest, "invalid_request_error", "%s", err.Error())
}
