package transport

import (
	"context"
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
	aidb "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/billingledger"
	"xiaodou/dai/internal/ai/billingcontrol"
	"xiaodou/dai/internal/ai/commercial"
	"xiaodou/dai/internal/ai/filestore"
	"xiaodou/dai/internal/ai/identitycontrol"
	"xiaodou/dai/internal/ai/imageassets"
	"xiaodou/dai/internal/ai/observabilitycontrol"
	"xiaodou/dai/internal/ai/riskcontrol"
	"xiaodou/dai/internal/ai/subscription"
	"xiaodou/dai/internal/ai/upstreamaccess"
	"xiaodou/dai/internal/ai/upstreamcontrol"
	workspacesvc "xiaodou/dai/internal/ai/workspace"
	aitransport "xiaodou/dai/internal/ai/transport"
	"xiaodou/dai/libs/go/banstate"
	serviceaccess "xiaodou/dai/internal/serviceaccess"
)

// Version 是 D-AI 对外版本号。
const Version = "0.1.0"

// Deps 汇集 transport 层注册端点所需的全部领域依赖（URM + AI 统一）。
type Deps struct {
	// 基础设施
	Service string
	Version string
	Pool    *pgxpool.Pool
	Redis   *redis.Client
	Logger  *zap.Logger
	Config  *config.Config

	// URM 域
	JWT           *auth.JWTService
	Blacklist     *auth.BlacklistService
	Session       *auth.SessionService
	SSO           config.SSOConfig
	Legal         config.LegalConfig
	UserService   *userpkg.UserService
	Deduction     *billingsvc.DeductionService
	CreditLeases  *billingsvc.CreditLeaseService
	BillingRepo   *billingpg.BillingRepository
	Invite        *invitepkg.InviteService
	Payment       *paymentsvc.PaymentService
	ServiceAccess *serviceaccess.Service
	Announcements *announcementpkg.Service
	Delegation    config.DelegationConfig

	// AI 域
	Queries            *aidb.Queries
	BillingCoordinator *billingledger.Coordinator
	JWKSValidator      JWKSValidator
	BanChecker         *banstate.Checker
	Security           config.SecurityConfig
	Audit              config.AuditConfig
	AsyncTasks         config.AsyncTaskConfig
	Files              config.FileStoreConfig
	Image              config.ImageConfig
	Pricing            config.PricingConfig

	// AI 服务
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

// JWKSValidator 是 JWT 验证接口（进程内实现，不再走 HTTP）
type JWKSValidator interface {
	ValidateToken(ctx context.Context, token string) (*auth.Claims, error)
}

// InProcessJWKSValidator 进程内 JWKS 验证器——直接读 jwtSvc 公钥，不走 HTTP
type InProcessJWKSValidator struct {
	jwt *auth.JWTService
}

func NewInProcessJWKSValidator(jwt *auth.JWTService) *InProcessJWKSValidator {
	return &InProcessJWKSValidator{jwt: jwt}
}

func (v *InProcessJWKSValidator) ValidateToken(ctx context.Context, token string) (*auth.Claims, error) {
	return v.jwt.ParseToken(token)
}

// Register 在 Huma API 上注册全部端点（URM + AI）。
func Register(api huma.API, d Deps) {
	registerMeta(api, d)
	registerPublicPlane(api, d)
	// service plane 已移除——合并后不再有跨服务调用
}

func registerMeta(api huma.API, d Deps) {
	server.Health(api, d.Service, d.Version)
	registerInfo(api, d.Version)
	registerJWKS(api, d.JWT)
}

func registerPublicPlane(api huma.API, d Deps) {
	// OAuth2 受保护端点（用户 JWT + 黑名单）
	usrAuth := huma.Middlewares{userAuth(api, d.JWT, d.Blacklist)}
	registerOAuth2Protected(api, d, usrAuth)

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

	// 内部计费端点（原 service plane，合并后移到 public plane）
	registerInternalUsers(api, d, usrAuth)
	registerInternalBilling(api, d, usrAuth, usrAuth)

	// AI 域端点
	registerAITransport(api, d)
}

// registerAITransport 将 AI transport 注册到统一 Huma API 上。
// 目前传入零值 AIDeps（端点结构注册但不执行 handler），
// 后续随 AI 服务装配补全逐步填充真实依赖。
func registerAITransport(api huma.API, d Deps) {
	aiDeps := aitransport.AIDeps{
		Service:              "dai",
		Version:              Version,
		Postgres:             d.Pool,
		Redis:                d.Redis,
		Queries:              d.Queries,
		Logger:               d.Logger,
		JWKSValidator:        d.JWKSValidator,
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
	aitransport.RegisterAI(api, aiDeps)
}

// RegisterRaw 注册非 JSON 契约的 chi 原生端点。
func RegisterRaw(mux *chi.Mux, d Deps) {
	RegisterPublicRaw(mux, d)
}

func RegisterPublicRaw(mux *chi.Mux, d Deps) {
	h := newAuthHandlers(d)
	mux.Post("/api/oauth2/token", h.token)
	mux.Get("/api/oauth2/authorize", h.authorize)
	mux.Post("/api/oauth2/authorize", h.authorize)
	mux.Get("/api/oauth2/login", h.ssoLogin)
	mux.Post("/api/oauth2/login", h.ssoLogin)
	mux.Get("/api/oauth2/logout", h.ssoLogout)

	// 微信支付回调（无认证，验签即鉴权）
	notifyHandler := newPaymentNotifyHandlers(d)
	mux.Post("/api/v1/payments/wechat/notify", notifyHandler.wechatNotify)
	registerTenantBrandingRaw(mux, d)
}

var _ = http.MethodGet
