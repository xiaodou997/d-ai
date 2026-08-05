package identitycontrol

import (
	"context"
	"errors"
	"strings"
	"testing"

	"xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
)

type mockRepo struct {
	createParams identity.APIKeyCreate
	updateParams identity.APIKeyUpdate
	rotateParams identity.APIKeyRotate
	listFilter   identity.APIKeyListFilter
	rotateOld    string
	updateHash   string
	deleteHash   string
	revealCipher string
	err          error
}

func (m *mockRepo) Create(ctx context.Context, p identity.APIKeyCreate) (identity.APIKey, error) {
	m.createParams = p
	if m.err != nil {
		return identity.APIKey{}, m.err
	}
	return identity.APIKey{ID: "k1", Name: p.Name, Status: p.Status, QuotaLimitMicro: p.QuotaLimitMicro}, nil
}

func (m *mockRepo) List(ctx context.Context, f identity.APIKeyListFilter) ([]identity.APIKey, error) {
	m.listFilter = f
	return nil, m.err
}

func (m *mockRepo) Update(ctx context.Context, p identity.APIKeyUpdate) (identity.APIKey, string, error) {
	m.updateParams = p
	return identity.APIKey{ID: p.ID, Name: p.Name}, m.updateHash, m.err
}

func (m *mockRepo) UpdateStatus(ctx context.Context, id, tenantID, status string) (identity.APIKey, string, error) {
	return identity.APIKey{ID: id, Status: status}, m.updateHash, m.err
}

func (m *mockRepo) Rotate(ctx context.Context, p identity.APIKeyRotate) (identity.APIKey, string, error) {
	m.rotateParams = p
	return identity.APIKey{ID: p.ID}, m.rotateOld, m.err
}

func (m *mockRepo) Reveal(ctx context.Context, id, tenantID string) (string, error) {
	return m.revealCipher, m.err
}

func (m *mockRepo) Delete(ctx context.Context, id, tenantID string) (string, error) {
	return m.deleteHash, m.err
}

type mockCache struct{ deleted []string }

func (c *mockCache) Del(ctx context.Context, keyHash string) error {
	c.deleted = append(c.deleted, keyHash)
	return nil
}

func TestCreate_EmptyNameIsValidationError(t *testing.T) {
	svc := New(&mockRepo{}, nil, func(v string) (string, error) { return "enc:" + v, nil }, func(v string) (string, error) { return v, nil })
	_, err := svc.Create(context.Background(), CreateInput{TenantID: "t1"})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation, got %v", err)
	}
}

func TestCreate_NegativeCreditsIsValidationError(t *testing.T) {
	neg := int64(-1)
	svc := New(&mockRepo{}, nil, func(v string) (string, error) { return "enc:" + v, nil }, func(v string) (string, error) { return v, nil })
	_, err := svc.Create(context.Background(), CreateInput{TenantID: "t1", GroupID: "g1", Name: "k", QuotaLimitCredits: &neg})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation, got %v", err)
	}
}

