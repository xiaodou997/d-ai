package limit

import (
	"context"
	"errors"
	"testing"

	"xiaodou/unihub/ai-service/internal/domain"
)

type mockRepo struct {
	createWrite PolicyWrite
	updateWrite PolicyWrite
	updateID    string
	statusVal   string
	err         error
}

func (m *mockRepo) Create(ctx context.Context, w PolicyWrite) (domain.RuntimeLimitPolicy, error) {
	m.createWrite = w
	return domain.RuntimeLimitPolicy{ScopeType: w.ScopeType, Status: w.Status, CapabilityType: w.CapabilityType}, m.err
}
func (m *mockRepo) List(ctx context.Context) ([]domain.RuntimeLimitPolicy, error) {
	return nil, m.err
}
func (m *mockRepo) Update(ctx context.Context, id string, w PolicyWrite) (domain.RuntimeLimitPolicy, error) {
	m.updateID, m.updateWrite = id, w
	return domain.RuntimeLimitPolicy{ID: id, Status: w.Status}, m.err
}
func (m *mockRepo) UpdateStatus(ctx context.Context, id, status string) (domain.RuntimeLimitPolicy, error) {
	m.statusVal = status
	return domain.RuntimeLimitPolicy{ID: id, Status: status}, m.err
}

func i32(v int32) *int32 { return &v }

func TestCreate_RequiresScope(t *testing.T) {
	svc := New(&mockRepo{})
	if _, err := svc.Create(context.Background(), PolicyInput{ScopeID: "t1", RpmLimit: i32(1)}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation for missing scope_type, got %v", err)
	}
	if _, err := svc.Create(context.Background(), PolicyInput{ScopeType: "tenant", RpmLimit: i32(1)}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation for missing scope_id, got %v", err)
	}
}

func TestCreate_RejectsInvalidScopeType(t *testing.T) {
	svc := New(&mockRepo{})
	if _, err := svc.Create(context.Background(), PolicyInput{ScopeType: "bogus", ScopeID: "x", RpmLimit: i32(1)}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation, got %v", err)
	}
}

func TestCreate_RejectsInvalidCapability(t *testing.T) {
	svc := New(&mockRepo{})
	if _, err := svc.Create(context.Background(), PolicyInput{ScopeType: "tenant", ScopeID: "x", CapabilityType: "bogus", RpmLimit: i32(1)}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation, got %v", err)
	}
}

func TestCreate_RejectsInvalidStatus(t *testing.T) {
	svc := New(&mockRepo{})
	if _, err := svc.Create(context.Background(), PolicyInput{ScopeType: "tenant", ScopeID: "x", Status: "bogus", RpmLimit: i32(1)}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation, got %v", err)
	}
}

func TestCreate_RequiresAtLeastOneLimit(t *testing.T) {
	svc := New(&mockRepo{})
	if _, err := svc.Create(context.Background(), PolicyInput{ScopeType: "tenant", ScopeID: "x"}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation, got %v", err)
	}
}

func TestCreate_RejectsNonPositiveLimit(t *testing.T) {
	svc := New(&mockRepo{})
	if _, err := svc.Create(context.Background(), PolicyInput{ScopeType: "tenant", ScopeID: "x", RpmLimit: i32(0)}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation, got %v", err)
	}
}

func TestCreate_DefaultsCapabilityAndStatus(t *testing.T) {
	repo := &mockRepo{}
	svc := New(repo)
	if _, err := svc.Create(context.Background(), PolicyInput{ScopeType: "tenant", ScopeID: "x", RpmLimit: i32(10)}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.createWrite.CapabilityType != defaultCapability {
		t.Fatalf("want default capability %q, got %q", defaultCapability, repo.createWrite.CapabilityType)
	}
	if repo.createWrite.Status != domain.APIKeyStatusActive {
		t.Fatalf("want default active, got %q", repo.createWrite.Status)
	}
	if repo.createWrite.RpmLimit == nil || *repo.createWrite.RpmLimit != 10 {
		t.Fatalf("rpm not passed: %v", repo.createWrite.RpmLimit)
	}
}

func TestUpdate_ValidatesAndPassesID(t *testing.T) {
	repo := &mockRepo{}
	svc := New(repo)
	if _, err := svc.Update(context.Background(), "id9", PolicyInput{ScopeType: "user", ScopeID: "u1", ConcurrencyLimit: i32(3)}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.updateID != "id9" {
		t.Fatalf("id not passed: %q", repo.updateID)
	}
	if repo.updateWrite.Status != domain.APIKeyStatusActive {
		t.Fatalf("want default active, got %q", repo.updateWrite.Status)
	}
}

func TestUpdate_RejectsInvalid(t *testing.T) {
	svc := New(&mockRepo{})
	if _, err := svc.Update(context.Background(), "id", PolicyInput{ScopeType: "tenant", ScopeID: "x"}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation (no limit), got %v", err)
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
	p, err := svc.UpdateStatus(context.Background(), "id", "inactive")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Status != "inactive" {
		t.Fatalf("want inactive, got %q", p.Status)
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
	if _, err := svc.Create(context.Background(), PolicyInput{ScopeType: "tenant", ScopeID: "x", RpmLimit: i32(1)}); err == nil {
		t.Fatal("want repo error")
	}
}
