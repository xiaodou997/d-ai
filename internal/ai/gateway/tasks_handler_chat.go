package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strings"

	"xiaodou/dai/internal/ai/asynctask"
	coreruntime "xiaodou/dai/internal/ai/core/runtime"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/serving"
)

const (
	chatCompletionsClientPath = "/v1/chat/completions"
	chatJSONContentType       = "application/json"
)

type chatTaskInput struct {
	Body json.RawMessage `json:"body"`
}

type chatTaskHandler struct {
	admission taskAdmission
	replayer  taskReplayer
}

func (h *chatTaskHandler) Prepare(ctx context.Context, sub asynctask.Submission) (asynctask.Prepared, error) {
	if err := requireJSONContentType(sub.ContentType); err != nil {
		return asynctask.Prepared{}, err
	}
	body, model, err := normalizeChatCompletionBody(sub.Body)
	if err != nil {
		return asynctask.Prepared{}, err
	}
	if h.admission != nil {
		if err := h.admission.Admit(ctx, serving.AdmissionInput{
			Subject: sub.Subject, ModelCode: model, RequestedModel: model,
			CapabilityType: domain.CapabilityChat, ClientProtocol: domain.ProtocolOpenAIChat,
		}); err != nil {
			return asynctask.Prepared{}, chatTaskPrepareError(err)
		}
	}
	persisted, err := json.Marshal(chatTaskInput{Body: body})
	if err != nil {
		return asynctask.Prepared{}, fmt.Errorf("marshal chat task input: %w", err)
	}
	return asynctask.Prepared{Input: persisted, ModelCode: model}, nil
}

func (h *chatTaskHandler) Execute(ctx context.Context, task asynctask.Task) (asynctask.Result, error) {
	if h == nil || h.replayer == nil {
		return asynctask.Result{}, errors.New("chat task handler is not configured")
	}
	var input chatTaskInput
	if err := json.Unmarshal(task.Input, &input); err != nil {
		return asynctask.Result{}, fmt.Errorf("decode chat task input: %w", err)
	}
	body, _, err := normalizeChatCompletionBody(input.Body)
	if err != nil {
		return asynctask.Result{}, fmt.Errorf("decode persisted chat request: %w", err)
	}
	replayed := h.replayer.Replay(ctx, ReplayInput{
		Subject: task.Subject, ExecutionMode: coreruntime.ExecutionModeAsync, Capability: domain.CapabilityChat,
		Protocol: domain.ProtocolOpenAIChat, ClientPath: chatCompletionsClientPath,
		Body: body, ContentType: chatJSONContentType, RequestID: task.RequestID,
	})
	if err := ctx.Err(); err != nil {
		return asynctask.Result{}, err
	}
	return chatReplayTaskResult(replayed), nil
}

func requireJSONContentType(contentType string) error {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != chatJSONContentType {
		return asynctask.Errorf(http.StatusBadRequest, "invalid_content_type", "application/json is required for chat completions")
	}
	return nil
}

func normalizeChatCompletionBody(body []byte) (json.RawMessage, string, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil || payload == nil {
		return nil, "", asynctask.Errorf(http.StatusBadRequest, "invalid_body", "input must be a JSON object")
	}
	var model string
	if raw := payload["model"]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &model)
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, "", asynctask.Errorf(http.StatusBadRequest, "missing_required_parameter", "missing required parameter: model")
	}
	payload["stream"] = json.RawMessage("false")
	normalized, err := json.Marshal(payload)
	if err != nil {
		return nil, "", fmt.Errorf("marshal chat completions input: %w", err)
	}
	return normalized, model, nil
}

func chatReplayTaskResult(result ReplayResult) asynctask.Result {
	var output json.RawMessage
	if len(result.Body) > 0 {
		output = json.RawMessage(result.Body)
	}
	if result.Request == nil {
		message := ExtractResponseErrorMessage(result.Body)
		if message == "" {
			message = "runtime chat request failed"
		}
		return asynctask.Result{
			Status: domain.TaskFailed, Output: output,
			Failure: &asynctask.Failure{
				Code: "runtime_failed", Message: message,
				InternalDetail: string(result.Body), Step: "runtime",
			},
		}
	}
	if result.Request.RequestStatus == string(domain.RequestSuccess) && result.StatusCode < http.StatusBadRequest {
		return asynctask.Result{
			Status: domain.TaskCompleted, Output: output,
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
		message = "runtime chat request failed"
	}
	return asynctask.Result{
		Status: domain.TaskFailed, Output: output,
		CallerCharge: ReplayCallerChargeMicro(result.Request),
		Failure: &asynctask.Failure{
			Code: code, Message: message, InternalDetail: result.Request.InternalErrorDetail,
			Step: result.Request.FailedStep,
		},
	}
}

func chatTaskPrepareError(err error) error {
	var taskErr *asynctask.Error
	if errors.As(err, &taskErr) {
		return err
	}
	var apiErr *serving.APIError
	if errors.As(err, &apiErr) {
		return asynctask.Errorf(apiErr.Status, apiErr.Code, "%s", apiErr.Message)
	}
	return asynctask.Errorf(http.StatusBadRequest, "invalid_request_error", "%s", err.Error())
}
