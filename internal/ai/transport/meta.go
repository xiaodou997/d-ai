package transport

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/billingcontrol"
	"xiaodou/dai/internal/ai/clientcatalog"
	"xiaodou/dai/internal/ai/commercial"
	coreruntime "xiaodou/dai/internal/ai/core/runtime"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/identitycontrol"
	"xiaodou/dai/internal/ai/observabilitycontrol"
	"xiaodou/dai/internal/ai/riskcontrol"
	"xiaodou/dai/internal/ai/routing"
	"xiaodou/dai/internal/ai/serving"
	"xiaodou/dai/internal/ai/subscription"
	"xiaodou/dai/internal/ai/upstreamaccess"
	"xiaodou/dai/internal/ai/upstreamcontrol"
	workspacesvc "xiaodou/dai/internal/ai/workspace"
	"xiaodou/dai/libs/go/httpx"
)

// InfrastructureDeps contains concrete process clients used by legacy
// transport adapters. It is isolated as a group so the next migration can
// replace these fields with application ports without changing every route.
type InfrastructureDeps struct {
	Postgres   *pgxpool.Pool
	Redis      *redis.Client
	Queries    *dbgen.Queries
	HTTPClient HTTPDoer
	Logger     *zap.Logger
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
	APIKeySvc         *identitycontrol.Service
	WorkspaceSvc      *workspacesvc.Service
	IdentityProvider  IdentityProvider
	TokenVerifier     TokenVerifier
	TokenRevocations  TokenRevocationChecker
	BanChecker        HumaBanChecker
	TenantEndUsers    TenantEndUserVerifier
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

// BillingDeps contains AI subscription and prepaid billing collaborators.
type BillingDeps struct {
	Subscriptions *subscription.Service
}

// CatalogDeps contains provider, model, price and upstream control-plane
// collaborators.
type CatalogDeps struct {
	ClientCatalog     ClientCatalogResolver
	PriceBookSvc      *billingcontrol.Service
	AccountSvc        *upstreamcontrol.Service
	UpstreamAccessSvc *upstreamaccess.Service
	CommercialSvc     *commercial.Service
	GroupTransferSvc  *commercial.GroupTransferService
}

// ClientCatalogResolver is the narrow model-discovery port needed by OAuth
// pool management. Cache policy and provider inspection stay in the concrete
// clientcatalog implementation owned by composition root.
type ClientCatalogResolver interface {
	Resolve(ctx context.Context, pool domain.CredentialPool) clientcatalog.Result
}

// RuntimeDeps contains request execution state and runtime policy.
type RuntimeDeps struct {
	Health          routing.HealthTracker
	Weights         ScoreWeightsStore
	ProviderSecrets ProviderSecretCodec
}

// ProviderSecretCodec is the minimal encryption capability needed by HTTP
// management flows. Raw master key material stays inside its implementation.
type ProviderSecretCodec interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

// ScoreWeightsStore is the minimal port required by the system endpoints.
// PostgreSQL-backed caching and persistence belong to the adapter package;
// transport only needs to read and update effective weights.
type ScoreWeightsStore interface {
	Get(ctx context.Context, scope string) serving.ScoreWeights
	Upsert(ctx context.Context, scope string, weights serving.ScoreWeights) error
}

// OperationsDeps contains dashboards, audit and risk-control collaborators.
type OperationsDeps struct {
	DashboardSvc *observabilitycontrol.DashboardService
	UsageSvc     *observabilitycontrol.UsageService
	AuditSvc     *observabilitycontrol.AuditService
	// 风控中心（内容安全审核）。四者始终一起装配，nil 只会发生在测试里未注入的场景。
	RiskControlConfigSvc *riskcontrol.ConfigService
	RiskControlLogSvc    *riskcontrol.LogService
	RiskControlEventSvc  *riskcontrol.EventService
	RiskControlChecker   *riskcontrol.Checker // 供 /risk-control/test 复用 Detect()，不落库
}

// AIDeps groups the explicit dependencies required by AI HTTP registration.
type AIDeps struct {
	InfrastructureDeps
	IdentityDeps
	BillingDeps
	CatalogDeps
	RuntimeDeps
	OperationsDeps
}

func RegisterAI(api huma.API, d AIDeps) {
	management := huma.NewGroup(api)
	management.UseMiddleware(platformUserAuth(api, d))
	registerPriceBooks(management, d)
	registerUpstreamAccounts(management, d)
	registerUpstreamDiscovery(management, d)
	registerUpstreamAccountTest(management, d)
	registerUpstreamModelBindings(management, d)
	registerDashboard(management, d)
	registerUsage(management, d)
	registerAudit(management, d)
	registerRiskControl(management, d)
	registerLimits(management, d)
	registerTenantUpstreamAccess(management, d)
	registerAPIKeys(management, d)
	registerOAuthPools(management, d)
	registerSystem(management, d)
	tenant := huma.NewGroup(api)
	tenant.UseMiddleware(tenantUserAuth(api, d))
	registerGroups(tenant, d)
	registerGroupTransfer(tenant, d)
	registerTenantSelfPricing(tenant, d)
	registerTenantPriceBooks(tenant, d)
	registerTenantUpstreamCatalog(tenant, d)
	registerTenantSelfAPIKeys(tenant, d)
	registerTenantSelf(tenant, d)
	registerTenantSelfWorkspace(tenant, d)
	registerTenantSelfSubscriptions(tenant, d)

	userSelf := huma.NewGroup(api)
	userSelf.UseMiddleware(endUserAuth(api, d))
	registerUserSelf(userSelf, d)
	registerUserSelfWorkspace(userSelf, d)
	registerUserSelfSubscriptions(userSelf, d)
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
	case errors.Is(err, domain.ErrNotFound), errors.Is(err, pgx.ErrNoRows):
		return httpx.ErrNotFound.WithDetail("resource not found").WithCause(err)
	case errors.Is(err, domain.ErrConflict):
		detail := "resource already exists"
		if err.Error() != domain.ErrConflict.Error() {
			detail = err.Error()
		}
		return httpx.ErrConflict.WithDetail(detail).WithCause(err)
	case errors.Is(err, domain.ErrForbidden):
		return httpx.ErrForbidden.WithDetail("forbidden").WithCause(err)
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

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return httpx.ErrConflict.WithDetail("resource already exists").WithCause(err)
		case "23503":
			return httpx.ErrBadRequest.WithDetail("referenced resource not found").WithCause(err)
		case "23514":
			return httpx.ErrBadRequest.WithDetail("invalid field value").WithCause(err)
		case "22P02":
			return httpx.ErrBadRequest.WithDetail("invalid input format").WithCause(err)
		}
	}

	return httpx.ErrInternal.WithCause(err)
}

func isInvalidUUIDError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "cannot parse UUID")
}