func TestCreate_DefaultsStatusAndConvertsCredits(t *testing.T) {
	repo := &mockRepo{}
	svc := New(repo, nil, func(v string) (string, error) { return "enc:" + v, nil }, func(v string) (string, error) { return v, nil })
	limit := int64(5)
	out, err := svc.Create(context.Background(), CreateInput{
		OwnerScope: identity.ScopeTenant, TenantID: "t1", GroupID: "g1", Name: "k", QuotaLimitCredits: &limit,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.createParams.Status != domain.APIKeyStatusActive {
		t.Fatalf("want default status %q, got %q", domain.APIKeyStatusActive, repo.createParams.Status)
	}
	if repo.createParams.GroupID != "g1" {
		t.Fatalf("group id = %q, want g1", repo.createParams.GroupID)
	}
	if repo.createParams.QuotaLimitMicro == nil || *repo.createParams.QuotaLimitMicro != 5*domain.MicroCreditsPerCredit {
		t.Fatalf("credits->micro conversion wrong: %v", repo.createParams.QuotaLimitMicro)
	}
	if !strings.HasPrefix(out.PlaintextKey, "sk-ai-") {
		t.Fatalf("plaintext key missing prefix: %q", out.PlaintextKey)
	}
	if repo.createParams.KeyHash == "" || repo.createParams.LastFour == "" {
		t.Fatalf("key hash/last-four not populated")
	}
	if repo.createParams.KeyCiphertext == "" {
		t.Fatalf("key ciphertext not populated")
	}
}

func TestCreate_ExceedsMaxCreditsIsValidationError(t *testing.T) {
	over := int64(maxCreditsPerField + 1)
	svc := New(&mockRepo{}, nil, func(v string) (string, error) { return "enc:" + v, nil }, func(v string) (string, error) { return v, nil })
	_, err := svc.Create(context.Background(), CreateInput{TenantID: "t1", GroupID: "g1", Name: "k", QuotaLimitCredits: &over})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation, got %v", err)
	}
}

func TestCreate_RepoErrorPropagates(t *testing.T) {
	svc := New(&mockRepo{err: errors.New("boom")}, nil, func(v string) (string, error) { return "enc:" + v, nil }, func(v string) (string, error) { return v, nil })
	_, err := svc.Create(context.Background(), CreateInput{TenantID: "t1", GroupID: "g1", Name: "k"})
	if err == nil {
		t.Fatal("want repo error to propagate")
	}
}

func TestUpdate_EmptyNameIsValidationError(t *testing.T) {
	svc := New(&mockRepo{}, nil, func(v string) (string, error) { return "enc:" + v, nil }, func(v string) (string, error) { return v, nil })
	_, err := svc.Update(context.Background(), UpdateInput{ID: "k1", TenantID: "t1"})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation, got %v", err)
	}
}

func TestUpdate_ConvertsCredits(t *testing.T) {
	repo := &mockRepo{}
	svc := New(repo, nil, func(v string) (string, error) { return "enc:" + v, nil }, func(v string) (string, error) { return v, nil })
	limit := int64(3)
	if _, err := svc.Update(context.Background(), UpdateInput{ID: "k1", TenantID: "t1", GroupID: "g1", Name: "k", QuotaLimitCredits: &limit}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.updateParams.QuotaLimitMicro == nil || *repo.updateParams.QuotaLimitMicro != 3*domain.MicroCreditsPerCredit {
		t.Fatalf("credits->micro conversion wrong: %v", repo.updateParams.QuotaLimitMicro)
	}
}

func TestUpdateStatus_EmptyIsValidationError(t *testing.T) {
	svc := New(&mockRepo{}, nil, func(v string) (string, error) { return "enc:" + v, nil }, func(v string) (string, error) { return v, nil })
	_, err := svc.UpdateStatus(context.Background(), "k1", "t1", "")
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation, got %v", err)
	}
}

func TestUpdateStatus_OK(t *testing.T) {
	svc := New(&mockRepo{}, nil, func(v string) (string, error) { return "enc:" + v, nil }, func(v string) (string, error) { return v, nil })
	key, err := svc.UpdateStatus(context.Background(), "k1", "t1", "disabled")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key.Status != "disabled" {
		t.Fatalf("want status disabled, got %q", key.Status)
	}
}

