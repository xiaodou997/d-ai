package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"uni-ai-api/backend/internal/config"
	"uni-ai-api/backend/internal/db"
)

const defaultSeedFile = "seeds/local_dev.sql"

func main() {
	seedFile := defaultSeedFile
	if len(os.Args) > 2 {
		fmt.Fprintln(os.Stderr, "usage: seed [seed-file]")
		os.Exit(2)
	}
	if len(os.Args) == 2 {
		seedFile = os.Args[1]
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config failed", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pg, err := db.Open(ctx, cfg.Postgres)
	if err != nil {
		slog.Error("connect postgres failed", "error", err)
		os.Exit(1)
	}
	defer pg.Close()

	sqlText, err := os.ReadFile(seedFile)
	if err != nil {
		slog.Error("read seed file failed", "file", seedFile, "error", err)
		os.Exit(1)
	}

	if _, err := pg.Exec(ctx, string(sqlText)); err != nil {
		slog.Error("execute seed failed", "file", seedFile, "error", err)
		os.Exit(1)
	}

	slog.Info("seed completed", "file", seedFile)
}
