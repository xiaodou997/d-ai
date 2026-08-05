package console

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"xiaodou/dai/internal/ai/asynctask"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
)

type portalTaskOwnerDTO struct {
	Scope  string `json:"scope"`
	UserID string `json:"user_id,omitempty"`
}

type portalTaskPermissionsDTO struct {
	ReadOnly  bool `json:"read_only"`
	CanCancel bool `json:"can_cancel"`
	CanDelete bool `json:"can_delete"`
}

type portalTaskErrorDTO struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type portalTaskUsageDTO struct {
	CostCredits float64 `json:"cost_credits"`
}

type portalTaskResultSummaryDTO struct {
	ImageCount  int `json:"image_count,omitempty"`
	ChoiceCount int `json:"choice_count,omitempty"`
}

type portalTaskDTO struct {
	ID          string                   `json:"id"`
	Type        string                   `json:"type"`
	Source      string                   `json:"source"`
	Status      string                   `json:"status"`
	Model       string                   `json:"model,omitempty"`
	Owner       portalTaskOwnerDTO       `json:"owner"`
	Permissions portalTaskPermissionsDTO `json:"permissions"`

	RequestID string `json:"request_id,omitempty"`
	Attempt   int    `json:"attempt"`

	Error           *portalTaskErrorDTO         `json:"error,omitempty"`
	Usage           *portalTaskUsageDTO         `json:"usage,omitempty"`
	ResultAvailable bool                        `json:"result_available"`
	ResultSummary   *portalTaskResultSummaryDTO `json:"result_summary,omitempty"`
	Result          json.RawMessage             `json:"result,omitempty"`

	CreatedAt   int64  `json:"created_at"`
	StartedAt   *int64 `json:"started_at,omitempty"`
	CompletedAt *int64 `json:"completed_at,omitempty"`
}

type portalTaskListDTO struct {
	Items   []portalTaskDTO `json:"items"`
	HasMore bool            `json:"has_more"`
}

func (s *Console) handlePortalTaskList(w http.ResponseWriter, r *http.Request) {
	subject, ok := s.consoleRuntimeSubject(w, r)
	if !ok {
		return
	}
	if s.asyncTasks == nil {
		writeErr(w, http.StatusServiceUnavailable, BizErrInternal, "async task service is not configured")
		return
	}
	filter, err := decodePortalTaskListFilter(r)
	if err != nil {
		writeConsoleImageTaskErr(w, err)
		return
	}
	page, err := s.asyncTasks.List(r.Context(), *subject, filter)
	if err != nil {
		writeConsoleImageTaskErr(w, err)
		return
	}
	items := make([]portalTaskDTO, 0, len(page.Data))
	for _, view := range page.Data {
		if _, ok := portalTaskWireType(view.Type); !ok {
			continue
		}
		items = append(items, portalTaskDTOFromView(view, *subject, false))
	}
	writeOK(w, portalTaskListDTO{Items: items, HasMore: page.HasMore})
}

func (s *Console) handlePortalTaskGet(w http.ResponseWriter, r *http.Request) {
	subject, view, ok := s.portalTaskFromRequest(w, r)
	if !ok {
		return
	}
	writeOK(w, portalTaskDTOFromView(view, *subject, true))
}

func (s *Console) handlePortalTaskCancel(w http.ResponseWriter, r *http.Request) {
	subject, view, ok := s.portalTaskFromRequest(w, r)
	if !ok {
		return
	}
	if !portalCanManageTask(*subject, view) {
		writeErr(w, http.StatusForbidden, BizErrForbidden, "user tasks are read-only in the tenant portal")
		return
	}
	updated, err := s.asyncTasks.Cancel(r.Context(), *subject, view.ID)
	if err != nil {
		writeConsoleImageTaskErr(w, err)
		return
	}
	writeOK(w, portalTaskDTOFromView(updated, *subject, true))
}

