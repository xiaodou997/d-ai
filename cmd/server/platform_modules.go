package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/proxy"
	announcementpkg "xiaodou/dai/internal/announcement"
	announcementpg "xiaodou/dai/internal/announcement/pg"
	"xiaodou/dai/internal/auth"
	authpg "xiaodou/dai/internal/auth/pg"
	billingpg "xiaodou/dai/internal/billing/pg"
	billingsvc "xiaodou/dai/internal/billing/service"
	cleanuppkg "xiaodou/dai/internal/cleanup"
	"xiaodou/dai/internal/clientsecret"
	"xiaodou/dai/internal/config"
	invitepkg "xiaodou/dai/internal/invite"
	invitepg "xiaodou/dai/internal/invite/pg"
	notificationpkg "xiaodou/dai/internal/notification"
	paymentsvc "xiaodou/dai/internal/payment/service"
	"xiaodou/dai/internal/payment/wechat"
	"xiaodou/dai/internal/scheduler"
	systempkg "xiaodou/dai/internal/system"
	systempg "xiaodou/dai/internal/system/pg"
	tenantpkg "xiaodou/dai/internal/tenant"
	tenantpg "xiaodou/dai/internal/tenant/pg"
	userpkg "xiaodou/dai/internal/user"
	userpg "xiaodou/dai/internal/user/pg"
)

// platformModules owns platform identity, billing, and operations services.
// It keeps their construction and process-level workers together so the
// composition root only has to consume a stable dependency bundle.
type platformModules struct {
	JWT                   *auth.JWTService
	Sessions              *auth.SessionService
	Activations           *auth.ActivationService
	MFA                   *auth.MFAService
	RecentAuth            *auth.RecentAuthService
	Blacklist             *auth.BlacklistService
	Security              *auth.AccountSecurityService
	UserService           *userpkg.UserService
	AuthAccounts          *authpg.AuthRepository
	AuthRateLimiters      *auth.RateLimiters
	TenantRepo            *tenantpg.TenantRepository
	TenantLifecycle       *tenantpkg.AdminTenantLifecycleService
	TenantBranding        *tenantpg.PortalBrandingRepository
	TenantSelf            *tenantpkg.SelfService
	AdminAccounts         *userpg.AdminAccountRepository
	AdminAccountLifecycle *userpkg.AdminAccountLifecycleService
	AdminEndUsers         *userpg.AdminEndUserRepository
	AdminEndUserLifecycle *userpkg.AdminEndUserLifecycleService
	Invite                *invitepkg.InviteService

	Deduction      *billingsvc.DeductionService
	Recharge       *billingsvc.RechargeService
	Payment        *paymentsvc.PaymentService
	AccountQueries *billingsvc.AccountQueryService

	Announcements *announcementpkg.Service
	Notifications *notificationpkg.Service
	Modules       *systempkg.Service
	Dashboard     *systempg.SystemRepository
	ProxyNodes    *proxy.Service
	DataCleanup   *cleanuppkg.Service

	banReconciler *auth.BanReconciler
	sched         *scheduler.Scheduler
	lifecycleMu   sync.Mutex
	stopMu        sync.Mutex
	started       bool
	stopped       bool
	startOnce     sync.Once
}

func configureSecretKeyring(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("configuration is required")
	}
	previousSecretKeys, err := config.ParsePreviousSecretKeys(cfg.Security.SecretMasterKeyPrevious)
	if err != nil {
		return fmt.Errorf("invalid sensitive configuration keyring: %w", err)
	}
	secretKeyring, err := clientsecret.NewKeyring(
		cfg.Security.SecretMasterKeyID,
		cfg.Security.SecretMasterKey,
		previousSecretKeys,
	)
	if err != nil {
		return fmt.Errorf("sensitive configuration crypto init failed: %w", err)
	}
	if err := clientsecret.ConfigureKeyring(secretKeyring); err != nil {
		return fmt.Errorf("sensitive configuration crypto init failed: %w", err)
	}
	return nil
}

