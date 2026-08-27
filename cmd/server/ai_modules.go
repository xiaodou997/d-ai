package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
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
	"xiaodou/dai/internal/ai/externalmodels"
	"xiaodou/dai/internal/ai/filestore"
	"xiaodou/dai/internal/ai/gateway"
	"xiaodou/dai/internal/ai/identitycontrol"
	"xiaodou/dai/internal/ai/imageassets"
	aiobservability "xiaodou/dai/internal/ai/observability"
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
// Its public surface is intentionally limited to AI HTTP dependencies and the
// route owners consumed by the composition root; platform route wiring stays
// in the composition root.
type aiModules struct {
	AIHTTPDeps        transport.AIHTTPDeps
	FileStore         *filestore.Service
	ImageAssets       *imageassets.Service
	RuntimeGateway    *gateway.Gateway
	ManagementConsole *console.Console
	MetricsHandler    http.Handler
	AsyncTasks        *asynctask.Engine

	priceBookSvc       *billingcontrol.Service
	clientRuntime      *clientruntime.Runtime
	clientCatalog      *clientcatalog.Service
	runtimeBinder      *coreruntime.CachedBindingResolver
	subscriptionSvc    *subscription.Service
	riskControlWorker  *riskcontrol.Worker
	auditWorker        *audit.Worker
	refresher          *tokenrefresh.Refresher
	settlementConsumer *billingoutbox.Consumer
	logger             *zap.Logger
	workerCtx          context.Context
	workerCancel       context.CancelFunc
	lifecycleMu        sync.Mutex
	stopMu             sync.Mutex
	started            bool
	stopped            bool
	startOnce          sync.Once
}

