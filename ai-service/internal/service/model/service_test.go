package model

import (
	"context"
	"errors"
	"testing"

	"xiaodou/unihub/ai-service/internal/domain"
)

type mockRepo struct {
	createWrite ModelWrite
	updateWrite ModelWrite
	updateID    string
	statusVal   string
	err         error
}

func (m *mockRepo) Create(ctx context.Context, w ModelWrite) (domain.ManagedModel, error) {
	m.createWrite = w
	return domain.ManagedModel{ModelCode: w.ModelCode, CapabilityType: w.CapabilityType, Status: w.Status}, m.err
}
func (m *mockRepo) List(ctx context.Context) ([]domain.ManagedModel, error) {
	return nil, m.err
}
func (m *mockRepo) Update(ctx context.Context, id string, w ModelWrite) (domain.ManagedModel, error) {
	m.updateID, m.updateWrite = id, w
	return domain.ManagedModel{ModelCode: w.ModelCode, Status: w.Status}, m.err
}
func (m *mockRepo) UpdateStatus(ctx context.Context, id, status string) (domain.ManagedModel, error) {
	m.statusVal = status
	return domain.ManagedModel{Status: status}, m.err
}

func i32(v int32) *int32 { return &v }

func TestCreate_RequiresModelCode(t *testing.T) {
	svc := New(&mockRepo{})
	if _, err := svc.Create(context.Background(), ModelInput{CapabilityType: "chat"}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation for missing model_code, got %v", err)
	}
}

func TestCreate_DefaultsCapabilityStatusAndMaxOut(t *testing.T) {
	repo := &mockRepo{}
	svc := New(repo)
	if _, err := svc.Create(context.Background(), ModelInput{ModelCode: "gpt-x"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.createWrite.CapabilityType != "chat" {
		t.Fatalf("want default capability chat, got %q", repo.createWrite.CapabilityType)
	}
	if repo.createWrite.Status != domain.APIKeyStatusActive {
		t.Fatalf("want default active, got %q", repo.createWrite.Status)
	}
	if repo.createWrite.DefaultMaxOutputTokens != defaultMaxOutputTokens {
		t.Fatalf("want default max_out %d, got %d", defaultMaxOutputTokens, repo.createWrite.DefaultMaxOutputTokens)
	}
}

func TestCreate_KeepsExplicitValues(t *testing.T) {
	repo := &mockRepo{}
	svc := New(repo)
	if _, err := svc.Create(context.Background(), ModelInput{
		ModelCode: "m", CapabilityType: "image", DefaultMaxOutputTokens: i32(99), ContextWindow: i32(8000),
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.createWrite.CapabilityType != "image" {
		t.Fatalf("want kept capability, got %q", repo.createWrite.CapabilityType)
	}
	if repo.createWrite.DefaultMaxOutputTokens != 99 {
		t.Fatalf("want explicit max_out 99, got %d", repo.createWrite.DefaultMaxOutputTokens)
	}
	if repo.createWrite.ContextWindow == nil || *repo.createWrite.ContextWindow != 8000 {
		t.Fatalf("context_window not passed: %v", repo.createWrite.ContextWindow)
	}
}

func TestUpdate_ValidatesAndPassesID(t *testing.T) {
	repo := &mockRepo{}
	svc := New(repo)
	if _, err := svc.Update(context.Background(), "id9", ModelInput{ModelCode: "m"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.updateID != "id9" {
		t.Fatalf("id not passed: %q", repo.updateID)
	}
	if repo.updateWrite.DefaultMaxOutputTokens != defaultMaxOutputTokens {
		t.Fatalf("want default max_out, got %d", repo.updateWrite.DefaultMaxOutputTokens)
	}
}

func TestUpdate_RequiresModelCode(t *testing.T) {
	svc := New(&mockRepo{})
	if _, err := svc.Update(context.Background(), "id", ModelInput{CapabilityType: "chat"}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation, got %v", err)
	}
}

func TestUpdateStatus_RequiresStatus(t *testing.T) {
	svc := New(&mockRepo{})
	if _, err := svc.UpdateStatus(context.Background(), "id", ""); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation, got %v", err)
	}
}

func TestUpdateStatus_OK(t *testing.T) {
	svc := New(&mockRepo{})
	m, err := svc.UpdateStatus(context.Background(), "id", "inactive")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Status != "inactive" {
		t.Fatalf("want inactive, got %q", m.Status)
	}
}

func TestList_PassThrough(t *testing.T) {
	svc := New(&mockRepo{})
	if _, err := svc.List(context.Background()); err != nil {
		t.Fatalf("List: %v", err)
	}
}

func TestCreate_RepoErrorPropagates(t *testing.T) {
	svc := New(&mockRepo{err: errors.New("boom")})
	if _, err := svc.Create(context.Background(), ModelInput{ModelCode: "m"}); err == nil {
		t.Fatal("want repo error")
	}
}
