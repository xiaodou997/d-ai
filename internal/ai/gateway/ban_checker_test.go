package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

type stubBanChecker struct {
	userBanned   bool
	tenantBanned bool
	userErr      error
	tenantErr    error
}

func (s stubBanChecker) IsBanned(context.Context, string) (bool, error) {
	return s.userBanned, s.userErr
}

func (s stubBanChecker) IsTenantBanned(context.Context, string) (bool, error) {
	return s.tenantBanned, s.tenantErr
}

func TestRejectIfBannedReturnsServiceUnavailableOnCheckerError(t *testing.T) {
	gw := &Gateway{
		logger:     zap.NewNop(),
		banChecker: stubBanChecker{tenantErr: errors.New("redis down")},
	}
	rec := httptest.NewRecorder()

	if !gw.rejectIfBanned(rec, context.Background(), "tenant-a", "user-a") {
		t.Fatal("rejectIfBanned should block on checker error")
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	var out openAIErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if out.Error.Code != "service_unavailable" {
		t.Fatalf("error code = %q, want service_unavailable", out.Error.Code)
	}
}

func TestRejectIfBannedReturnsForbiddenForTenantBan(t *testing.T) {
	gw := &Gateway{
		logger:     zap.NewNop(),
		banChecker: stubBanChecker{tenantBanned: true},
	}
	rec := httptest.NewRecorder()

	if !gw.rejectIfBanned(rec, context.Background(), "tenant-a", "user-a") {
		t.Fatal("rejectIfBanned should block banned tenant")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}

	var out openAIErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if out.Error.Code != "tenant_banned" {
		t.Fatalf("error code = %q, want tenant_banned", out.Error.Code)
	}
}
