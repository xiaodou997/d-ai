package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	// ── AI 域 ──
	"xiaodou/dai/internal/ai/adapters/bridgefmt"
	"xiaodou/dai/internal/ai/adapters/clientcredentials"
	"xiaodou/dai/internal/ai/adapters/clienttransport"
	aiadapters "xiaodou/dai/internal/ai/adapters/postgres"
	redisadapter "xiaodou/dai/internal/ai/adapters/redis"
	"xiaodou/dai/internal/ai/apikey"
	"xiaodou/dai/internal/ai/asynctask"
	"xiaodou/dai/internal/ai/audit"
	"xiaodou/dai/internal/ai/billingcontrol"
	billingledger "xiaodou/dai/internal/ai/billingledger"
	"xiaodou/dai/internal/ai/blobstore"
	"xiaodou/dai/internal/ai/clientcatalog"
	"xiaodou/dai/internal/ai/clientruntime"
	"xiaodou/dai/internal/ai/commercial"
	"xiaodou/dai/internal/ai/console"
	coreruntime "xiaodou/dai/internal/ai/core/runtime"
	aidb "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/filestore"
	"xiaodou/dai/internal/ai/gateway"
	"xiaodou/dai/internal/ai/identitycontrol"
	"xiaodou/dai/internal/ai/imageassets"
	aimetrics "xiaodou/dai/internal/ai/observability/metrics"
	"xiaodou/dai/internal/ai/observabilitycontrol"
	"xiaodou/dai/internal/ai/riskcontrol"
	"xiaodou/dai/internal/ai/routing"
	"xiaodou/dai/internal/ai/secret"
	"xiaodou/dai/internal/ai/serving"
	"xiaodou/dai/internal/ai/subscription"
	"xiaodou/dai/internal/ai/tokenrefresh"
	"xiaodou/dai/internal/ai/upstreamaccess"
	"xiaodou/dai/internal/ai/upstreamcontrol"
	workspacesvc "xiaodou/dai/internal/ai/workspace"

	// AI transport (for upstream HTTP client)
	aitransport "xiaodou/dai/internal/ai/transport"

	// URM libs
	// (serviceaccess imported as internal above)

	// ── URM 域 ──
	announcementpkg "xiaodou/dai/internal/announcement"
	announcementpg "xiaodou/dai/internal/announcement/pg"
	"xiaodou/dai/internal/auth"
	billingpg "xiaodou/dai/internal/billing/pg"
	billingsvc "xiaodou/dai/internal/billing/service"
	"xiaodou/dai/internal/cache"
	"xiaodou/dai/internal/clientsecret"
	"xiaodou/dai/internal/config"
	daidb "xiaodou/dai/internal/db"
	invitepkg "xiaodou/dai/internal/invite"
	invitepg "xiaodou/dai/internal/invite/pg"
	paymentsvc "xiaodou/dai/internal/payment/service"
	"xiaodou/dai/internal/payment/wechat"
	"xiaodou/dai/internal/scheduler"
	"xiaodou/dai/internal/serviceaccess"
	"xiaodou/dai/internal/transport"
	userpkg "xiaodou/dai/internal/user"
	userpg "xiaodou/dai/internal/user/pg"

	// ── 公共库 ──
	"xiaodou/dai/libs/go/banstate"
	"xiaodou/dai/libs/go/logger"
	"xiaodou/dai/libs/go/server"
)

//go:embed all:frontend_dist
var frontendFS embed.FS

