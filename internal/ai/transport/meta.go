package transport

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/billingcontrol"
	"xiaodou/dai/internal/ai/clientcatalog"
	"xiaodou/dai/internal/ai/commercial"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
	coreruntime "xiaodou/dai/internal/ai/core/runtime"
	"xiaodou/dai/internal/ai/core/surface"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/identitycontrol"
	"xiaodou/dai/internal/ai/upstreamaccess"
	"xiaodou/dai/internal/ai/upstreamcontrol"
	"xiaodou/dai/internal/ai/workspace"
	"xiaodou/dai/libs/go/httpx"
)

// InfrastructureDeps contains process-level capabilities shared by AI HTTP
// handlers. Concrete clients remain owned by composition and adapters.
type InfrastructureDeps struct {
	HTTPClient HTTPDoer
}

// HTTPDoer is the only outbound HTTP capability required by AI transport.
// Connection pooling, redirects and transport-level timeouts remain owned by
// the concrete client constructed at the composition root.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// IdentityDeps contains authentication, API key and workspace identity
// collaborators used by AI routes.
type IdentityDeps struct {
	CredentialCreator OAuthCredentialCreator
	CredentialReader  OAuthCredentialReader
	CredentialWriter  OAuthCredentialWriter
	PoolReader        OAuthPoolReader
	PoolWriter        OAuthPoolWriter
	PoolHealthReader  OAuthPoolHealthReader
	TokenRefresher    OAuthTokenRefresher
	APIKeys           APIKeyReader
	APIKeyWriter      APIKeyWriter
	APIKeyLifecycle   APIKeyLifecycleManager
	APIKeySecrets     APIKeySecretManager
	WorkspaceOverview workspace.OverviewReader
	WorkspaceModels   workspace.ChatModelReader
	WorkspaceSessions workspace.ChatSessionReader
	WorkspaceManager  workspace.ChatSessionManager
	WorkspaceImages   workspace.ImageJobReader
	IdentityProvider  IdentityProvider
	TokenVerifier     TokenVerifier
	TokenRevocations  TokenRevocationChecker
	BanChecker        HumaBanChecker
	TenantEndUsers    TenantEndUserVerifier
}

// APIKeyReader exposes only non-secret API key summaries. Ownership checks use
// the same scoped lists so callers cannot probe keys outside their tenant/user.
type APIKeyReader interface {
	ListForTenant(ctx context.Context, tenantID string) ([]coreidentity.APIKey, error)
	ListForUser(ctx context.Context, tenantID, userID string) ([]coreidentity.APIKey, error)
}

// APIKeyWriter contains API key creation and mutable metadata operations.
type APIKeyWriter interface {
	Create(ctx context.Context, in identitycontrol.CreateInput) (identitycontrol.Created, error)
	Update(ctx context.Context, in identitycontrol.UpdateInput) (coreidentity.APIKey, error)
}

// APIKeyLifecycleManager owns status transitions and deletion. Creation routes
// also require this port because limit-policy failure triggers a compensating
// key deletion.
type APIKeyLifecycleManager interface {
	UpdateStatus(ctx context.Context, id, tenantID, status string) (coreidentity.APIKey, error)
	Delete(ctx context.Context, id, tenantID string) error
}

// APIKeySecretManager isolates plaintext access for an existing key. Newly
// minted plaintext remains part of APIKeyWriter.Create's creation result.
type APIKeySecretManager interface {
	Reveal(ctx context.Context, id, tenantID string) (string, error)
	Rotate(ctx context.Context, id, tenantID string) (identitycontrol.Created, error)
}

// OAuthPoolHealthReader is the aggregate query port used by the pool health
// management endpoint.
type OAuthPoolHealthReader interface {
	GetPoolHealthSummary(ctx context.Context) ([]domain.OAuthPoolHealthSummary, error)
}

// OAuthCredentialCreator is the secret-bearing write port needed only by the
// credential import endpoint. Serving and token refresh use separate ports.
type OAuthCredentialCreator interface {
	Create(ctx context.Context, poolID string, in domain.OAuthCredentialCreate) (string, error)
}