func (s *Console) handlePortalTaskDelete(w http.ResponseWriter, r *http.Request) {
	subject, view, ok := s.portalTaskFromRequest(w, r)
	if !ok {
		return
	}
	if !portalCanManageTask(*subject, view) {
		writeErr(w, http.StatusForbidden, BizErrForbidden, "user tasks are read-only in the tenant portal")
		return
	}
	if err := s.asyncTasks.DeleteTerminal(r.Context(), *subject, view.ID); err != nil {
		writeConsoleImageTaskErr(w, err)
		return
	}
	writeOK(w, struct {
		Deleted bool `json:"deleted"`
	}{Deleted: true})
}

func (s *Console) portalTaskFromRequest(
	w http.ResponseWriter,
	r *http.Request,
) (*coreidentity.Subject, asynctask.TaskView, bool) {
	subject, ok := s.consoleRuntimeSubject(w, r)
	if !ok {
		return nil, asynctask.TaskView{}, false
	}
	if s.asyncTasks == nil {
		writeErr(w, http.StatusServiceUnavailable, BizErrInternal, "async task service is not configured")
		return nil, asynctask.TaskView{}, false
	}
	taskID := strings.TrimSpace(chi.URLParam(r, "taskID"))
	if taskID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "task_id is required")
		return nil, asynctask.TaskView{}, false
	}
	view, err := s.asyncTasks.Get(r.Context(), *subject, taskID)
	if err != nil {
		writeConsoleImageTaskErr(w, err)
		return nil, asynctask.TaskView{}, false
	}
	if _, ok := portalTaskWireType(view.Type); !ok {
		writeErr(w, http.StatusNotFound, BizErrNotFound, "not found")
		return nil, asynctask.TaskView{}, false
	}
	return subject, view, true
}

func decodePortalTaskListFilter(r *http.Request) (asynctask.ListFilter, error) {
	query := r.URL.Query()
	types, err := portalTaskRegistryTypes(query.Get("type"))
	if err != nil {
		return asynctask.ListFilter{}, err
	}
	var status domain.TaskStatus
	if raw := strings.TrimSpace(query.Get("status")); raw != "" {
		status = domain.TaskStatus(raw)
		switch status {
		case domain.TaskPending, domain.TaskRunning, domain.TaskCompleted, domain.TaskFailed, domain.TaskCancelled:
		default:
			return asynctask.ListFilter{}, asynctask.Errorf(http.StatusBadRequest, "invalid_task_status", "status is not supported")
		}
	}
	ownerScope := coreidentity.Scope(strings.TrimSpace(query.Get("owner_scope")))
	if ownerScope != "" && ownerScope != coreidentity.ScopeTenant && ownerScope != coreidentity.ScopeUser {
		return asynctask.ListFilter{}, asynctask.Errorf(http.StatusBadRequest, "invalid_owner_scope", "owner scope is not supported")
	}
	ownerUserID := strings.TrimSpace(query.Get("user_id"))
	if ownerScope == coreidentity.ScopeTenant && ownerUserID != "" {
		return asynctask.ListFilter{}, asynctask.Errorf(http.StatusBadRequest, "invalid_owner_filter", "tenant tasks cannot have a user filter")
	}
	limit := 20
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			return asynctask.ListFilter{}, asynctask.Errorf(http.StatusBadRequest, "invalid_limit", "limit must be an integer from 1 to 100")
		}
	}
	return asynctask.ListFilter{
		Types: types, Status: status, OwnerScope: ownerScope, OwnerUserID: ownerUserID,
		Limit: limit, StartingAfter: strings.TrimSpace(query.Get("starting_after")),
	}, nil
}

func portalTaskRegistryTypes(wireType string) ([]string, error) {
	switch strings.TrimSpace(wireType) {
	case "":
		return []string{
			consoleImageGenerationTaskType, consoleImageEditTaskType,
			"api.images.generation", "api.images.edit", "app.images.generation", "app.images.edit",
			"api.chat.completions", "app.chat.completions",
		}, nil
	case "images.generation":
		return []string{consoleImageGenerationTaskType, "api.images.generation", "app.images.generation"}, nil
	case "images.edit":
		return []string{consoleImageEditTaskType, "api.images.edit", "app.images.edit"}, nil
	case "chat.completions":
		return []string{"api.chat.completions", "app.chat.completions"}, nil
	default:
		return nil, asynctask.Errorf(http.StatusBadRequest, "unsupported_task_type", "task type is not supported")
	}
}

