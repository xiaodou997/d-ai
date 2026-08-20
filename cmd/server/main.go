package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

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
	"xiaodou/dai/internal/ai/observability/tracing"
	"xiaodou/dai/internal/ai/observabilitycontrol"
	"xiaodou/dai/internal/ai/privacy"
	proxypkg "xiaodou/dai/internal/ai/proxy"
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

	// ── 平台身份、计费与运营域 ──
	announcementpkg "xiaodou/dai/internal/announcement"
	announcementpg "xiaodou/dai/internal/announcement/pg"
	"xiaodou/dai/internal/auth"
	billingoutbox "xiaodou/dai/internal/billing/outbox"
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
	"xiaodou/dai/internal/transport"
	userpkg "xiaodou/dai/internal/user"
	userpg "xiaodou/dai/internal/user/pg"
	"xiaodou/dai/internal/weborigin"

	// ── 公共库 ──
	"xiaodou/dai/libs/go/banstate"
	"xiaodou/dai/libs/go/logger"
	"xiaodou/dai/libs/go/server"
)

//go:embed all:frontend_dist
var frontendFS embed.FS

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "D-AI startup failed: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	appLogger := logger.InitLogger(cfg.App.Env, logger.LogConfig{
		Level:  cfg.Log.Level,
		File:   cfg.Log.File,
		Redact: cfg.Log.Redact,
	})
	defer appLogger.Sync()
	logger.SetGlobal(appLogger)

	originResolver, err := weborigin.NewResolver(cfg.Server.PublicBaseURL, cfg.Server.TrustedProxyCIDRs)
	if err != nil {
		return fmt.Errorf("invalid public origin or trusted proxy configuration: %w", err)
	}

	appLogger.Info("configuration loaded",
		zap.String("server_addr", cfg.Server.Addr),
		zap.String("env", cfg.App.Env),
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	shutdownTracing := tracing.Init(ctx)
	shutdowns := &shutdownStack{}
	defer func() {
		// Cancel first so context-owned workers stop before their database and
		// cache dependencies are released by the stack.
		stop()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = shutdownTracing(shutdownCtx)
		if err := shutdowns.Close(shutdownCtx); err != nil {
			appLogger.Error("resource shutdown failed", zap.Error(err))
		}
	}()

	// ──────────────────────────────────────────────────────
	// 1. 基础设施：PostgreSQL（单库）+ Redis
	// ──────────────────────────────────────────────────────

	infra, err := openInfrastructure(ctx, cfg, appLogger)
	if err != nil {
		return err
	}
	pool, redisClient := infra.pool, infra.redis
	// Dependencies are registered immediately after construction. Any later
	// module assembly error therefore releases them before run() returns.
	shutdowns.Add("postgres", func(context.Context) error {
		pool.Close()
		return nil
	})
	shutdowns.Add("redis", func(context.Context) error {
		return redisClient.Close()
	})

	// ──────────────────────────────────────────────────────
	// 2. 平台身份与计费域装配
	// ──────────────────────────────────────────────────────

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

	jwtSvc := auth.NewJWTService(cfg.JWT, pool)
	sessionSvc := auth.NewSessionService(pool, jwtSvc, cfg.JWT.RefreshExpiration)
	activationSvc := auth.NewActivationService(pool, cfg.Auth.ActivationExpiration)
	blacklist := auth.NewBlacklistService(redisClient, appLogger)
	mfaSvc := auth.NewMFAService(pool, redisClient)
	recentAuthSvc := auth.NewRecentAuthService(redisClient)

	// Billing services — 进程内直接调用的核心
	deductionSvc := billingsvc.NewDeductionService(pool, appLogger)

	// User / Invite / Payment / Announcement
	userRepo := userpg.NewUserRepository(pool)
	userSvc := userpkg.NewUserService(userRepo, blacklist, appLogger)
	inviteSvc := invitepkg.NewInviteService(invitepg.NewInviteRepository(pool), appLogger)
	wechatCfgStore := wechat.NewConfigStore(pool)
	paymentSvc := paymentsvc.New(pool, wechat.NewGateway(wechatCfgStore), wechatCfgStore, appLogger)
	announcementSvc := announcementpkg.NewService(announcementpg.NewRepository(pool))
	moduleSvc := systempkg.NewService(pool)
	proxySvc := proxypkg.NewService(pool, moduleSvc)
	notificationSvc := notificationpkg.NewService(pool)
	dataCleanupSvc := cleanuppkg.NewService(pool, appLogger)

	// Ban reconciler
	banReconciler := auth.NewBanReconciler(pool, redisClient, appLogger, 5*time.Minute)
	banReconciler.Start()
	defer banReconciler.Stop()

	// Scheduler
	sched := scheduler.NewScheduler(pool, jwtSvc, paymentSvc, appLogger)
	sched.Start()
	defer sched.Stop()

	// ──────────────────────────────────────────────────────
	// 3. AI 域服务装配
	// ──────────────────────────────────────────────────────

	q := aidb.New(pool)
	banChecker := banstate.NewChecker(redisClient)

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
		return secret.EncryptProviderKey(cfg.Security.SecretMasterKey, plaintext)
	})
	upstreamAccessSvc := upstreamaccess.New(aiadapters.NewUpstreamAccessRepo(pool))

	// API Keys
	apiKeyCache := apikey.NewCache(redisClient)
	apiKeySvc := identitycontrol.New(
		aiadapters.NewAPIKeyRepo(q),
		apiKeyCacheForSvc(apiKeyCache),
		func(plaintext string) (string, error) {
			return secret.EncryptProviderKey(cfg.Security.SecretMasterKey, plaintext)
		},
		func(ciphertext string) (string, error) {
			return secret.DecryptProviderKey(cfg.Security.SecretMasterKey, ciphertext)
		},
	)

	// Risk Control
	riskControlRepo := aiadapters.NewRiskControlRepo(q)
	riskControlConfigSvc := riskcontrol.NewConfigService(riskControlRepo)
	riskControlLogSvc := riskcontrol.NewLogService(riskControlRepo)
	riskControlEventSvc := riskcontrol.NewEventService(riskControlRepo)
	riskControlChecker := &riskcontrol.Checker{
		Config:          riskControlConfigSvc,
		Logs:            riskControlLogSvc,
		Events:          riskControlEventSvc,
		HTTPClient:      &http.Client{},
		SecretMasterKey: cfg.Security.SecretMasterKey,
		Logger:          appLogger,
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
	purchaser := subscription.NewBillingPurchaser(pool, "dai")
	subsSvc := subscription.NewService(aiadapters.NewSubscriptionRepo(q, pool), purchaser, appLogger)

	// Workspace
	workspaceRepo := aiadapters.NewWorkspaceRepo(pool, aiadapters.NewGroupAccessReader(pool), aiadapters.NewRouteInspector(pool))
	workspaceSvc := workspacesvc.NewService(
		workspaceRepo,
		workspacesvc.WithChatCatalog(aiadapters.NewWorkspaceChatCatalog(workspaceRepo)),
		workspacesvc.WithChatWriter(workspaceRepo),
		workspacesvc.WithChatRuntimeWriter(workspaceRepo),
	)

	// File store
	fileStore, fsErr := filestore.New(pool, filestore.Config{
		StorageDir: filepath.Join(cfg.Storage.DataDir, "files"),
		AssetTTL:   cfg.Files.AssetTTL,
		URLTTL:     cfg.Files.URLTTL,
		MaxBytes:   cfg.Files.MaxBytes,
	})
	if fsErr != nil {
		return fmt.Errorf("file store init failed: %w", fsErr)
	}

	// Image assets
	imageAssetSvc := imageassets.New(imageassets.Config{
		StorageDir: filepath.Join(cfg.Storage.DataDir, "images"),
		Retention:  cfg.Image.Retention,
	}, &http.Client{Timeout: 60 * time.Second})

	// ── AI Serving Pipeline + Gateway Runtime ──

	// OAuth credentials store + token refresher
	oauthCreds := aiadapters.NewOAuthCredentialStore(pool, cfg.Security.SecretMasterKey)
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
	// The default in-flight cap is what makes billing overshoot a finite number:
	// settlement is post-paid, so an account overdraws by at most this many
	// concurrent requests' worth before admission refuses the next one.
	rateLimiter := redisadapter.NewRateLimiter(redisClient, q).
		WithDefaultInFlight(cfg.Runtime.DefaultInFlightPerAccount)
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
		aiadapters.NewRuntimeTargetBinder(q, pool, cfg.Security.SecretMasterKey).WithBridgeSupport(bridgeRuntime),
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
		WithLogger(appLogger).
		WithAuditEnqueuer(auditStore)

	// Balance settlement. The runtime only enqueues charges; this consumer is
	// what actually moves money, so it must be running for balances to advance.
	go billingoutbox.NewConsumer(pool, appLogger).Run(ctx)

	// API key cache (already created above, wire to usage logger)
	usageLogger.WithAPIKeyCacheInvalidator(apiKeyCache)

	// Content moderation step (already created riskControlChecker above)
	contentModerationStep := &serving.ContentModerationStep{Checker: riskControlChecker, Worker: riskControlWorker}

	// Pipeline steps
	quotaCheckStep := &serving.QuotaCheckStep{}
	subscriptionGateStep := &serving.SubscriptionGateStep{Subs: subsPort(subsSvc), Logger: appLogger}
	balanceGateStep := &serving.BalanceGateStep{Resolver: aiadapters.NewRuntimeBalanceResolver(pool)}
	billingGuardStep := &serving.BillingGuardStep{Resolver: priceBookBiller}
	routeCandidatesStep := &serving.RouteCandidatesStep{Selector: runtimeRouteSelector, Sticky: stickyStore}
	rateLimitStep := &serving.RateLimitStep{Limiter: rateLimiter}

	// Upstream HTTP transport + client runtime
	upstreamHTTPTransport := aitransport.NewClient(0)
	upstreamHTTPTransport.SetProxySelector(proxySvc)
	fixedClientRuntime := clientruntime.New(
		clienttransport.New(upstreamHTTPTransport),
		clientcredentials.New(oauthCreds, refresher),
	)
	poolModelCatalog := clientcatalog.New(oauthCreds, fixedClientRuntime, appLogger)
	managementHTTPClient := &http.Client{}

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
		ModuleGate:      moduleSvc,
		Privacy:         privacy.NewProtector(),
	}
	usageCompletionFinalizer := &serving.UsageLogFinalizer{Logger: usageLogger, Metrics: metricsGW}
	auditFinalizer := &serving.AuditFinalizer{Worker: auditWorker}
	rateLimitFinalizer := serving.RateLimitFinalizer{}

	pipeline := serving.NewPipeline(
		&serving.AuthNStep{Resolver: aiadapters.NewAPIKeyResolver(q)},
		contentModerationStep,
		quotaCheckStep,
		subscriptionGateStep,
		balanceGateStep,
		routeCandidatesStep,
		billingGuardStep,
		rateLimitStep,
		executeStep,
	).WithFinalizers(
		usageCompletionFinalizer,
		auditFinalizer,
		rateLimitFinalizer,
	)

	// Runtime engine + gateway
	runtimeEngine := gateway.NewRuntimeEngine(pipeline)
	taskAdmission := serving.NewAdmissionGate(
		quotaCheckStep, subscriptionGateStep, balanceGateStep, routeCandidatesStep, billingGuardStep,
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
		Subjects:     gateway.NewTaskSubjectResolver(q),
		RedactDetail: serving.RedactInternalErrorDetail,
	})
	if asynctaskErr != nil {
		return fmt.Errorf("build async task engine failed: %w", asynctaskErr)
	}

	var runtimeGateway = gateway.New(gateway.Deps{
		Logger:        appLogger,
		Postgres:      pool,
		Pipeline:      pipeline,
		Queries:       q,
		APIKeyCache:   apiKeyCache,
		BanChecker:    banChecker,
		RuntimeEngine: runtimeEngine,
		AsyncTasks:    asyncTasks,
		TaskAdmission: taskAdmission,
	})
	runtimeGateway.RegisterTaskHandlers(asyncTasks)

	// Management console
	mgmtConsole := console.New(console.Deps{
		Postgres:       pool,
		Redis:          redisClient,
		Logger:         appLogger,
		Queries:        q,
		TokenVerifier:  jwtSvc,
		BanChecker:     banChecker,
		HTTPClient:     managementHTTPClient,
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
	asyncTasks.Start(ctx)
	shutdowns.Add("async task engine", func(ctx context.Context) error {
		asyncTasks.Stop(ctx)
		return nil
	})
	dataCleanupSvc.Start(ctx)

	// Hourly cleanups
	go runHourlyCleanup(ctx, func() { imageAssetSvc.CleanupExpired() })
	go runHourlyCleanup(ctx, func() { fileStore.CleanupExpired(ctx, 500) })
	go runHourlyCleanup(ctx, func() {
		if _, err := sessionSvc.DeleteExpired(ctx, 5000); err != nil {
			appLogger.Warn("expired auth session cleanup failed", zap.Error(err))
		}
	})
	go runHourlyCleanup(ctx, func() {
		if _, err := activationSvc.DeleteExpired(ctx, 5000); err != nil {
			appLogger.Warn("expired activation credential cleanup failed", zap.Error(err))
		}
	})

	// ──────────────────────────────────────────────────────
	// 4. 统一 Transport 装配
	// ──────────────────────────────────────────────────────

	deps := transport.Deps{
		InfrastructureDeps: transport.InfrastructureDeps{
			Version: version,
			Pool:    pool,
			Redis:   redisClient,
			Logger:  appLogger,
		},
		PortalDeps: transport.PortalDeps{
			SecureCookies: cfg.App.Env == "production",
			Legal:         cfg.Legal,
		},
		IdentityDeps: transport.IdentityDeps{
			JWT:         jwtSvc,
			Sessions:    sessionSvc,
			Activations: activationSvc,
			MFA:         mfaSvc,
			RecentAuth:  recentAuthSvc,
			Blacklist:   blacklist,
			UserService: userSvc,
			Invite:      inviteSvc,
		},
		BillingDeps: transport.BillingDeps{
			Deduction: deductionSvc,
			Payment:   paymentSvc,
		},
		OperationsDeps: transport.OperationsDeps{
			Announcements: announcementSvc,
			Notifications: notificationSvc,
			Modules:       moduleSvc,
			ProxyNodes:    proxySvc,
			DataCleanup:   dataCleanupSvc,
		},
		AIDeps: transport.AIDeps{
			AIInfrastructureDeps: transport.AIInfrastructureDeps{
				Queries:         q,
				SecretMasterKey: cfg.Security.SecretMasterKey,
				AIHTTPClient:    managementHTTPClient,
				Health:          healthTracker,
				Weights:         routeWeightsStore,
				BanChecker:      banChecker,
			},
			AIIdentityDeps: transport.AIIdentityDeps{
				OAuth:          oauthCreds,
				TokenRefresher: refresher,
				APIKeySvc:      apiKeySvc,
				WorkspaceSvc:   workspaceSvc,
			},
			AIBillingDeps: transport.AIBillingDeps{
				Subscriptions: subsSvc,
			},
			AICatalogDeps: transport.AICatalogDeps{
				ClientCatalog:     poolModelCatalog,
				PriceBookSvc:      priceBookSvc,
				CommercialSvc:     commercialSvc,
				GroupTransferSvc:  groupTransferSvc,
				AccountSvc:        accountSvc,
				UpstreamAccessSvc: upstreamAccessSvc,
			},
			AIOperationsDeps: transport.AIOperationsDeps{
				DashboardSvc:         dashboardSvc,
				UsageSvc:             usageSvc,
				AuditSvc:             auditSvc,
				RiskControlConfigSvc: riskControlConfigSvc,
				RiskControlLogSvc:    riskControlLogSvc,
				RiskControlEventSvc:  riskControlEventSvc,
				RiskControlChecker:   riskControlChecker,
			},
		},
	}

	router, api := server.New(server.Options{
		Title:        "D-AI",
		Version:      version,
		Logger:       appLogger,
		HSTS:         cfg.App.Env == "production",
		MaxBodyBytes: cfg.Server.MaxBodyBytes,
	})

	transport.Register(api, deps)
	transport.RegisterRaw(router, deps)

	// AI gateway + console + fileStore 路由
	fileStore.Routes(router)
	runtimeGateway.Routes(router)
	mgmtConsole.Routes(router)

	// 健康检查
	healthHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "version": version})
	})
	readyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
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
	router.Handle("/health", healthHandler)

	// Prometheus and management probes stay off the public business listener.
	// The default management address is loopback; deployments that need a
	// remote scraper must expose this listener through a private management
	// network or an authenticated proxy.
	managementMux := http.NewServeMux()
	managementMux.Handle("/metrics", aimetrics.Handler())
	managementMux.Handle("/health", healthHandler)
	managementMux.Handle("/ready", readyHandler)

	// ──────────────────────────────────────────────────────
	// 5. 前端静态文件 embed
	// ──────────────────────────────────────────────────────

	if frontendSub, err := fs.Sub(frontendFS, "frontend_dist"); err == nil {
		router.Handle("/*", newPortalHandler(frontendSub))
	} else {
		appLogger.Warn("frontend embed not available, serving API only", zap.Error(err))
	}

	// ──────────────────────────────────────────────────────
	// 6. 启动 HTTP 服务
	// ──────────────────────────────────────────────────────

	addr := cfg.Server.Addr

	httpRuntime := newHTTPServers(httpServerOptions{
		PublicAddr:        addr,
		ManagementAddr:    strings.TrimSpace(cfg.Server.ManagementAddr),
		PublicHandler:     weborigin.Middleware(router, originResolver),
		ManagementHandler: server.SecurityHeaders(cfg.App.Env == "production")(server.NoStoreAPI(server.RequestBodyLimit(1 << 20)(managementMux))),
		ReadTimeout:       time.Duration(cfg.Server.ReadTimeout) * time.Second,
		IdleTimeout:       time.Duration(cfg.Server.IdleTimeout) * time.Second,
		MaxHeaderBytes:    cfg.Server.MaxHeaderBytes,
		Logger:            appLogger,
		Version:           version,
	})
	httpRuntime.Start(stop)

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpRuntime.Shutdown(shutdownCtx); err != nil {
		appLogger.Warn("HTTP listener shutdown incomplete", zap.Error(err))
	}
	appLogger.Info("server shutdown complete")
	return nil
}