// OAuthTokenRefresher is the manual-refresh port needed by credential
// management endpoints. The background polling implementation stays outside
// the transport package.
type OAuthTokenRefresher interface {
	RefreshByID(ctx context.Context, credID string) error
}

// OAuthPoolReader is the non-secret pool query port used by management,
// model-binding and credential import routes.
type OAuthPoolReader interface {
	ListPools(ctx context.Context) ([]domain.CredentialPool, error)
	GetPool(ctx context.Context, poolID string) (*domain.CredentialPool, error)
}

// OAuthPoolWriter contains the pool management mutations. Credential serving
// and health aggregation are separate concerns.
type OAuthPoolWriter interface {
	CreatePool(ctx context.Context, in domain.CredentialPoolCreate) (string, error)
	UpdatePool(ctx context.Context, poolID string, in domain.CredentialPoolUpdate) error
	UpdatePoolStatus(ctx context.Context, poolID, status string) error
	DeletePool(ctx context.Context, poolID string) error
}

// OAuthCredentialReader is the narrow non-secret read port needed by
// credential management endpoints. Ciphertexts and write/lifecycle operations
// remain inside the transitional OAuth store.
type OAuthCredentialReader interface {
	ListForPool(ctx context.Context, poolID string) ([]domain.OAuthCredentialSummary, error)
	GetSummaryByID(ctx context.Context, credID string) (*domain.OAuthCredentialSummary, error)
}

// OAuthCredentialWriter contains the management mutations for an existing
// credential. Import and serving lifecycle operations are separate concerns.
type OAuthCredentialWriter interface {
	UpdateStatus(ctx context.Context, credID string, status string) error
	UpdateWeight(ctx context.Context, credID string, weight int) error
	Delete(ctx context.Context, credID string) error
}

// CatalogDeps contains provider, model, price and upstream control-plane
// collaborators.
type CatalogDeps struct {
	ClientCatalog      ClientCatalogResolver
	ModelCapabilities  ModelCapabilityResolver
	AccountReader      UpstreamAccountReader
	ModelBindings      UpstreamModelBindingStore
	ModelCatalog       ModelCatalogReader
	PriceBooks         PriceBookReader
	PlatformPriceBooks PlatformPriceBookManager
	TenantPriceBooks   TenantPriceBookManager
	PriceBookSync      PriceBookSyncManager
	Accounts           UpstreamAccountCatalog
	AccountManager     UpstreamAccountManager
	AccountHealth      UpstreamAccountHealthWriter
	UpstreamAccess     UpstreamAccessManager
	Groups             CommercialGroupCatalog
	GroupManager       CommercialGroupManager
	DispatchRules      CommercialDispatchRuleManager
	GroupTargets       CommercialGroupTargetManager
	UserBindings       CommercialUserBindingManager
	LimitPolicies      CommercialLimitPolicyManager
	GroupTransfer      GroupTransferManager
}

// UpstreamAccessManager is the tenant policy surface required by management
// routes. Runtime authorization remains a separate adapter concern.
type UpstreamAccessManager interface {
	ListForTenant(ctx context.Context, tenantID string) ([]upstreamaccess.ResourceAccess, error)
	ReplacePolicies(ctx context.Context, tenantID string, policies []upstreamaccess.TenantResourcePolicy) error
}

// GroupTransferManager is the complete group configuration transfer workflow
// used by tenant routes. Persistence and planning stay in the commercial module.
type GroupTransferManager interface {
	Export(ctx context.Context, tenantID string, groupIDs []string) (commercial.GroupTransferBundle, error)
	Preview(ctx context.Context, tenantID string, request commercial.GroupImportRequest) (commercial.GroupImportPreview, error)
	Import(ctx context.Context, tenantID string, request commercial.GroupImportRequest) (commercial.GroupImportResult, error)
}

// CommercialGroupCatalog is the tenant-scoped group read and visibility
// surface shared by management, self-service pricing and API key validation.
type CommercialGroupCatalog interface {
	ListGroups(ctx context.Context, tenantID string) ([]commercial.Group, error)
	GetGroup(ctx context.Context, scope commercial.TenantGroupScope) (commercial.Group, error)
	ListVisibleGroupsForTenant(ctx context.Context, tenantID string) ([]commercial.AccessibleGroup, error)
	ListVisibleGroupsForUser(ctx context.Context, tenantID, userID string) ([]commercial.AccessibleGroup, error)
}