func buildAIModules(cfg *config.Config, pool, billingPool *pgxpool.Pool, redisClient *redis.Client, appLogger *zap.Logger, platform *platformModules) (*aiModules, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	if platform == nil {
		return nil, fmt.Errorf("platform modules are required")
	}
	if appLogger == nil {
		appLogger = zap.NewNop()
	}
	if billingPool == nil {
		billingPool = pool
	}

	q := aiadapters.NewQueries(pool)
	banChecker := banstate.NewChecker(redisClient)
	redisHealth := redisadapter.NewHealthProbe(redisClient)
	databaseHealth := aiadapters.NewHealthProbe(pool)
	identityEnrichmentFailures := aiobservability.NewIdentityEnrichmentLogger(appLogger)
	providerSecrets := secret.NewProviderKeyCodec(cfg.Security.SecretMasterKey)

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

	accountSvc := upstreamcontrol.New(aiadapters.NewAccountRepo(q, pool), providerSecrets.Encrypt)
	modelBindings := aiadapters.NewUpstreamModelBindingStore(pool)
	modelCatalog := aiadapters.NewModelCatalogReader(pool)
	upstreamAccessSvc := upstreamaccess.New(aiadapters.NewUpstreamAccessRepo(pool))

	apiKeyCache := apikey.NewCache(redisClient)
	apiKeySvc := identitycontrol.New(
		aiadapters.NewAPIKeyRepo(q),
		apiKeyCacheForSvc(apiKeyCache),
		providerSecrets.Encrypt,
		providerSecrets.Decrypt,
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

	purchaser := subscription.NewBillingPurchaser(billingPool, "dai")
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
	modelCapabilities := externalmodels.New(redisClient, managementHTTPClient)

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
	imageAssetSvc.SetTaskRetainer(func(ctx context.Context, taskID string) (bool, error) {
		_, err := asyncTasks.Inspect(ctx, taskID)
		if errors.Is(err, asynctask.ErrNotFound) {
			return false, nil
		}
		return err == nil, err
	})

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

		WorkspaceModels:   workspaceSvc,
		WorkspaceSessions: workspaceSvc,
		WorkspaceManager:  workspaceSvc,
		WorkspaceMessages: workspaceSvc,
		WorkspaceImages:   workspaceSvc,

		ImageAssets: imageAssetSvc,
		FileStore:   fileStore,
		AsyncTasks:  cfg.AsyncTasks,
	})
	mgmtConsole.RegisterImageTaskHandlers(asyncTasks)

	modules := &aiModules{
		AIHTTPDeps: transport.AIHTTPDeps{
			Core: transport.AICoreHTTPDeps{
				PlatformPriceBooks:         priceBookSvc,
				PriceBookSync:              priceBookSvc,
				LimitPolicies:              commercialSvc,
				BanChecker:                 banChecker,
				IdentityEnrichmentFailures: identityEnrichmentFailures,
			},
			Subscriptions: transport.AISubscriptionHTTPDeps{
				SubscriptionPlans:          subsSvc,
				SubscriptionPlanWriter:     subsSvc,
				SubscriptionPurchases:      subsSvc,
				Subscriptions:              subsSvc,
				SubscriptionOrders:         subsSvc,
				SubscriptionGroupNames:     subsSvc,
				BanChecker:                 banChecker,
				IdentityEnrichmentFailures: identityEnrichmentFailures,
			},
			RiskControl: transport.AIRiskControlHTTPDeps{
				ProviderSecrets:     providerSecrets,
				RiskControlConfig:   riskControlConfigSvc,
				RiskControlDetector: riskControlChecker,
				RiskControlLogs:     riskControlLogSvc,
				RiskEvents:          riskControlEventSvc,
				BanChecker:          banChecker,
			},
			AuditLog: transport.AIAuditLogHTTPDeps{
				AuditLogs:  auditSvc,
				BanChecker: banChecker,
			},
			System: transport.AISystemHTTPDeps{
				DatabaseHealth: databaseHealth,
				RedisHealth:    redisHealth,
				Health:         healthTracker,
				Weights:        routeWeightsStore,
				BanChecker:     banChecker,
			},
			Dashboard: transport.AIDashboardHTTPDeps{
				DashboardQueries:           dashboardSvc,
				BanChecker:                 banChecker,
				IdentityEnrichmentFailures: identityEnrichmentFailures,
			},
			Usage: transport.AIUsageHTTPDeps{
				UsageQueries:               usageSvc,
				BanChecker:                 banChecker,
				IdentityEnrichmentFailures: identityEnrichmentFailures,
			},
			OAuthManagement: transport.AIOAuthManagementHTTPDeps{
				CredentialCreator: oauthCreds,
				CredentialReader:  oauthCreds,
				CredentialWriter:  oauthCreds,
				PoolReader:        oauthCreds,
				PoolWriter:        oauthCreds,
				PoolHealthReader:  oauthCreds,
				TokenRefresher:    refresher,
				ClientCatalog:     poolModelCatalog,
				ModelBindings:     modelBindings,
				BanChecker:        banChecker,
			},
			ModelBindings: transport.AIModelBindingHTTPDeps{
				AccountReader: accountSvc,
				PoolReader:    oauthCreds,
				ModelBindings: modelBindings,
				BanChecker:    banChecker,
			},
			UpstreamDiagnostics: transport.AIUpstreamDiagnosticsHTTPDeps{
				AccountReader:     accountSvc,
				ModelBindings:     modelBindings,
				ProviderSecrets:   providerSecrets,
				HTTPClient:        managementHTTPClient,
				AccountHealth:     accountSvc,
				ModelCapabilities: modelCapabilities,
				BanChecker:        banChecker,
			},
			UpstreamAccounts: transport.AIUpstreamAccountManagementHTTPDeps{
				Accounts:        accountSvc,
				AccountManager:  accountSvc,
				AccountReader:   accountSvc,
				ProviderSecrets: providerSecrets,
				ModelBindings:   modelBindings,
				PriceBooks:      priceBookSvc,
				AdminAudit:      auditSvc,
				BanChecker:      banChecker,
			},
			UpstreamAccess: transport.AIUpstreamAccessManagementHTTPDeps{
				UpstreamAccess: upstreamAccessSvc,
				BanChecker:     banChecker,
			},
			TenantCatalog: transport.AITenantCatalogHTTPDeps{
				ModelCatalog:     modelCatalog,
				Groups:           commercialSvc,
				TenantPriceBooks: priceBookSvc,
				PriceBookSync:    priceBookSvc,
				BanChecker:       banChecker,
			},
			TenantSelfControl: transport.AITenantSelfControlHTTPDeps{
				APIKeys:         apiKeySvc,
				APIKeyWriter:    apiKeySvc,
				APIKeyLifecycle: apiKeySvc,
				APIKeySecrets:   apiKeySvc,
				Groups:          commercialSvc,
				LimitPolicies:   commercialSvc,
				BanChecker:      banChecker,
			},
			TenantGroups: transport.AITenantGroupManagementHTTPDeps{
				Groups:           commercialSvc,
				GroupManager:     commercialSvc,
				DispatchRules:    commercialSvc,
				GroupTargets:     commercialSvc,
				UserBindings:     commercialSvc,
				TenantPriceBooks: priceBookSvc,
				GroupTransfer:    groupTransferSvc,
				AdminAudit:       auditSvc,
				BanChecker:       banChecker,
			},
			APIKeyManagement: transport.AIAPIKeyManagementHTTPDeps{
				APIKeys:         apiKeySvc,
				APIKeyWriter:    apiKeySvc,
				APIKeyLifecycle: apiKeySvc,
				APIKeySecrets:   apiKeySvc,
				Groups:          commercialSvc,
				LimitPolicies:   commercialSvc,
				BanChecker:      banChecker,
			},
			TenantSelfRead: transport.AITenantSelfReadHTTPDeps{
				DashboardQueries: dashboardSvc,
				UsageQueries:     usageSvc,
				BanChecker:       banChecker,
			},
			Workspace: transport.AIWorkspaceHTTPDeps{
				WorkspaceOverview: workspaceSvc,
				WorkspaceModels:   workspaceSvc,
				WorkspaceSessions: workspaceSvc,
				WorkspaceManager:  workspaceSvc,
				WorkspaceImages:   workspaceSvc,
				DashboardQueries:  dashboardSvc,
				UsageQueries:      usageSvc,
				BanChecker:        banChecker,
			},
			UserSelfControl: transport.AIUserSelfControlHTTPDeps{
				APIKeys:         apiKeySvc,
				APIKeyWriter:    apiKeySvc,
				APIKeyLifecycle: apiKeySvc,
				APIKeySecrets:   apiKeySvc,
				Groups:          commercialSvc,
				LimitPolicies:   commercialSvc,
				BanChecker:      banChecker,
			},
			UserSelfRead: transport.AIUserSelfReadHTTPDeps{
				Groups:        commercialSvc,
				ModelCatalog:  modelCatalog,
				UserUsageLogs: usageSvc,
				UsageQueries:  usageSvc,
				BanChecker:    banChecker,
			},
		},
		FileStore:          fileStore,
		ImageAssets:        imageAssetSvc,
		RuntimeGateway:     runtimeGateway,
		ManagementConsole:  mgmtConsole,
		MetricsHandler:     aimetrics.Handler(),
		AsyncTasks:         asyncTasks,
		priceBookSvc:       priceBookSvc,
		clientRuntime:      fixedClientRuntime,
		clientCatalog:      poolModelCatalog,
		runtimeBinder:      runtimeBinder,
		subscriptionSvc:    subsSvc,
		riskControlWorker:  riskControlWorker,
		auditWorker:        auditWorker,
		refresher:          refresher,
		settlementConsumer: billingoutbox.NewConsumer(billingPool, appLogger),
		logger:             appLogger,
	}
	if err := validateAIAssembly(modules); err != nil {
		return nil, err
	}
	return modules, nil
}

