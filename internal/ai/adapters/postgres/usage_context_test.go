package postgres

import (
	"context"
	"testing"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
)

type usageCacheInvalidatorStub struct {
	ctx      context.Context
	apiKeyID string
}

func (s *usageCacheInvalidatorStub) DelByID(ctx context.Context, apiKeyID string) error {
	s.ctx = ctx
	s.apiKeyID = apiKeyID
	return ctx.Err()
}

func TestUsageLoggerInvalidatesAPIKeyCacheWithCompletionContext(t *testing.T) {
	type contextKey struct{}
	base := context.WithValue(context.Background(), contextKey{}, "completion")
	canceled, cancel := context.WithCancel(base)
	cancel()
	invalidator := &usageCacheInvalidatorStub{}
	logger := NewUsageLogger(nil, nil)
	logger.WithAPIKeyCacheInvalidator(invalidator)
	logger.invalidateAPIKeyCache(canceled, &coreidentity.Subject{
		APIKeyID: "api-key-1",
	})

	if invalidator.apiKeyID != "api-key-1" {
		t.Fatalf("invalidated API key = %q, want api-key-1", invalidator.apiKeyID)
	}
	if invalidator.ctx == nil || invalidator.ctx.Value(contextKey{}) != "completion" {
		t.Fatal("cache invalidation did not retain the completion context value")
	}
	if invalidator.ctx.Err() != context.Canceled {
		t.Fatalf("cache invalidation context error = %v, want context.Canceled", invalidator.ctx.Err())
	}
}

func TestUsageLoggerInvalidatesOnlyAPIKeySubjects(t *testing.T) {
	invalidator := &usageCacheInvalidatorStub{}
	logger := &UsageLogger{apiKeyInvalidator: invalidator}
	logger.invalidateAPIKeyCache(context.Background(), &coreidentity.Subject{
		AuthMethod: coreidentity.AuthMethodJWT,
	})
	if invalidator.ctx != nil {
		t.Fatal("JWT subject unexpectedly triggered API key cache invalidation")
	}
}