// CommercialGroupManager owns group configuration and client-surface policy
// mutations. Group discovery remains on CommercialGroupCatalog.
type CommercialGroupManager interface {
	CreateGroup(ctx context.Context, tenantID string, input commercial.GroupWrite) (commercial.Group, error)
	UpdateGroup(ctx context.Context, scope commercial.TenantGroupScope, input commercial.GroupWrite) (commercial.Group, error)
	UpdateGroupStatus(ctx context.Context, scope commercial.TenantGroupScope, status commercial.Status) (commercial.Group, error)
	DeleteGroup(ctx context.Context, scope commercial.TenantGroupScope) error
	GetGroupClientSurfacePolicy(ctx context.Context, scope commercial.TenantGroupScope) (commercial.GroupClientSurfacePolicy, error)
	ReplaceGroupClientSurfacePolicy(ctx context.Context, scope commercial.TenantGroupScope, input commercial.GroupClientSurfacePolicyWrite) (commercial.GroupClientSurfacePolicy, error)
}

// CommercialDispatchRuleManager owns model dispatch configuration and its
// management-time preview projections.
type CommercialDispatchRuleManager interface {
	ListDispatchRules(ctx context.Context, scope commercial.TenantGroupScope) ([]commercial.DispatchRule, error)
	PreviewDispatch(ctx context.Context, scope commercial.TenantGroupScope, requestedModel string, clientSurface surface.ID) (commercial.DispatchPreview, error)
	AddDispatchRule(ctx context.Context, scope commercial.TenantGroupScope, input commercial.DispatchRuleWrite) (commercial.DispatchRule, error)
	UpdateDispatchRule(ctx context.Context, scope commercial.TenantGroupScope, id string, input commercial.DispatchRuleWrite) (commercial.DispatchRule, error)
	UpdateDispatchRuleStatus(ctx context.Context, scope commercial.TenantGroupScope, id string, status commercial.Status) (commercial.DispatchRule, error)
	ListDispatchModels(ctx context.Context, scope commercial.TenantGroupScope, clientSurface surface.ID) ([]commercial.DispatchModel, error)
	DeleteDispatchRule(ctx context.Context, scope commercial.TenantGroupScope, id string) error
}

// CommercialGroupTargetManager owns group-to-upstream bindings.
type CommercialGroupTargetManager interface {
	ListGroupTargetDetails(ctx context.Context, scope commercial.TenantGroupScope) ([]commercial.GroupTargetDetail, error)
	AddGroupTarget(ctx context.Context, scope commercial.TenantGroupScope, input commercial.GroupTargetWrite) (commercial.GroupTarget, error)
	GetGroupTargetDetail(ctx context.Context, scope commercial.TenantGroupScope, id string) (commercial.GroupTargetDetail, error)
	UpdateGroupTarget(ctx context.Context, scope commercial.TenantGroupScope, id string, input commercial.GroupTargetWrite) (commercial.GroupTarget, error)
	DeleteGroupTarget(ctx context.Context, scope commercial.TenantGroupScope, id string) error
}

// CommercialUserBindingManager owns explicit end-user group assignments.
type CommercialUserBindingManager interface {
	ListUserBindings(ctx context.Context, tenantID, userID string) ([]commercial.UserGroupBinding, error)
	UpsertUserBinding(ctx context.Context, input commercial.UserGroupBindingWrite) (commercial.UserGroupBinding, error)
	DeleteUserBinding(ctx context.Context, tenantID, userID, groupID string) error
}

