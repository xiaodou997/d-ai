package postgres

import (
	"context"
	"strings"
	"testing"

	"xiaodou/dai/internal/ai/testsupport"
)

func TestHealthProbe(t *testing.T) {
	ctx := context.Background()
	var unconfigured *HealthProbe
	if err := unconfigured.Check(ctx); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("unconfigured Check error = %v", err)
	}

	pool, cleanup, err := testsupport.OpenAsyncTaskTestPool(ctx, testsupport.AsyncTaskPoolOptions{MaxConns: 1})
	if err != nil {
		t.Skipf("open postgres health test pool: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(ctx) })
	if err := NewHealthProbe(pool).Check(ctx); err != nil {
		t.Fatalf("Check: %v", err)
	}
}
