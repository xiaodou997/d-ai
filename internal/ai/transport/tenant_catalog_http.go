package transport

import "github.com/danielgtaylor/huma/v2"

// TenantCatalogHTTPDeps is the dependency boundary for tenant self-service
// model, price-book and upstream-resource catalog routes.
type TenantCatalogHTTPDeps struct {
	Auth             HTTPAuthDeps
	ModelCatalog     ModelCatalogReader
	Groups           CommercialGroupCatalog
	TenantPriceBooks TenantPriceBookManager
	PriceBookSync    PriceBookSyncManager
}

// RegisterTenantCatalog owns the tenant-user authenticated catalog surface.
func RegisterTenantCatalog(api huma.API, d TenantCatalogHTTPDeps) {
	tenant := huma.NewGroup(api)
	tenant.UseMiddleware(tenantUserSensitiveAuth(api, d.Auth))
	registerTenantSelfPricing(tenant, d)
	registerTenantPriceBooks(tenant, d)
	registerTenantUpstreamCatalog(tenant, d)
}
