package transport

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"xiaodou/dai/libs/go/server"

	announcementpkg "xiaodou/dai/internal/announcement"
	"xiaodou/dai/internal/auth"
	billingsvc "xiaodou/dai/internal/billing/service"
	cleanuppkg "xiaodou/dai/internal/cleanup"
	"xiaodou/dai/internal/config"
	invitepkg "xiaodou/dai/internal/invite"
	notificationpkg "xiaodou/dai/internal/notification"
	paymentsvc "xiaodou/dai/internal/payment/service"
	systempkg "xiaodou/dai/internal/system"
	userpkg "xiaodou/dai/internal/user"

	// AI 域
	pgadapter "xiaodou/dai/internal/ai/adapters/postgres"
	"xiaodou/dai/internal/ai/billingcontrol"
	"xiaodou/dai/internal/ai/clientcatalog"
	"xiaodou/dai/internal/ai/commercial"
	aidb "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/identitycontrol"
	"xiaodou/dai/internal/ai/observabilitycontrol"
	proxypkg "xiaodou/dai/internal/ai/proxy"
	"xiaodou/dai/internal/ai/riskcontrol"
	"xiaodou/dai/internal/ai/routing"
	"xiaodou/dai/internal/ai/subscription"
	"xiaodou/dai/internal/ai/tokenrefresh"
	aitransport "xiaodou/dai/internal/ai/transport"
	"xiaodou/dai/internal/ai/upstreamaccess"
	"xiaodou/dai/internal/ai/upstreamcontrol"
	workspacesvc "xiaodou/dai/internal/ai/workspace"
	"xiaodou/dai/libs/go/banstate"
)

// Deps 汇集统一 transport 层注册端点所需的领域依赖。
type Deps struct {
	// 基础设施
	Version string
	Pool    *pgxpool.Pool
	Redis   *redis.Client
	Logger  *zap.Logger

	// 平台身份与计费域
	JWT           *auth.JWTService
	Blacklist     *auth.BlacklistService
	Legal         config.LegalConfig
	UserService   *userpkg.UserService
	Deduction     *billingsvc.DeductionService
	Invite        *invitepkg.InviteService
	Payment       *paymentsvc.PaymentService
	Announcements *announcementpkg.Service
	Notifications *notificationpkg.Service

	// AI 域
	Queries         *aidb.Queries
	OAuth           *pgadapter.OAuthCredentialStore
	TokenRefresher  *tokenrefresh.Refresher
	ClientCatalog   *clientcatalog.Service
	SecretMasterKey string
	AIHTTPClient    *http.Client
	Health          routing.HealthTracker
	Weights         *pgadapter.RouteWeightsStore
	BanChecker      *banstate.Checker

	// AI 域
	PriceBookSvc         *billingcontrol.Service
	CommercialSvc        *commercial.Service
	GroupTransferSvc     *commercial.GroupTransferService
	DashboardSvc         *observabilitycontrol.DashboardService
	UsageSvc             *observabilitycontrol.UsageService
	AuditSvc             *observabilitycontrol.AuditService
	AccountSvc           *upstreamcontrol.Service
	UpstreamAccessSvc    *upstreamaccess.Service
	APIKeySvc            *identitycontrol.Service
	WorkspaceSvc         *workspacesvc.Service
	Subscriptions        *subscription.Service
	RiskControlConfigSvc *riskcontrol.ConfigService
	RiskControlLogSvc    *riskcontrol.LogService
	RiskControlEventSvc  *riskcontrol.EventService
	RiskControlChecker   *riskcontrol.Checker
	Modules              *systempkg.Service
	ProxyNodes           *proxypkg.Service
	DataCleanup          *cleanuppkg.Service
}

// Register 在 Huma API 上注册全部端点。
func Register(api huma.API, d Deps) {
	registerMeta(api, d)
	registerPublicPlane(api, d)
}

