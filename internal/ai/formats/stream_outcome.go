package formats

import (
	"bytes"
	"encoding/json"
	"strings"

	"xiaodou/dai/internal/ai/domain"
)

type StreamOutcome struct {
	Event     string
	State     domain.ProviderTerminalState
	Code      string
	Message   string
	BareError bool
}

// InspectStreamOutcome is shared by precommit validation and lifecycle
// observation. Nested failure status takes precedence over a success label.
func InspectStreamOutcome(data []byte, event string) StreamOutcome {
	out := StreamOutcome{Event: strings.ToLower(strings.TrimSpace(event))}
	if bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
		out.Event = "[DONE]"
		out.State = domain.ProviderTerminalCompleted
		return out
	}
	var root struct {
		Type     string          `json:"type"`
		Status   string          `json:"status"`
		Error    json.RawMessage `json:"error"`
		Response *struct {
			Status string          `json:"status"`
			Error  json.RawMessage `json:"error"`
		} `json:"response"`
	}
	_ = json.Unmarshal(data, &root)
	if out.Event == "" {
		out.Event = strings.ToLower(root.Type)
	}
	status, rawError := root.Status, root.Error
	if root.Response != nil {
		if root.Response.Status != "" {
			status = root.Response.Status
		}
		if ErrorFieldIsPresent(root.Response.Error) {
			rawError = root.Response.Error
		}
	}
	switch {
	case ErrorFieldIsPresent(rawError), status == "failed", out.Event == "error", out.Event == "response.failed", out.Event == "response.error":
		out.State = domain.ProviderTerminalFailed
		out.BareError = out.Event == "error"
		var detail struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(rawError, &detail)
		out.Code, out.Message = detail.Code, detail.Message
	case status == "incomplete", out.Event == "response.incomplete":
		out.State = domain.ProviderTerminalIncomplete
	case status == "cancelled", status == "canceled", out.Event == "response.cancelled", out.Event == "response.canceled", out.Event == "cancelled", out.Event == "canceled":
		out.State = domain.ProviderTerminalCancelled
	case status == "completed", status == "complete", status == "succeeded", out.Event == "response.completed", out.Event == "response.complete", out.Event == "response.done", out.Event == "message_stop", out.Event == "done":
		out.State = domain.ProviderTerminalCompleted
	}
	return out
}
