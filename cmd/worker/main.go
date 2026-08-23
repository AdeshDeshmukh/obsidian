package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"obsidian/internal/config"
	"obsidian/internal/db"
	"obsidian/internal/worker"

	"github.com/google/uuid"
)

func main() {
	slog.Info("Starting Worker Daemon...")

	cfg := config.Load()

	ctx := context.Background()

	// Initial short timeout connection check
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

	workerID := cfg.WorkerID
	if workerID == "" {
		workerID = uuid.New().String()
	}

	concurrency := cfg.Concurrency

	queueID := cfg.QueueID
	if queueID == "" {
		slog.Info("Running in queue-agnostic mode: polling all active non-paused queues")
	} else {
		slog.Info("Worker bound to queue", "queue_id", queueID)
	}

	// Initialize and run worker pool
	wPool := worker.New(pool, workerID, queueID, concurrency)
	wPool.Run(ctx)
}
