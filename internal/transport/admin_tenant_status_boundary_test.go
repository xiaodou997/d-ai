package transport

import (
	"context"
	"errors"
	"net/http"
	"testing"

	tenantports "xiaodou/dai/internal/tenant/ports"
	"xiaodou/dai/libs/go/httpx"
)

type tenantProfileWriterStub struct {
	updateCalls int
	lastUpdate  tenantports.TenantUpdateCommand
}

func (s *tenantProfileWriterStub) CreateTenant(context.Context, tenantports.TenantCreateCommand) error {
	return nil
}

func (s *tenantProfileWriterStub) UpdateTenant(_ context.Context, input tenantports.TenantUpdateCommand) (bool, error) {
	s.updateCalls++
	s.lastUpdate = input
	return true, nil
}

func (s *tenantProfileWriterStub) DeleteTenant(context.Context, string) (bool, error) {
	return true, nil
}

func TestTenantProfileUpdateCannotBypassStatusLifecycle(t *testing.T) {
	writer := &tenantProfileWriterStub{}
	h := &adminHandlers{tenantWriter: writer}

	withStatus := &updateTenantInput{ID: "tenant-1"}
	withStatus.Body.TenantName = "Tenant One"
	withStatus.Body.Status = 2
	if _, err := h.updateTenant(context.Background(), withStatus); err == nil {
		t.Fatal("profile update accepted a tenant status mutation")
	} else {
		var appErr *httpx.AppError
		if !errors.As(err, &appErr) || appErr.Status != http.StatusBadRequest {
			t.Fatalf("profile status mutation error = %v, want 400", err)
		}
	}
	if writer.updateCalls != 0 {
		t.Fatalf("tenant writer called %d times for rejected status mutation", writer.updateCalls)
	}

	profileOnly := &updateTenantInput{ID: "tenant-1"}
	profileOnly.Body.TenantName = "Tenant Renamed"
	profileOnly.Body.ContactEmail = "owner@example.com"
	if _, err := h.updateTenant(context.Background(), profileOnly); err != nil {
		t.Fatalf("profile-only tenant update failed: %v", err)
	}
	if writer.updateCalls != 1 || writer.lastUpdate.TenantID != "tenant-1" ||
		writer.lastUpdate.TenantName != "Tenant Renamed" {
		t.Fatalf("profile update command = %#v, calls=%d", writer.lastUpdate, writer.updateCalls)
	}
}
