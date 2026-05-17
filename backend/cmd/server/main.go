package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	pgadapter "uni-ai-api/backend/internal/adapters/postgres"
	"uni-ai-api/backend/internal/cache"
	"uni-ai-api/backend/internal/config"
	"uni-ai-api/backend/internal/db"
	"uni-ai-api/backend/internal/httpserver"
	obslogger "uni-ai-api/backend/internal/observability/logger"
	"uni-ai-api/backend/internal/observability/tracing"
	"uni-ai-api/backend/internal/tokenrefresh"
	"uni-ai-api/backend/internal/urm"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config failed", "error", err)
		os.Exit(1)
	}

	logger := obslogger.New()
	slog.SetDefault(logger)
	logger.Info("configuration loaded",
		"http_addr", cfg.Server.Addr,
		"urm_base_url", cfg.URM.BaseURL,
		"urm_client_id", cfg.URM.ClientID,
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	tracingShutdown := tracing.Init(ctx)
	defer func() {
		_ = tracingShutdown(context.Background())
	}()

	pg, err := db.Open(ctx, cfg.Postgres)
	if err != nil {
		logger.Error("connect postgres failed", "error", err)
		os.Exit(1)
	}
	defer pg.Close()
	logger.Info("postgres connected",
		"max_conns", cfg.Postgres.MaxConns,
		"min_conns", cfg.Postgres.MinConns,
	)

	redisClient, err := cache.Open(ctx, cfg.Redis)
	if err != nil {
		logger.Error("connect redis failed", "error", err)
		os.Exit(1)
	}
	if redisClient != nil {
		defer redisClient.Close()
		logger.Info("redis connected", "addr", cfg.Redis.Addr)
	} else {
		logger.Info("redis disabled")
	}

	urmBillingClient, err := urm.NewClient(cfg.URM)
	if err != nil {
		logger.Error("failed to create URM client", "error", err)
		os.Exit(1)
	}
	logger.Info("URM client registered successfully", "client_id", cfg.URM.ClientID)

	jwksValidator := urm.NewJWKSValidator(cfg.URM.BaseURL, cfg.URM.Timeout)
	if err := jwksValidator.Start(ctx); err != nil {
		logger.Warn("jwks initial fetch failed, will retry on first request", "error", err)
	} else {
		logger.Info("jwks loaded", "urm_base_url", cfg.URM.BaseURL)
	}

	var banSubscriber *urm.BanSubscriber
	if redisClient != nil {
		banSubscriber = urm.NewBanSubscriber(redisClient, logger)
		banSubscriber.Start(ctx)
		logger.Info("ban subscriber started")
	}

	// Start OAuth token refresher as a background goroutine.
	oauthCreds := pgadapter.NewOAuthCredentialStore(pg, cfg.Security.ProviderKeyMaster)
	refresher := tokenrefresh.New(oauthCreds, logger)
	go refresher.Start(ctx)
	logger.Info("oauth token refresher started")

	server := httpserver.New(httpserver.Config{
		Server:        cfg.Server,
		Security:      cfg.Security,
		URM:           urmBillingClient,
		URMClientID:   cfg.URM.ClientID,
		JWKSValidator: jwksValidator,
		BanSubscriber: banSubscriber,
		Postgres:      pg,
		Redis:         redisClient,
		Logger:        logger,
	})

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server starting", "addr", cfg.Server.Addr)
		errCh <- server.Start(cfg.Server.Addr)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("server shutdown failed", "error", err)
			os.Exit(1)
		}
		logger.Info("server stopped")
	case err := <-errCh:
		if err != nil {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}
}
