package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	pgadapter "xiaodou/unihub/ai-service/internal/adapters/postgres"
	redisadapter "xiaodou/unihub/ai-service/internal/adapters/redis"
	"xiaodou/unihub/ai-service/internal/apikey"
	"xiaodou/unihub/ai-service/internal/audit"
	"xiaodou/unihub/ai-service/internal/blobstore"
	"xiaodou/unihub/ai-service/internal/cache"
	"xiaodou/unihub/ai-service/internal/config"
	"xiaodou/unihub/ai-service/internal/console"
	"xiaodou/unihub/ai-service/internal/db"
	dbgen "xiaodou/unihub/ai-service/internal/db/gen"
	"xiaodou/unihub/ai-service/internal/domain"
	"xiaodou/unihub/ai-service/internal/gateway"
	"xiaodou/unihub/ai-service/internal/httpx"
	"xiaodou/unihub/ai-service/internal/ledger"
	"xiaodou/unihub/ai-service/internal/observability/metrics"
	"xiaodou/unihub/ai-service/internal/observability/tracing"
	"xiaodou/unihub/ai-service/internal/routing"
	"xiaodou/unihub/ai-service/internal/secret"
	apikeysvc "xiaodou/unihub/ai-service/internal/service/apikey"
	auditsvc "xiaodou/unihub/ai-service/internal/service/audit"
	dashboardsvc "xiaodou/unihub/ai-service/internal/service/dashboard"
	grantsvc "xiaodou/unihub/ai-service/internal/service/grant"
	limitsvc "xiaodou/unihub/ai-service/internal/service/limit"
	modelsvc "xiaodou/unihub/ai-service/internal/service/model"
	pricebooksvc "xiaodou/unihub/ai-service/internal/service/pricebook"
	providersvc "xiaodou/unihub/ai-service/internal/service/provider"
	usagesvc "xiaodou/unihub/ai-service/internal/service/usage"
	"xiaodou/unihub/ai-service/internal/serving"
	"xiaodou/unihub/ai-service/internal/tokenrefresh"
	"xiaodou/unihub/ai-service/internal/transport"
	"xiaodou/unihub/ai-service/internal/urm"
	"xiaodou/unihub/shared/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	logCfg := logger.LogConfig{
		Level:  cfg.Log.Level,
		File:   cfg.Log.File,
		Redact: cfg.Log.Redact,
	}
	appLogger := logger.InitLogger(cfg.App.Env, logCfg)
	defer appLogger.Sync()

	// Set global zap logger so zap.L() / zap.S() work everywhere
	logger.SetGlobal(appLogger)

	appLogger.Info("configuration loaded",
		zap.String("http_addr", cfg.Server.Addr),
		zap.String("urm_base_url", cfg.URM.BaseURL),
		zap.String("urm_client_id", cfg.URM.ClientID),
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	tracingShutdown := tracing.Init(ctx)
	defer func() {
		_ = tracingShutdown(context.Background())
	}()

	pg, err := db.Open(ctx, cfg.Postgres)
	if err != nil {
		appLogger.Error("connect postgres failed", zap.Error(err))
		os.Exit(1)
	}
	defer pg.Close()
	appLogger.Info("postgres connected",
		zap.Int32("max_conns", cfg.Postgres.MaxConns),
		zap.Int32("min_conns", cfg.Postgres.MinConns),
	)

	redisClient, err := cache.Open(ctx, cfg.Redis)
	if err != nil {
		appLogger.Error("connect redis failed", zap.Error(err))
		os.Exit(1)
	}
	if redisClient != nil {
		defer redisClient.Close()
		appLogger.Info("redis connected", zap.String("addr", cfg.Redis.Addr))
	} else {
		appLogger.Info("redis disabled")
	}

	urmBillingClient, err := urm.NewClient(cfg.URM)
	if err != nil {
		appLogger.Error("failed to create URM client", zap.Error(err))
		os.Exit(1)
	}
	appLogger.Info("URM client registered successfully", zap.String("client_id", cfg.URM.ClientID))

	jwksValidator := urm.NewJWKSValidator(cfg.URM.BaseURL, cfg.URM.Timeout)
	if err := jwksValidator.Start(ctx); err != nil {
		appLogger.Warn("jwks initial fetch failed, will retry on first request", zap.Error(err))
	} else {
		appLogger.Info("jwks loaded", zap.String("urm_base_url", cfg.URM.BaseURL))
	}

	var banSubscriber *urm.BanSubscriber
	if redisClient != nil {
		banSubscriber = urm.NewBanSubscriber(redisClient, appLogger)
		banSubscriber.Start(ctx)
		appLogger.Info("ban subscriber started")
	}

	// Start OAuth token refresher as a background goroutine.
	oauthCreds := pgadapter.NewOAuthCredentialStore(pg, cfg.Security.ProviderKeyMaster)
	refresher := tokenrefresh.New(oauthCreds, appLogger)
	go refresher.Start(ctx)
	appLogger.Info("oauth token refresher started")

	// ========================================================================
	// 装配：repository → serving pipeline → 运行时 gateway + 管理 console
	// ========================================================================
	q := dbgen.New(pg)
	grantChecker := pgadapter.NewModelGrantChecker(q)
	routeSelector := pgadapter.NewRouteSelector(q, pg, cfg.Security.ProviderKeyMaster, grantChecker).
		WithOAuthCredentialStore(oauthCreds).
		WithDefaultTimeouts(domain.RouteTimeouts{
			Connect:     cfg.Serving.Timeouts.Connect,
			FirstByte:   cfg.Serving.Timeouts.FirstByte,
			Idle:        cfg.Serving.Timeouts.Idle,
			MaxDuration: cfg.Serving.Timeouts.MaxDuration,
		})

	// Shared HealthTracker. With Redis: multi-node sync via Pub/Sub; otherwise
	// single-node in-memory only.
	innerTracker := routing.DefaultInMemoryTracker()
	var healthTracker routing.HealthTracker = innerTracker
	if redisClient != nil {
		rht := routing.NewRedisHealthTracker(innerTracker, redisClient)
		go rht.Start(context.Background())
		healthTracker = rht
	}
	routeSelector.WithHealth(healthTracker)
	metricsGW := metrics.NewGateway()

	var rateLimiter serving.RateLimiter
	if redisClient != nil {
		rateLimiter = redisadapter.NewRateLimiter(redisClient, q, 4096)
	}

	// 分账层：每请求结束后 LedgerStep finalizer 累加到 pending_*_micro；settle
	// worker 按 tick/阈值触发批量 URM Consume。
	var creditLedger *ledger.Ledger
	var settleWorker *ledger.Worker
	if urmBillingClient != nil {
		creditLedger = ledger.New(pg, urmBillingClient, appLogger)
		settleWorker = ledger.NewWorker(creditLedger, ledger.WorkerConfig{})
	}

	// P3 multi-dim scorer. RouteStats backed by Redis when available.
	routeWeightsStore := pgadapter.NewRouteWeightsStore(pg)
	var routeStats routing.RouteStatsStore = routing.NoopRouteStats{}
	if redisClient != nil {
		routeStats = redisadapter.NewRedisRouteStats(redisClient)
	}
	scorer := &serving.MultiDimScorer{
		Health:  healthTracker,
		Stats:   routeStats,
		Weights: routeWeightsStore,
	}

	// P4 sticky routing (disabled gracefully when Redis is nil).
	var stickyStore routing.StickyStore
	if redisClient != nil {
		stickyStore = redisadapter.NewRedisSticky(redisClient)
	}

	// Audit log: async structured request/response persistence.
	auditStore := pgadapter.NewAuditStore(pg)
	blobStore := blobstore.NewPGStore(pg)
	auditWorker := audit.NewWorker(auditStore, blobStore)

	// Unified Price Book pricing: one service drives the management API, the
	// fail-closed billing guard, and post-hoc usage billing.
	priceBookSvc := pricebooksvc.New(pgadapter.NewPriceBookRepo(q), pricebooksvc.NewHTTPFetcher(cfg.Pricing.LiteLLMURL))
	priceBookBiller := pgadapter.NewPriceBookBiller(priceBookSvc, appLogger)

	usageLogger := pgadapter.NewUsageLogger(q, priceBookBiller)

	var apiKeyCache *apikey.Cache
	if redisClient != nil {
		apiKeyCache = apikey.NewCache(redisClient)
	}

	pipeline := serving.NewPipeline(
		&serving.AuthNStep{Resolver: pgadapter.NewAPIKeyResolver(q)},
		&serving.AuthZStep{Checker: grantChecker},
		&serving.BillingGuardStep{Checker: priceBookBiller},
		&serving.QuotaCheckStep{},
		&serving.RouteCandidatesStep{Selector: routeSelector, Sticky: stickyStore},
		&serving.RateLimitStep{Limiter: rateLimiter},
		&serving.QuotaReserveStep{Reserver: pgadapter.NewQuotaReserver(q)},
		&serving.ExecuteStep{
			Transport: transport.NewClient(120 * time.Second),
			Health:    healthTracker,
			OAuthPool: oauthCreds,
			Budget:    serving.DefaultRetryBudget(),
			Scorer:    scorer,
			Stats:     routeStats,
			Sticky:    stickyStore,
		},
		&serving.UsageLogStep{Logger: usageLogger, Metrics: metricsGW},
	).WithFinalizers(
		&serving.LedgerStep{Ledger: creditLedger, Trigger: workerTrigger(settleWorker)},
		&serving.AuditFinalizer{Worker: auditWorker},
	)

	runtimeGateway := gateway.New(gateway.Deps{
		Logger:      appLogger,
		Pipeline:    pipeline,
		Queries:     q,
		APIKeyCache: apiKeyCache,
	})

	mgmtConsole := console.New(console.Deps{
		Postgres:          pg,
		Redis:             redisClient,
		Logger:            appLogger,
		Queries:           q,
		Security:          cfg.Security,
		URMClient:         urmBillingClient,
		URMClientID:       cfg.URM.ClientID,
		JWKSValidator:     jwksValidator,
		BanSubscriber:     banSubscriber,
		HTTPClient:        &http.Client{Timeout: 0},
		OAuthCreds:        oauthCreds,
		TokenRefresher:    refresher,
		RouteSelector:     routeSelector,
		RouteWeightsStore: routeWeightsStore,
		APIKeyCache:       apiKeyCache,
		Gateway:           runtimeGateway,

		AuditSvc:      auditsvc.New(pgadapter.NewAuditRepo(q)),
		GrantSvc:      grantsvc.New(pgadapter.NewGrantRepo(q)),
		LimitSvc:      limitsvc.New(pgadapter.NewLimitRepo(q)),
		DashboardSvc:  dashboardsvc.New(pgadapter.NewDashboardRepo(q)),
		UsageSvc:      usagesvc.New(pgadapter.NewUsageRepo(q, pg)),
		ModelSvc:      modelsvc.New(pgadapter.NewModelRepo(q)),
		ModelRouteSvc: modelsvc.NewRouteService(pgadapter.NewModelRouteRepo(q, pg)),
		PriceBookSvc:  priceBookSvc,
		ProviderSvc: providersvc.New(pgadapter.NewProviderRepo(q, pg), func(plaintext string) (string, error) {
			return secret.EncryptProviderKey(cfg.Security.ProviderKeyMaster, plaintext)
		}),
		APIKeySvc: apikeysvc.New(pgadapter.NewAPIKeyRepo(q), apiKeyCacheForSvc(apiKeyCache)),
	})

	// ========================================================================
	// Router：共享中间件 + infra 端点 + 两个面挂同一 router
	// ========================================================================
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(httpx.RequestLogger(appLogger))

	router.Get("/health", handleHealth)
	router.Get("/ready", handleReady(pg, redisClient))
	router.Get("/metrics", metrics.Handler().ServeHTTP)

	runtimeGateway.Routes(router)
	mgmtConsole.Routes(router)

	httpServer := &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      0,
		IdleTimeout:       60 * time.Second,
	}

	// 后台 worker 生命周期
	auditCtx, auditCancel := context.WithCancel(context.Background())
	defer auditCancel()
	auditWorker.Start(auditCtx)
	if settleWorker != nil {
		settleCtx, settleCancel := context.WithCancel(context.Background())
		defer settleCancel()
		go settleWorker.Run(settleCtx)
	}

	errCh := make(chan error, 1)
	go func() {
		appLogger.Info("server starting", zap.String("addr", cfg.Server.Addr))
		if serr := httpServer.ListenAndServe(); serr != nil && serr != http.ErrServerClosed {
			errCh <- serr
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			appLogger.Error("server shutdown failed", zap.Error(err))
			os.Exit(1)
		}
		appLogger.Info("server stopped")
	case err := <-errCh:
		if err != nil {
			appLogger.Error("server failed", zap.Error(err))
			os.Exit(1)
		}
	}
}