// CommercialLimitPolicyManager owns tenant, user and API-key concurrency
// policies used by both administrator and self-service routes.
type CommercialLimitPolicyManager interface {
	CreateLimitPolicy(ctx context.Context, input commercial.LimitPolicyWrite) (commercial.LimitPolicy, error)
	ListLimitPolicies(ctx context.Context, filter commercial.LimitPolicyFilter) ([]commercial.LimitPolicy, error)
	UpdateLimitPolicy(ctx context.Context, id string, input commercial.LimitPolicyWrite) (commercial.LimitPolicy, error)
	UpdateLimitPolicyStatus(ctx context.Context, id string, status commercial.Status) (commercial.LimitPolicy, error)
	DeleteLimitPolicies(ctx context.Context, filter commercial.LimitPolicyFilter) error
}

// ClientCatalogResolver is the narrow model-discovery port needed by OAuth
// pool management. Cache policy and provider inspection stay in the concrete
// clientcatalog implementation owned by composition root.
type ClientCatalogResolver interface {
	Resolve(ctx context.Context, pool domain.CredentialPool) clientcatalog.Result
}

// ModelCapabilityResolver is the read-only capability suggestion port used by
// upstream model forms. External directory, Redis and cache policy stay in the
// concrete resolver assembled at the composition root.
type ModelCapabilityResolver interface {
	Lookup(ctx context.Context, modelCode string) (domain.CapabilityType, bool)
}

// UpstreamAccountReader exposes the secret-bearing account fields needed by
// management flows without leaking generated persistence rows into transport.
type UpstreamAccountReader interface {
	GetAccountSecret(ctx context.Context, id string) (upstreamcontrol.AccountSecret, error)
}

// UpstreamAccountCatalog is the non-secret account list projection used by
// management and transfer workflows.
type UpstreamAccountCatalog interface {
	ListAccounts(ctx context.Context) ([]domain.UpstreamAccount, error)
}

// UpstreamAccountManager owns administrator-initiated account mutations.
type UpstreamAccountManager interface {
	CreateAccount(ctx context.Context, input upstreamcontrol.CreateAccountInput) (domain.UpstreamAccount, error)
	UpdateAccount(ctx context.Context, input upstreamcontrol.UpdateAccountInput) (domain.UpstreamAccount, error)
	UpdateAccountStatus(ctx context.Context, id, status string) (domain.UpstreamAccount, error)
	DeleteAccount(ctx context.Context, id string) error
}

// UpstreamAccountHealthWriter owns runtime status reconciliation after a
// connectivity probe. It cannot edit account configuration or delete rows.
type UpstreamAccountHealthWriter interface {
	UpdateAccountStatus(ctx context.Context, id, status string) (domain.UpstreamAccount, error)
	MarkAccountInvalid(ctx context.Context, id, reason string) (domain.UpstreamAccount, error)
}

// UpstreamModelBindingStore owns management persistence and the atomic model
// discovery import. Transport retains request validation and DTO mapping.
type UpstreamModelBindingStore interface {
	List(ctx context.Context, scope domain.UpstreamModelBindingScope) ([]domain.UpstreamModelBinding, error)
	ListModelCodes(ctx context.Context, scope domain.UpstreamModelBindingScope) ([]string, error)
	FindByModel(ctx context.Context, scope domain.UpstreamModelBindingScope, modelCode string) (domain.UpstreamModelBinding, error)
	Get(ctx context.Context, scope domain.UpstreamModelBindingScope, bindingID string) (domain.UpstreamModelBinding, error)
	Create(ctx context.Context, scope domain.UpstreamModelBindingScope, write domain.UpstreamModelBindingWrite) (domain.UpstreamModelBinding, error)
	Update(ctx context.Context, scope domain.UpstreamModelBindingScope, bindingID string, write domain.UpstreamModelBindingWrite) (domain.UpstreamModelBinding, error)
	Delete(ctx context.Context, scope domain.UpstreamModelBindingScope, bindingID string) error
	BatchDelete(ctx context.Context, scope domain.UpstreamModelBindingScope, bindingIDs []string) (int64, error)
	Import(ctx context.Context, scope domain.UpstreamModelBindingScope, writes []domain.UpstreamModelBindingWrite) (domain.UpstreamModelBindingImportResult, error)
}

