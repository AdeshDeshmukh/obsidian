package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"obsidian/internal/config"
	"obsidian/internal/db"
	"obsidian/internal/scheduler"
)

func main() {
	slog.Info("Starting Scheduler Daemon...")

	cfg := config.Load()

	ctx := context.Background()

	startupCtx, startupCancel := context.WithTimeout(ctx, 10*time.Second)
	pool, err := db.NewPool(startupCtx, cfg.DatabaseURL)
	startupCancel()
	if err != nil {
		slog.Error("Database connection failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := db.AutoMigrate(ctx, pool); err != nil {
		slog.Error("Failed to initialize database schema", "error", err)
		os.Exit(1)
	}

	// Poll cron tables and expired leases using config interval
	scheduler.RunScheduler(ctx, pool, cfg.PollInterval)
}
