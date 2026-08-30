package transport

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image/png"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"

	"xiaodou/dai/internal/auth"
	tenantports "xiaodou/dai/internal/tenant/ports"
	"xiaodou/dai/libs/go/httpx"
)

const (
	maxTenantBrandNameLength = 80
	maxFaviconBytes          = 512 << 10
	minFaviconDimension      = 64
	maxFaviconDimension      = 512
)

type tenantBrandingHandlers struct {
	reader tenantports.PortalBrandingReader
	writer tenantports.PortalBrandingWriter
}

func newTenantBrandingHandlers(reader tenantports.PortalBrandingReader, writer tenantports.PortalBrandingWriter) *tenantBrandingHandlers {
	return &tenantBrandingHandlers{reader: reader, writer: writer}
}

type tenantBrandingOutput struct {
	Body struct {
		TenantName       string `json:"tenantName"`
		CustomerSiteName string `json:"customerSiteName"`
		FaviconPath      string `json:"faviconPath,omitempty"`
	}
}

type customerPortalBrandOutput struct {
	Body struct {
		SiteName    string `json:"siteName"`
		FaviconPath string `json:"faviconPath,omitempty"`
	}
}

type updateTenantBrandingInput struct {
	Body struct {
		TenantName       string `json:"tenantName" maxLength:"80"`
		CustomerSiteName string `json:"customerSiteName" required:"false" maxLength:"80"`
	}
}

type updateTenantFaviconInput struct {
	Body struct {
		DataURL string `json:"dataUrl" maxLength:"700000"`
	}
}

func registerTenantBranding(api huma.API, d tenantBrandingModule) {
	h := newTenantBrandingHandlers(d.reader, d.writer)
	ua := userAuth(api, d.auth.JWT, d.auth.Blacklist)
	tenantOnly := huma.Middlewares{ua, requireCapability(api, auth.CapabilityTenantSelf)}
	customerOnly := huma.Middlewares{ua, requireCapability(api, auth.CapabilityCustomerSelf)}
	tenantOnly = append(tenantOnly, requireRecentAuthForMutation(api, d.auth.RecentAuth))

	huma.Register(api, huma.Operation{OperationID: "tenant-get-branding", Method: http.MethodGet, Path: "/api/v1/tenant/branding",
		Summary: "当前租户名称与用户门户品牌", Tags: []string{"tenant-branding"}, Middlewares: tenantOnly}, h.getTenantBranding)
	huma.Register(api, huma.Operation{OperationID: "tenant-update-branding", Method: http.MethodPut, Path: "/api/v1/tenant/branding",
		Summary: "更新租户名称与用户门户网站名称", Tags: []string{"tenant-branding"}, Middlewares: tenantOnly}, h.updateTenantBranding)
	huma.Register(api, huma.Operation{OperationID: "tenant-update-branding-favicon", Method: http.MethodPut, Path: "/api/v1/tenant/branding/favicon",
		Summary: "上传用户门户小图标", Tags: []string{"tenant-branding"}, Middlewares: tenantOnly}, h.updateTenantFavicon)
	huma.Register(api, huma.Operation{OperationID: "tenant-delete-branding-favicon", Method: http.MethodDelete, Path: "/api/v1/tenant/branding/favicon",
		Summary: "恢复默认用户门户小图标", Tags: []string{"tenant-branding"}, Middlewares: tenantOnly}, h.deleteTenantFavicon)
	huma.Register(api, huma.Operation{OperationID: "customer-get-portal-brand", Method: http.MethodGet, Path: "/api/v1/customer/portal-brand",
		Summary: "当前用户门户品牌", Tags: []string{"customer-branding"}, Middlewares: customerOnly}, h.getCustomerPortalBrand)
}

func registerTenantBrandingRaw(mux *chi.Mux, d tenantBrandingModule) {
	h := newTenantBrandingHandlers(d.reader, nil)
	mux.Get("/api/v1/public/tenant-brands/{tenantId}/favicon", h.serveFavicon)
}

func (h *tenantBrandingHandlers) getTenantBranding(ctx context.Context, _ *struct{}) (*tenantBrandingOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	if h.reader == nil {
		return nil, httpx.ErrUnavailable.WithDetail("租户品牌服务不可用")
	}
	branding, err := h.reader.Get(ctx, claims.TenantID)
	if err != nil {
		return nil, tenantBrandingHTTPError(err)
	}
	return tenantBrandingResponse(branding), nil
}

func (h *tenantBrandingHandlers) updateTenantBranding(ctx context.Context, in *updateTenantBrandingInput) (*tenantBrandingOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	if h.writer == nil {
		return nil, httpx.ErrUnavailable.WithDetail("租户品牌服务不可用")
	}
	tenantName, err := normalizeTenantBrandName(in.Body.TenantName, "租户名称", false)
	if err != nil {
		return nil, err
	}
	siteName, err := normalizeTenantBrandName(in.Body.CustomerSiteName, "网站名称", true)
	if err != nil {
		return nil, err
	}
	branding, err := h.writer.UpdateSettings(ctx, claims.TenantID, tenantName, siteName)
	if err != nil {
		return nil, tenantBrandingHTTPError(err)
	}
	return tenantBrandingResponse(branding), nil
}

