package grant

import (
	"context"
	"errors"
	"testing"

	"xiaodou/unihub/ai-service/internal/domain"
)

type mockRepo struct {
	grantWrite GrantWrite
	listTenant string
	upStatus   string
	err        error
}

func (m *mockRepo) GrantToTenant(ctx context.Context, w GrantWrite) (domain.TenantModelGrant, error) {
	m.grantWrite = w
	return domain.TenantModelGrant{TenantID: w.TenantID, ModelID: w.ModelID, Status: w.Status}, m.err
}
func (m *mockRepo) ListForTenant(ctx context.Context, tenantID string) ([]domain.TenantModelGrant, error) {
	m.listTenant = tenantID
	return nil, m.err
}
func (m *mockRepo) UpdateStatus(ctx context.Context, tenantID, modelID, status string) (domain.TenantModelGrant, error) {
	m.upStatus = status
	return domain.TenantModelGrant{TenantID: tenantID, ModelID: modelID, Status: status}, m.err
}

func TestGrant_RequiresModelID(t *testing.T) {
	svc := New(&mockRepo{})
	if _, err := svc.GrantToTenant(context.Background(), GrantInput{TenantID: "t1"}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation, got %v", err)
	}
}

func TestGrant_DefaultsStatus(t *testing.T) {
	repo := &mockRepo{}
	svc := New(repo)
	if _, err := svc.GrantToTenant(context.Background(), GrantInput{TenantID: "t1", ModelID: "m1"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.grantWrite.Status != domain.APIKeyStatusActive {
		t.Fatalf("want default active, got %q", repo.grantWrite.Status)
	}
}

func TestGrant_PassesCreatedBy(t *testing.T) {
	repo := &mockRepo{}
	svc := New(repo)
	if _, err := svc.GrantToTenant(context.Background(), GrantInput{TenantID: "t1", ModelID: "m1", CreatedBy: "admin", Status: "inactive"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.grantWrite.CreatedBy != "admin" || repo.grantWrite.Status != "inactive" {
		t.Fatalf("fields not passed: %+v", repo.grantWrite)
	}
}

func TestListForTenant_PassesTenant(t *testing.T) {
	repo := &mockRepo{}
	svc := New(repo)
	if _, err := svc.ListForTenant(context.Background(), "t9"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.listTenant != "t9" {
		t.Fatalf("tenant not passed: %q", repo.listTenant)
	}
}

func TestUpdateStatus_RequiresStatus(t *testing.T) {
	svc := New(&mockRepo{})
	if _, err := svc.UpdateStatus(context.Background(), "t1", "m1", ""); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation, got %v", err)
	}
}

func TestUpdateStatus_OK(t *testing.T) {
	svc := New(&mockRepo{})
	g, err := svc.UpdateStatus(context.Background(), "t1", "m1", "inactive")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g.Status != "inactive" {
		t.Fatalf("want inactive, got %q", g.Status)
	}
}

func TestGrant_RepoErrorPropagates(t *testing.T) {
	svc := New(&mockRepo{err: errors.New("boom")})
	if _, err := svc.GrantToTenant(context.Background(), GrantInput{TenantID: "t1", ModelID: "m1"}); err == nil {
		t.Fatal("want repo error")
	}
}
