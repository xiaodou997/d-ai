package transport

import (
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"xiaodou/dai/internal/auth"
	tenantports "xiaodou/dai/internal/tenant/ports"
	"xiaodou/dai/libs/go/httpx"
)

type tenantBrandingStoreStub struct {
	branding             *tenantports.PortalBranding
	updatedTenantID      string
	updatedTenantName    string
	updatedCustomerName  string
	updatedFaviconTenant string
	updatedFavicon       []byte
	clearedTenantID      string
}

func (s *tenantBrandingStoreStub) Get(context.Context, string) (*tenantports.PortalBranding, error) {
	return s.branding, nil
}

func (s *tenantBrandingStoreStub) UpdateSettings(_ context.Context, tenantID, tenantName, customerSiteName string) (*tenantports.PortalBranding, error) {
	s.updatedTenantID = tenantID
	s.updatedTenantName = tenantName
	s.updatedCustomerName = customerSiteName
	return s.branding, nil
}

func (s *tenantBrandingStoreStub) UpdateFavicon(_ context.Context, tenantID string, faviconPNG []byte) (*tenantports.PortalBranding, error) {
	s.updatedFaviconTenant = tenantID
	s.updatedFavicon = append([]byte(nil), faviconPNG...)
	return s.branding, nil
}

func (s *tenantBrandingStoreStub) ClearFavicon(_ context.Context, tenantID string) (*tenantports.PortalBranding, error) {
	s.clearedTenantID = tenantID
	return s.branding, nil
}

func brandingContext() context.Context {
	return context.WithValue(context.Background(), userClaimsCtxKey, &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		UserType: 3,
	})
}

func TestTenantBrandingHandlersUseApplicationPort(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	store := &tenantBrandingStoreStub{branding: &tenantports.PortalBranding{
		TenantID: "tenant-1", TenantName: "Tenant", CustomerSiteName: "Portal", FaviconUpdatedAt: &now,
	}}
	h := newTenantBrandingHandlers(store, store)

	input := &updateTenantBrandingInput{}
	input.Body.TenantName = "  New Tenant  "
	input.Body.CustomerSiteName = "  New Portal  "
	if _, err := h.updateTenantBranding(brandingContext(), input); err != nil {
		t.Fatalf("updateTenantBranding() error = %v", err)
	}
	if store.updatedTenantID != "tenant-1" || store.updatedTenantName != "New Tenant" || store.updatedCustomerName != "New Portal" {
		t.Fatalf("unexpected settings command: %#v", store)
	}

	if _, err := h.deleteTenantFavicon(brandingContext(), &struct{}{}); err != nil {
		t.Fatalf("deleteTenantFavicon() error = %v", err)
	}
	if store.clearedTenantID != "tenant-1" {
		t.Fatalf("clear favicon tenant = %q, want tenant-1", store.clearedTenantID)
	}
}

func TestTenantBrandingHTTPErrorUsesPortErrors(t *testing.T) {
	got := tenantBrandingHTTPError(tenantports.ErrTenantNotFound)
	if got == nil || got.(*httpx.AppError).Detail != "租户不存在" {
		t.Fatalf("not found mapping = %v", got)
	}
	got = tenantBrandingHTTPError(tenantports.ErrTenantNameTaken)
	if got == nil || got.(*httpx.AppError).Detail != "租户名称已存在" {
		t.Fatalf("name conflict mapping = %v", got)
	}
	if got := tenantBrandingHTTPError(errors.New("database unavailable")); got == nil {
		t.Fatal("expected unknown error mapping")
	}
}

func TestTenantBrandingRawHandlerUsesApplicationPort(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	store := &tenantBrandingStoreStub{branding: &tenantports.PortalBranding{
		TenantID: "tenant-1", FaviconPNG: []byte("png"), FaviconUpdatedAt: &now,
	}}
	h := newTenantBrandingHandlers(store, nil)
	req := httptest.NewRequest("GET", "/api/v1/public/tenant-brands/tenant-1/favicon", nil)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("tenantId", "tenant-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	res := httptest.NewRecorder()
	h.serveFavicon(res, req)
	if res.Code != 200 || res.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("favicon response = %d %q", res.Code, res.Header().Get("Content-Type"))
	}
	body, err := io.ReadAll(res.Result().Body)
	if err != nil {
		t.Fatalf("read favicon response: %v", err)
	}
	if string(body) != "png" {
		t.Fatalf("favicon body = %q, want png", body)
	}
}
