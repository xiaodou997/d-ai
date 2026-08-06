package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/apikey"
	"xiaodou/dai/internal/ai/application"
	"xiaodou/dai/internal/ai/asynctask"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	coreruntime "xiaodou/dai/internal/ai/core/runtime"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/testsupport"
)

type fixedModelTaskHandler struct{}

func (fixedModelTaskHandler) Prepare(_ context.Context, sub asynctask.Submission) (asynctask.Prepared, error) {
	return asynctask.Prepared{Input: sub.Body, ModelCode: "gpt-image-1"}, nil
}

func (fixedModelTaskHandler) Execute(context.Context, asynctask.Task) (asynctask.Result, error) {
	panic("not used: HTTP submission integration does not start workers")
}

type echoTaskHandler struct{}

func (echoTaskHandler) Prepare(_ context.Context, sub asynctask.Submission) (asynctask.Prepared, error) {
	var input struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(sub.Body, &input); err != nil {
		return asynctask.Prepared{}, err
	}
	return asynctask.Prepared{Input: sub.Body, ModelCode: input.Model}, nil
}

func (echoTaskHandler) Execute(context.Context, asynctask.Task) (asynctask.Result, error) {
	panic("not used: HTTP submission integration does not start workers")
}

type taskHTTPInvokeExpanderStub struct {
	*runInvokeExpanderStub
}

func (s *taskHTTPInvokeExpanderStub) ExpandByKeyID(
	context.Context,
	coreidentity.Scope,
	string, string, string,
	coreruntime.Request,
) (coreruntime.InvokeExpansion, error) {
	if s.err != nil {
		return coreruntime.InvokeExpansion{}, s.err
	}
	return s.expansion, nil
}

