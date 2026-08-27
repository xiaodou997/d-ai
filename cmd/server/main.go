package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/imageassets"
	"xiaodou/dai/internal/ai/observability/tracing"
	"xiaodou/dai/internal/config"
	"xiaodou/dai/internal/transport"
	"xiaodou/dai/internal/weborigin"

	// ── 公共库 ──
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
	lifecycle := newLifecycleHealth()
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
	pool, billingPool, redisClient := infra.pool, infra.billingPool, infra.redis
	// Dependencies are registered immediately after construction. Any later
	// module assembly error therefore releases them before run() returns.
	lifecycle.MarkStarted(healthPostgres)
	lifecycle.MarkStarted(healthBillingPostgres)
	lifecycle.MarkStarted(healthRedis)
	shutdowns.Add("postgres", func(context.Context) error {
		pool.Close()
		lifecycle.MarkStopped(healthPostgres)
		if billingPool == pool {
			lifecycle.MarkStopped(healthBillingPostgres)
		}
		return nil
	})
	if billingPool != pool {
		shutdowns.Add("billing postgres", func(context.Context) error {
			billingPool.Close()
			lifecycle.MarkStopped(healthBillingPostgres)
			return nil
		})
	}
	shutdowns.Add("redis", func(context.Context) error {
		err := redisClient.Close()
		if err == nil {
			lifecycle.MarkStopped(healthRedis)
		}
		return err
	})

	// ──────────────────────────────────────────────────────
	// 2. 平台身份与计费域装配
	// ──────────────────────────────────────────────────────

	platform, err := buildPlatformModules(cfg, pool, billingPool, redisClient, appLogger)
	if err != nil {
		return fmt.Errorf("build platform modules failed: %w", err)
	}
	platform.Start()
	shutdowns.Add("platform modules", func(context.Context) error {
		platform.Stop()
		lifecycle.MarkStopped(healthPlatformModules)
		if platform.banReconciler != nil {
			lifecycle.MarkStopped(healthBanReconciler)
		}
		if platform.sched != nil {
			lifecycle.MarkStopped(healthScheduler)
		}
		return nil
	})
	lifecycle.MarkStarted(healthPlatformModules)
	if platform.banReconciler != nil {
		lifecycle.MarkStarted(healthBanReconciler)
	}
	if platform.sched != nil {
		lifecycle.MarkStarted(healthScheduler)
	}
	sessionSvc := platform.Sessions
	activationSvc := platform.Activations
	dataCleanupSvc := platform.DataCleanup

	// ──────────────────────────────────────────────────────
	// 3. AI 域服务装配
	// ──────────────────────────────────────────────────────

	ai, err := buildAIModules(cfg, pool, billingPool, redisClient, appLogger, platform)
	if err != nil {
		return fmt.Errorf("build AI modules failed: %w", err)
	}
	ai.Start(ctx)
	shutdowns.Add("AI modules", func(ctx context.Context) error {
		ai.Stop(ctx)
		lifecycle.MarkStopped(healthAIModules)
		return nil
	})
	lifecycle.MarkStarted(healthAIModules)
	fileStore := ai.FileStore
	imageAssetSvc := ai.ImageAssets
	runtimeGateway := ai.RuntimeGateway
	mgmtConsole := ai.ManagementConsole

	dataCleanupSvc.Start(ctx)
	shutdowns.Add("data cleanup", func(ctx context.Context) error {
		dataCleanupSvc.Stop(ctx)
		lifecycle.MarkStopped(healthDataCleanup)
		return nil
	})
	lifecycle.MarkStarted(healthDataCleanup)

	// Hourly cleanups
	imageCleanupWorker := startHourlyCleanup(ctx, func(cleanupCtx context.Context) {
		if _, err := imageAssetSvc.CleanupExpired(cleanupCtx); err != nil && !errors.Is(err, imageassets.ErrCleanupAlreadyRunning) {
			appLogger.Warn("expired image asset cleanup failed", zap.Error(err))
		}
	})
	registerPeriodicWorker(shutdowns, lifecycle, "hourly image cleanup", healthHourlyImage, imageCleanupWorker)
	fileCleanupWorker := startHourlyCleanup(ctx, func(cleanupCtx context.Context) { fileStore.CleanupExpired(cleanupCtx, 500) })
	registerPeriodicWorker(shutdowns, lifecycle, "hourly file cleanup", healthHourlyFile, fileCleanupWorker)
	authSessionCleanupWorker := startHourlyCleanup(ctx, func(cleanupCtx context.Context) {
		if _, err := sessionSvc.DeleteExpired(cleanupCtx, 5000); err != nil {
			appLogger.Warn("expired auth session cleanup failed", zap.Error(err))
		}
	})
	registerPeriodicWorker(shutdowns, lifecycle, "hourly auth session cleanup", healthHourlyAuthSession, authSessionCleanupWorker)
	activationCleanupWorker := startHourlyCleanup(ctx, func(cleanupCtx context.Context) {
		if _, err := activationSvc.DeleteExpired(cleanupCtx, 5000); err != nil {
			appLogger.Warn("expired activation credential cleanup failed", zap.Error(err))
		}
	})
	registerPeriodicWorker(shutdowns, lifecycle, "hourly activation cleanup", healthHourlyActivation, activationCleanupWorker)

	// ──────────────────────────────────────────────────────
	// 4. 统一 Transport 装配
	// ──────────────────────────────────────────────────────

	deps := ai.Deps

	router, api := server.New(server.Options{
		Title:        "D-AI",
		Version:      version,
		Logger:       appLogger,
		HSTS:         cfg.App.Env == "production",
		MaxBodyBytes: cfg.Server.MaxBodyBytes,
	})

	transport.Register(api, deps, ai.AIHTTPDeps)
	transport.RegisterRaw(router, deps)

	// AI gateway + console + fileStore 路由
	fileStore.Routes(router)
	runtimeGateway.Routes(router)
	mgmtConsole.Routes(router)

	// 健康检查
	healthHandler := newHealthHandler(version, func() any {
		if platform != nil && platform.sched != nil {
			return platform.sched.Health()
		}
		return nil
	}, lifecycle)
	readyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if err := pool.Ping(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "error", "component": "postgres"})
			return
		}
		if billingPool != pool {
			if err := billingPool.Ping(r.Context()); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]any{"status": "error", "component": "billing_postgres"})
				return
			}
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
	managementMux.Handle("/metrics", ai.MetricsHandler)
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
	lifecycle.MarkStarted(healthHTTPPublic)
	if httpRuntime.management != nil {
		lifecycle.MarkStarted(healthHTTPManagement)
	}

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpRuntime.Shutdown(shutdownCtx); err != nil {
		appLogger.Warn("HTTP listener shutdown incomplete", zap.Error(err))
	} else {
		lifecycle.MarkStopped(healthHTTPPublic)
		if httpRuntime.management != nil {
			lifecycle.MarkStopped(healthHTTPManagement)
		}
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

// startHourlyCleanup runs cleanup immediately, then every hour until its
// worker context is cancelled. The returned worker must be registered with
// shutdownStack so shutdown waits for an in-flight cleanup to leave its
// dependency graph.
func startHourlyCleanup(ctx context.Context, cleanup func(context.Context)) *periodicWorker {
	if ctx == nil {
		ctx = context.Background()
	}
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		runHourlyCleanup(workerCtx, cleanup)
	}()
	return &periodicWorker{cancel: cancel, done: done}
}

// runHourlyCleanup runs cleanup immediately, then every hour until ctx cancels.
func runHourlyCleanup(ctx context.Context, cleanup func(context.Context)) {
	if cleanup == nil {
		return
	}
	cleanup(ctx)
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanup(ctx)
		}
	}
}
