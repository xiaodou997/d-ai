package postgres

import (
	"context"
	"github.com/google/uuid"
	"testing"
	"xiaodou/dai/internal/ai/core/catalog"
	"xiaodou/dai/internal/ai/core/surface"
	coreupstream "xiaodou/dai/internal/ai/core/upstream"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/secret"
	"xiaodou/dai/internal/ai/testsupport"
)

func TestRuntimeBinderUsesSingleEndpointWithExplicitConversionPolicy(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup(ctx)
	const master = "0123456789abcdef0123456789abcdef"
	cipher, err := secret.EncryptProviderKey(master, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	if _, err = pool.Exec(ctx, `INSERT INTO ai_upstream_accounts(id,name,tenant_display_name,api_key_ciphertext,status) VALUES($1::uuid,$1::text,$1::text,$2,'active')`, id, cipher); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO ai_upstream_models(upstream_kind,upstream_id,model_code,capability_type,upstream_model_name) VALUES('direct_upstream',$1,'model','chat','physical-model')`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO ai_upstream_account_endpoints(account_id,api_format,base_url) VALUES($1,'openai_responses','https://example.test')`, id); err != nil {
		t.Fatal(err)
	}
	binder := NewRuntimeTargetBinder(dbgen.New(pool), pool, master)
	req := coreupstream.RuntimeBindingRequest{TargetMode: coreupstream.AccessModeDirect, TargetID: id, ResolvedModelID: "model", Capability: catalog.CapabilityChat, ClientSurface: surface.OpenAIChat, AllowProtocolConversion: true}
	binding, err := binder.ResolveRuntimeBinding(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if binding.ModelBinding.RequestSurface != surface.OpenAIResponses || len(binding.Alternatives) != 0 || binding.EndpointID == "" {
		t.Fatalf("binding=%+v", binding)
	}
	req.AllowProtocolConversion = false
	binding, err = binder.ResolveRuntimeBinding(ctx, req)
	if err == nil {
		t.Fatalf("conversion disabled: %+v %v", binding, err)
	}
}
