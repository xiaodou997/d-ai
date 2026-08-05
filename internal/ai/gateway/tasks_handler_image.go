package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"

	"xiaodou/dai/internal/ai/application"
	"xiaodou/dai/internal/ai/asynctask"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
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

type appImageTaskInput struct {
	Body json.RawMessage `json:"body"`
}

type imageTaskHandler struct {
	operation string
	admission taskAdmission
	replayer  taskReplayer
}

type appImageTaskHandler struct {
	operation      string
	admission      taskAdmission
	replayer       taskReplayer
	invokeExpander taskRuntimeInvokeExpander
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
	engine.Register(appImageGenerationTaskType, &appImageTaskHandler{
		operation: "generation", admission: s.taskAdmission, replayer: s,
		invokeExpander: s.taskInvokeExpander,
	}, asynctask.Options{MaxAttempts: 1})
	engine.Register(appImageEditTaskType, &appImageTaskHandler{
		operation: "edit", admission: s.taskAdmission, replayer: s,
		invokeExpander: s.taskInvokeExpander,
	}, asynctask.Options{MaxAttempts: 1})
	engine.Register(apiChatCompletionTaskType, &chatTaskHandler{
		admission: s.taskAdmission, replayer: s,
	}, asynctask.Options{MaxAttempts: 1})
	engine.Register(appChatCompletionTaskType, &appChatTaskHandler{
		admission: s.taskAdmission, replayer: s, invokeExpander: s.taskInvokeExpander,
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

func (h *appImageTaskHandler) Prepare(ctx context.Context, sub asynctask.Submission) (asynctask.Prepared, error) {
	expansion, err := h.expand(ctx, sub.Subject)
	if err != nil {
		return asynctask.Prepared{}, imageTaskPrepareError(err)
	}
	persisted, runtimeBody, err := h.prepareInput(sub.Body, sub.ContentType, expansion)
	if err != nil {
		return asynctask.Prepared{}, imageTaskPrepareError(err)
	}
	if h.admission != nil {
		if err := h.admission.Admit(ctx, serving.AdmissionInput{
			Subject:        expansion.Subject,
			ModelCode:      expansion.BoundModel,
			RequestedModel: expansion.BoundModel,
			CapabilityType: domain.CapabilityImage,
			ClientProtocol: domain.ProtocolOpenAIImages,
		}); err != nil {
			return asynctask.Prepared{}, imageTaskPrepareError(err)
		}
	}
	if len(runtimeBody) == 0 {
		return asynctask.Prepared{}, errors.New("app image task produced no runtime body")
	}
	return asynctask.Prepared{Input: persisted, ModelCode: expansion.BoundModel}, nil
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

func (h *appImageTaskHandler) Execute(ctx context.Context, task asynctask.Task) (asynctask.Result, error) {
	if h == nil || h.replayer == nil || h.invokeExpander == nil {
		return asynctask.Result{}, errors.New("app image task handler is not configured")
	}
	var input appImageTaskInput
	if err := json.Unmarshal(task.Input, &input); err != nil {
		return asynctask.Result{}, fmt.Errorf("decode app image task input: %w", err)
	}
	expansion, err := h.expand(ctx, task.Subject)
	if err != nil {
		return appImageTaskResolutionFailure(err), nil
	}
	runtimeBody := []byte(input.Body)
	if len(runtimeBody) == 0 {
		return asynctask.Result{}, errors.New("app image task produced no persisted runtime body")
	}
	meta, err := formats.ParseRequestMeta(runtimeBody, imageedit.TransportJSON)
	if err != nil {
		return asynctask.Result{}, fmt.Errorf("decode app image runtime body: %w", err)
	}
	clientPath, err := imageTaskClientPath(h.operation)
	if err != nil {
		return asynctask.Result{}, err
	}
	replayed := h.replayer.Replay(ctx, ReplayInput{
		Subject:           expansion.Subject,
		ExecutionMode:     coreruntime.ExecutionModeAsync,
		Capability:        domain.CapabilityImage,
		Protocol:          domain.ProtocolOpenAIImages,
		ClientPath:        clientPath,
		Body:              runtimeBody,
		ContentType:       imageedit.TransportJSON,
		RequestID:         task.RequestID,
		StreamExpected:    meta.Stream,
		HideRevisedPrompt: true,
	})
	if err := ctx.Err(); err != nil {
		return asynctask.Result{}, err
	}
	return imageReplayTaskResult(replayed), nil
}

func (h *appImageTaskHandler) prepareInput(
	body []byte,
	contentType string,
	expansion coreruntime.InvokeExpansion,
) (json.RawMessage, []byte, error) {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, nil, asynctask.Errorf(http.StatusBadRequest, "invalid_content_type", "a valid Content-Type is required")
	}
	var runtimeBody []byte
	switch h.operation {
	case "generation":
		if mediaType != imageedit.TransportJSON {
			return nil, nil, asynctask.Errorf(http.StatusBadRequest, "invalid_content_type", "application/json is required for image generation")
		}
		req, err := decodeRunImageGenerationRequestBody(body)
		if err != nil {
			return nil, nil, err
		}
		runtimeBody, err = buildRunImageGenerationBodyFromExpansion(expansion, req)
	case "edit":
		if mediaType != imageedit.TransportJSON && mediaType != imageedit.TransportMultipart {
			return nil, nil, asynctask.Errorf(http.StatusBadRequest, "invalid_content_type", "application/json or multipart/form-data is required for image edits")
		}
		req, variables, err := decodeAppRunImageEditRequestBody(body, contentType)
		if err != nil {
			return nil, nil, err
		}
		runtimeBody, err = buildRunImageEditBodyFromExpansion(expansion, req, variables)
	default:
		return nil, nil, fmt.Errorf("unsupported app image operation %q", h.operation)
	}
	if err != nil {
		return nil, nil, err
	}
	persisted, err := json.Marshal(appImageTaskInput{Body: runtimeBody})
	if err != nil {
		return nil, nil, err
	}
	return persisted, runtimeBody, nil
}

func (h *appImageTaskHandler) expand(ctx context.Context, subject coreidentity.Subject) (coreruntime.InvokeExpansion, error) {
	if h == nil || h.invokeExpander == nil {
		return coreruntime.InvokeExpansion{}, errors.New("app image task expander is not configured")
	}
	expansion, err := h.invokeExpander.ExpandByKeyID(
		ctx, subject.Scope, subject.TenantID, subject.UserID, subject.InvokeKeyID, coreruntime.Request{},
	)
	if err != nil {
		return coreruntime.InvokeExpansion{}, err
	}
	if err := validateTaskInvokeExpansion(expansion); err != nil {
		return coreruntime.InvokeExpansion{}, err
	}
	expectedType := application.AppTypeImageGenerationAgent
	if h.operation == "edit" {
		expectedType = application.AppTypeImageEditAgent
	}
	if expansion.App.App.AppType != expectedType {
		return coreruntime.InvokeExpansion{}, fmt.Errorf("bound app type %q does not match %s", expansion.App.App.AppType, h.operation)
	}
	return expansion, nil
}

func appImageTaskResolutionFailure(err error) asynctask.Result {
	return asynctask.Result{
		Status: domain.TaskFailed,
		Failure: &asynctask.Failure{
			Code: "task_resolution_failed", Message: "the image task can no longer be resolved",
			InternalDetail: err.Error(), Step: "resolve",
		},
	}
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
