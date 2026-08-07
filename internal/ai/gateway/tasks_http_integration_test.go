package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/apikey"
	"xiaodou/dai/internal/ai/asynctask"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/testsupport"
)

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

func TestAPITaskCreateAndGetAreIdempotent(t *testing.T) {
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
		VALUES ($1::uuid, $2, 'p3 group', '33333333-3333-3333-3333-333333333333'::uuid);
		INSERT INTO ai_api_keys (id, owner_type, tenant_id, group_id, key_hash, key_ciphertext, last_four, name, status)
		VALUES ($3::uuid, 'tenant', $2, $1::uuid, $4, '', 'key1', 'p3 key', 'active')
	`, groupID, tenantID, keyID, apikey.Hash(rawKey)); err != nil {
		t.Fatalf("seed API key: %v", err)
	}

	engine, err := asynctask.New(asynctask.Config{}, asynctask.Deps{
		Pool: pool, Logger: zap.NewNop(),
		Subjects: asynctask.SubjectResolverFunc(func(_ context.Context, ref asynctask.SubjectRef) (coreidentity.Subject, error) {
			return coreidentity.Subject{AuthMethod: ref.AuthMethod, TenantID: ref.TenantID, APIKeyID: ref.APIKeyID}, nil
		}),
	})
	if err != nil {
		t.Fatalf("build engine: %v", err)
	}
	engine.Register(apiImageGenerationTaskType, echoTaskHandler{}, asynctask.Options{MaxAttempts: 1})
	g := New(Deps{Logger: zap.NewNop(), Postgres: pool, Queries: dbgen.New(pool), AsyncTasks: engine})
	router := chi.NewRouter()
	g.Routes(router)

	body := []byte(`{"type":"images.generation","input":{"model":"gpt-image-1","prompt":"draw a lighthouse"}}`)
	create := func() map[string]any {
		req := httptest.NewRequest(http.MethodPost, "/v1/tasks", bytes.NewReader(body))
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
	first, second := create(), create()
	if first["id"] == "" || second["id"] != first["id"] || first["type"] != imageGenerationWireTaskType {
		t.Fatalf("idempotent responses = %#v and %#v", first, second)
	}

	get := httptest.NewRequest(http.MethodGet, "/v1/tasks/"+first["id"].(string), nil)
	get.Header.Set("Authorization", "Bearer "+rawKey)
	getRes := httptest.NewRecorder()
	router.ServeHTTP(getRes, get)
	if getRes.Code != http.StatusOK {
		t.Fatalf("GET task status = %d, body = %s", getRes.Code, getRes.Body.String())
	}
}
