package gateway

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"xiaodou/dai/internal/ai/application"
	"xiaodou/dai/internal/ai/asynctask"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
)

type createTaskEnvelope struct {
	Type       string          `json:"type"`
	Input      json.RawMessage `json:"input"`
	Metadata   json.RawMessage `json:"metadata"`
	WebhookURL string          `json:"webhook_url"`
}

type taskCreateResponse struct {
	ID             string          `json:"id"`
	Object         string          `json:"object"`
	Type           string          `json:"type"`
	Status         string          `json:"status"`
	Model          string          `json:"model"`
	IdempotencyKey string          `json:"idempotency_key"`
	Metadata       json.RawMessage `json:"metadata"`
	WebhookURL     string          `json:"webhook_url,omitempty"`
	CreatedAt      int64           `json:"created_at"`
}

type taskErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type taskUsageResponse struct {
	CostCredits float64 `json:"cost_credits"`
}

type taskGetResponse struct {
	taskCreateResponse
	Result      json.RawMessage    `json:"result"`
	Error       *taskErrorResponse `json:"error"`
	Usage       *taskUsageResponse `json:"usage"`
	RequestID   string             `json:"request_id"`
	Attempt     int                `json:"attempt"`
	StartedAt   *int64             `json:"started_at"`
	CompletedAt *int64             `json:"completed_at"`
}

type taskListResponse struct {
	Object  string            `json:"object"`
	Data    []taskGetResponse `json:"data"`
	HasMore bool              `json:"has_more"`
}

func taskCreateResponseFromView(view asynctask.TaskView, wireType string) taskCreateResponse {
	return taskCreateResponse{
		ID:             view.ID,
		Object:         "task",
		Type:           wireType,
		Status:         string(view.Status),
		Model:          view.ModelCode,
		IdempotencyKey: view.IdempotencyKey,
		Metadata:       view.Metadata,
		WebhookURL:     view.WebhookURL,
		CreatedAt:      view.CreatedAt.Unix(),
	}
}

func taskGetResponseFromView(view asynctask.TaskView, wireType string) taskGetResponse {
	response := taskGetResponse{
		taskCreateResponse: taskCreateResponseFromView(view, wireType),
		Result:             view.Output,
		RequestID:          view.RequestID,
		Attempt:            view.Attempt,
		StartedAt:          unixTimePointer(view.StartedAt),
		CompletedAt:        unixTimePointer(view.CompletedAt),
	}
	if view.ErrorCode != "" || view.ErrorMessage != "" {
		response.Error = &taskErrorResponse{Code: view.ErrorCode, Message: view.ErrorMessage}
	}
	if view.Status == domain.TaskCompleted || view.Status == domain.TaskFailed || view.Status == domain.TaskCancelled {
		response.Usage = &taskUsageResponse{CostCredits: domain.MicroToCreditsFloat(view.CallerCharge)}
	}
	return response
}

func unixTimePointer(value *time.Time) *int64 {
	if value == nil {
		return nil
	}
	seconds := value.Unix()
	return &seconds
}

const (
	imageGenerationWireTaskType = "images.generation"
	imageEditWireTaskType       = "images.edit"
	chatCompletionWireTaskType  = "chat.completions"

	apiImageGenerationTaskType = "api." + imageGenerationWireTaskType
	apiImageEditTaskType       = "api." + imageEditWireTaskType
	appImageGenerationTaskType = "app." + imageGenerationWireTaskType
	appImageEditTaskType       = "app." + imageEditWireTaskType
	apiChatCompletionTaskType  = "api." + chatCompletionWireTaskType
	appChatCompletionTaskType  = "app." + chatCompletionWireTaskType
)

// resolveTaskType maps the client-visible capability name to the durable
// registry key. The surface comes from authentication, never from client input.
// appType is reserved for the app-key inference added in P4.
func resolveTaskType(authMethod coreidentity.AuthMethod, wireType string, appType application.AppType) (string, error) {
	wireType = strings.TrimSpace(wireType)
	switch authMethod {
	case coreidentity.AuthMethodAPIKey:
		if wireType == "" {
			return "", asynctask.Errorf(http.StatusBadRequest, "task_type_required", "task type is required")
		}
		switch wireType {
		case imageGenerationWireTaskType:
			return apiImageGenerationTaskType, nil
		case imageEditWireTaskType:
			return apiImageEditTaskType, nil
		case chatCompletionWireTaskType:
			return apiChatCompletionTaskType, nil
		default:
			return "", asynctask.Errorf(http.StatusBadRequest, "unsupported_task_type",
				"task type %q is not supported", wireType)
		}
	case coreidentity.AuthMethodInvokeKey:
		var expectedWireType, registryType string
		switch appType {
		case application.AppTypeImageGenerationAgent:
			expectedWireType, registryType = imageGenerationWireTaskType, appImageGenerationTaskType
		case application.AppTypeImageEditAgent:
			expectedWireType, registryType = imageEditWireTaskType, appImageEditTaskType
		case application.AppTypeChatAgent:
			expectedWireType, registryType = chatCompletionWireTaskType, appChatCompletionTaskType
		default:
			return "", asynctask.Errorf(http.StatusBadRequest, "unsupported_task_type",
				"the bound application does not support async tasks")
		}
		if wireType != "" && wireType != expectedWireType {
			return "", asynctask.Errorf(http.StatusBadRequest, "task_type_mismatch",
				"task type %q does not match the bound application", wireType)
		}
		return registryType, nil
	default:
		return "", asynctask.Errorf(http.StatusBadRequest, "unsupported_task_auth_method",
			"this authentication method cannot submit tasks")
	}
}

func wireTaskType(registryType string) (string, bool) {
	switch registryType {
	case apiImageGenerationTaskType:
		return imageGenerationWireTaskType, true
	case apiImageEditTaskType:
		return imageEditWireTaskType, true
	case appImageGenerationTaskType:
		return imageGenerationWireTaskType, true
	case appImageEditTaskType:
		return imageEditWireTaskType, true
	case apiChatCompletionTaskType, appChatCompletionTaskType:
		return chatCompletionWireTaskType, true
	default:
		return "", false
	}
}

func publicTaskRegistryTypes(wireType string) ([]string, error) {
	switch strings.TrimSpace(wireType) {
	case "":
		return []string{
			apiImageGenerationTaskType, apiImageEditTaskType,
			appImageGenerationTaskType, appImageEditTaskType,
			apiChatCompletionTaskType, appChatCompletionTaskType,
		}, nil
	case imageGenerationWireTaskType:
		return []string{apiImageGenerationTaskType, appImageGenerationTaskType}, nil
	case imageEditWireTaskType:
		return []string{apiImageEditTaskType, appImageEditTaskType}, nil
	case chatCompletionWireTaskType:
		return []string{apiChatCompletionTaskType, appChatCompletionTaskType}, nil
	default:
		return nil, asynctask.Errorf(http.StatusBadRequest, "unsupported_task_type",
			"task type %q is not supported", wireType)
	}
}