func registerMeta(api huma.API, d Deps) {
	server.Health(api, "dai", d.Version)
	registerInfo(api, d.Version)
	registerJWKS(api, d.JWT)
}

func registerPublicPlane(api huma.API, d Deps) {
	registerAuthPublic(api, d)

	// Portal 账号端点（用户 JWT + 黑名单）
	usrAuth := huma.Middlewares{userAuth(api, d.JWT, d.Blacklist)}
	registerAuthProtected(api, d, usrAuth)

	// 管理资源端点（/api/v1，用户 JWT + 类型守卫）
	registerAdminTenants(api, d)
	registerAdminUsers(api, d)
	registerAdminFinance(api, d)
	registerAdminUsageBilling(api, d)
	registerAdminDashboard(api, d)
	registerAdminEndUsers(api, d)
	registerAccount(api, d)
	registerTenantSelf(api, d)
	registerTenantBranding(api, d)
	registerJWTKeys(api, d)
	registerPayment(api, d)
	registerTenantCash(api, d)
	registerAdminPayment(api, d)
	registerAnnouncements(api, d)
	registerProxyNodes(api, d)
	registerNotifications(api, d)
	registerModules(api, d)
	registerDataCleanup(api, d)

	// 公开端点（无认证）
	registerPublic(api, d)

	// AI 域端点
	registerAITransport(api, d)
}

// registerAITransport 将 AI 域端点注册到统一 Huma API 上。
func registerAITransport(api huma.API, d Deps) {
	aiDeps := buildAIDeps(d)
	aitransport.RegisterAI(api, aiDeps)
}

func buildAIDeps(d Deps) aitransport.AIDeps {
	identity := newAIIdentityAdapter(d.Pool, d.UserService)
	aiDeps := aitransport.AIDeps{
		Postgres:             d.Pool,
		Redis:                d.Redis,
		Queries:              d.Queries,
		OAuth:                d.OAuth,
		TokenRefresher:       d.TokenRefresher,
		ClientCatalog:        d.ClientCatalog,
		Logger:               d.Logger,
		HTTPClient:           d.AIHTTPClient,
		Health:               d.Health,
		Weights:              d.Weights,
		TokenVerifier:        d.JWT,
		TokenRevocations:     d.Blacklist,
		BanChecker:           d.BanChecker,
		SecretMasterKey:      d.SecretMasterKey,
		PriceBookSvc:         d.PriceBookSvc,
		CommercialSvc:        d.CommercialSvc,
		GroupTransferSvc:     d.GroupTransferSvc,
		DashboardSvc:         d.DashboardSvc,
		UsageSvc:             d.UsageSvc,
		AuditSvc:             d.AuditSvc,
		AccountSvc:           d.AccountSvc,
		UpstreamAccessSvc:    d.UpstreamAccessSvc,
		APIKeySvc:            d.APIKeySvc,
		WorkspaceSvc:         d.WorkspaceSvc,
		Subscriptions:        d.Subscriptions,
		RiskControlConfigSvc: d.RiskControlConfigSvc,
		RiskControlLogSvc:    d.RiskControlLogSvc,
		RiskControlEventSvc:  d.RiskControlEventSvc,
		RiskControlChecker:   d.RiskControlChecker,
	}
	if identity != nil {
		aiDeps.IdentityProvider = identity
		aiDeps.TenantEndUsers = identity
	}
	return aiDeps
}

// RegisterRaw 注册非 JSON 契约的 chi 原生端点。
func RegisterRaw(mux *chi.Mux, d Deps) {
	RegisterPublicRaw(mux, d)
}

func RegisterPublicRaw(mux *chi.Mux, d Deps) {
	// 微信支付回调（无认证，验签即鉴权）
	notifyHandler := newPaymentNotifyHandlers(d)
	mux.Post("/api/v1/payments/wechat/notify", notifyHandler.wechatNotify)
	registerTenantBrandingRaw(mux, d)
}

var _ = http.MethodGet
