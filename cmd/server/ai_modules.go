package main

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

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
	"xiaodou/dai/internal/ai/observabilitycontrol"
	"xiaodou/dai/internal/ai/privacy"
	"xiaodou/dai/internal/ai/riskcontrol"
	"xiaodou/dai/internal/ai/routing"
	"xiaodou/dai/internal/ai/secret"
	"xiaodou/dai/internal/ai/serving"
	"xiaodou/dai/internal/ai/subscription"
	"xiaodou/dai/internal/ai/tokenrefresh"
	aitransport "xiaodou/dai/internal/ai/transport"
	"xiaodou/dai/internal/ai/upstreamaccess"
	"xiaodou/dai/internal/ai/upstreamcontrol"
	workspacesvc "xiaodou/dai/internal/ai/workspace"
	billingoutbox "xiaodou/dai/internal/billing/outbox"
	"xiaodou/dai/internal/config"
	"xiaodou/dai/internal/transport"
	"xiaodou/dai/libs/go/banstate"
)

// aiModules owns the AI control-plane services, runtime pipeline, and workers.
// Its public surface is intentionally limited to the assembled transport
// dependencies and the route owners consumed by the composition root.
type aiModules struct {
	Deps              transport.Deps
	AIDeps            transport.AIDeps
	FileStore         *filestore.Service
	ImageAssets       *imageassets.Service
	RuntimeGateway    *gateway.Gateway
	ManagementConsole *console.Console
	MetricsHandler    http.Handler
	AsyncTasks        *asynctask.Engine

	priceBookSvc       *billingcontrol.Service
	riskControlWorker  *riskcontrol.Worker
	auditWorker        *audit.Worker
	refresher          *tokenrefresh.Refresher
	settlementConsumer *billingoutbox.Consumer
	logger             *zap.Logger
	startOnce          sync.Once
	stopOnce           sync.Once
}

