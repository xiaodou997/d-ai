package tenant

import (
	"context"
	"errors"
	"testing"
	"time"

	tenantports "xiaodou/dai/internal/tenant/ports"
)

type adminTenantLifecycleContextKey struct{}

type adminTenantLifecycleWriterStub struct {
	result   tenantports.AdminTenantStatusResult
	ctx      context.Context
	tenantID string
	status   string
}

func (s *adminTenantLifecycleWriterStub) UpdateStatus(ctx context.Context, tenantID, status string) (tenantports.AdminTenantStatusResult, error) {
	s.ctx, s.tenantID, s.status = ctx, tenantID, status
	return s.result, nil
}

type adminTenantLifecycleSecurityStub struct {
	err         error
	ctx         context.Context
	tenantID    string
	status      string
	restoredIDs []string
}

func (s *adminTenantLifecycleSecurityStub) IsEnabled() bool                       { return true }
func (s *adminTenantLifecycleSecurityStub) BanUser(context.Context, string) error { return nil }
func (s *adminTenantLifecycleSecurityStub) RevokeAccessToken(context.Context, string, time.Duration) error {
	return nil
}
func (s *adminTenantLifecycleSecurityStub) InvalidateUserSessions(context.Context, string) error {
	return nil
}
func (s *adminTenantLifecycleSecurityStub) SyncUserStatus(context.Context, string, string) error {
	return nil
}
func (s *adminTenantLifecycleSecurityStub) SyncTenantStatus(ctx context.Context, tenantID, status string, restoredUserIDs []string) error {
	s.ctx, s.tenantID, s.status = ctx, tenantID, status
	s.restoredIDs = append([]string(nil), restoredUserIDs...)
	return s.err
}

func TestAdminTenantLifecycleSynchronizesCommittedStatus(t *testing.T) {
	ctx := context.WithValue(context.Background(), adminTenantLifecycleContextKey{}, "request")
	writer := &adminTenantLifecycleWriterStub{result: tenantports.AdminTenantStatusResult{Updated: true, RestoredUserIDs: []string{"user-1"}}}
	security := &adminTenantLifecycleSecurityStub{}
	service := NewAdminTenantLifecycleService(writer, security)

	result, err := service.UpdateStatus(ctx, "tenant-1", "active")
	if err != nil || !result.Updated {
		t.Fatalf("UpdateStatus = %#v err:%v", result, err)
	}
	if writer.ctx != ctx || writer.tenantID != "tenant-1" || writer.status != "active" {
		t.Fatalf("writer call = ctx:%v tenant:%q status:%q", writer.ctx, writer.tenantID, writer.status)
	}
	if security.ctx != ctx || security.tenantID != "tenant-1" || security.status != "active" || len(security.restoredIDs) != 1 || security.restoredIDs[0] != "user-1" {
		t.Fatalf("security call = ctx:%v tenant:%q status:%q restored:%v", security.ctx, security.tenantID, security.status, security.restoredIDs)
	}
}

func TestAdminTenantLifecycleWrapsSecurityFailure(t *testing.T) {
	sentinel := errors.New("redis unavailable")
	writer := &adminTenantLifecycleWriterStub{result: tenantports.AdminTenantStatusResult{Updated: true}}
	security := &adminTenantLifecycleSecurityStub{err: sentinel}
	service := NewAdminTenantLifecycleService(writer, security)

	result, err := service.UpdateStatus(context.Background(), "tenant-1", "disabled")
	var securityErr *tenantports.AdminTenantSecurityError
	if !result.Updated || !errors.As(err, &securityErr) || !errors.Is(err, sentinel) {
		t.Fatalf("security failure = result:%#v err:%v classified:%v", result, err, securityErr)
	}
}