var version = "dev"

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	appLogger := logger.InitLogger(cfg.App.Env, logger.LogConfig{
		Level:  cfg.Log.Level,
		File:   cfg.Log.File,
		Redact: cfg.Log.Redact,
	})
	defer appLogger.Sync()
	logger.SetGlobal(appLogger)

	appLogger.Info("configuration loaded",
		zap.String("server_addr", cfg.Server.Addr),
		zap.String("env", cfg.App.Env),
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// ──────────────────────────────────────────────────────
	// 1. 基础设施：PostgreSQL（单库）+ Redis
	// ──────────────────────────────────────────────────────

	dsn := cfg.Database.DSNString()
	if dsn == "" {
		appLogger.Fatal("database connection string is required")
	}

	pgConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		appLogger.Fatal("parse database DSN failed", zap.Error(err))
	}
	if cfg.Database.MaxConns > 0 {
		pgConfig.MaxConns = cfg.Database.MaxConns
	}
	if cfg.Database.MinConns > 0 {
		pgConfig.MinConns = cfg.Database.MinConns
	}
	if cfg.Database.MaxConnLifetime > 0 {
		pgConfig.MaxConnLifetime = cfg.Database.MaxConnLifetime
	}

	pool, err := pgxpool.NewWithConfig(ctx, pgConfig)
	if err != nil {
		appLogger.Fatal("connect postgres failed", zap.Error(err))
	}
	defer pool.Close()

	if err := daidb.VerifySchema(ctx, pool); err != nil {
		appLogger.Fatal("database schema verification failed", zap.Error(err))
	}
	appLogger.Info("database schema verified", zap.Int("version", daidb.ExpectedSchemaVersion))

	// Redis
	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		appLogger.Fatal("connect redis failed", zap.Error(err))
	}
	defer redisClient.Close()
	appLogger.Info("redis connected", zap.String("addr", cfg.Redis.Addr))

	// ──────────────────────────────────────────────────────
	// 2. URM 域服务装配
	// ──────────────────────────────────────────────────────

	jwtSvc := auth.NewJWTService(cfg.JWT, pool)
	blacklist := auth.NewBlacklistService(redisClient, appLogger)
	sessionSvc := auth.NewSessionService(redisClient, cfg.SSO.SessionTTL, cfg.SSO.SessionMaxTTL)
	serviceAccessSvc := serviceaccess.New(pool, redisClient, appLogger)
	if saErr := serviceAccessSvc.Start(5 * time.Minute); saErr != nil {
		appLogger.Fatal("service access reconciliation failed", zap.Error(saErr))
	}
	defer serviceAccessSvc.Stop()

	if cfg.Security.SecretMasterKey != "" {
		if err := clientsecret.Configure(cfg.Security.SecretMasterKey); err != nil {
			appLogger.Fatal("sensitive configuration crypto init failed", zap.Error(err))
		}
	}

	// Billing services — 进程内直接调用的核心
	billingRepo := billingpg.NewBillingRepository(pool)
	redisCache, cacheErr := cache.NewRedisService(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB, appLogger)
	if cacheErr != nil {
		appLogger.Fatal("redis cache service init failed", zap.Error(cacheErr))
	}
	deductionSvc := billingsvc.NewDeductionService(pool, redisCache, appLogger)
	creditLeaseSvc := billingsvc.NewCreditLeaseService(pool, appLogger)

	// User / Invite / Payment / Announcement
	userRepo := userpg.NewUserRepository(pool)
	userSvc := userpkg.NewUserService(userRepo, blacklist, appLogger)
	inviteSvc := invitepkg.NewInviteService(invitepg.NewInviteRepository(pool), appLogger)
	wechatCfgStore := wechat.NewConfigStore(pool)
	paymentSvc := paymentsvc.New(pool, wechat.NewGateway(wechatCfgStore), wechatCfgStore, appLogger)
	announcementSvc := announcementpkg.NewService(announcementpg.NewRepository(pool))

	// Ban reconciler
	banReconciler := auth.NewBanReconciler(pool, redisClient, appLogger, 5*time.Minute)
	banReconciler.Start()
	defer banReconciler.Stop()

	// Scheduler
	sched := scheduler.NewScheduler(pool, jwtSvc, paymentSvc, appLogger, cfg.Scheduler.PreAuthTimeoutMinutes)
	sched.WithCreditLeaseReaper(creditLeaseSvc)
	sched.Start()
	defer sched.Stop()

	// ──────────────────────────────────────────────────────
	// 3. AI 域服务装配
	// ──────────────────────────────────────────────────────

	q := aidb.New(pool)
	banChecker := banstate.NewChecker(redisClient)

	// ★ 关键改造：通过 InProcessLeasePort 直接调 billing service，不走 HTTP ★
	leasePort := billingledger.NewInProcessLeasePort(creditLeaseSvc, "dai-ai-gateway")
	billingCoordinator := billingledger.New(
		pool,
		leasePort, // ← 进程内调用，不再 HTTP POST /internal/v3/ledger/leases
		billingledger.Config{
			RequestedTenantMicro: cfg.Billing.RequestedTenantMicro,
			RequestedUserMicro:   cfg.Billing.RequestedUserMicro,
			LeaseTTL:             cfg.Billing.LeaseTTL,
			LeaseGrace:           cfg.Billing.LeaseGrace,
			WindowMaxAge:         cfg.Billing.WindowMaxAge,
			RenewLead:            cfg.Billing.RenewLead,
			AdmissionHeadroom:    cfg.Billing.AdmissionHeadroom,
			WorkerInterval:       cfg.Billing.WorkerInterval,
			DispatchLease:        cfg.Billing.DispatchLease,
			PickLimit:            cfg.Billing.PickLimit,
		},
		appLogger,
	)
	go billingCoordinator.Run(ctx)

	// JWKS 验证改为进程内直接读 jwtSvc 公钥
	jwksValidator := transport.NewInProcessJWKSValidator(jwtSvc)

	// ── AI 领域服务（管理端 + 工作端）──

	// Price Book
	priceBookSvc := billingcontrol.New(aiadapters.NewPriceBookRepo(q, pool), billingcontrol.NewHTTPFetcher(cfg.Pricing.LiteLLMURL))
	priceBookSvc.Start(ctx)

	// Commercial
	commercialRepo := aiadapters.NewCommercialRepo(q, pool)
	commercialSvc := commercial.NewService(commercialRepo)
	groupTransferSvc := commercial.NewGroupTransferService(commercialRepo, commercial.GroupTransferOptions{})

	// Observability (Dashboard / Usage / Audit)
	dashboardSvc := observabilitycontrol.NewDashboardService(aiadapters.NewDashboardRepo(q))
	usageSvc := observabilitycontrol.NewUsageService(aiadapters.NewUsageRepo(q, pool))
	auditSvc := observabilitycontrol.NewAuditService(aiadapters.NewAuditRepo(q))

	// Upstream accounts
	accountSvc := upstreamcontrol.New(aiadapters.NewAccountRepo(q, pool), func(plaintext string) (string, error) {
		return secret.EncryptProviderKey(cfg.Security.ProviderKeyMaster, plaintext)
	})
	upstreamAccessSvc := upstreamaccess.New(aiadapters.NewUpstreamAccessRepo(pool))

	// API Keys
	apiKeyCache := apikey.NewCache(redisClient)
	apiKeySvc := identitycontrol.New(
		aiadapters.NewAPIKeyRepo(q),
		apiKeyCacheForSvc(apiKeyCache),
		func(plaintext string) (string, error) {
			return secret.EncryptProviderKey(cfg.Security.ProviderKeyMaster, plaintext)
		},
		func(ciphertext string) (string, error) {
			return secret.DecryptProviderKey(cfg.Security.ProviderKeyMaster, ciphertext)
		},
	)

	// Risk Control
	riskControlRepo := aiadapters.NewRiskControlRepo(q)
	riskControlConfigSvc := riskcontrol.NewConfigService(riskControlRepo)
	riskControlLogSvc := riskcontrol.NewLogService(riskControlRepo)
	riskControlEventSvc := riskcontrol.NewEventService(riskControlRepo)
	riskControlChecker := &riskcontrol.Checker{
		Config:            riskControlConfigSvc,
		Logs:              riskControlLogSvc,
		Events:            riskControlEventSvc,
		HTTPClient:        &http.Client{},
		ProviderKeyMaster: cfg.Security.ProviderKeyMaster,
		Logger:            appLogger,
	}
	riskControlWorker := riskcontrol.NewWorker(riskControlChecker, appLogger)
	go riskControlWorker.Start(ctx, 0)

	// Audit worker
	auditStore := aiadapters.NewAuditStore(pool)
	blobStore := blobstore.NewPGStore(pool)
	auditWorker := audit.NewWorker(auditStore, blobStore, audit.WorkerOptions{
		StoreImageBlobs: cfg.Audit.StoreImageBlobs,
	})
	go auditWorker.Start(ctx)

	// Subscription
	purchaser := subscription.NewInProcessPurchaser(deductionSvc, "dai-ai-gateway")
	subsSvc := subscription.NewService(aiadapters.NewSubscriptionRepo(q, pool), purchaser, appLogger)

	// Workspace
	workspaceRepo := aiadapters.NewWorkspaceRepo(pool, aiadapters.NewGroupAccessReader(pool), aiadapters.NewRouteInspector(pool))
	workspaceSvc := workspacesvc.NewService(workspaceRepo)

	// File store
	fileStore, fsErr := filestore.New(pool, filestore.Config{
		StorageDir:    cfg.Files.StorageDir,
		AssetTTL:      cfg.Files.AssetTTL,
		URLTTL:        cfg.Files.URLTTL,
		PublicBaseURL: cfg.Files.PublicBaseURL,
		MaxBytes:      cfg.Files.MaxBytes,
	})
	if fsErr != nil {
		appLogger.Fatal("file store init failed", zap.Error(fsErr))
	}

	// Image assets
	imageAssetSvc := imageassets.New(imageassets.Config{
		StorageDir:     cfg.Image.StorageDir,
		Retention:      cfg.Image.Retention,
		PublicBasePath: cfg.Image.PublicBasePath,
	}, &http.Client{Timeout: 60 * time.Second})

	// ── AI Serving Pipeline + Gateway Runtime ──

	// OAuth credentials store + token refresher
	oauthCreds := aiadapters.NewOAuthCredentialStore(pool, cfg.Security.ProviderKeyMaster)
	refresher := tokenrefresh.New(oauthCreds, appLogger)
	go refresher.Start(ctx)
	appLogger.Info("oauth token refresher started")

	// Bridge runtime + route inspector
	bridgeRuntime := bridgefmt.NewRuntime()
	grantChecker := aiadapters.NewGroupAccessReader(pool)
	routeInspector := aiadapters.NewRouteInspector(pool)
	routeInspector.WithBridgeSupport(bridgeRuntime)

	// Health tracker (Redis-backed for multi-node)
	innerTracker := routing.DefaultInMemoryTracker()
	rht := routing.NewRedisHealthTracker(innerTracker, redisClient)
	healthTracker := routing.HealthTracker(rht)
	metricsGW := aimetrics.NewGateway()

	// Rate limiters
	rateLimiter := redisadapter.NewRateLimiter(redisClient, q)
	upstreamConcurrencyLimiter := redisadapter.NewUpstreamConcurrencyLimiter(redisClient)

	// Route stats + sticky store + scorer
	routeWeightsStore := aiadapters.NewRouteWeightsStore(pool)
	routeStats := redisadapter.NewRedisRouteStats(redisClient)
	stickyStore := routing.StickyStore(redisadapter.NewRedisSticky(redisClient))
	scorer := &serving.MultiDimScorer{
		Health:  healthTracker,
		Stats:   routeStats,
		Weights: routeWeightsStore,
	}

	// Runtime binding resolver + planner
	runtimeBinder := coreruntime.NewCachedBindingResolver(
		aiadapters.NewRuntimeTargetBinder(q, pool, cfg.Security.ProviderKeyMaster).WithBridgeSupport(bridgeRuntime),
		coreruntime.BindingResolverOptions{
			DisableCache: true,
			Authorizer:   aiadapters.NewRuntimeBindingAuthorizer(pool),
		},
	)
	runtimePlanner := coreruntime.NewResolver(coreruntime.NewPlanner(commercialSvc), runtimeBinder)
	commercialRepo.WithRuntimeResolver(runtimePlanner)
	runtimeRouteSelector := gateway.NewRuntimeRouteSelector(runtimePlanner, appLogger)

	// Price book biller + usage logger
	priceBookBiller := aiadapters.NewPriceBookBiller(priceBookSvc, q, pool)
	usageLogger := aiadapters.NewUsageLogger(pool, priceBookBiller).
		WithBillingCoordinator(billingCoordinator).
		WithRecoveryQueue(redisClient, appLogger)
	go usageLogger.RunRecovery(ctx)

	// API key cache (already created above, wire to usage logger)
	usageLogger.WithAPIKeyCacheInvalidator(apiKeyCache)

	// Content moderation step (already created riskControlChecker above)
	contentModerationStep := &serving.ContentModerationStep{Checker: riskControlChecker, Worker: riskControlWorker}

	// Pipeline steps
	quotaCheckStep := &serving.QuotaCheckStep{}
	subscriptionGateStep := &serving.SubscriptionGateStep{Subs: subsPort(subsSvc), Logger: appLogger}
	billingGuardStep := &serving.BillingGuardStep{Resolver: priceBookBiller}
	routeCandidatesStep := &serving.RouteCandidatesStep{Selector: runtimeRouteSelector, Sticky: stickyStore}
	billingAdmissionStep := &serving.BillingAdmissionStep{
		Coordinator: billingCoordinator, Logger: appLogger, Metrics: metricsGW,
	}
	rateLimitStep := &serving.RateLimitStep{Limiter: rateLimiter}

	// Upstream HTTP transport + client runtime
	upstreamHTTPTransport := aitransport.NewClient(0)
	fixedClientRuntime := clientruntime.New(
		clienttransport.New(upstreamHTTPTransport),
		clientcredentials.New(oauthCreds, refresher),
	)
	poolModelCatalog := clientcatalog.New(oauthCreds, fixedClientRuntime, appLogger)
	_ = poolModelCatalog // used by AIDeps later

	executeStep := &serving.ExecuteStep{
		Transport:       upstreamHTTPTransport,
		ClientRuntime:   fixedClientRuntime,
		UpstreamLimiter: upstreamConcurrencyLimiter,
		Bridge:          bridgeRuntime,
		Health:          healthTracker,
		OAuthPool:       oauthCreds,
		AccountState:    accountSvc,
		Budget:          serving.DefaultRetryBudget(),
		Scorer:          scorer,
		Stats:           routeStats,
		Sticky:          stickyStore,
		ImageNormalizer: fileStore,
	}
	usageCompletionFinalizer := &serving.UsageLogFinalizer{Logger: usageLogger, Metrics: metricsGW}
	auditFinalizer := &serving.AuditFinalizer{Worker: auditWorker}
	rateLimitFinalizer := serving.RateLimitFinalizer{}

	pipeline := serving.NewPipeline(
		&serving.AuthNStep{Resolver: aiadapters.NewAPIKeyResolver(q)},
		contentModerationStep,
		quotaCheckStep,
		subscriptionGateStep,
		routeCandidatesStep,
		billingGuardStep,
		billingAdmissionStep,
		rateLimitStep,
		executeStep,
	).WithFinalizers(
		usageCompletionFinalizer,
		auditFinalizer,
		rateLimitFinalizer,
	)

	// Runtime engine + gateway
	runtimeInvokeExpander := coreruntime.NewInvokeExpander(aiadapters.NewApplicationRuntimeRepo(pool))
	runtimeEngine := gateway.NewRuntimeEngine(pipeline)
	taskAdmission := serving.NewAdmissionGate(
		quotaCheckStep, subscriptionGateStep, routeCandidatesStep, billingGuardStep,
	)
	asyncTasks, asynctaskErr := asynctask.New(asynctask.Config{
		Workers:              cfg.AsyncTasks.Workers,
		PollInterval:         cfg.AsyncTasks.PollInterval,
		LeaseTTL:             cfg.AsyncTasks.LeaseTTL,
		MaxInFlightPerTenant: cfg.AsyncTasks.MaxInFlightPerTenant,
		Retention:            cfg.AsyncTasks.Retention,
		ReapInterval:         cfg.AsyncTasks.ReapInterval,
		ReapBatch:            cfg.AsyncTasks.ReapBatch,
		WebhookWorkers:       cfg.AsyncTasks.WebhookWorkers,
		WebhookPollInterval:  cfg.AsyncTasks.WebhookPollInterval,
		WebhookLeaseTTL:      cfg.AsyncTasks.WebhookLeaseTTL,
	}, asynctask.Deps{
		Pool:         pool,
		Logger:       appLogger,
		Subjects:     gateway.NewTaskSubjectResolver(q, runtimeInvokeExpander),
		RedactDetail: serving.RedactInternalErrorDetail,
	})
	if asynctaskErr != nil {
		appLogger.Fatal("build async task engine failed", zap.Error(asynctaskErr))
	}

	var runtimeGateway = gateway.New(gateway.Deps{
		Logger:                   appLogger,
		Postgres:                 pool,
		Pipeline:                 pipeline,
		Queries:                  q,
		APIKeyCache:              apiKeyCache,
		ServiceAccess:            gatewayServiceAccessAdapter{svc: serviceAccessSvc},
		ExpectedClientID:         "dai-ai-gateway",
		BanChecker:               banChecker,
		RuntimeInvokeExpander:    runtimeInvokeExpander,
		RuntimeEngine:            runtimeEngine,
		AsyncTasks:               asyncTasks,
		TaskAdmission:            taskAdmission,
		TaskInvokeExpander:       runtimeInvokeExpander,
		JWKSValidator:            jwksValidator,
		DelegationAllowedClients: cfg.Delegation.AllowedClients,
	})
	runtimeGateway.RegisterTaskHandlers(asyncTasks)

	// Management console
	mgmtConsole := console.New(console.Deps{
		Postgres:       pool,
		Redis:          redisClient,
		Logger:         appLogger,
		Queries:        q,
		Security:       cfg.Security,
		URMClient:      console.NewInProcessURMExchanger(jwtSvc),
		URMClientID:    "dai-ai-gateway",
		JWKSValidator:  jwksValidator,
		BanChecker:     banChecker,
		HTTPClient:     &http.Client{Timeout: 0},
		OAuthCreds:     oauthCreds,
		TokenRefresher: refresher,
		RouteInspector: routeInspector,
		APIKeyCache:    apiKeyCache,
		Gateway:        runtimeGateway,
		GrantChecker:   grantChecker,
		WorkspaceSvc:   workspaceSvc,
		ImageAssets:    imageAssetSvc,
		FileStore:      fileStore,
		AsyncTasks:     cfg.AsyncTasks,
	})
	mgmtConsole.RegisterImageTaskHandlers(asyncTasks)

	// Hourly cleanups
	go runHourlyCleanup(ctx, func() { imageAssetSvc.CleanupExpired() })
	go runHourlyCleanup(ctx, func() { fileStore.CleanupExpired(ctx, 500) })

	// ──────────────────────────────────────────────────────
	// 4. 统一 Transport 装配
	// ──────────────────────────────────────────────────────

	deps := transport.Deps{
		Service:       "dai",
		Version:       version,
		Pool:          pool,
		Redis:         redisClient,
		Logger:        appLogger,
		Config:        cfg,
		JWT:           jwtSvc,
		Blacklist:     blacklist,
		Session:       sessionSvc,
		SSO:           cfg.SSO,
		Legal:         cfg.Legal,
		UserService:   userSvc,
		Deduction:     deductionSvc,
		CreditLeases:  creditLeaseSvc,
		BillingRepo:   billingRepo,
		Invite:        inviteSvc,
		Payment:       paymentSvc,
		ServiceAccess: serviceAccessSvc,
		Announcements: announcementSvc,
		Delegation:    cfg.Delegation,

		Queries:            q,
		BillingCoordinator: billingCoordinator,
		JWKSValidator:      jwksValidator,
		BanChecker:         banChecker,
		Security:           cfg.Security,
		Audit:              cfg.Audit,
		AsyncTasks:         cfg.AsyncTasks,
		Files:              cfg.Files,
		Image:              cfg.Image,
		Pricing:            cfg.Pricing,

		// AI 服务
		PriceBookSvc:         priceBookSvc,
		CommercialSvc:        commercialSvc,
		GroupTransferSvc:     groupTransferSvc,
		DashboardSvc:         dashboardSvc,
		UsageSvc:             usageSvc,
		AuditSvc:             auditSvc,
		AccountSvc:           accountSvc,
		UpstreamAccessSvc:    upstreamAccessSvc,
		APIKeySvc:            apiKeySvc,
		WorkspaceSvc:         workspaceSvc,
		Subscriptions:        subsSvc,
		FileStore:            fileStore,
		ImageAssets:          imageAssetSvc,
		RiskControlConfigSvc: riskControlConfigSvc,
		RiskControlLogSvc:    riskControlLogSvc,
		RiskControlEventSvc:  riskControlEventSvc,
		RiskControlChecker:   riskControlChecker,
	}

	router, api := server.New(server.Options{
		Title:   "D-AI",
		Version: version,
		Logger:  appLogger,
	})

	transport.Register(api, deps)
	transport.RegisterRaw(router, deps)

	// AI gateway + console + fileStore 路由
	fileStore.Routes(router)
	runtimeGateway.Routes(router)
	runtimeGateway.RegisterInternalTaskRoutes(router)
	mgmtConsole.Routes(router)

	// 健康检查
	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "version": version})
	})
	router.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := pool.Ping(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "error", "component": "postgres"})
			return
		}
		if err := redisClient.Ping(r.Context()).Err(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "error", "component": "redis"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})

	// ──────────────────────────────────────────────────────
	// 5. 前端静态文件 embed
	// ──────────────────────────────────────────────────────

	if frontendSub, err := fs.Sub(frontendFS, "frontend_dist"); err == nil {
		router.Handle("/*", http.FileServer(http.FS(frontendSub)))
	} else {
		appLogger.Warn("frontend embed not available, serving API only", zap.Error(err))
	}

	// ──────────────────────────────────────────────────────
	// 6. 启动 HTTP 服务
	// ──────────────────────────────────────────────────────

	addr := cfg.Server.Addr
	if addr == "" {
		addr = ":" + strconv.Itoa(cfg.Server.Port)
	}

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout:      0,
		IdleTimeout:       time.Duration(cfg.Server.IdleTimeout) * time.Second,
	}

	go func() {
		appLogger.Info("D-AI server listening", zap.String("addr", addr), zap.String("version", version))
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			appLogger.Error("server failed", zap.Error(err))
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
	appLogger.Info("server shutdown complete")
}

