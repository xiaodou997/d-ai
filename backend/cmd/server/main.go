package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"uni-ai-api/backend/internal/cache"
	"uni-ai-api/backend/internal/config"
	"uni-ai-api/backend/internal/db"
	"uni-ai-api/backend/internal/httpserver"
	"uni-ai-api/backend/internal/urm"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config failed", "error", err)
		os.Exit(1)
	}

	logger := newLogger(cfg.App.LogLevel)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pg, err := db.Open(ctx, cfg.Postgres)
	if err != nil {
		logger.Error("connect postgres failed", "error", err)
		os.Exit(1)
	}
	defer pg.Close()

	redisClient, err := cache.Open(ctx, cfg.Redis)
	if err != nil {
		logger.Error("connect redis failed", "error", err)
		os.Exit(1)
	}
	if redisClient != nil {
		defer redisClient.Close()
	}

	server := httpserver.New(httpserver.Config{
		App:      cfg.App,
		Security: cfg.Security,
		URM:      urm.NewClient(cfg.URM),
		Postgres: pg,
		Redis:    redisClient,
		Logger:   logger,
	})

	errCh := make(chan error, 1)
	go func() {
		logger.Info("starting server", "addr", cfg.App.HTTPAddr)
		errCh <- server.Start(cfg.App.HTTPAddr)
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

func newLogger(level string) *slog.Logger {
	var slogLevel slog.Level
	switch level {
	case "debug":
		slogLevel = slog.LevelDebug
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slogLevel,
	}))
}
