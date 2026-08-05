package transport

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/upstreamaccess"
	"xiaodou/dai/libs/go/httpx"
)

type tenantUpstreamAccessInput struct {
	TenantID string `path:"tenantID" doc:"租户 ID"`
}

type tenantUpstreamAccessDTO struct {
	ResourceKind              string   `json:"resource_kind" enum:"direct_upstream,oauth_pool"`
	ResourceID                string   `json:"resource_id"`
	InternalName              string   `json:"internal_name"`
	TenantDisplayName         string   `json:"tenant_display_name"`
	AccessMode                string   `json:"access_mode" enum:"public,restricted"`
	Status                    string   `json:"status"`
	AccessGranted             bool     `json:"access_granted"`
	Allowed                   bool     `json:"allowed"`
	DefaultTenantMultiplier   float64  `json:"default_tenant_multiplier"`
	TenantMultiplierOverride  *float64 `json:"tenant_multiplier_override,omitempty"`
	EffectiveTenantMultiplier float64  `json:"effective_tenant_multiplier"`
}

type tenantUpstreamAccessOutput struct {
	Body struct {
		Items []tenantUpstreamAccessDTO `json:"items"`
		Total int                       `json:"total"`
	}
}

type replaceTenantUpstreamAccessInput struct {
	TenantID string `path:"tenantID" doc:"租户 ID"`
	Body     struct {
		Policies []upstreamaccess.TenantResourcePolicy `json:"policies"`
	}
}

type replaceTenantUpstreamAccessOutput struct {
	Body struct {
		Updated bool `json:"updated"`
	}
}

func registerTenantUpstreamAccess(api huma.API, d AIDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-list-tenant-upstream-access",
		Method:      http.MethodGet,
		Path:        "/api/v1/tenants/{tenantID}/upstream-access",
		Summary:     "租户上游访问策略",
		Description: "列出平台上游资源及指定租户的访问状态，不返回地址、密钥或凭据。",
		Tags:        []string{"tenant-policies"},
	}, func(ctx context.Context, in *tenantUpstreamAccessInput) (*tenantUpstreamAccessOutput, error) {
		if d.UpstreamAccessSvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("upstream access service is not configured")
		}
		items, err := d.UpstreamAccessSvc.ListForTenant(ctx, in.TenantID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &tenantUpstreamAccessOutput{}
		out.Body.Items = make([]tenantUpstreamAccessDTO, 0, len(items))
		for _, item := range items {
			out.Body.Items = append(out.Body.Items, tenantUpstreamAccessDTO{
				ResourceKind: item.Kind, ResourceID: item.ID,
				InternalName: item.InternalName, TenantDisplayName: item.TenantDisplayName,
				AccessMode: item.AccessMode, Status: item.Status,
				AccessGranted: item.AccessGranted, Allowed: item.Allowed,
				DefaultTenantMultiplier:   item.DefaultTenantMultiplier,
				TenantMultiplierOverride:  item.TenantMultiplierOverride,
				EffectiveTenantMultiplier: item.EffectiveTenantMultiplier,
			})
		}
		out.Body.Total = len(out.Body.Items)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-replace-tenant-upstream-access",
		Method:      http.MethodPut,
		Path:        "/api/v1/tenants/{tenantID}/upstream-access",
		Summary:     "更新租户上游策略",
		Description: "全量替换指定租户的上游访问与结算倍率覆盖策略；public 资源始终可访问，倍率覆盖对两种开放方式都生效。",
		Tags:        []string{"tenant-policies"},
	}, func(ctx context.Context, in *replaceTenantUpstreamAccessInput) (*replaceTenantUpstreamAccessOutput, error) {
		if d.UpstreamAccessSvc == nil {
			return nil, httpx.ErrUnavailable.WithDetail("upstream access service is not configured")
		}
		if err := d.UpstreamAccessSvc.ReplacePolicies(ctx, in.TenantID, in.Body.Policies); err != nil {
			return nil, mapServiceError(err)
		}
		out := &replaceTenantUpstreamAccessOutput{}
		out.Body.Updated = true
		return out, nil
	})
}
