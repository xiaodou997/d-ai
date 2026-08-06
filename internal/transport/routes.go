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
	billingpg "xiaodou/dai/internal/billing/pg"
	billingsvc "xiaodou/dai/internal/billing/service"
	"xiaodou/dai/internal/config"
	invitepkg "xiaodou/dai/internal/invite"
	paymentsvc "xiaodou/dai/internal/payment/service"
	userpkg "xiaodou/dai/internal/user"

	// AI 域
	"xiaodou/dai/internal/ai/billingcontrol"
	"xiaodou/dai/internal/ai/billingledger"
	"xiaodou/dai/internal/ai/commercial"
	aidb "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/filestore"
	"xiaodou/dai/internal/ai/identitycontrol"
	"xiaodou/dai/internal/ai/imageassets"
	"xiaodou/dai/internal/ai/observabilitycontrol"
	"xiaodou/dai/internal/ai/riskcontrol"
	"xiaodou/dai/internal/ai/subscription"
	aitransport "xiaodou/dai/internal/ai/transport"
	"xiaodou/dai/internal/ai/upstreamaccess"
	"xiaodou/dai/internal/ai/upstreamcontrol"
	workspacesvc "xiaodou/dai/internal/ai/workspace"
	"xiaodou/dai/libs/go/banstate"
)

// Deps 汇集统一 transport 层注册端点所需的领域依赖。
type Deps struct {
	// 基础设施
	Version       string
	Pool          *pgxpool.Pool
	Redis         *redis.Client
	Logger        *zap.Logger
	PortalBaseURL string

	// 平台身份与计费域
	JWT           *auth.JWTService
	Blacklist     *auth.BlacklistService
	Legal         config.LegalConfig
	UserService   *userpkg.UserService
	Deduction     *billingsvc.DeductionService
	CreditLeases  *billingsvc.CreditLeaseService
	BillingRepo   *billingpg.BillingRepository
	Invite        *invitepkg.InviteService
	Payment       *paymentsvc.PaymentService
	Announcements *announcementpkg.Service

	// AI 域
	Queries            *aidb.Queries
	BillingCoordinator *billingledger.Coordinator
	BanChecker         *banstate.Checker
	Security           config.SecurityConfig
	Audit              config.AuditConfig
	AsyncTasks         config.AsyncTaskConfig
	Files              config.FileStoreConfig
	Image              config.ImageConfig
	Pricing            config.PricingConfig

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
	FileStore            *filestore.Service
	ImageAssets          *imageassets.Service
	RiskControlConfigSvc *riskcontrol.ConfigService
	RiskControlLogSvc    *riskcontrol.LogService
	RiskControlEventSvc  *riskcontrol.EventService
	RiskControlChecker   *riskcontrol.Checker
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
	registerAdminDashboard(api, d)
	registerAdminBillingEvents(api, d)
	registerAdminEndUsers(api, d)
	registerAccount(api, d)
	registerTenantSelf(api, d)
	registerTenantBranding(api, d)
	registerJWTKeys(api, d)
	registerPayment(api, d)
	registerTenantCash(api, d)
	registerAdminPayment(api, d)
	registerAnnouncements(api, d)

	// 公开端点（无认证）
	registerPublic(api, d)

	// AI 域端点
	registerAITransport(api, d)
}

// registerAITransport 将 AI 域端点注册到统一 Huma API 上。
func registerAITransport(api huma.API, d Deps) {
	identity := newAIIdentityAdapter(d.Pool, d.UserService)
	aiDeps := aitransport.AIDeps{
		Postgres:             d.Pool,
		Redis:                d.Redis,
		Queries:              d.Queries,
		Logger:               d.Logger,
		TokenVerifier:        d.JWT,
		BanChecker:           d.BanChecker,
		ProviderKeyMaster:    d.Security.ProviderKeyMaster,
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
	aitransport.RegisterAI(api, aiDeps)
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
