package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	pgadapter "xiaodou/uni-ai-api/internal/adapters/postgres"
	"xiaodou/uni-ai-api/internal/cache"
	"xiaodou/uni-ai-api/internal/config"
	"xiaodou/uni-ai-api/internal/db"
	"xiaodou/uni-ai-api/internal/httpserver"
	"xiaodou/uni-ai-api/internal/logger"
	"xiaodou/uni-ai-api/internal/observability/tracing"
	"xiaodou/uni-ai-api/internal/tokenrefresh"
	"xiaodou/uni-ai-api/internal/urm"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	logCfg := logger.LogConfig{
		Level: cfg.Log.Level,
		File:  cfg.Log.File,
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

	server := httpserver.New(httpserver.Config{
		Server:        cfg.Server,
		Security:      cfg.Security,
		URM:           urmBillingClient,
		URMClientID:   cfg.URM.ClientID,
		JWKSValidator: jwksValidator,
		BanSubscriber: banSubscriber,
		Postgres:      pg,
		Redis:         redisClient,
		Logger:        appLogger,
	})

	errCh := make(chan error, 1)
	go func() {
		appLogger.Info("server starting", zap.String("addr", cfg.Server.Addr))
		errCh <- server.Start(cfg.Server.Addr)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
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