// ── 辅助函数 ──────────────────────────────────────────

func newPortalHandler(dist fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(dist))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		filePath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if info, err := fs.Stat(dist, filePath); err == nil && !info.IsDir() {
			setPortalCachePolicy(w, r.URL.Path, filePath)
			fileServer.ServeHTTP(w, r)
			return
		}

		if (r.Method != http.MethodGet && r.Method != http.MethodHead) || isBackendPath(r.URL.Path) {
			http.NotFound(w, r)
			return
		}

		request := r.Clone(r.Context())
		request.URL.Path = "/"
		request.URL.RawPath = ""
		w.Header().Set("Cache-Control", "no-store")
		fileServer.ServeHTTP(w, request)
	})
}

func setPortalCachePolicy(w http.ResponseWriter, requestPath, filePath string) {
	if filePath == "index.html" || requestPath == "/" {
		w.Header().Set("Cache-Control", "no-store")
		return
	}
	if strings.HasPrefix(requestPath, "/assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	// Brand images and other embedded static files are public but not assumed
	// content-hashed, so keep their cache lifetime bounded.
	w.Header().Set("Cache-Control", "public, max-age=86400")
}

func isBackendPath(requestPath string) bool {
	for _, prefix := range []string{
		"/api",
		"/runtime",
		"/v1",
		"/v1beta",
		"/.well-known",
		"/assets",
		"/docs",
		"/openapi.json",
		"/healthz",
		"/health",
		"/ready",
		"/metrics",
	} {
		if requestPath == prefix || strings.HasPrefix(requestPath, prefix+"/") {
			return true
		}
	}
	return false
}

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
