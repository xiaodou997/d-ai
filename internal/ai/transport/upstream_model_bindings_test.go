package transport

import (
	"context"
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

type poolReaderStub struct {
	poolID string
	pool   *domain.CredentialPool
	pools  []domain.CredentialPool
}

func (s *poolReaderStub) ListPools(context.Context) ([]domain.CredentialPool, error) {
	return s.pools, nil
}

func (s *poolReaderStub) GetPool(_ context.Context, poolID string) (*domain.CredentialPool, error) {
	s.poolID = poolID
	return s.pool, nil
}

func TestEnsurePoolExistsUsesReadOnlyPoolPort(t *testing.T) {
	reader := &poolReaderStub{pool: &domain.CredentialPool{ID: "pool-1"}}

	got, err := ensurePoolExists(context.Background(), AIDeps{
		IdentityDeps: IdentityDeps{PoolReader: reader},
	}, "pool-1")
	if err != nil {
		t.Fatalf("ensurePoolExists() error = %v", err)
	}
	if got != reader.pool {
		t.Fatalf("ensurePoolExists() pool = %#v, want %#v", got, reader.pool)
	}
	if reader.poolID != "pool-1" {
		t.Fatalf("GetPool() pool ID = %q, want %q", reader.poolID, "pool-1")
	}
}

func TestEnsurePoolExistsRequiresPoolReader(t *testing.T) {
	if _, err := ensurePoolExists(context.Background(), AIDeps{}, "pool-1"); err == nil {
		t.Fatal("ensurePoolExists() error = nil, want unavailable error")
	}
}