// validateAIAssembly keeps the AI runtime role fail-fast: every process-owned
// worker and request-path owner must be present before routes are registered.
func validateAIAssembly(m *aiModules) error {
	if m == nil {
		return fmt.Errorf("AI assembly is nil")
	}
	missing := make([]string, 0, 8)
	if m.RuntimeGateway == nil {
		missing = append(missing, "runtime_gateway")
	}
	if m.ManagementConsole == nil {
		missing = append(missing, "management_console")
	}
	if m.FileStore == nil {
		missing = append(missing, "file_store")
	}
	if m.AsyncTasks == nil {
		missing = append(missing, "async_tasks")
	}
	if m.clientRuntime == nil {
		missing = append(missing, "client_runtime")
	}
	if m.clientCatalog == nil {
		missing = append(missing, "client_catalog")
	}
	if m.subscriptionSvc == nil {
		missing = append(missing, "subscription")
	}
	if m.riskControlWorker == nil {
		missing = append(missing, "risk_control_worker")
	}
	if m.auditWorker == nil {
		missing = append(missing, "audit_worker")
	}
	if m.refresher == nil {
		missing = append(missing, "token_refresher")
	}
	if m.settlementConsumer == nil {
		missing = append(missing, "settlement_consumer")
	}
	if len(missing) > 0 {
		return fmt.Errorf("AI assembly missing required dependencies: %s", strings.Join(missing, ", "))
	}
	return nil
}

func (m *aiModules) Start(ctx context.Context) {
	if m == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	m.startOnce.Do(func() {
		m.stopMu.Lock()
		defer m.stopMu.Unlock()
		m.lifecycleMu.Lock()
		if m.stopped {
			m.lifecycleMu.Unlock()
			return
		}
		m.workerCtx, m.workerCancel = context.WithCancel(ctx)
		workerCtx := m.workerCtx
		m.started = true
		m.lifecycleMu.Unlock()
		if m.RuntimeGateway != nil {
			m.RuntimeGateway.Start()
		}
		if m.priceBookSvc != nil {
			m.priceBookSvc.Start(ctx)
		}
		if m.clientRuntime != nil {
			m.clientRuntime.Start(workerCtx)
		}
		if m.clientCatalog != nil {
			m.clientCatalog.Start(workerCtx)
		}
		if m.runtimeBinder != nil {
			m.runtimeBinder.Start(workerCtx)
		}
		if m.subscriptionSvc != nil {
			m.subscriptionSvc.Start(workerCtx)
		}
		if m.riskControlWorker != nil {
			m.riskControlWorker.Start(workerCtx, 0)
		}
		if m.auditWorker != nil {
			m.auditWorker.Start(workerCtx)
		}
		if m.refresher != nil {
			m.refresher.Start(workerCtx)
			if m.logger != nil {
				m.logger.Info("oauth token refresher started")
			}
		}
		if m.AsyncTasks != nil {
			m.AsyncTasks.Start(ctx)
		}
		if m.settlementConsumer != nil {
			go m.settlementConsumer.Run(workerCtx)
		}
	})
}

