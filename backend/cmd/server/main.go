package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"uni-ai-api/backend/internal/cache"
	"uni-ai-api/backend/internal/config"
	"uni-ai-api/backend/internal/db"
	"uni-ai-api/backend/internal/httpserver"
	obslogger "uni-ai-api/backend/internal/observability/logger"
	"uni-ai-api/backend/internal/urm"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config failed", "error", err)
		os.Exit(1)
	}

	logger := obslogger.New(obslogger.Config{
		ServiceName: cfg.App.ServiceName,
		Env:         cfg.App.Env,
		Version:     cfg.App.Version,
		Logging:     cfg.Logging,
	})
	slog.SetDefault(logger)
	logger.Info("configuration loaded",
		"config_path", configPathForLog(cfg.SourcePath),
		"env", cfg.App.Env,
		"log_level", cfg.Logging.Level,
		"log_format", cfg.Logging.Format,
		"http_addr", cfg.Server.HTTPAddr,
	)
	logger.Debug("runtime configuration",
		"go_version", runtime.Version(),
		"postgres_max_conns", cfg.Postgres.MaxConns,
		"postgres_min_conns", cfg.Postgres.MinConns,
		"postgres_max_conn_lifetime", cfg.Postgres.MaxConnLifetime.String(),
		"redis_enabled", cfg.Redis.Enabled,
		"redis_addr", cfg.Redis.Addr,
		"redis_db", cfg.Redis.DB,
		"urm_base_url", cfg.URM.BaseURL,
		"urm_timeout", cfg.URM.Timeout.String(),
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pg, err := db.Open(ctx, cfg.Postgres)
	if err != nil {
		logger.Error("connect postgres failed", "error", err)
		os.Exit(1)
	}
	defer pg.Close()
	logger.Info("postgres connected",
		"max_conns", cfg.Postgres.MaxConns,
		"min_conns", cfg.Postgres.MinConns,
		"max_conn_lifetime", cfg.Postgres.MaxConnLifetime.String(),
	)

	redisClient, err := cache.Open(ctx, cfg.Redis)
	if err != nil {
		logger.Error("connect redis failed", "error", err)
		os.Exit(1)
	}
	if redisClient != nil {
		defer redisClient.Close()
		logger.Info("redis connected", "addr", cfg.Redis.Addr, "db", cfg.Redis.DB)
	} else {
		logger.Info("redis disabled")
	}
	logger.Info("urm client configured",
		"base_url", cfg.URM.BaseURL,
		"timeout", cfg.URM.Timeout.String(),
	)

	urmBillingClient := urm.NewClient(cfg.URM)

	jwksValidator := urm.NewJWKSValidator(cfg.URM.BaseURL, cfg.URM.JWKSRefreshInterval, cfg.URM.Timeout)
	if err := jwksValidator.Start(ctx); err != nil {
		logger.Warn("jwks initial fetch failed, will retry on first request", "error", err)
	} else {
		logger.Info("jwks loaded", "urm_base_url", cfg.URM.BaseURL)
	}

	server := httpserver.New(httpserver.Config{
		App:           cfg.App,
		Server:        cfg.Server,
		Logging:       cfg.Logging,
		Security:      cfg.Security,
		URM:           urmBillingClient,
		URMAppKey:     cfg.URM.AppKey,
		JWKSValidator: jwksValidator,
		Postgres:      pg,
		Redis:         redisClient,
		Logger:        logger,
	})

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server starting", "addr", cfg.Server.HTTPAddr)
		errCh <- server.Start(cfg.Server.HTTPAddr)
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

func configPathForLog(path string) string {
	if path == "" {
		return "(defaults)"
	}
	return path
}