// apiKeyCacheForSvc adapts the concrete *apikey.Cache (which may be a nil
// pointer when Redis is disabled) into the apikey service's KeyCache port,
// returning a genuinely nil interface so the service's nil-check works (a typed
// nil pointer wrapped in an interface is not == nil).
func apiKeyCacheForSvc(c *apikey.Cache) apikeysvc.KeyCache {
	if c == nil {
		return nil
	}
	return c
}

// workerTrigger returns w.Trigger() or nil when the worker is unset, so
// LedgerStep.Trigger receives a typed-nil channel that select drops on.
func workerTrigger(w *ledger.Worker) chan<- struct{} {
	if w == nil {
		return nil
	}
	return w.Trigger()
}

type healthResponse struct {
	Status     string                     `json:"status"`
	Components map[string]componentStatus `json:"components,omitempty"`
	Timestamp  time.Time                  `json:"timestamp"`
}

type componentStatus struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok", Timestamp: time.Now().UTC()})
}

func handleReady(pg *pgxpool.Pool, redisClient *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		components := map[string]componentStatus{}
		ready := true

		if err := pg.Ping(ctx); err != nil {
			ready = false
			components["postgres"] = componentStatus{Status: "error", Error: err.Error()}
		} else {
			components["postgres"] = componentStatus{Status: "ok"}
		}

		if redisClient != nil {
			if err := redisClient.Ping(ctx).Err(); err != nil {
				ready = false
				components["redis"] = componentStatus{Status: "error", Error: err.Error()}
			} else {
				components["redis"] = componentStatus{Status: "ok"}
			}
		} else {
			components["redis"] = componentStatus{Status: "disabled"}
		}

		status := http.StatusOK
		bodyStatus := "ok"
		if !ready {
			status = http.StatusServiceUnavailable
			bodyStatus = "error"
		}

		writeJSON(w, status, healthResponse{Status: bodyStatus, Components: components, Timestamp: time.Now().UTC()})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