func (m *aiModules) Stop(ctx context.Context) error {
	if m == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	m.stopMu.Lock()
	defer m.stopMu.Unlock()

	m.lifecycleMu.Lock()
	if !m.stopped {
		m.stopped = true
	}
	started := m.started
	workerCancel := m.workerCancel
	m.lifecycleMu.Unlock()
	if !started {
		return nil
	}
	var errs []error
	if workerCancel != nil {
		workerCancel()
	}
	if m.clientCatalog != nil {
		if err := m.clientCatalog.Stop(ctx); err != nil {
			if m.logger != nil {
				m.logger.Warn("client catalog shutdown incomplete", zap.Error(err))
			}
			errs = append(errs, fmt.Errorf("stop client catalog: %w", err))
		}
	}
	if m.clientRuntime != nil {
		if err := m.clientRuntime.Stop(ctx); err != nil {
			if m.logger != nil {
				m.logger.Warn("client runtime shutdown incomplete", zap.Error(err))
			}
			errs = append(errs, fmt.Errorf("stop client runtime: %w", err))
		}
	}
	if m.RuntimeGateway != nil {
		if err := m.RuntimeGateway.Stop(ctx); err != nil {
			if m.logger != nil {
				m.logger.Warn("runtime gateway shutdown incomplete", zap.Error(err))
			}
			errs = append(errs, fmt.Errorf("stop runtime gateway: %w", err))
		}
	}
	if m.runtimeBinder != nil {
		if err := m.runtimeBinder.Stop(ctx); err != nil {
			if m.logger != nil {
				m.logger.Warn("runtime binding resolver shutdown incomplete", zap.Error(err))
			}
			errs = append(errs, fmt.Errorf("stop runtime binding resolver: %w", err))
		}
	}
	if m.subscriptionSvc != nil {
		if err := m.subscriptionSvc.Stop(ctx); err != nil {
			if m.logger != nil {
				m.logger.Warn("subscription janitor shutdown incomplete", zap.Error(err))
			}
			errs = append(errs, fmt.Errorf("stop subscription janitor: %w", err))
		}
	}
	if m.riskControlWorker != nil {
		if err := m.riskControlWorker.Stop(ctx); err != nil {
			if m.logger != nil {
				m.logger.Warn("risk control worker shutdown incomplete", zap.Error(err))
			}
			errs = append(errs, fmt.Errorf("stop risk control worker: %w", err))
		}
	}
	if m.auditWorker != nil {
		if err := m.auditWorker.Stop(ctx); err != nil {
			if m.logger != nil {
				m.logger.Warn("audit worker shutdown incomplete", zap.Error(err))
			}
			errs = append(errs, fmt.Errorf("stop audit worker: %w", err))
		}
	}
	if m.refresher != nil {
		if err := m.refresher.Stop(ctx); err != nil {
			if m.logger != nil {
				m.logger.Warn("oauth token refresher shutdown incomplete", zap.Error(err))
			}
			errs = append(errs, fmt.Errorf("stop oauth token refresher: %w", err))
		}
	}
	if m.settlementConsumer != nil {
		if err := m.settlementConsumer.Stop(ctx); err != nil {
			if m.logger != nil {
				m.logger.Warn("settlement consumer shutdown incomplete", zap.Error(err))
			}
			errs = append(errs, fmt.Errorf("stop settlement consumer: %w", err))
		}
	}
	if m.AsyncTasks != nil {
		if err := m.AsyncTasks.Stop(ctx); err != nil {
			if m.logger != nil {
				m.logger.Warn("async task engine shutdown incomplete", zap.Error(err))
			}
			errs = append(errs, fmt.Errorf("stop async task engine: %w", err))
		}
	}
	if m.priceBookSvc != nil {
		if err := m.priceBookSvc.Stop(ctx); err != nil {
			if m.logger != nil {
				m.logger.Warn("LiteLLM price refresh shutdown incomplete", zap.Error(err))
			}
			errs = append(errs, fmt.Errorf("stop LiteLLM price refresh: %w", err))
		}
	}
	return errors.Join(errs...)
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
