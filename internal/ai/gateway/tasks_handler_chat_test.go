package gateway

import (
	"context"
	"encoding/json"
	"testing"

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
