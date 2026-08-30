package user

import (
	"context"
	"errors"
	"testing"
	"time"

	userports "xiaodou/dai/internal/user/ports"
)

type endUserLifecycleContextKey struct{}

type endUserLifecycleSecurityStub struct {
	statusErr   error
	banErr      error
	statusCtx   context.Context
	statusID    string
	statusValue string
	logoutCtx   context.Context
	logoutID    string
	banCtx      context.Context
	banID       string
}

func (s *endUserLifecycleSecurityStub) IsEnabled() bool { return s.banErr == nil }
func (s *endUserLifecycleSecurityStub) BanUser(ctx context.Context, userID string) error {
	s.banCtx, s.banID = ctx, userID
	return s.banErr
}
func (s *endUserLifecycleSecurityStub) RevokeAccessToken(context.Context, string, time.Duration) error {
	return nil
}
func (s *endUserLifecycleSecurityStub) InvalidateUserSessions(ctx context.Context, userID string) error {
	s.logoutCtx, s.logoutID = ctx, userID
	return nil
}
func (s *endUserLifecycleSecurityStub) SyncUserStatus(ctx context.Context, userID, status string) error {
	s.statusCtx, s.statusID, s.statusValue = ctx, userID, status
	return s.statusErr
}
func (s *endUserLifecycleSecurityStub) SyncTenantStatus(context.Context, string, string, []string) error {
	return nil
}

type endUserLifecycleWriterStub struct {
	updated    bool
	credential userports.ActivationCredentialResult
	deleted    userports.AdminEndUserDeleteResult
}

func (s *endUserLifecycleWriterStub) CreateEndUser(context.Context, userports.AdminEndUserCreate) error {
	return nil
}
func (s *endUserLifecycleWriterStub) UpdateEndUser(context.Context, userports.AdminEndUserUpdate) (bool, error) {
	return s.updated, nil
}
func (s *endUserLifecycleWriterStub) UpdateEndUserStatus(context.Context, userports.AdminEndUserStatusUpdate) (bool, error) {
	return s.updated, nil
}
func (s *endUserLifecycleWriterStub) ResetEndUserPassword(context.Context, userports.AdminEndUserPasswordReset) (userports.ActivationCredentialResult, error) {
	return s.credential, nil
}
func (s *endUserLifecycleWriterStub) DeleteEndUser(ctx context.Context, input userports.AdminEndUserDeleteCommand) (userports.AdminEndUserDeleteResult, error) {
	if input.BeforeCommit != nil {
		if err := input.BeforeCommit(ctx, input.UserID); err != nil {
			return userports.AdminEndUserDeleteResult{}, &userports.AdminEndUserDeleteGuardError{Cause: err}
		}
	}
	return s.deleted, nil
}

func TestAdminEndUserLifecycleProjectsStatusAndSessions(t *testing.T) {
	ctx := context.WithValue(context.Background(), endUserLifecycleContextKey{}, "request")
	writer := &endUserLifecycleWriterStub{updated: true, credential: userports.ActivationCredentialResult{Token: "token"}, deleted: userports.AdminEndUserDeleteResult{Found: true, Deleted: true}}
	security := &endUserLifecycleSecurityStub{}
	service := NewAdminEndUserLifecycleService(writer, security)
	if updated, err := service.UpdateEndUserStatus(ctx, userports.AdminEndUserStatusUpdate{UserID: "u-1", Status: "disabled"}); err != nil || !updated || security.statusCtx != ctx {
		t.Fatalf("status lifecycle = updated:%v err:%v securityCtx:%v", updated, err, security.statusCtx)
	}
	if result, err := service.ResetEndUserPassword(ctx, userports.AdminEndUserPasswordReset{UserID: "u-1"}); err != nil || result.Token != "token" || security.logoutCtx != ctx || security.logoutID != "u-1" {
		t.Fatalf("reset lifecycle = %#v err:%v logoutCtx:%v logoutID:%q", result, err, security.logoutCtx, security.logoutID)
	}
	if result, err := service.DeleteEndUser(ctx, userports.AdminEndUserDeleteCommand{UserID: "u-1"}); err != nil || !result.Deleted || security.banCtx != ctx || security.banID != "u-1" {
		t.Fatalf("delete lifecycle = %#v err:%v banCtx:%v banID:%q", result, err, security.banCtx, security.banID)
	}
}

func TestAdminEndUserLifecycleWrapsSecurityFailure(t *testing.T) {
	sentinel := errors.New("redis unavailable")
	writer := &endUserLifecycleWriterStub{updated: true}
	security := &endUserLifecycleSecurityStub{statusErr: sentinel}
	service := NewAdminEndUserLifecycleService(writer, security)
	updated, err := service.UpdateEndUserStatus(context.Background(), userports.AdminEndUserStatusUpdate{UserID: "u-1", Status: "disabled"})
	var securityErr *userports.AdminEndUserSecurityError
	if !updated || !errors.As(err, &securityErr) || !errors.Is(err, sentinel) {
		t.Fatalf("security failure = updated:%v err:%v classified:%v", updated, err, securityErr)
	}
}
