package transport

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/libs/go/httpx"
)

type tenantUpstreamModelDTO struct {
	ModelCode      string             `json:"model_code"`
	CapabilityType string             `json:"capability_type"`
	APIFormat      string             `json:"api_format"`
	Availability   string             `json:"availability" enum:"available,no_price_configured"`
	Price          *priceBookEntryDTO `json:"price,omitempty"`
}

type tenantUpstreamResourceDTO struct {
	ID                string                   `json:"id"`
	ResourceKind      string                   `json:"resource_kind" enum:"direct_upstream,oauth_pool"`
	Name              string                   `json:"name"`
	TenantMultiplier  float64                  `json:"tenant_multiplier"`
	PriceBookID       string                   `json:"price_book_id,omitempty"`
	PriceBookName     string                   `json:"price_book_name,omitempty"`
	PriceBookRevision int64                    `json:"price_book_revision,omitempty"`
	Models            []tenantUpstreamModelDTO `json:"models"`
}

type tenantUpstreamResourcesOutput struct {
	Body struct {
		Items []tenantUpstreamResourceDTO `json:"items"`
		Total int                         `json:"total"`
	}
}

func registerTenantUpstreamCatalog(api huma.API, d TenantCatalogHTTPDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-list-tenant-upstream-resources",
		Method:      http.MethodGet,
		Path:        "/api/v1/tenants/me/upstream-resources",
		Summary:     "租户可选上游资源",
		Description: "返回平台开放的上游账号、结算倍率、公共模型与价格；不会返回地址、密钥、请求头、OAuth 凭据或内部模型名。",
		Tags:        []string{"upstream-resources"},
	}, func(ctx context.Context, _ *struct{}) (*tenantUpstreamResourcesOutput, error) {
		if d.ModelCatalog == nil {
			return nil, httpx.ErrUnavailable.WithDetail("model catalog reader is not configured")
		}
		tenantID := strings.TrimSpace(tenantIDFromContext(ctx))
		if tenantID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("tenant id is required")
		}
		items, err := loadTenantUpstreamResources(ctx, d.ModelCatalog, tenantID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &tenantUpstreamResourcesOutput{}
		out.Body.Items = items
		out.Body.Total = len(items)
		return out, nil
	})
}

func loadTenantUpstreamResources(ctx context.Context, reader ModelCatalogReader, tenantID string) ([]tenantUpstreamResourceDTO, error) {
	if reader == nil {
		return nil, httpx.ErrUnavailable.WithDetail("model catalog reader is not configured")
	}
	resources, err := reader.ListTenantUpstreamResources(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	items := make([]tenantUpstreamResourceDTO, 0, len(resources))
	for _, resource := range resources {
		item := tenantUpstreamResourceDTO{
			ID: resource.ID, ResourceKind: string(resource.Kind), Name: resource.Name,
			TenantMultiplier: resource.TenantMultiplier, PriceBookID: resource.PriceBookID,
			PriceBookName: resource.PriceBookName, PriceBookRevision: resource.PriceBookRevision,
			Models: make([]tenantUpstreamModelDTO, 0, len(resource.Models)),
		}
		for _, source := range resource.Models {
			model := tenantUpstreamModelDTO{
				ModelCode: source.ModelCode, CapabilityType: source.CapabilityType,
				APIFormat: source.APIFormat, Availability: "no_price_configured",
			}
			if source.Price != nil {
				price := priceBookEntryToDTO(*source.Price)
				model.Price = &price
				model.Availability = "available"
			}
			item.Models = append(item.Models, model)
		}
		items = append(items, item)
	}
	return items, nil
}