func TestUpdate_InvalidatesCache(t *testing.T) {
	repo := &mockRepo{updateHash: "h-upd"}
	cache := &mockCache{}
	svc := New(repo, cache, func(v string) (string, error) { return "enc:" + v, nil }, func(v string) (string, error) { return v, nil })
	if _, err := svc.Update(context.Background(), UpdateInput{ID: "k1", TenantID: "t1", GroupID: "g1", Name: "k"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cache.deleted) != 1 || cache.deleted[0] != "h-upd" {
		t.Fatalf("update must invalidate the key hash, got %v", cache.deleted)
	}
}

func TestUpdateStatus_InvalidatesCache(t *testing.T) {
	repo := &mockRepo{updateHash: "h-st"}
	cache := &mockCache{}
	svc := New(repo, cache, func(v string) (string, error) { return "enc:" + v, nil }, func(v string) (string, error) { return v, nil })
	if _, err := svc.UpdateStatus(context.Background(), "k1", "t1", "disabled"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cache.deleted) != 1 || cache.deleted[0] != "h-st" {
		t.Fatalf("status change must invalidate the key hash, got %v", cache.deleted)
	}
}

func TestUpdateStatus_InvalidStatusIsValidationError(t *testing.T) {
	svc := New(&mockRepo{}, nil, func(v string) (string, error) { return "enc:" + v, nil }, func(v string) (string, error) { return v, nil })
	if _, err := svc.UpdateStatus(context.Background(), "k1", "t1", "inactive"); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation, got %v", err)
	}
}

func TestListForTenant_ScopesFilter(t *testing.T) {
	repo := &mockRepo{}
	svc := New(repo, nil, func(v string) (string, error) { return "enc:" + v, nil }, func(v string) (string, error) { return v, nil })
	if _, err := svc.ListForTenant(context.Background(), "t1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.listFilter.OwnerScope != identity.ScopeTenant || repo.listFilter.TenantID != "t1" {
		t.Fatalf("filter not scoped correctly: %+v", repo.listFilter)
	}
}

func TestListForUser_ScopesFilter(t *testing.T) {
	repo := &mockRepo{}
	svc := New(repo, nil, func(v string) (string, error) { return "enc:" + v, nil }, func(v string) (string, error) { return v, nil })
	if _, err := svc.ListForUser(context.Background(), "t1", "u1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.listFilter.OwnerScope != identity.ScopeUser || repo.listFilter.UserID != "u1" || repo.listFilter.TenantID != "t1" {
		t.Fatalf("filter not scoped correctly: %+v", repo.listFilter)
	}
}

func TestRotate_InvalidatesOldHash(t *testing.T) {
	repo := &mockRepo{rotateOld: "oldhash"}
	cache := &mockCache{}
	svc := New(repo, cache, func(v string) (string, error) { return "enc:" + v, nil }, func(v string) (string, error) { return v, nil })
	out, err := svc.Rotate(context.Background(), "k1", "t1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.PlaintextKey == "" {
		t.Fatalf("rotate did not return new plaintext")
	}
	if len(cache.deleted) != 1 || cache.deleted[0] != "oldhash" {
		t.Fatalf("want old hash invalidated, got %v", cache.deleted)
	}
}

func TestDelete_InvalidatesHash(t *testing.T) {
	repo := &mockRepo{deleteHash: "h"}
	cache := &mockCache{}
	svc := New(repo, cache, func(v string) (string, error) { return "enc:" + v, nil }, func(v string) (string, error) { return v, nil })
	if err := svc.Delete(context.Background(), "k1", "t1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cache.deleted) != 1 || cache.deleted[0] != "h" {
		t.Fatalf("want hash invalidated, got %v", cache.deleted)
	}
}

func TestRotate_NilCacheIsSafe(t *testing.T) {
	svc := New(&mockRepo{rotateOld: "x"}, nil, func(v string) (string, error) { return "enc:" + v, nil }, func(v string) (string, error) { return v, nil })
	if _, err := svc.Rotate(context.Background(), "k1", "t1"); err != nil {
		t.Fatalf("nil cache should be safe, got %v", err)
	}
}

func TestReveal_DecryptsCiphertext(t *testing.T) {
	svc := New(
		&mockRepo{revealCipher: "cipher"},
		nil,
		func(v string) (string, error) { return "enc:" + v, nil },
		func(v string) (string, error) { return "plain:" + v, nil },
	)
	plain, err := svc.Reveal(context.Background(), "k1", "t1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plain != "plain:cipher" {
		t.Fatalf("unexpected plaintext: %q", plain)
	}
}
