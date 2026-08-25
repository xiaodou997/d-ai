package transport

import (
	"context"
	"testing"
	"time"

	"xiaodou/dai/internal/auth"
	tenantports "xiaodou/dai/internal/tenant/ports"
)

type tenantSelfServiceStub struct {
	user            *tenantports.TenantUser
	invites         []tenantports.InviteCodeItem
	created         tenantports.InvitationCreateCommand
	updated         tenantports.InvitationUpdateCommand
	deletedID       int64
	deletedTenantID string
	createResult    tenantports.InviteCodeItem
	createErr       error
	listErr         error
	userErr         error
	updateErr       error
	deleteErr       error
}

func (s *tenantSelfServiceStub) GetByUserID(context.Context, string) (*tenantports.TenantUser, error) {
	return s.user, s.userErr
}

func (s *tenantSelfServiceStub) ListInvitationCodes(context.Context, string, int, int) ([]tenantports.InviteCodeItem, int64, error) {
	return s.invites, int64(len(s.invites)), s.listErr
}

func (s *tenantSelfServiceStub) CreateInvitation(_ context.Context, input tenantports.InvitationCreateCommand) (tenantports.InviteCodeItem, error) {
	s.created = input
	return s.createResult, s.createErr
}

func (s *tenantSelfServiceStub) UpdateInvitation(_ context.Context, input tenantports.InvitationUpdateCommand) error {
	s.updated = input
	return s.updateErr
}

func (s *tenantSelfServiceStub) DeleteInvitation(_ context.Context, id int64, tenantID string) error {
	s.deletedID = id
	s.deletedTenantID = tenantID
	return s.deleteErr
}

func (s *tenantSelfServiceStub) GetTenantOverviewStats(context.Context, string, *time.Time, *time.Time) (*tenantports.TenantOverviewStats, error) {
	return &tenantports.TenantOverviewStats{}, nil
}

func (s *tenantSelfServiceStub) GetClientConsumption(context.Context, string, *time.Time, *time.Time) ([]tenantports.ClientConsumptionItem, error) {
	return []tenantports.ClientConsumptionItem{}, nil
}

func (s *tenantSelfServiceStub) GetUserConsumptionRanking(context.Context, string, *time.Time, *time.Time, int) ([]tenantports.UserConsumptionItem, error) {
	return []tenantports.UserConsumptionItem{}, nil
}

func TestTenantSelfHandlersUseTenantSelfApplicationPort(t *testing.T) {
	lastLogin := int64(1700000000000)
	service := &tenantSelfServiceStub{
		user: &tenantports.TenantUser{
			UserID: "user-1", TenantID: "tenant-1", Username: "alice", Email: "a@example.com",
			Status: 1, LastLoginTime: &lastLogin, CreatedTime: 1690000000000,
		},
		invites: []tenantports.InviteCodeItem{{Code: "ABC23456", TenantID: "tenant-1"}},
		createResult: tenantports.InviteCodeItem{
			Code: "XYZ23456", TenantID: "tenant-1", Description: "new", MaxUses: 2,
		},
	}
	h := newTenantSelfHandlers(service)
	ctx := context.WithValue(context.Background(), userClaimsCtxKey, &auth.Claims{
		UserID: "user-1", TenantID: "tenant-1", UserType: 3,
	})

	me, err := h.me(ctx, &struct{}{})
	if err != nil || me.Body.UserID != "user-1" || me.Body.TenantID != "tenant-1" {
		t.Fatalf("me() = %#v, error = %v", me, err)
	}

	list, err := h.listInvitations(ctx, &listInvitationsInput{Page: 1, Size: 20})
	if err != nil || list.Body.Total != 1 || list.Body.Items[0].Code != "ABC23456" {
		t.Fatalf("listInvitations() = %#v, error = %v", list, err)
	}

	createInput := &createInvitationInput{}
	createInput.Body.Description = "new"
	createInput.Body.MaxUses = 2
	created, err := h.createInvitation(ctx, createInput)
	if err != nil || created.Body.Code != "XYZ23456" || service.created.TenantID != "tenant-1" || service.created.CreatedBy != "user-1" {
		t.Fatalf("createInvitation() = %#v, command = %#v, error = %v", created, service.created, err)
	}

	updateInput := &updateInvitationInput{ID: 7}
	updateInput.Body.Status = 2
	updateInput.Body.Description = "disabled"
	if _, err := h.updateInvitation(ctx, updateInput); err != nil {
		t.Fatalf("updateInvitation() error = %v", err)
	}
	if service.updated.ID != 7 || service.updated.TenantID != "tenant-1" || service.updated.Status != 2 {
		t.Fatalf("update command = %#v", service.updated)
	}

	if _, err := h.deleteInvitation(ctx, &invitationIDInput{ID: 7}); err != nil {
		t.Fatalf("deleteInvitation() error = %v", err)
	}
	if service.deletedID != 7 || service.deletedTenantID != "tenant-1" {
		t.Fatalf("delete command = %d/%s", service.deletedID, service.deletedTenantID)
	}
}

func TestTenantSelfHandlerMapsMissingUserAndUnavailableService(t *testing.T) {
	ctx := context.WithValue(context.Background(), userClaimsCtxKey, &auth.Claims{UserID: "user-1", TenantID: "tenant-1", UserType: 3})
	h := newTenantSelfHandlers(&tenantSelfServiceStub{userErr: tenantports.ErrTenantUserNotFound})
	if _, err := h.me(ctx, &struct{}{}); err == nil || err.Error() != "Not Found: 用户不存在" {
		t.Fatalf("missing user error = %v", err)
	}
	if _, err := newTenantSelfHandlers(nil).me(ctx, &struct{}{}); err == nil || err.Error() != "Service Unavailable: 租户自助服务不可用" {
		t.Fatalf("unavailable service error = %v", err)
	}
}