// ModelCatalogReader owns the aggregate visibility and pricing queries used
// by tenant and user catalog endpoints.
type ModelCatalogReader interface {
	ListAvailableModelPrices(ctx context.Context, scope domain.ModelCatalogScope) ([]domain.RoutedModelPrice, error)
	ListRoutedGroupPrices(ctx context.Context, groupID string) ([]domain.RoutedModelPrice, error)
	ListTenantUpstreamResources(ctx context.Context, tenantID string) ([]domain.TenantUpstreamResource, error)
}

type PriceBookReader interface {
	GetPriceBook(ctx context.Context, id string) (domain.PriceBook, error)
}

// PlatformPriceBookManager owns platform-scoped price books and their manual
// entries. Tenant ownership and external catalog synchronization are separate
// capabilities.
type PlatformPriceBookManager interface {
	ListPriceBooks(ctx context.Context) ([]domain.PriceBook, error)
	CreatePriceBook(ctx context.Context, name, description string) (domain.PriceBook, error)
	GetPlatformPriceBook(ctx context.Context, id string) (domain.PriceBook, error)
	UpdatePriceBook(ctx context.Context, id, name, description, status string) (domain.PriceBook, error)
	DeletePriceBook(ctx context.Context, id string) error
	ListEntries(ctx context.Context, priceBookID string) ([]domain.PriceBookEntry, error)
	UpsertEntry(ctx context.Context, entry domain.PriceBookEntry) (domain.PriceBookEntry, error)
	DeleteEntry(ctx context.Context, priceBookID, modelCode, capabilityType string) error
}

// TenantPriceBookManager owns tenant-scoped price books, visible platform
// projections and portable transfer operations.
type TenantPriceBookManager interface {
	ListVisiblePriceBooks(ctx context.Context, tenantID string) ([]domain.PriceBook, error)
	CreateTenantPriceBook(ctx context.Context, tenantID, name, description string) (domain.PriceBook, error)
	GetVisiblePriceBook(ctx context.Context, tenantID, id string) (domain.PriceBook, error)
	UpdateTenantPriceBook(ctx context.Context, tenantID, id, name, description, status string) (domain.PriceBook, error)
	DeleteTenantPriceBook(ctx context.Context, tenantID, id string) error
	ListVisibleEntries(ctx context.Context, tenantID, priceBookID string) ([]domain.PriceBookEntry, error)
	UpsertTenantEntry(ctx context.Context, tenantID string, entry domain.PriceBookEntry) (domain.PriceBookEntry, error)
	DeleteTenantEntry(ctx context.Context, tenantID, priceBookID, modelCode, capabilityType string) error
	CloneVisiblePriceBook(ctx context.Context, tenantID, sourceID, name string) (domain.PriceBook, error)
}

// PriceBookSyncManager is the external pricing catalog surface shared by
// platform and tenant synchronization routes.
type PriceBookSyncManager interface {
	SearchLiteLLM(ctx context.Context, query string, limit int) ([]billingcontrol.LiteLLMModelInfo, error)
	ImportFromLiteLLM(ctx context.Context, priceBookID string) (billingcontrol.ImportResult, error)
	SyncCommonModels(ctx context.Context, priceBookID string) (billingcontrol.SyncResult, error)
	ImportTenantFromLiteLLM(ctx context.Context, tenantID, priceBookID string) (billingcontrol.ImportResult, error)
	SyncTenantCommonModels(ctx context.Context, tenantID, priceBookID string) (billingcontrol.SyncResult, error)
}

// UserUsageLogReader exposes the current user's scoped usage projection while
// keeping generated query parameters and rows inside the persistence adapter.
type UserUsageLogReader interface {
	ListUserLogs(ctx context.Context, tenantID, userID, requestSource string, limit int32) ([]domain.UsageLog, error)
}

// AdminAuditRecorder is the write-only audit port used by management
// mutations. Persistence failures are handled by each caller's policy.
type AdminAuditRecorder interface {
	Record(ctx context.Context, event domain.AdminAuditEvent) error
}

