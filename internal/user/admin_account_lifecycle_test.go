package user

import (
	"context"
	"errors"
	"testing"
	"time"

	userports "xiaodou/dai/internal/user/ports"
)

type adminAccountLifecycleContextKey struct{}

type adminAccountLifecycleWriterStub struct {
	updated      bool
	mutation     userports.AdminAccountMutationResult
	credential   userports.ActivationCredentialResult
	statusCtx    context.Context
	statusUserID string
	status       string
	resetCtx     context.Context
	resetUserID  string
}

func (s *adminAccountLifecycleWriterStub) CreateSystemAdmin(context.Context, userports.AdminAccountCreate) error {
	return nil
}

func (s *adminAccountLifecycleWriterStub) CreateTenantUser(context.Context, userports.AdminAccountCreate) error {
	return nil
}

func (s *adminAccountLifecycleWriterStub) UpdateSystemAdmin(context.Context, userports.AdminAccountUpdate) (userports.AdminAccountMutationResult, error) {
	return s.mutation, nil
}

func (s *adminAccountLifecycleWriterStub) UpdateTenantUser(context.Context, userports.AdminAccountUpdate) (bool, error) {
	return s.updated, nil
}

func (s *adminAccountLifecycleWriterStub) UpdateTenantUserStatus(ctx context.Context, userID, status string) (bool, error) {
	s.statusCtx, s.statusUserID, s.status = ctx, userID, status
	return s.updated, nil
}

func (s *adminAccountLifecycleWriterStub) ResetSystemAdminPassword(ctx context.Context, userID string) (userports.ActivationCredentialResult, error) {
	s.resetCtx, s.resetUserID = ctx, userID
	return s.credential, nil
}

func (s *adminAccountLifecycleWriterStub) ResetTenantUserPassword(ctx context.Context, userID string) (userports.ActivationCredentialResult, error) {
	s.resetCtx, s.resetUserID = ctx, userID
	return s.credential, nil
}

func (s *adminAccountLifecycleWriterStub) DeleteSystemAdmin(context.Context, string) (userports.AdminAccountMutationResult, error) {
	return userports.AdminAccountMutationResult{}, nil
}

type adminAccountLifecycleSecurityStub struct {
	syncErr       error
	invalidateErr error
	syncCtx       context.Context
	invalidateCtx context.Context
	syncUserID    string
	syncStatus    string
	invalidateID  string
}

func (s *adminAccountLifecycleSecurityStub) IsEnabled() bool                       { return true }
func (s *adminAccountLifecycleSecurityStub) BanUser(context.Context, string) error { return nil }
func (s *adminAccountLifecycleSecurityStub) RevokeAccessToken(context.Context, string, time.Duration) error {
	return nil
}
func (s *adminAccountLifecycleSecurityStub) InvalidateUserSessions(ctx context.Context, userID string) error {
	s.invalidateCtx, s.invalidateID = ctx, userID
	return s.invalidateErr
}
func (s *adminAccountLifecycleSecurityStub) SyncUserStatus(ctx context.Context, userID, status string) error {
	s.syncCtx, s.syncUserID, s.syncStatus = ctx, userID, status
	return s.syncErr
}
func (s *adminAccountLifecycleSecurityStub) SyncTenantStatus(context.Context, string, string, []string) error {
	return nil
}

func TestAdminAccountLifecycleSynchronizesCommittedMutations(t *testing.T) {
	ctx := context.WithValue(context.Background(), adminAccountLifecycleContextKey{}, "request")
	writer := &adminAccountLifecycleWriterStub{updated: true, credential: userports.ActivationCredentialResult{Token: "token", ExpiresIn: 60}}
	security := &adminAccountLifecycleSecurityStub{}
	service := NewAdminAccountLifecycleService(writer, security)

	updated, err := service.UpdateTenantUserStatus(ctx, "user-1", "disabled")
	if err != nil || !updated {
		t.Fatalf("UpdateTenantUserStatus = updated:%v err:%v", updated, err)
	}
	if security.syncCtx != ctx || security.syncUserID != "user-1" || security.syncStatus != "disabled" {
		t.Fatalf("security sync = ctx:%v user:%q status:%q", security.syncCtx, security.syncUserID, security.syncStatus)
	}

	credential, err := service.ResetSystemAdminPassword(ctx, "admin-1")
	if err != nil || credential.Token != "token" {
		t.Fatalf("ResetSystemAdminPassword = %#v err:%v", credential, err)
	}
	if security.invalidateCtx != ctx || security.invalidateID != "admin-1" {
		t.Fatalf("session invalidation = ctx:%v user:%q", security.invalidateCtx, security.invalidateID)
	}
}

func TestAdminAccountLifecycleWrapsSecurityFailure(t *testing.T) {
	sentinel := errors.New("redis unavailable")
	writer := &adminAccountLifecycleWriterStub{updated: true}
	security := &adminAccountLifecycleSecurityStub{syncErr: sentinel}
	service := NewAdminAccountLifecycleService(writer, security)

	updated, err := service.UpdateTenantUserStatus(context.Background(), "user-1", "disabled")
	var securityErr *userports.AdminAccountSecurityError
	if !updated || !errors.As(err, &securityErr) || !errors.Is(err, sentinel) {
		t.Fatalf("security failure = updated:%v err:%v classified:%v", updated, err, securityErr)
	}
}
