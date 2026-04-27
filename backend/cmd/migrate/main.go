package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"uni-ai-api/backend/internal/config"
	"uni-ai-api/backend/internal/db"
)

const migrationFile = "migrations/00001_init.sql"

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: migrate up|down")
		os.Exit(2)
	}

	direction := os.Args[1]
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

	sqlText, err := migrationSQL(direction)
	if err != nil {
		slog.Error("load migration failed", "error", err)
		os.Exit(1)
	}

	if _, err := pg.Exec(ctx, sqlText); err != nil {
		slog.Error("execute migration failed", "direction", direction, "error", err)
		os.Exit(1)
	}

	slog.Info("migration completed", "direction", direction)
}

func migrationSQL(direction string) (string, error) {
	content, err := os.ReadFile(migrationFile)
	if err != nil {
		return "", fmt.Errorf("read migration file: %w", err)
	}

	text := string(content)
	upMarker := "-- +goose Up"
	downMarker := "-- +goose Down"
	upIndex := strings.Index(text, upMarker)
	downIndex := strings.Index(text, downMarker)
	if upIndex < 0 || downIndex < 0 || downIndex <= upIndex {
		return "", errors.New("migration file must contain goose up and down markers")
	}

	switch direction {
	case "up":
		return strings.TrimSpace(text[upIndex+len(upMarker) : downIndex]), nil
	case "down":
		return strings.TrimSpace(text[downIndex+len(downMarker):]), nil
	default:
		return "", errors.New("direction must be up or down")
	}
}