func (h *tenantBrandingHandlers) updateTenantFavicon(ctx context.Context, in *updateTenantFaviconInput) (*tenantBrandingOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	if h.writer == nil {
		return nil, httpx.ErrUnavailable.WithDetail("租户品牌服务不可用")
	}
	faviconPNG, err := decodeTenantFavicon(in.Body.DataURL)
	if err != nil {
		return nil, httpx.ErrBadRequest.WithDetail(err.Error())
	}
	branding, err := h.writer.UpdateFavicon(ctx, claims.TenantID, faviconPNG)
	if err != nil {
		return nil, tenantBrandingHTTPError(err)
	}
	return tenantBrandingResponse(branding), nil
}

func (h *tenantBrandingHandlers) deleteTenantFavicon(ctx context.Context, _ *struct{}) (*tenantBrandingOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	if h.writer == nil {
		return nil, httpx.ErrUnavailable.WithDetail("租户品牌服务不可用")
	}
	branding, err := h.writer.ClearFavicon(ctx, claims.TenantID)
	if err != nil {
		return nil, tenantBrandingHTTPError(err)
	}
	return tenantBrandingResponse(branding), nil
}

func (h *tenantBrandingHandlers) getCustomerPortalBrand(ctx context.Context, _ *struct{}) (*customerPortalBrandOutput, error) {
	claims := userClaimsFromCtx(ctx)
	if claims == nil {
		return nil, httpx.ErrUnauthorized
	}
	if h.reader == nil {
		return nil, httpx.ErrUnavailable.WithDetail("租户品牌服务不可用")
	}
	branding, err := h.reader.Get(ctx, claims.TenantID)
	if err != nil {
		return nil, tenantBrandingHTTPError(err)
	}
	out := &customerPortalBrandOutput{}
	out.Body.SiteName = effectiveCustomerSiteName(branding)
	out.Body.FaviconPath = tenantFaviconPath(branding)
	return out, nil
}

func (h *tenantBrandingHandlers) serveFavicon(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantId")
	if strings.TrimSpace(tenantID) == "" {
		http.NotFound(w, r)
		return
	}
	if h.reader == nil {
		http.NotFound(w, r)
		return
	}
	branding, err := h.reader.Get(r.Context(), tenantID)
	if err != nil || len(branding.FaviconPNG) == 0 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	modifiedAt := time.Time{}
	if branding.FaviconUpdatedAt != nil {
		modifiedAt = *branding.FaviconUpdatedAt
	}
	http.ServeContent(w, r, "favicon.png", modifiedAt, bytes.NewReader(branding.FaviconPNG))
}

func tenantBrandingResponse(branding *tenantports.PortalBranding) *tenantBrandingOutput {
	out := &tenantBrandingOutput{}
	out.Body.TenantName = branding.TenantName
	out.Body.CustomerSiteName = branding.CustomerSiteName
	out.Body.FaviconPath = tenantFaviconPath(branding)
	return out
}

func tenantFaviconPath(branding *tenantports.PortalBranding) string {
	if branding == nil || len(branding.FaviconPNG) == 0 || branding.FaviconUpdatedAt == nil {
		return ""
	}
	return publicTenantFaviconPath(branding.TenantID, branding.FaviconUpdatedAt.UnixMilli())
}

func publicTenantFaviconPath(tenantID string, version int64) string {
	return fmt.Sprintf("/api/v1/public/tenant-brands/%s/favicon?v=%d", url.PathEscape(tenantID), version)
}

func effectiveCustomerSiteName(branding *tenantports.PortalBranding) string {
	if strings.TrimSpace(branding.CustomerSiteName) != "" {
		return branding.CustomerSiteName
	}
	return branding.TenantName
}

func normalizeTenantBrandName(value, label string, allowEmpty bool) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" && !allowEmpty {
		return "", httpx.ErrBadRequest.WithDetail(label + "不能为空")
	}
	if len([]rune(normalized)) > maxTenantBrandNameLength {
		return "", httpx.ErrBadRequest.WithDetail(fmt.Sprintf("%s不能超过 %d 个字符", label, maxTenantBrandNameLength))
	}
	return normalized, nil
}

func decodeTenantFavicon(dataURL string) ([]byte, error) {
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(dataURL, prefix) {
		return nil, errors.New("小图标必须是 PNG 图片")
	}
	encoded := strings.TrimPrefix(dataURL, prefix)
	if encoded == "" || len(encoded) > base64.StdEncoding.EncodedLen(maxFaviconBytes) {
		return nil, fmt.Errorf("小图标不能超过 %d KB", maxFaviconBytes>>10)
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("小图标内容无效")
	}
	if len(data) == 0 || len(data) > maxFaviconBytes {
		return nil, fmt.Errorf("小图标不能超过 %d KB", maxFaviconBytes>>10)
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("小图标必须是有效的 PNG 图片")
	}
	if config.Width != config.Height || config.Width < minFaviconDimension || config.Width > maxFaviconDimension {
		return nil, fmt.Errorf("小图标必须是 %d 到 %d 像素的正方形 PNG", minFaviconDimension, maxFaviconDimension)
	}
	return data, nil
}

func tenantBrandingHTTPError(err error) error {
	switch {
	case errors.Is(err, tenantports.ErrTenantNotFound):
		return httpx.ErrNotFound.WithDetail("租户不存在")
	case errors.Is(err, tenantports.ErrTenantNameTaken):
		return httpx.ErrConflict.WithDetail("租户名称已存在")
	default:
		return httpx.ErrInternal.WithCause(err)
	}
}
