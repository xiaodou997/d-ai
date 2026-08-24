package main

import (
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
	userpkg "xiaodou/dai/internal/user"
	userpg "xiaodou/dai/internal/user/pg"
)

// platformModules owns platform identity, billing, and operations services.
// It keeps their construction and process-level workers together so the
// composition root only has to consume a stable dependency bundle.
type platformModules struct {
	JWT           *auth.JWTService
	Sessions      *auth.SessionService
	Activations   *auth.ActivationService
	MFA           *auth.MFAService
	RecentAuth    *auth.RecentAuthService
	Blacklist     *auth.BlacklistService
	UserService   *userpkg.UserService
	AdminAccounts *userpg.AdminAccountRepository
	AdminEndUsers *userpg.AdminEndUserRepository
	Invite        *invitepkg.InviteService

	Deduction *billingsvc.DeductionService
	Payment   *paymentsvc.PaymentService

	Announcements *announcementpkg.Service
	Notifications *notificationpkg.Service
	Modules       *systempkg.Service
	ProxyNodes    *proxy.Service
	DataCleanup   *cleanuppkg.Service

	banReconciler *auth.BanReconciler
	sched         *scheduler.Scheduler
	startOnce     sync.Once
	stopOnce      sync.Once
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

func buildPlatformModules(cfg *config.Config, pool *pgxpool.Pool, redisClient *redis.Client, appLogger *zap.Logger) (*platformModules, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	if err := configureSecretKeyring(cfg); err != nil {
		return nil, err
	}
	if appLogger == nil {
		appLogger = zap.NewNop()
	}

	jwtSvc := auth.NewJWTService(cfg.JWT, pool)
	sessionSvc := auth.NewSessionService(pool, jwtSvc, cfg.JWT.RefreshExpiration)
	activationSvc := auth.NewActivationService(pool, cfg.Auth.ActivationExpiration)
	blacklist := auth.NewBlacklistService(redisClient, appLogger)
	mfaSvc := auth.NewMFAService(pool, redisClient)
	recentAuthSvc := auth.NewRecentAuthService(redisClient)

	deductionSvc := billingsvc.NewDeductionService(pool, appLogger)
	userRepo := userpg.NewUserRepository(pool)
	userSvc := userpkg.NewUserService(userRepo, blacklist, appLogger)
	adminAccountRepo := userpg.NewAdminAccountRepository(pool, activationSvc)
	adminEndUserRepo := userpg.NewAdminEndUserRepository(pool, activationSvc)
	inviteSvc := invitepkg.NewInviteService(invitepg.NewInviteRepository(pool), appLogger)
	wechatCfgStore := wechat.NewConfigStore(pool)
	paymentSvc := paymentsvc.New(pool, wechat.NewGateway(wechatCfgStore), wechatCfgStore, appLogger)
	announcementSvc := announcementpkg.NewService(announcementpg.NewRepository(pool))
	moduleSvc := systempkg.NewService(pool)
	proxySvc := proxy.NewService(pool, moduleSvc)
	notificationSvc := notificationpkg.NewService(pool)
	dataCleanupSvc := cleanuppkg.NewService(pool, appLogger)

	return &platformModules{
		JWT:           jwtSvc,
		Sessions:      sessionSvc,
		Activations:   activationSvc,
		MFA:           mfaSvc,
		RecentAuth:    recentAuthSvc,
		Blacklist:     blacklist,
		UserService:   userSvc,
		AdminAccounts: adminAccountRepo,
		AdminEndUsers: adminEndUserRepo,
		Invite:        inviteSvc,
		Deduction:     deductionSvc,
		Payment:       paymentSvc,
		Announcements: announcementSvc,
		Notifications: notificationSvc,
		Modules:       moduleSvc,
		ProxyNodes:    proxySvc,
		DataCleanup:   dataCleanupSvc,
		banReconciler: auth.NewBanReconciler(pool, redisClient, appLogger, 5*time.Minute),
		sched:         scheduler.NewScheduler(pool, jwtSvc, paymentSvc, appLogger),
	}, nil
}

func (m *platformModules) Start() {
	if m == nil {
		return
	}
	m.startOnce.Do(func() {
		if m.banReconciler != nil {
			m.banReconciler.Start()
		}
		if m.sched != nil {
			m.sched.Start()
		}
	})
}

func (m *platformModules) Stop() {
	if m == nil {
		return
	}
	m.stopOnce.Do(func() {
		if m.sched != nil {
			m.sched.Stop()
		}
		if m.banReconciler != nil {
			m.banReconciler.Stop()
		}
	})
}
