package gateway

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"xiaodou/dai/internal/ai/application"
	"xiaodou/dai/internal/ai/asynctask"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	coreruntime "xiaodou/dai/internal/ai/core/runtime"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/serving"
)

type recordingChatTaskAdmission struct {
	input serving.AdmissionInput
}

func (a *recordingChatTaskAdmission) Admit(_ context.Context, input serving.AdmissionInput) error {
	a.input = input
	return nil
}

type recordingChatTaskReplayer struct {
	input ReplayInput
}

func (r *recordingChatTaskReplayer) Replay(_ context.Context, input ReplayInput) ReplayResult {
	r.input = input
	return ReplayResult{
		Request:    &coreruntime.Result{RequestStatus: string(domain.RequestSuccess), CallerChargeMicro: 7300},
		Body:       []byte(`{"id":"chatcmpl-1","choices":[{"message":{"role":"assistant","content":"hello"}}]}`),
		StatusCode: 200,
	}
}

func TestChatTaskForcesNonStreamingInputAndReplay(t *testing.T) {
	admission := &recordingChatTaskAdmission{}
	replayer := &recordingChatTaskReplayer{}
	handler := &chatTaskHandler{admission: admission, replayer: replayer}
	subject := coreidentity.Subject{
		AuthMethod: coreidentity.AuthMethodAPIKey, RequestSource: coreidentity.RequestSourceAPIKey,
		TenantID: "tenant-a", APIKeyID: "api-key-1",
	}

	prepared, err := handler.Prepare(context.Background(), asynctask.Submission{
		Subject: subject, Type: apiChatCompletionTaskType,
		Body:        []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":true}`),
		ContentType: "application/json; charset=utf-8",
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if prepared.ModelCode != "gpt-5.4" {
		t.Fatalf("model = %q, want gpt-5.4", prepared.ModelCode)
	}
	if admission.input.CapabilityType != domain.CapabilityChat || admission.input.ClientProtocol != domain.ProtocolOpenAIChat {
		t.Fatalf("admission input = %+v", admission.input)
	}
	var persisted chatTaskInput
	if err := json.Unmarshal(prepared.Input, &persisted); err != nil {
		t.Fatalf("decode persisted input: %v", err)
	}
	assertChatStreamFalse(t, persisted.Body)

	result, err := handler.Execute(context.Background(), asynctask.Task{
		ID: "task-chat", Type: apiChatCompletionTaskType, ModelCode: prepared.ModelCode,
		Input: prepared.Input, Subject: subject, RequestID: "atsk_task-chat_1",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Status != domain.TaskCompleted || result.CallerCharge != 7300 {
		t.Fatalf("result = %+v, want completed with cost 7300", result)
	}
	if replayer.input.ClientPath != chatCompletionsClientPath || replayer.input.Protocol != domain.ProtocolOpenAIChat ||
		replayer.input.ExecutionMode != coreruntime.ExecutionModeAsync {
		t.Fatalf("replay input = %+v", replayer.input)
	}
	assertChatStreamFalse(t, replayer.input.Body)
}

func TestAppChatTaskFreezesRenderedPromptAtSubmission(t *testing.T) {
	expansion := chatAppExpansion(map[string]any{"creativity": "precise"})
	expansion.Subject = coreidentity.Subject{
		AuthMethod: coreidentity.AuthMethodInvokeKey, RequestSource: coreidentity.RequestSourceInvokeKey,
		Scope: coreidentity.ScopeTenant, TenantID: "tenant-a", InvokeKeyID: "invoke-chat",
	}
	expansion.InvokeKey = application.InvokeKey{ID: "invoke-chat", Status: application.StatusActive}
	expansion.App.App.ID = "app-chat"
	expander := &taskInvokeExpanderStub{expansion: expansion}
	replayer := &recordingChatTaskReplayer{}
	handler := &appChatTaskHandler{
		admission: &recordingChatTaskAdmission{}, replayer: replayer, invokeExpander: expander,
	}

	prepared, err := handler.Prepare(context.Background(), asynctask.Submission{
		Subject: expansion.Subject, Type: appChatCompletionTaskType,
		Body:        []byte(`{"input":"say hi","variables":{"name":"alice"},"stream":true}`),
		ContentType: "application/json",
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if prepared.ModelCode != "gpt-5.4" {
		t.Fatalf("model = %q, want gpt-5.4", prepared.ModelCode)
	}
	if !strings.Contains(string(prepared.Input), "hello alice") || !strings.Contains(string(prepared.Input), "gpt-5.4") {
		t.Fatalf("persisted app input does not contain the execution snapshot: %s", prepared.Input)
	}
	var persisted appChatTaskInput
	if err := json.Unmarshal(prepared.Input, &persisted); err != nil {
		t.Fatalf("decode persisted app input: %v", err)
	}
	assertChatStreamFalse(t, persisted.Body)

	// Application configuration changes while the task waits in the queue.
	expander.expansion.App.PromptBindings[0].TemplateText = "updated {{name}}"
	result, err := handler.Execute(context.Background(), asynctask.Task{
		ID: "task-app-chat", Type: appChatCompletionTaskType, ModelCode: prepared.ModelCode,
		Input: prepared.Input, Subject: expansion.Subject, RequestID: "atsk_task-app-chat_1",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Status != domain.TaskCompleted {
		t.Fatalf("result = %+v, want completed", result)
	}
	assertChatStreamFalse(t, replayer.input.Body)
	var replayBody struct {
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(replayer.input.Body, &replayBody); err != nil {
		t.Fatalf("decode replay body: %v", err)
	}
	if len(replayBody.Messages) != 2 || replayBody.Messages[0].Content != "hello alice" {
		t.Fatalf("replayed messages = %+v", replayBody.Messages)
	}
	if replayer.input.Subject.InvokeKeyID != "invoke-chat" || replayer.input.RequestID != "atsk_task-app-chat_1" {
		t.Fatalf("replay identity = %+v", replayer.input)
	}
}

func assertChatStreamFalse(t *testing.T, body []byte) {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode chat body: %v", err)
	}
	if stream, ok := payload["stream"].(bool); !ok || stream {
		t.Fatalf("stream = %#v, want false", payload["stream"])
	}
}
