package announcement

import (
	"context"
	"errors"
	"testing"
	"time"
)

type createDraftRepository struct {
	created Announcement
}

func TestCreateDraftRejectsInvalidPlatformAudience(t *testing.T) {
	now := time.Now().UTC()
	later := now.Add(time.Hour)
	tests := []struct {
		name      string
		audiences []AudienceRule
		startsAt  *time.Time
		endsAt    *time.Time
		wantErr   error
	}{
		{
			name: "admin cannot be tenant scoped",
			audiences: []AudienceRule{{
				Kind: AudienceAdmin, ScopeType: AudienceScopeTenant, TenantID: "TENANT_A",
			}},
			wantErr: ErrInvalidAudience,
		},
		{
			name: "global and tenant scope cannot overlap",
			audiences: []AudienceRule{
				{Kind: AudienceEndUser, ScopeType: AudienceScopeAll},
				{Kind: AudienceEndUser, ScopeType: AudienceScopeTenant, TenantID: "TENANT_A"},
			},
			wantErr: ErrInvalidAudience,
		},
		{
			name: "start must precede end",
			audiences: []AudienceRule{{
				Kind: AudienceAdmin, ScopeType: AudienceScopeAll,
			}},
			startsAt: &later,
			endsAt:   &now,
			wantErr:  ErrInvalidSchedule,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(&createDraftRepository{})
			_, err := service.CreateDraft(context.Background(), Actor{UserType: 1, UserID: "SA_ROOT"}, DraftInput{
				Title: "公告", ContentMarkdown: "内容", Audiences: tt.audiences,
				StartsAt: tt.startsAt, EndsAt: tt.endsAt,
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("CreateDraft() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

type lifecycleRepository struct {
	createDraftRepository
	item        Announcement
	updateCalls int
	deleteCalls int
}

type transitionOnlyRepository struct {
	createDraftRepository
	item Announcement
}

func (r *transitionOnlyRepository) Publish(_ context.Context, _ Actor, _ string, now time.Time) (Announcement, error) {
	r.item.Status = StatusPublished
	r.item.PublishedAt = &now
	return r.item, nil
}

func (r *lifecycleRepository) DeleteDraft(_ context.Context, _ Actor, _ string) error {
	r.deleteCalls++
	if r.item.Status != StatusDraft {
		return ErrInvalidTransition
	}
	return nil
}

func (r *lifecycleRepository) GetManaged(_ context.Context, _ Actor, _ string) (Announcement, error) {
	return r.item, nil
}

func (r *lifecycleRepository) Publish(_ context.Context, _ Actor, _ string, now time.Time) (Announcement, error) {
	r.item.Status = StatusPublished
	r.item.PublishedAt = &now
	return r.item, nil
}

func (r *lifecycleRepository) UpdateDraft(_ context.Context, _ Actor, item Announcement) (Announcement, error) {
	r.updateCalls++
	r.item = item
	return item, nil
}

func (r *createDraftRepository) CreateDraft(_ context.Context, item Announcement) (Announcement, error) {
	r.created = item
	return item, nil
}

func TestCreateDraftTenantForcesOwnEndUsersAudience(t *testing.T) {
	repo := &createDraftRepository{}
	service := NewService(repo)

	created, err := service.CreateDraft(context.Background(), Actor{
		UserType: 3,
		UserID:   "TU_A",
		TenantID: "TENANT_A",
	}, DraftInput{
		Title:           "升级通知",
		ContentMarkdown: "系统将在今晚升级。",
		Audiences: []AudienceRule{{
			Kind:      AudienceAdmin,
			ScopeType: AudienceScopeAll,
		}},
	})
	if err != nil {
		t.Fatalf("CreateDraft() error = %v", err)
	}
	if created.PublisherType != PublisherTenant || created.PublisherTenantID != "TENANT_A" {
		t.Fatalf("publisher = %q/%q, want tenant/TENANT_A", created.PublisherType, created.PublisherTenantID)
	}
	if len(created.Audiences) != 1 {
		t.Fatalf("audiences = %#v, want one forced rule", created.Audiences)
	}
	want := AudienceRule{Kind: AudienceEndUser, ScopeType: AudienceScopeTenant, TenantID: "TENANT_A"}
	if created.Audiences[0] != want {
		t.Fatalf("audience = %#v, want %#v", created.Audiences[0], want)
	}
	if repo.created.Audiences[0] != want {
		t.Fatalf("persisted audience = %#v, want %#v", repo.created.Audiences[0], want)
	}
}

func TestAnnouncementAuthorizationUsesSharedCapabilities(t *testing.T) {
	tests := []struct {
		name      string
		actor     Actor
		wantValid bool
	}{
		{name: "super admin", actor: NewActor("sa", "", 1), wantValid: true},
		{name: "platform admin", actor: NewActor("pa", "", 2), wantValid: true},
		{name: "tenant", actor: NewActor("tu", "tenant-a", 3), wantValid: true},
		{name: "customer", actor: NewActor("u", "tenant-a", 4), wantValid: true},
		{name: "tenant without scope", actor: NewActor("tu", "", 3)},
		{name: "customer without scope", actor: NewActor("u", "", 4)},
		{name: "unknown role", actor: NewActor("x", "tenant-a", 99)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validatePrincipal(tt.actor); (err == nil) != tt.wantValid {
				t.Fatalf("validatePrincipal(%#v) = %v, want valid=%v", tt.actor, err, tt.wantValid)
			}
		})
	}

	service := NewService(&createDraftRepository{})
	if _, err := service.CreateDraft(context.Background(), NewActor("u", "tenant-a", 4), DraftInput{
		Title: "customer cannot publish", ContentMarkdown: "content",
	}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("customer CreateDraft() error = %v, want %v", err, ErrForbidden)
	}
}

func TestPublishedAnnouncementCannotBeEdited(t *testing.T) {
	repo := &lifecycleRepository{item: Announcement{
		ID: "ANN_1", PublisherType: PublisherPlatform, Status: StatusDraft,
		Title: "升级", ContentMarkdown: "原内容", Audiences: []AudienceRule{{Kind: AudienceAdmin, ScopeType: AudienceScopeAll}},
	}}
	service := NewService(repo)
	actor := Actor{UserType: 1, UserID: "SA_ROOT"}

	if _, err := service.Publish(context.Background(), actor, "ANN_1"); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	_, err := service.UpdateDraft(context.Background(), actor, "ANN_1", DraftInput{
		Title: "篡改", ContentMarkdown: "新内容", Audiences: []AudienceRule{{Kind: AudienceAdmin, ScopeType: AudienceScopeAll}},
	})
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("UpdateDraft() error = %v, want %v", err, ErrInvalidTransition)
	}
	if repo.updateCalls != 0 {
		t.Fatalf("UpdateDraft repository calls = %d, want 0", repo.updateCalls)
	}
	if err := service.DeleteDraft(context.Background(), actor, "ANN_1"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("DeleteDraft() error = %v, want %v", err, ErrInvalidTransition)
	}
	if repo.deleteCalls != 1 {
		t.Fatalf("DeleteDraft repository calls = %d, want 1", repo.deleteCalls)
	}
}

func TestPublishDoesNotRequireManagedReader(t *testing.T) {
	repo := &transitionOnlyRepository{item: Announcement{ID: "ANN_1", Status: StatusDraft}}
	service := NewService(repo)
	if _, err := service.Publish(context.Background(), Actor{UserType: 1, UserID: "SA_ROOT"}, repo.item.ID); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
}