func buildPlatformModules(cfg *config.Config, pool, billingPool *pgxpool.Pool, redisClient *redis.Client, appLogger *zap.Logger) (*platformModules, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	if err := configureSecretKeyring(cfg); err != nil {
		return nil, err
	}
	if appLogger == nil {
		appLogger = zap.NewNop()
	}
	if billingPool == nil {
		billingPool = pool
	}

	jwtSvc := auth.NewJWTService(cfg.JWT, pool)
	sessionSvc := auth.NewSessionService(pool, jwtSvc, cfg.JWT.RefreshExpiration)
	activationSvc := auth.NewActivationService(pool, cfg.Auth.ActivationExpiration)
	blacklist := auth.NewBlacklistService(redisClient, appLogger)
	security := auth.NewAccountSecurityService(blacklist)
	mfaSvc := auth.NewMFAService(pool, redisClient)
	recentAuthSvc := auth.NewRecentAuthService(redisClient)

	userRepo := userpg.NewUserRepository(pool)
	userSvc := userpkg.NewUserService(userRepo, blacklist, appLogger)
	authAccountRepo := authpg.NewAuthRepository(pool)
	authRateLimiters := auth.NewRateLimiters(redisClient)
	tenantRepo := tenantpg.NewTenantRepository(pool)
	tenantBrandingRepo := tenantpg.NewPortalBrandingRepository(pool)
	tenantSelfRepo := tenantpg.NewTenantRepo(pool)
	tenantSelfSvc := tenantpkg.NewSelfService(tenantSelfRepo, tenantSelfRepo)
	accountRepo := billingpg.NewAccountRepository(pool)
	accountQueries := billingsvc.NewAccountQueryService(accountRepo)
	deductionSvc := billingsvc.NewDeductionService(billingPool, appLogger)
	rechargeSvc := billingsvc.NewRechargeService(billingPool, tenantRepo)
	tenantLifecycle := tenantpkg.NewAdminTenantLifecycleService(tenantRepo, security)
	adminAccountRepo := userpg.NewAdminAccountRepository(pool, activationSvc)
	adminEndUserRepo := userpg.NewAdminEndUserRepository(pool, activationSvc)
	adminAccountLifecycle := userpkg.NewAdminAccountLifecycleService(adminAccountRepo, security)
	adminEndUserLifecycle := userpkg.NewAdminEndUserLifecycleService(adminEndUserRepo, security)
	inviteSvc := invitepkg.NewInviteService(invitepg.NewInviteRepository(pool), appLogger)
	wechatCfgStore := wechat.NewConfigStore(billingPool)
	paymentSvc := paymentsvc.New(billingPool, wechat.NewGateway(wechatCfgStore), wechatCfgStore, appLogger, deductionSvc)
	announcementSvc := announcementpkg.NewService(announcementpg.NewRepository(pool))
	moduleSvc := systempkg.NewService(pool)
	dashboardRepo := systempg.NewSystemRepository(pool)
	proxySvc := proxy.NewService(pool, moduleSvc)
	notificationSvc := notificationpkg.NewService(pool)
	dataCleanupSvc := cleanuppkg.NewService(pool, appLogger)

	return &platformModules{
		JWT:                   jwtSvc,
		Sessions:              sessionSvc,
		Activations:           activationSvc,
		MFA:                   mfaSvc,
		RecentAuth:            recentAuthSvc,
		Blacklist:             blacklist,
		Security:              security,
		UserService:           userSvc,
		AuthAccounts:          authAccountRepo,
		AuthRateLimiters:      authRateLimiters,
		TenantRepo:            tenantRepo,
		TenantLifecycle:       tenantLifecycle,
		TenantBranding:        tenantBrandingRepo,
		TenantSelf:            tenantSelfSvc,
		AdminAccounts:         adminAccountRepo,
		AdminAccountLifecycle: adminAccountLifecycle,
		AdminEndUsers:         adminEndUserRepo,
		AdminEndUserLifecycle: adminEndUserLifecycle,
		Invite:                inviteSvc,
		Deduction:             deductionSvc,
		Recharge:              rechargeSvc,
		Payment:               paymentSvc,
		AccountQueries:        accountQueries,
		Announcements:         announcementSvc,
		Notifications:         notificationSvc,
		Modules:               moduleSvc,
		Dashboard:             dashboardRepo,
		ProxyNodes:            proxySvc,
		DataCleanup:           dataCleanupSvc,
		banReconciler:         auth.NewBanReconciler(pool, redisClient, appLogger, 5*time.Minute),
		sched:                 scheduler.NewScheduler(billingPool, jwtSvc, paymentSvc, appLogger),
	}, nil
}

func (m *platformModules) Start(ctxs ...context.Context) {
	if m == nil {
		return
	}
	ctx := context.Background()
	if len(ctxs) > 0 && ctxs[0] != nil {
		ctx = ctxs[0]
	}
	m.startOnce.Do(func() {
		m.stopMu.Lock()
		defer m.stopMu.Unlock()
		m.lifecycleMu.Lock()
		if m.stopped {
			m.lifecycleMu.Unlock()
			return
		}
		m.started = true
		m.lifecycleMu.Unlock()
		if m.banReconciler != nil {
			m.banReconciler.Start(ctx)
		}
		if m.sched != nil {
			m.sched.Start(ctx)
		}
	})
}

func (m *platformModules) Stop(ctxs ...context.Context) error {
	if m == nil {
		return nil
	}
	ctx := context.Background()
	if len(ctxs) > 0 && ctxs[0] != nil {
		ctx = ctxs[0]
	}
	m.stopMu.Lock()
	defer m.stopMu.Unlock()

	m.lifecycleMu.Lock()
	if !m.stopped {
		m.stopped = true
	}
	started := m.started
	m.lifecycleMu.Unlock()
	if !started {
		return nil
	}

	var errs []error
	if m.sched != nil {
		if err := m.sched.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("stop scheduler: %w", err))
		}
	}
	if m.banReconciler != nil {
		if err := m.banReconciler.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("stop ban reconciler: %w", err))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	return nil
}
