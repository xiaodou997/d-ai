package console

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	pgadapter "xiaodou/dai/internal/ai/adapters/postgres"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/testsupport"
)

func TestConsoleImageModelsQueryMatchesCanonicalSchema(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("console image models test database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	const groupID = "11111111-1111-1111-1111-111111111111"
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_groups (id, tenant_id, name, retail_price_book_id, status)
		VALUES ($1::uuid, 'tenant-1', 'default', '22222222-2222-2222-2222-222222222222'::uuid, 'active')
	`, groupID); err != nil {
		t.Fatalf("seed image model group: %v", err)
	}

	service := &Console{
		postgres:     pool,
		grantChecker: pgadapter.NewGroupAccessReader(pool),
	}
	req := httptest.NewRequest(http.MethodGet, "/runtime/v1/images/models", nil)
	_, err = service.consoleGrantedImageModels(req, &coreidentity.Subject{
		Scope:    coreidentity.ScopeTenant,
		TenantID: "tenant-1",
	})
	if err != nil {
		t.Fatalf("list console image models against canonical schema: %v", err)
	}
}