func TestAPITaskCreateAndGetAreIdempotentAndEchoMetadata(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("async task test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	const (
		rawKey   = "sk-ai-p3-primary-key"
		keyID    = "11111111-1111-1111-1111-111111111111"
		groupID  = "22222222-2222-2222-2222-222222222222"
		tenantID = "tenant-p3"
	)
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_groups (id, tenant_id, name, retail_price_book_id)
		VALUES ($1::uuid, $2, 'p3 group', '33333333-3333-3333-3333-333333333333'::uuid)
	`, groupID, tenantID); err != nil {
		t.Fatalf("seed group: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_api_keys (id, owner_type, tenant_id, group_id, key_hash, key_ciphertext, last_four, name, status)
		VALUES ($1::uuid, 'tenant', $2, $3::uuid, $4, '', 'key1', 'p3 key', 'active')
	`, keyID, tenantID, groupID, apikey.Hash(rawKey)); err != nil {
		t.Fatalf("seed API key: %v", err)
	}

	engine, err := asynctask.New(asynctask.Config{}, asynctask.Deps{
		Pool:   pool,
		Logger: zap.NewNop(),
		Subjects: asynctask.SubjectResolverFunc(func(_ context.Context, ref asynctask.SubjectRef) (coreidentity.Subject, error) {
			return coreidentity.Subject{AuthMethod: ref.AuthMethod, TenantID: ref.TenantID, APIKeyID: ref.APIKeyID}, nil
		}),
	})
	if err != nil {
		t.Fatalf("build engine: %v", err)
	}
	engine.Register(apiImageGenerationTaskType, echoTaskHandler{}, asynctask.Options{MaxAttempts: 1})
	engine.Register(apiChatCompletionTaskType, &chatTaskHandler{}, asynctask.Options{MaxAttempts: 1})

	g := New(Deps{
		Logger:     zap.NewNop(),
		Postgres:   pool,
		Queries:    dbgen.New(pool),
		AsyncTasks: engine,
	})
	router := chi.NewRouter()
	g.Routes(router)

	const webhookURL = "https://hooks.example.com/task-events"
	body := []byte(`{"type":"images.generation","input":{"model":"gpt-image-1","prompt":"draw a lighthouse"},"metadata":{"order_id":"A-1"},"webhook_url":"` + webhookURL + `"}`)
	create := func(payload []byte) map[string]any {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/v1/tasks", bytes.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+rawKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "order-A-1")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != http.StatusAccepted {
			t.Fatalf("POST /v1/tasks status = %d, body = %s", res.Code, res.Body.String())
		}
		var response map[string]any
		if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode create response: %v", err)
		}
		return response
	}

	first := create(body)
	second := create(body)
	if first["id"] == "" || second["id"] != first["id"] {
		t.Fatalf("idempotent task ids = %v and %v", first["id"], second["id"])
	}
	if first["object"] != "task" || first["type"] != imageGenerationWireTaskType || first["status"] != "pending" {
		t.Fatalf("create response = %#v", first)
	}
	if first["model"] != "gpt-image-1" || first["idempotency_key"] != "order-A-1" {
		t.Fatalf("create correlation fields = %#v", first)
	}
	if first["webhook_url"] != webhookURL {
		t.Fatalf("create webhook_url = %#v", first["webhook_url"])
	}
	metadata, ok := first["metadata"].(map[string]any)
	if !ok || metadata["order_id"] != "A-1" {
		t.Fatalf("create metadata = %#v", first["metadata"])
	}

	taskID := first["id"].(string)
	var storedWebhookURL string
	if err := pool.QueryRow(ctx, `
		SELECT COALESCE(webhook_url, '')
		FROM ai_async_tasks WHERE id = $1::uuid
	`, taskID).Scan(&storedWebhookURL); err != nil {
		t.Fatalf("load webhook snapshot: %v", err)
	}
	if storedWebhookURL != webhookURL {
		t.Fatalf("stored webhook URL = %q", storedWebhookURL)
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/tasks/"+taskID, nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("GET /v1/tasks/{id} status = %d, body = %s", res.Code, res.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if got["id"] != taskID || got["idempotency_key"] != "order-A-1" {
		t.Fatalf("get response correlation fields = %#v", got)
	}
	if got["webhook_url"] != webhookURL {
		t.Fatalf("get webhook_url = %#v", got["webhook_url"])
	}
	gotMetadata, ok := got["metadata"].(map[string]any)
	if !ok || gotMetadata["order_id"] != "A-1" {
		t.Fatalf("get metadata = %#v", got["metadata"])
	}

	chatReq := httptest.NewRequest(http.MethodPost, "/v1/tasks", strings.NewReader(
		`{"type":"chat.completions","input":{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":true}}`,
	))
	chatReq.Header.Set("Authorization", "Bearer "+rawKey)
	chatReq.Header.Set("Content-Type", "application/json")
	chatRes := httptest.NewRecorder()
	router.ServeHTTP(chatRes, chatReq)
	if chatRes.Code != http.StatusAccepted {
		t.Fatalf("POST chat task status = %d, body = %s", chatRes.Code, chatRes.Body.String())
	}
	var chatCreated map[string]any
	if err := json.Unmarshal(chatRes.Body.Bytes(), &chatCreated); err != nil {
		t.Fatalf("decode chat create response: %v", err)
	}
	if chatCreated["type"] != chatCompletionWireTaskType || chatCreated["model"] != "gpt-5.4" {
		t.Fatalf("chat create response = %#v", chatCreated)
	}
	var chatInput []byte
	if err := pool.QueryRow(ctx, `SELECT input_payload FROM ai_async_tasks WHERE id = $1::uuid`, chatCreated["id"]).Scan(&chatInput); err != nil {
		t.Fatalf("load persisted chat input: %v", err)
	}
	var persistedChat chatTaskInput
	if err := json.Unmarshal(chatInput, &persistedChat); err != nil {
		t.Fatalf("decode persisted chat task: %v", err)
	}
	assertChatStreamFalse(t, persistedChat.Body)

	listReq := httptest.NewRequest(http.MethodGet, "/v1/tasks?type=images.generation&status=pending&limit=1", nil)
	listReq.Header.Set("Authorization", "Bearer "+rawKey)
	listRes := httptest.NewRecorder()
	router.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("GET /v1/tasks status = %d, body = %s", listRes.Code, listRes.Body.String())
	}
	var listed struct {
		Object  string           `json:"object"`
		Data    []map[string]any `json:"data"`
		HasMore bool             `json:"has_more"`
	}
	if err := json.Unmarshal(listRes.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if listed.Object != "list" || listed.HasMore || len(listed.Data) != 1 || listed.Data[0]["id"] != taskID {
		t.Fatalf("list response = %#v", listed)
	}
	listedMetadata, ok := listed.Data[0]["metadata"].(map[string]any)
	if !ok || listedMetadata["order_id"] != "A-1" || listed.Data[0]["idempotency_key"] != "order-A-1" {
		t.Fatalf("listed correlation fields = %#v", listed.Data[0])
	}

	cancelReq := httptest.NewRequest(http.MethodPost, "/v1/tasks/"+taskID+"/cancel", nil)
	cancelReq.Header.Set("Authorization", "Bearer "+rawKey)
	cancelRes := httptest.NewRecorder()
	router.ServeHTTP(cancelRes, cancelReq)
	if cancelRes.Code != http.StatusOK {
		t.Fatalf("POST cancel status = %d, body = %s", cancelRes.Code, cancelRes.Body.String())
	}
	var cancelled map[string]any
	if err := json.Unmarshal(cancelRes.Body.Bytes(), &cancelled); err != nil {
		t.Fatalf("decode cancel response: %v", err)
	}
	if cancelled["id"] != taskID || cancelled["status"] != "cancelled" {
		t.Fatalf("cancel response = %#v", cancelled)
	}

	repeatCancel := httptest.NewRequest(http.MethodPost, "/v1/tasks/"+taskID+"/cancel", nil)
	repeatCancel.Header.Set("Authorization", "Bearer "+rawKey)
	repeatCancelRes := httptest.NewRecorder()
	router.ServeHTTP(repeatCancelRes, repeatCancel)
	if repeatCancelRes.Code != http.StatusConflict {
		t.Fatalf("second cancel status = %d, body = %s", repeatCancelRes.Code, repeatCancelRes.Body.String())
	}
}