// DashboardQueryReader is the aggregate analytics projection shared by
// management, tenant self-service and workspace overview routes.
type DashboardQueryReader interface {
	Summary(ctx context.Context, filter domain.DashboardFilter) (domain.DashboardSummary, error)
	TopModels(ctx context.Context, filter domain.DashboardFilter, limit int32) ([]domain.DashboardTopModel, error)
	TopTenants(ctx context.Context, filter domain.DashboardFilter, limit int32) ([]domain.DashboardTopTenant, error)
	RecentErrors(ctx context.Context, filter domain.DashboardFilter, limit int32) ([]domain.DashboardRecentError, error)
}

// UsageQueryReader contains the aggregate and paginated usage reads shared by
// management, tenant self-service, user self-service and workspace routes.
// The dedicated UserUsageLogReader remains separate for the restricted user
// log projection.
type UsageQueryReader interface {
	DailyTrend(ctx context.Context, dateFrom, dateTo *time.Time) ([]domain.DailyTrendRow, error)
	ListLogs(ctx context.Context, filter domain.UsageFilter, limit, offset int32) (domain.UsageLogPage, error)
	GetLogDetail(ctx context.Context, requestID string) (domain.UsageLogDetail, error)
	Summary(ctx context.Context, filter domain.UsageSummaryFilter) ([]domain.UsageSummaryRow, error)
	UnitSummary(ctx context.Context, filter domain.UsageSummaryFilter) ([]domain.UsageUnitSummaryRow, error)
	UpstreamSummary(ctx context.Context, filter domain.UsageSummaryFilter) ([]domain.UsageUpstreamSummaryRow, error)
	UserRanking(ctx context.Context, filter domain.UsageSummaryFilter, limit int32) ([]domain.UsageUserRankingRow, error)
	UserSummary(ctx context.Context, tenantID, userID, requestSource string) (domain.UserUsageSummary, error)
}

// RuntimeDeps contains request execution state and runtime policy.
type RuntimeDeps struct {
	ProviderSecrets ProviderSecretCodec
}

// ProviderSecretCodec is the minimal encryption capability needed by HTTP
// management flows. Raw master key material stays inside its implementation.
type ProviderSecretCodec interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

// OperationsDeps contains dashboard, usage, audit and enrichment collaborators.
type OperationsDeps struct {
	DashboardQueries           DashboardQueryReader
	UsageQueries               UsageQueryReader
	UserUsageLogs              UserUsageLogReader
	AdminAudit                 AdminAuditRecorder
	IdentityEnrichmentFailures IdentityEnrichmentFailureObserver
}

// IdentityEnrichmentFailureObserver records fail-open identity lookup errors
// without coupling transport to a logging implementation or field type.
type IdentityEnrichmentFailureObserver interface {
	ObserveFailure(kind string, err error)
}

// AIDeps groups the explicit dependencies required by AI HTTP registration.
type AIDeps struct {
	InfrastructureDeps
	IdentityDeps
	CatalogDeps
	RuntimeDeps
	OperationsDeps
}

// RegisterAICore registers the shared AI control-plane routes that remain in
// the core module. Vertically extracted modules register separately.
func RegisterAICore(api huma.API, d AIDeps) {
	auth := httpAuthDepsFromAI(d)
	management := huma.NewGroup(api)
	management.UseMiddleware(platformUserAuth(api, auth))
	registerPriceBooks(management, d)
	registerUpstreamAccounts(management, d)
	registerUpstreamDiscovery(management, d)
	registerUpstreamAccountTest(management, d)
	registerUpstreamModelBindings(management, d)
	registerDashboard(management, d)
	registerUsage(management, d)
	registerLimits(management, d)
	registerTenantUpstreamAccess(management, d)
	registerAPIKeys(management, d)
	registerOAuthPools(management, d)
	tenant := huma.NewGroup(api)
	tenant.UseMiddleware(tenantUserAuth(api, auth))
	registerGroups(tenant, d)
	registerGroupTransfer(tenant, d)
	registerTenantSelfPricing(tenant, d)
	registerTenantPriceBooks(tenant, d)
	registerTenantUpstreamCatalog(tenant, d)
	registerTenantSelfAPIKeys(tenant, d)
	registerTenantSelf(tenant, d)
	registerTenantSelfWorkspace(tenant, d)

	userSelf := huma.NewGroup(api)
	userSelf.UseMiddleware(endUserAuth(api, auth))
	registerUserSelf(userSelf, d)
	registerUserSelfWorkspace(userSelf, d)
}

