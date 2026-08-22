package transport

import "github.com/danielgtaylor/huma/v2"

// TenantGroupManagementHTTPDeps is the dependency boundary for tenant-owned
// groups, dispatch rules, upstream targets, user bindings and group transfer.
type TenantGroupManagementHTTPDeps struct {
	Auth             HTTPAuthDeps
	Groups           CommercialGroupCatalog
	GroupManager     CommercialGroupManager
	DispatchRules    CommercialDispatchRuleManager
	GroupTargets     CommercialGroupTargetManager
	UserBindings     CommercialUserBindingManager
	TenantEndUsers   TenantEndUserVerifier
	TenantPriceBooks TenantPriceBookManager
	GroupTransfer    GroupTransferManager
	AdminAudit       AdminAuditRecorder
}

// RegisterTenantGroupManagement owns the tenant-user authenticated commercial
// group control-plane routes and their portable transfer workflow.
func RegisterTenantGroupManagement(api huma.API, d TenantGroupManagementHTTPDeps) {
	tenant := huma.NewGroup(api)
	tenant.UseMiddleware(tenantUserAuth(api, d.Auth))
	registerGroups(tenant, d)
	registerGroupTransfer(tenant, d)
}
