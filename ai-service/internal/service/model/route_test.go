package model

import (
	"context"
	"errors"
	"testing"

	"xiaodou/unihub/ai-service/internal/domain"
)

type mockRouteRepo struct {
	createWrite RouteWrite
	updateWrite RouteUpdate
	delModel    string
	delRoute    string
	err         error
}

func (m *mockRouteRepo) Create(ctx context.Context, w RouteWrite) (domain.ModelRoute, error) {
	m.createWrite = w
	return domain.ModelRoute{ModelID: w.ModelID, Status: w.Status, Priority: w.Priority, Weight: w.Weight, SupportsStream: w.SupportsStream}, m.err
}
func (m *mockRouteRepo) List(ctx context.Context, modelID string) ([]domain.ModelRouteListItem, error) {
	return nil, m.err
}
func (m *mockRouteRepo) Get(ctx context.Context, routeID string) (domain.ModelRoute, error) {
	return domain.ModelRoute{ID: routeID}, m.err
}
func (m *mockRouteRepo) Update(ctx context.Context, w RouteUpdate) (domain.ModelRoute, error) {
	m.updateWrite = w
	return domain.ModelRoute{ID: w.RouteID, Status: w.Status}, m.err
}
func (m *mockRouteRepo) UpdateStatus(ctx context.Context, modelID, routeID, status string) (domain.ModelRoute, error) {
	return domain.ModelRoute{ID: routeID, Status: status}, m.err
}
func (m *mockRouteRepo) Delete(ctx context.Context, modelID, routeID string) error {
	m.delModel, m.delRoute = modelID, routeID
	return m.err
}

func TestRouteCreate_RequiresDeploymentID(t *testing.T) {
	svc := NewRouteService(&mockRouteRepo{})
	if _, err := svc.Create(context.Background(), RouteInput{ModelID: "m"}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation, got %v", err)
	}
}

func TestRouteCreate_AppliesDefaults(t *testing.T) {
	repo := &mockRouteRepo{}
	svc := NewRouteService(repo)
	if _, err := svc.Create(context.Background(), RouteInput{ModelID: "m", UpstreamDeploymentID: "d"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.createWrite.Priority != defaultRoutePriority || repo.createWrite.Weight != defaultRouteWeight {
		t.Fatalf("defaults not applied: p=%d w=%d", repo.createWrite.Priority, repo.createWrite.Weight)
	}
	if !repo.createWrite.SupportsStream {
		t.Fatalf("supports_stream should default true")
	}
	if repo.createWrite.Status != domain.APIKeyStatusActive {
		t.Fatalf("want default active, got %q", repo.createWrite.Status)
	}
}

func TestRouteCreate_RespectsExplicit(t *testing.T) {
	repo := &mockRouteRepo{}
	svc := NewRouteService(repo)
	p, w, ss := int32(5), int32(7), false
	if _, err := svc.Create(context.Background(), RouteInput{
		ModelID: "m", UpstreamDeploymentID: "d", Priority: &p, Weight: &w, SupportsStream: &ss, Status: "inactive",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.createWrite.Priority != 5 || repo.createWrite.Weight != 7 || repo.createWrite.SupportsStream {
		t.Fatalf("explicit values overridden: %+v", repo.createWrite)
	}
	if repo.createWrite.Status != "inactive" {
		t.Fatalf("want inactive, got %q", repo.createWrite.Status)
	}
}

func TestRouteUpdate_RequiresDeploymentID(t *testing.T) {
	svc := NewRouteService(&mockRouteRepo{})
	if _, err := svc.Update(context.Background(), RouteInput{ModelID: "m", RouteID: "r"}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation, got %v", err)
	}
}

func TestRouteUpdate_PassesIDs(t *testing.T) {
	repo := &mockRouteRepo{}
	svc := NewRouteService(repo)
	if _, err := svc.Update(context.Background(), RouteInput{ModelID: "m1", RouteID: "r1", UpstreamDeploymentID: "d"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.updateWrite.ModelID != "m1" || repo.updateWrite.RouteID != "r1" {
		t.Fatalf("ids not passed: %+v", repo.updateWrite)
	}
}

func TestRouteUpdateStatus_RequiresStatus(t *testing.T) {
	svc := NewRouteService(&mockRouteRepo{})
	if _, err := svc.UpdateStatus(context.Background(), "m", "r", ""); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("want ErrValidation, got %v", err)
	}
}

func TestRouteUpdateStatus_OK(t *testing.T) {
	svc := NewRouteService(&mockRouteRepo{})
	rt, err := svc.UpdateStatus(context.Background(), "m", "r", "inactive")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt.Status != "inactive" {
		t.Fatalf("want inactive, got %q", rt.Status)
	}
}

func TestRouteDelete_PassesIDs(t *testing.T) {
	repo := &mockRouteRepo{err: domain.ErrNotFound}
	svc := NewRouteService(repo)
	if err := svc.Delete(context.Background(), "m2", "r2"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if repo.delModel != "m2" || repo.delRoute != "r2" {
		t.Fatalf("ids not passed: %q %q", repo.delModel, repo.delRoute)
	}
}

func TestRouteListAndGet_PassThrough(t *testing.T) {
	svc := NewRouteService(&mockRouteRepo{})
	if _, err := svc.List(context.Background(), "m"); err != nil {
		t.Fatalf("List: %v", err)
	}
	if _, err := svc.Get(context.Background(), "r"); err != nil {
		t.Fatalf("Get: %v", err)
	}
}