func TestAppKeyCreatesTaskWithTypeInferredFromBoundApplication(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 4})
	if err != nil {
		t.Skipf("async task test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	engine, err := asynctask.New(asynctask.Config{}, asynctask.Deps{
		Pool: pool, Logger: zap.NewNop(),
		Subjects: asynctask.SubjectResolverFunc(func(_ context.Context, ref asynctask.SubjectRef) (coreidentity.Subject, error) {
			return coreidentity.Subject{
				AuthMethod: ref.AuthMethod, Scope: coreidentity.ScopeTenant,
				TenantID: ref.TenantID, InvokeKeyID: ref.InvokeKeyID,
			}, nil
		}),
	})
	if err != nil {
		t.Fatalf("build engine: %v", err)
	}
	const invokeKeyID = "77777777-7777-7777-7777-777777777777"
	expander := &taskHTTPInvokeExpanderStub{runInvokeExpanderStub: &runInvokeExpanderStub{expansion: coreruntime.InvokeExpansion{
		Subject: coreidentity.Subject{
			AuthMethod: coreidentity.AuthMethodInvokeKey, RequestSource: coreidentity.RequestSourceInvokeKey,
			Scope: coreidentity.ScopeTenant, TenantID: "tenant-app", InvokeKeyID: invokeKeyID,
		},
		InvokeKey:  application.InvokeKey{ID: invokeKeyID, Status: application.StatusActive},
		BoundModel: "gpt-image-1",
		App: &application.RuntimeApp{App: application.App{
			ID: "app-image", AppType: application.AppTypeImageGenerationAgent,
			BoundModelID: "gpt-image-1", Status: application.StatusActive,
		}},
	}}}
	engine.Register(appImageGenerationTaskType, fixedModelTaskHandler{}, asynctask.Options{MaxAttempts: 1})
	engine.Register(appChatCompletionTaskType, &appChatTaskHandler{invokeExpander: expander}, asynctask.Options{MaxAttempts: 1})
	g := New(Deps{
		Logger: zap.NewNop(), AsyncTasks: engine, RuntimeInvokeExpander: expander,
	})
	router := chi.NewRouter()
	g.Routes(router)

	const (
		rawAppKey     = "rk_demo"
		appWebhookURL = "https://hooks.example.com/app-task-events"
	)
	req := httptest.NewRequest(http.MethodPost, "/v1/tasks", strings.NewReader(
		`{"input":{"input":"draw a lighthouse"},"metadata":{"order_id":"APP-1"},"webhook_url":"`+appWebhookURL+`"}`,
	))
	req.Header.Set("Authorization", "Bearer "+rawAppKey)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("POST /v1/tasks status = %d, body = %s", res.Code, res.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created["type"] != imageGenerationWireTaskType || created["model"] != "gpt-image-1" {
		t.Fatalf("inferred task response = %#v", created)
	}
	if created["webhook_url"] != appWebhookURL {
		t.Fatalf("app task webhook_url = %#v", created["webhook_url"])
	}

	taskID := created["id"].(string)
	get := httptest.NewRequest(http.MethodGet, "/v1/tasks/"+taskID, nil)
	get.Header.Set("Authorization", "Bearer "+rawAppKey)
	getRes := httptest.NewRecorder()
	router.ServeHTTP(getRes, get)
	if getRes.Code != http.StatusOK {
		t.Fatalf("GET app task status = %d, body = %s", getRes.Code, getRes.Body.String())
	}

	expander.expansion.BoundModel = "gpt-5.4"
	expander.expansion.App.App = application.App{
		ID: "app-chat", AppType: application.AppTypeChatAgent,
		BoundModelID: "gpt-5.4", Status: application.StatusActive,
		// ai_apps.prompt_strategy is NOT NULL and validated on write, so a stored
		// app always carries one. Leaving it empty here built an app that could
		// not exist and made the request fail on strategy lookup rather than on
		// the task-type inference this test is about. The bound prompt has a
		// placeholder the caller fills via variables, which is caller_variables.
		PromptStrategy: application.PromptStrategyCallerVariables,
		DefaultOptions: map[string]any{"chat": map[string]any{"creativity": "precise"}},
	}
	expander.expansion.App.PromptBindings = []application.RuntimePromptBinding{{
		Role: application.PromptBindingSystem, TemplateText: "hello {{name}}",
	}}
	chatReq := httptest.NewRequest(http.MethodPost, "/v1/tasks", strings.NewReader(
		`{"input":{"input":"say hi","variables":{"name":"alice"},"stream":true}}`,
	))
	chatReq.Header.Set("Authorization", "Bearer "+rawAppKey)
	chatReq.Header.Set("Content-Type", "application/json")
	chatRes := httptest.NewRecorder()
	router.ServeHTTP(chatRes, chatReq)
	if chatRes.Code != http.StatusAccepted {
		t.Fatalf("POST app chat task status = %d, body = %s", chatRes.Code, chatRes.Body.String())
	}
	var chatCreated map[string]any
	if err := json.Unmarshal(chatRes.Body.Bytes(), &chatCreated); err != nil {
		t.Fatalf("decode app chat response: %v", err)
	}
	if chatCreated["type"] != chatCompletionWireTaskType || chatCreated["model"] != "gpt-5.4" {
		t.Fatalf("app chat response = %#v", chatCreated)
	}
	var appChatInput []byte
	if err := pool.QueryRow(ctx, `SELECT input_payload FROM ai_async_tasks WHERE id = $1::uuid`, chatCreated["id"]).Scan(&appChatInput); err != nil {
		t.Fatalf("load persisted app chat input: %v", err)
	}
	// An app task freezes its expansion at submit time: the bound prompt is
	// rendered once, here, rather than re-resolved when the worker runs. That
	// keeps a queued task's result independent of prompt edits made while it
	// waits. The payload is therefore the upstream body, not the raw invocation.
	var persistedAppChat struct {
		Body struct {
			Stream   bool `json:"stream"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		} `json:"body"`
	}
	if err := json.Unmarshal(appChatInput, &persistedAppChat); err != nil {
		t.Fatalf("decode persisted app chat input: %v", err)
	}
	if persistedAppChat.Body.Stream {
		t.Errorf("queued task must not carry client streaming: %s", appChatInput)
	}
	if len(persistedAppChat.Body.Messages) != 2 ||
		persistedAppChat.Body.Messages[0].Content != "hello alice" ||
		persistedAppChat.Body.Messages[1].Content != "say hi" {
		t.Fatalf("persisted app chat semantics = %s", appChatInput)
	}

	// The rendered prompt is the tenant's own asset. It may live in the stored
	// payload, which no public transport serializes, but it must never reach the
	// app-key caller through the task API.
	appTaskGet := httptest.NewRequest(http.MethodGet, "/v1/tasks/"+chatCreated["id"].(string), nil)
	appTaskGet.Header.Set("Authorization", "Bearer "+rawAppKey)
	appTaskGetRes := httptest.NewRecorder()
	router.ServeHTTP(appTaskGetRes, appTaskGet)
	if appTaskGetRes.Code != http.StatusOK {
		t.Fatalf("GET app chat task status = %d, body = %s", appTaskGetRes.Code, appTaskGetRes.Body.String())
	}
	if strings.Contains(appTaskGetRes.Body.String(), "hello alice") {
		t.Errorf("task API leaked the tenant's rendered prompt: %s", appTaskGetRes.Body.String())
	}
}