func portalTaskWireType(registryType string) (string, bool) {
	switch registryType {
	case consoleImageGenerationTaskType, "api.images.generation", "app.images.generation":
		return "images.generation", true
	case consoleImageEditTaskType, "api.images.edit", "app.images.edit":
		return "images.edit", true
	case "api.chat.completions", "app.chat.completions":
		return "chat.completions", true
	default:
		return "", false
	}
}

func portalTaskDTOFromView(view asynctask.TaskView, viewer coreidentity.Subject, includeResult bool) portalTaskDTO {
	wireType, _ := portalTaskWireType(view.Type)
	ownerScope := coreidentity.ScopeTenant
	if view.Subject.UserID != "" {
		ownerScope = coreidentity.ScopeUser
	}
	manageable := portalCanManageTask(viewer, view)
	active := view.Status == domain.TaskPending || view.Status == domain.TaskRunning
	terminal := view.Status == domain.TaskCompleted || view.Status == domain.TaskFailed || view.Status == domain.TaskCancelled
	dto := portalTaskDTO{
		ID: view.ID, Type: wireType, Source: portalTaskSource(view.Type), Status: string(view.Status), Model: view.ModelCode,
		Owner: portalTaskOwnerDTO{Scope: string(ownerScope), UserID: view.Subject.UserID},
		Permissions: portalTaskPermissionsDTO{
			ReadOnly: !manageable, CanCancel: manageable && active, CanDelete: manageable && terminal,
		},
		RequestID: view.RequestID, Attempt: view.Attempt,
		ResultAvailable: hasJSONValue(view.Output), ResultSummary: portalTaskResultSummary(view.Output),
		CreatedAt: view.CreatedAt.UnixMilli(), StartedAt: unixMilliPointer(view.StartedAt), CompletedAt: unixMilliPointer(view.CompletedAt),
	}
	if view.ErrorCode != "" || view.ErrorMessage != "" {
		dto.Error = &portalTaskErrorDTO{Code: view.ErrorCode, Message: view.ErrorMessage}
	}
	if terminal {
		dto.Usage = &portalTaskUsageDTO{CostCredits: domain.MicroToCreditsFloat(view.CallerCharge)}
	}
	if includeResult && manageable && hasJSONValue(view.Output) {
		dto.Result = view.Output
	}
	return dto
}

func portalCanManageTask(viewer coreidentity.Subject, view asynctask.TaskView) bool {
	if viewer.TenantID == "" || viewer.TenantID != view.Subject.TenantID {
		return false
	}
	if viewer.Scope == coreidentity.ScopeUser {
		return viewer.UserID != "" && viewer.UserID == view.Subject.UserID
	}
	return viewer.Scope == coreidentity.ScopeTenant && view.Subject.UserID == ""
}

func portalTaskSource(registryType string) string {
	switch {
	case strings.HasPrefix(registryType, "console."):
		return "portal"
	case strings.HasPrefix(registryType, "app."):
		return "app_key"
	default:
		return "api_key"
	}
}

func portalTaskResultSummary(raw json.RawMessage) *portalTaskResultSummaryDTO {
	if !hasJSONValue(raw) {
		return nil
	}
	var payload struct {
		Data       []json.RawMessage `json:"data"`
		Choices    []json.RawMessage `json:"choices"`
		ImageCount int               `json:"image_count"`
		Summary    struct {
			ImageCount int `json:"image_count"`
		} `json:"summary"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return nil
	}
	imageCount := payload.ImageCount
	if imageCount == 0 {
		imageCount = payload.Summary.ImageCount
	}
	if imageCount == 0 {
		imageCount = len(payload.Data)
	}
	if imageCount == 0 && len(payload.Choices) == 0 {
		return nil
	}
	return &portalTaskResultSummaryDTO{ImageCount: imageCount, ChoiceCount: len(payload.Choices)}
}

func hasJSONValue(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) && !bytes.Equal(trimmed, []byte("{}"))
}

func unixMilliPointer(value *time.Time) *int64 {
	if value == nil {
		return nil
	}
	millis := value.UnixMilli()
	return &millis
}