// ── 辅助函数 ──────────────────────────────────────────

// subsPort adapts a possibly-nil *subscription.Service into the serving
// Subscriptions port, returning a genuinely nil interface when the service is
// unset so the gate step's nil-check works.
func subsPort(s *subscription.Service) serving.Subscriptions {
	if s == nil {
		return nil
	}
	return s
}

// apiKeyCacheForSvc adapts *apikey.Cache into identitycontrol.KeyCache.
func apiKeyCacheForSvc(c *apikey.Cache) identitycontrol.KeyCache {
	if c == nil {
		return nil
	}
	return c
}

// runHourlyCleanup runs cleanup() immediately, then every hour until ctx cancels.
func runHourlyCleanup(ctx context.Context, cleanup func()) {
	cleanup()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanup()
		}
	}
}

// gatewayServiceAccessAdapter 适配 internal/serviceaccess.Service
// 到 gateway.Deps.ServiceAccess 接口（6 参数 Check）。
// 原始 libs/go/serviceaccess.Checker.Check 有 6 个参数（含 clientID），
// internal/serviceaccess.Service.Check 有 5 个参数（无 clientID）。
// 合并后 clientID 固定为 "dai-ai-gateway"，适配器注入这个常量。
type gatewayServiceAccessAdapter struct {
	svc *serviceaccess.Service
}

func (a gatewayServiceAccessAdapter) Check(ctx context.Context, userType int, userID, tenantID, serviceID, clientID string) error {
	return a.svc.Check(ctx, userType, userID, tenantID, serviceID)
}