func httpAuthDepsFromAI(d AIDeps) HTTPAuthDeps {
	return HTTPAuthDeps{
		TokenVerifier:    d.TokenVerifier,
		TokenRevocations: d.TokenRevocations,
		BanChecker:       d.BanChecker,
	}
}

func mapServiceError(err error) error {
	if err == nil {
		return nil
	}

	var verr *domain.ValidationError
	var commercialErr *commercial.ValidationError
	var priceConflict *domain.DispatchRulePriceConflictError
	var groupInUse *domain.GroupInUseError
	switch {
	case errors.As(err, &priceConflict):
		return httpx.New("dispatch_rule_price_conflict", http.StatusConflict, "Conflict").
			WithDetail("调度规则目标模型在分组零售价格表中缺少所需能力价格").
			WithMeta(map[string]any{"conflicts": priceConflict.Conflicts}).
			WithCause(err)
	case errors.As(err, &groupInUse):
		return httpx.New("group_in_use", http.StatusConflict, "Conflict").
			WithDetail("分组仍被业务配置引用，请先解除引用").
			WithMeta(map[string]any{
				"group_id":     groupInUse.GroupID,
				"group_name":   groupInUse.GroupName,
				"dependencies": groupInUse.Dependencies,
			}).
			WithCause(err)
	case errors.As(err, &commercialErr):
		detail := commercialErr.Message
		if commercialErr.Field != "" {
			detail = commercialErr.Field + ": " + commercialErr.Message
		}
		return httpx.ErrBadRequest.WithDetail(detail).WithCause(err)
	case errors.As(err, &verr):
		detail := verr.Message
		if verr.Field != "" {
			detail = verr.Field + ": " + verr.Message
		}
		return httpx.ErrBadRequest.WithDetail(detail).WithCause(err)
	case errors.Is(err, domain.ErrValidation):
		return httpx.ErrBadRequest.WithDetail(err.Error()).WithCause(err)
	case errors.Is(err, domain.ErrNotFound):
		return httpx.ErrNotFound.WithDetail("resource not found").WithCause(err)
	case errors.Is(err, domain.ErrConflict):
		detail := "resource already exists"
		if err.Error() != domain.ErrConflict.Error() {
			detail = err.Error()
		}
		return httpx.ErrConflict.WithDetail(detail).WithCause(err)
	case errors.Is(err, domain.ErrForbidden):
		return httpx.ErrForbidden.WithDetail("forbidden").WithCause(err)
	case errors.Is(err, domain.ErrReferencedResourceNotFound):
		return httpx.ErrBadRequest.WithDetail("referenced resource not found").WithCause(err)
	case errors.Is(err, domain.ErrInvalidFieldValue):
		return httpx.ErrBadRequest.WithDetail("invalid field value").WithCause(err)
	case errors.Is(err, domain.ErrInvalidInputFormat):
		return httpx.ErrBadRequest.WithDetail("invalid input format").WithCause(err)
	case errors.Is(err, commercial.ErrNoAccessibleGroup), errors.Is(err, coreruntime.ErrNoAllowedGroup):
		return httpx.ErrForbidden.WithDetail("no group is accessible to this caller").WithCause(err)
	case errors.Is(err, commercial.ErrClientSurfaceNotAllowed):
		return httpx.ErrForbidden.WithDetail("this API endpoint is not enabled for the group").WithCause(err)
	case errors.Is(err, coreruntime.ErrNoDispatchOption), errors.Is(err, coreruntime.ErrNoRouteCandidates):
		return httpx.New("no_available_route", http.StatusServiceUnavailable, "Service Unavailable").
			WithDetail("no available upstream route for this request").
			WithCause(err)
	case isInvalidUUIDError(err):
		return httpx.ErrBadRequest.WithDetail("invalid UUID").WithCause(err)
	}

	return httpx.ErrInternal.WithCause(err)
}

func isInvalidUUIDError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "cannot parse UUID")
}
