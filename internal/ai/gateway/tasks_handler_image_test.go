package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/textproto"
	"strings"
	"testing"

	"xiaodou/dai/internal/ai/application"
	"xiaodou/dai/internal/ai/asynctask"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	coreruntime "xiaodou/dai/internal/ai/core/runtime"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/imageedit"
	"xiaodou/dai/internal/ai/serving"
)

type allowImageTaskAdmission struct{}

func (allowImageTaskAdmission) Admit(context.Context, serving.AdmissionInput) error { return nil }

type recordingImageTaskReplayer struct {
	input ReplayInput
}

func (r *recordingImageTaskReplayer) Replay(_ context.Context, in ReplayInput) ReplayResult {
	r.input = in
	return ReplayResult{
		Request:    &coreruntime.Result{RequestStatus: string(domain.RequestSuccess), CallerChargeMicro: 4200},
		Body:       []byte(`{"created":1761200000,"data":[{"url":"https://example.com/final.png"}]}`),
		StatusCode: 200,
	}
}

func TestImageEditTaskPrepareAcceptsMultipartEnvelopeScalars(t *testing.T) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for name, value := range map[string]string{
		"type":        "images.edit",
		"metadata":    `{"order_id":"A-1"}`,
		"webhook_url": "https://caller.example.com/hooks/ai",
		"model":       "gpt-image-1",
		"prompt":      "retouch the lighting",
	} {
		if err := w.WriteField(name, value); err != nil {
			t.Fatalf("write field %s: %v", name, err)
		}
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="image[]"; filename="reference.png"`)
	header.Set("Content-Type", "image/png")
	part, err := w.CreatePart(header)
	if err != nil {
		t.Fatalf("create image part: %v", err)
	}
	_, _ = part.Write([]byte("png fixture"))
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}

	h := &imageTaskHandler{operation: "edit", admission: allowImageTaskAdmission{}}
	prepared, err := h.Prepare(context.Background(), asynctask.Submission{
		Subject: coreidentity.Subject{
			AuthMethod: coreidentity.AuthMethodAPIKey,
			TenantID:   "tenant-a",
			APIKeyID:   "11111111-1111-1111-1111-111111111111",
		},
		Type:        apiImageEditTaskType,
		Body:        body.Bytes(),
		ContentType: w.FormDataContentType(),
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if prepared.ModelCode != "gpt-image-1" {
		t.Fatalf("model = %q, want gpt-image-1", prepared.ModelCode)
	}

	var persisted imageTaskInput
	if err := json.Unmarshal(prepared.Input, &persisted); err != nil {
		t.Fatalf("decode persisted task input: %v", err)
	}
	if persisted.ContentType != imageedit.TransportJSON {
		t.Fatalf("persisted content type = %q, want %q", persisted.ContentType, imageedit.TransportJSON)
	}
	for _, secret := range []string{"order_id", "caller.example.com", "images.edit"} {
		if strings.Contains(string(persisted.Body), secret) {
			t.Fatalf("persisted execution input leaked envelope field %q: %s", secret, persisted.Body)
		}
	}
	req, err := imageedit.Decode(persisted.Body, persisted.ContentType)
	if err != nil {
		t.Fatalf("decode canonical persisted edit: %v", err)
	}
	if req.Model != "gpt-image-1" || len(req.Images) != 1 {
		t.Fatalf("canonical request = model %q, images %d", req.Model, len(req.Images))
	}
}

func TestImageTaskExecuteReplaysPersistedRequest(t *testing.T) {
	persisted, err := json.Marshal(imageTaskInput{
		Body:        json.RawMessage(`{"model":"gpt-image-1","prompt":"draw a lighthouse","stream":true}`),
		ContentType: imageedit.TransportJSON,
	})
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	subject := coreidentity.Subject{
		AuthMethod:    coreidentity.AuthMethodAPIKey,
		RequestSource: coreidentity.RequestSourceAPIKey,
		TenantID:      "tenant-a",
		APIKeyID:      "11111111-1111-1111-1111-111111111111",
	}
	replayer := &recordingImageTaskReplayer{}
	h := &imageTaskHandler{operation: "generation", replayer: replayer}

	result, err := h.Execute(context.Background(), asynctask.Task{
		ID:        "22222222-2222-2222-2222-222222222222",
		Type:      apiImageGenerationTaskType,
		ModelCode: "gpt-image-1",
		Input:     persisted,
		Subject:   subject,
		RequestID: "atsk_22222222-2222-2222-2222-222222222222_1",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if replayer.input.ExecutionMode != coreruntime.ExecutionModeAsync {
		t.Fatalf("replay execution mode = %q, want async", replayer.input.ExecutionMode)
	}
	if result.Status != domain.TaskCompleted || result.CallerCharge != 4200 {
		t.Fatalf("result = %+v, want completed with cost 4200", result)
	}
	if !json.Valid(result.Output) || !strings.Contains(string(result.Output), "final.png") {
		t.Fatalf("output = %s, want original Images response", result.Output)
	}
	if replayer.input.ClientPath != "/v1/images/generations" || replayer.input.RequestID != "atsk_22222222-2222-2222-2222-222222222222_1" {
		t.Fatalf("replay input = %+v", replayer.input)
	}
	if !replayer.input.StreamExpected {
		t.Fatal("stream=true was not preserved for response aggregation")
	}
	if replayer.input.Subject.APIKeyID != subject.APIKeyID || replayer.input.HideRevisedPrompt {
		t.Fatalf("replay subject/options = %+v", replayer.input)
	}
}

func TestAppImageTaskFreezesRenderedPromptAtSubmission(t *testing.T) {
	expansion := imageAppExpansion(
		application.AppTypeImageGenerationAgent,
		map[string]any{"resolution": "1024x1024"},
	)
	expansion.Subject = coreidentity.Subject{
		AuthMethod: coreidentity.AuthMethodInvokeKey, Scope: coreidentity.ScopeTenant,
		TenantID: "tenant-a", InvokeKeyID: "invoke-image",
	}
	expansion.InvokeKey = application.InvokeKey{ID: "invoke-image", Status: application.StatusActive}
	expansion.App.App.ID = "app-image"
	expansion.App.PromptBindings = []application.RuntimePromptBinding{{
		Role: application.PromptBindingInputTemplate, TemplateText: "poster {{topic}}",
	}}
	expander := &taskInvokeExpanderStub{expansion: expansion}
	replayer := &recordingImageTaskReplayer{}
	h := &appImageTaskHandler{
		operation: "generation", admission: allowImageTaskAdmission{},
		invokeExpander: expander, replayer: replayer,
	}
	subject := expansion.Subject

	prepared, err := h.Prepare(context.Background(), asynctask.Submission{
		Subject: subject, Type: appImageGenerationTaskType,
		Body:        []byte(`{"input":"with neon lights","variables":{"topic":"coffee"}}`),
		ContentType: imageedit.TransportJSON,
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if prepared.ModelCode != "gpt-image-1" {
		t.Fatalf("model = %q, want gpt-image-1", prepared.ModelCode)
	}
	if !strings.Contains(string(prepared.Input), "poster coffee") || !strings.Contains(string(prepared.Input), "gpt-image-1") {
		t.Fatalf("persisted app input does not contain the execution snapshot: %s", prepared.Input)
	}

	// The app changes while the task is queued. Execution must keep the prompt
	// snapshot rendered at submission time.
	expander.expansion.App.PromptBindings[0].TemplateText = "updated {{topic}}"
	result, err := h.Execute(context.Background(), asynctask.Task{
		ID: "task-app", Type: appImageGenerationTaskType, ModelCode: "gpt-image-1",
		Input: prepared.Input, Subject: subject, RequestID: "atsk_task-app_1",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Status != domain.TaskCompleted {
		t.Fatalf("result = %+v, want completed", result)
	}
	var replayBody map[string]any
	if err := json.Unmarshal(replayer.input.Body, &replayBody); err != nil {
		t.Fatalf("decode replay body: %v", err)
	}
	if replayBody["prompt"] != "poster coffee\n\nwith neon lights" {
		t.Fatalf("replayed prompt = %#v", replayBody["prompt"])
	}
	if !replayer.input.HideRevisedPrompt || replayer.input.Subject.InvokeKeyID != "invoke-image" {
		t.Fatalf("app replay options = %+v", replayer.input)
	}
}

func TestAppImageEditTaskPersistsRenderedExecutionSnapshot(t *testing.T) {
	expansion := imageAppExpansion(
		application.AppTypeImageEditAgent,
		map[string]any{"resolution": "1024x1024"},
	)
	expansion.Subject = coreidentity.Subject{
		AuthMethod: coreidentity.AuthMethodInvokeKey, Scope: coreidentity.ScopeTenant,
		TenantID: "tenant-a", InvokeKeyID: "invoke-edit",
	}
	expansion.InvokeKey = application.InvokeKey{ID: "invoke-edit", Status: application.StatusActive}
	expansion.App.App.ID = "app-edit"
	expansion.App.PromptBindings = []application.RuntimePromptBinding{{
		Role: application.PromptBindingInputTemplate, TemplateText: "edit in {{tone}} tones",
	}}
	expander := &taskInvokeExpanderStub{expansion: expansion}
	replayer := &recordingImageTaskReplayer{}
	h := &appImageTaskHandler{
		operation: "edit", admission: allowImageTaskAdmission{},
		invokeExpander: expander, replayer: replayer,
	}

	var form bytes.Buffer
	writer := multipart.NewWriter(&form)
	part, err := writer.CreateFormFile("image[]", "input.png")
	if err != nil {
		t.Fatalf("create image part: %v", err)
	}
	if _, err := part.Write(mustRunImageEditPNG(t)); err != nil {
		t.Fatalf("write image part: %v", err)
	}
	for name, value := range map[string]string{
		"input": "keep the composition", "variables": `{"tone":"warm"}`,
		"type": "images.edit", "metadata": `{"order_id":"EDIT-1"}`,
	} {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatalf("write field %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}

	prepared, err := h.Prepare(context.Background(), asynctask.Submission{
		Subject: expansion.Subject, Type: appImageEditTaskType,
		Body: form.Bytes(), ContentType: writer.FormDataContentType(),
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	var persisted appImageTaskInput
	if err := json.Unmarshal(prepared.Input, &persisted); err != nil {
		t.Fatalf("decode persisted input: %v", err)
	}
	if !strings.Contains(string(persisted.Body), "edit in warm tones") || strings.Contains(string(persisted.Body), "EDIT-1") {
		t.Fatalf("persisted edit input = %+v", persisted)
	}

	result, err := h.Execute(context.Background(), asynctask.Task{
		ID: "task-edit", Type: appImageEditTaskType, ModelCode: "gpt-image-1",
		Input: prepared.Input, Subject: expansion.Subject, RequestID: "atsk_task-edit_1",
	})
	if err != nil || result.Status != domain.TaskCompleted {
		t.Fatalf("Execute = %+v, %v", result, err)
	}
	var runtimeBody map[string]any
	if err := json.Unmarshal(replayer.input.Body, &runtimeBody); err != nil {
		t.Fatalf("decode replay body: %v", err)
	}
	if runtimeBody["prompt"] != "edit in warm tones\n\nkeep the composition" || runtimeBody["size"] != "1024x1024" {
		t.Fatalf("runtime edit body = %#v", runtimeBody)
	}
}