func buildAIModules(cfg *config.Config, pool *pgxpool.Pool, redisClient *redis.Client, appLogger *zap.Logger, platform *platformModules) (*aiModules, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	if platform == nil {
		return nil, fmt.Errorf("platform modules are required")
	}
	if appLogger == nil {
		appLogger = zap.NewNop()
	}

	q := aidb.New(pool)
	banChecker := banstate.NewChecker(redisClient)

	priceBookSvc := billingcontrol.New(
		aiadapters.NewPriceBookRepo(q, pool),
		billingcontrol.NewHTTPFetcher(cfg.Pricing.LiteLLMURL),
	)

	commercialRepo := aiadapters.NewCommercialRepo(q, pool)
	commercialSvc := commercial.NewService(commercialRepo)
	groupTransferSvc := commercial.NewGroupTransferService(commercialRepo, commercial.GroupTransferOptions{})

	dashboardSvc := observabilitycontrol.NewDashboardService(aiadapters.NewDashboardRepo(q))
	usageSvc := observabilitycontrol.NewUsageService(aiadapters.NewUsageRepo(q, pool))
	auditSvc := observabilitycontrol.NewAuditService(aiadapters.NewAuditRepo(q))

	accountSvc := upstreamcontrol.New(aiadapters.NewAccountRepo(q, pool), func(plaintext string) (string, error) {
		return secret.EncryptProviderKey(cfg.Security.SecretMasterKey, plaintext)
	})
	upstreamAccessSvc := upstreamaccess.New(aiadapters.NewUpstreamAccessRepo(pool))

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

	auditStore := aiadapters.NewAuditStore(pool)
	blobStore := blobstore.NewPGStore(pool)
	auditWorker := audit.NewWorker(auditStore, blobStore, audit.WorkerOptions{
		StoreImageBlobs: cfg.Audit.StoreImageBlobs,
	})

	purchaser := subscription.NewBillingPurchaser(pool, "dai")
	subsSvc := subscription.NewService(aiadapters.NewSubscriptionRepo(q, pool), purchaser, appLogger)

	workspaceRepo := aiadapters.NewWorkspaceRepo(pool, aiadapters.NewGroupAccessReader(pool), aiadapters.NewRouteInspector(pool))
	workspaceSvc := workspacesvc.NewService(
		workspaceRepo,
		workspacesvc.WithChatCatalog(aiadapters.NewWorkspaceChatCatalog(workspaceRepo)),
		workspacesvc.WithChatWriter(workspaceRepo),
		workspacesvc.WithChatRuntimeWriter(workspaceRepo),
	)

	fileStore, err := filestore.New(pool, filestore.Config{
		StorageDir: filepath.Join(cfg.Storage.DataDir, "files"),
		AssetTTL:   cfg.Files.AssetTTL,
		URLTTL:     cfg.Files.URLTTL,
		MaxBytes:   cfg.Files.MaxBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("file store init failed: %w", err)
	}

	imageAssetSvc := imageassets.New(imageassets.Config{
		StorageDir: filepath.Join(cfg.Storage.DataDir, "images"),
		Retention:  cfg.Image.Retention,
	}, &http.Client{Timeout: 60 * time.Second})

	oauthCreds := aiadapters.NewOAuthCredentialStore(pool, cfg.Security.SecretMasterKey)
	refresher := tokenrefresh.New(oauthCreds, appLogger)

	bridgeRuntime := bridgefmt.NewRuntime()
	grantChecker := aiadapters.NewGroupAccessReader(pool)
	routeInspector := aiadapters.NewRouteInspector(pool)
	routeInspector.WithBridgeSupport(bridgeRuntime)

	innerTracker := routing.DefaultInMemoryTracker()
	rht := routing.NewRedisHealthTracker(innerTracker, redisClient)
	healthTracker := routing.HealthTracker(rht)
	metricsGW := aimetrics.NewGateway()

	rateLimiter := redisadapter.NewRateLimiter(redisClient, q).
		WithDefaultInFlight(cfg.Runtime.DefaultInFlightPerAccount)
	upstreamConcurrencyLimiter := redisadapter.NewUpstreamConcurrencyLimiter(redisClient)

	routeWeightsStore := aiadapters.NewRouteWeightsStore(pool)
	routeStats := redisadapter.NewRedisRouteStats(redisClient)
	stickyStore := routing.StickyStore(redisadapter.NewRedisSticky(redisClient))
	scorer := &serving.MultiDimScorer{
		Health:  healthTracker,
		Stats:   routeStats,
		Weights: routeWeightsStore,
	}

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

	priceBookBiller := aiadapters.NewPriceBookBiller(priceBookSvc, q, pool)
	usageLogger := aiadapters.NewUsageLogger(pool, priceBookBiller).
		WithLogger(appLogger).
		WithAuditEnqueuer(auditStore)
	usageLogger.WithAPIKeyCacheInvalidator(apiKeyCache)

	contentModerationStep := &serving.ContentModerationStep{Checker: riskControlChecker, Worker: riskControlWorker}
	quotaCheckStep := &serving.QuotaCheckStep{}
	subscriptionGateStep := &serving.SubscriptionGateStep{Subs: subsPort(subsSvc), Logger: appLogger}
	balanceGateStep := &serving.BalanceGateStep{Resolver: aiadapters.NewRuntimeBalanceResolver(pool)}
	billingGuardStep := &serving.BillingGuardStep{Resolver: priceBookBiller}
	routeCandidatesStep := &serving.RouteCandidatesStep{Selector: runtimeRouteSelector, Sticky: stickyStore}
	rateLimitStep := &serving.RateLimitStep{Limiter: rateLimiter}

	upstreamHTTPTransport := aitransport.NewClient(0)
	upstreamHTTPTransport.SetProxySelector(platform.ProxyNodes)
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
		ModuleGate:      platform.Modules,
		Privacy:         privacy.NewProtector(),
	}
	usageCompletionFinalizer := &serving.UsageLogFinalizer{Logger: usageLogger, Metrics: metricsGW}
	auditFinalizer := &serving.AuditFinalizer{Worker: auditWorker}

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
		serving.RateLimitFinalizer{},
	)

	runtimeEngine := gateway.NewRuntimeEngine(pipeline)
	taskAdmission := serving.NewAdmissionGate(
		quotaCheckStep, subscriptionGateStep, balanceGateStep, routeCandidatesStep, billingGuardStep,
	)
	asyncTasks, err := asynctask.New(asynctask.Config{
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
	if err != nil {
		return nil, fmt.Errorf("build async task engine failed: %w", err)
	}

	runtimeGateway := gateway.New(gateway.Deps{
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

	mgmtConsole := console.New(console.Deps{
		Postgres:       pool,
		Redis:          redisClient,
		Logger:         appLogger,
		Queries:        q,
		TokenVerifier:  platform.JWT,
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

	return &aiModules{
		Deps: transport.Deps{
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
				JWT:         platform.JWT,
				Sessions:    platform.Sessions,
				Activations: platform.Activations,
				MFA:         platform.MFA,
				RecentAuth:  platform.RecentAuth,
				Blacklist:   platform.Blacklist,
				UserService: platform.UserService,
				Invite:      platform.Invite,
			},
			BillingDeps: transport.BillingDeps{
				Deduction: platform.Deduction,
				Payment:   platform.Payment,
			},
			OperationsDeps: transport.OperationsDeps{
				Announcements: platform.Announcements,
				Notifications: platform.Notifications,
				Modules:       platform.Modules,
				ProxyNodes:    platform.ProxyNodes,
				DataCleanup:   platform.DataCleanup,
			},
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
				PoolReader:     oauthCreds,
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
		FileStore:          fileStore,
		ImageAssets:        imageAssetSvc,
		RuntimeGateway:     runtimeGateway,
		ManagementConsole:  mgmtConsole,
		MetricsHandler:     aimetrics.Handler(),
		AsyncTasks:         asyncTasks,
		priceBookSvc:       priceBookSvc,
		riskControlWorker:  riskControlWorker,
		auditWorker:        auditWorker,
		refresher:          refresher,
		settlementConsumer: billingoutbox.NewConsumer(pool, appLogger),
		logger:             appLogger,
	}, nil
}

func (m *aiModules) Start(ctx context.Context) {
	if m == nil {
		return
	}
	m.startOnce.Do(func() {
		if m.priceBookSvc != nil {
			m.priceBookSvc.Start(ctx)
		}
		if m.riskControlWorker != nil {
			m.riskControlWorker.Start(ctx, 0)
		}
		if m.auditWorker != nil {
			m.auditWorker.Start(ctx)
		}
		if m.refresher != nil {
			go m.refresher.Start(ctx)
			if m.logger != nil {
				m.logger.Info("oauth token refresher started")
			}
		}
		if m.AsyncTasks != nil {
			m.AsyncTasks.Start(ctx)
		}
		if m.settlementConsumer != nil {
			go m.settlementConsumer.Run(ctx)
		}
	})
}

func (m *aiModules) Stop(ctx context.Context) {
	if m == nil {
		return
	}
	m.stopOnce.Do(func() {
		if m.AsyncTasks != nil {
			m.AsyncTasks.Stop(ctx)
		}
	})
}

// subsPort adapts a possibly-nil subscription service into the serving port.
func subsPort(s *subscription.Service) serving.Subscriptions {
	if s == nil {
		return nil
	}
	return s
}

// apiKeyCacheForSvc adapts *apikey.Cache into the identity-control port.
func apiKeyCacheForSvc(c *apikey.Cache) identitycontrol.KeyCache {
	if c == nil {
		return nil
	}
	return c
}
