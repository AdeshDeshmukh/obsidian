package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"obsidian/internal/api"
	"obsidian/internal/api/middleware"
	"obsidian/internal/config"
	"obsidian/internal/db"
	"obsidian/internal/metrics"
	"obsidian/internal/telemetry"
	"obsidian/internal/worker"

	"github.com/google/uuid"
)

func main() {
	slog.Info("Starting API Server...")

	cfg := config.Load()
	middleware.InitJWTSecret(cfg.JWTSecret)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	shutdown, err := telemetry.InitTracer(ctx, "obsidian-api")
	if err != nil {
		slog.Warn("Failed to init tracer", "error", err)
	} else {
		defer shutdown(ctx)
	}

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := db.AutoMigrate(ctx, pool); err != nil {
		slog.Error("Failed to initialize database schema", "error", err)
		os.Exit(1)
	}

	metrics.StartMetricsScraper(context.Background(), pool)

	// If running in single-service mode (e.g. Render Free Tier), start the embedded worker pool
	if os.Getenv("EMBED_WORKER") != "false" {
		workerID := uuid.New().String()
		concurrency := cfg.Concurrency
		if concurrency <= 0 {
			concurrency = 10
		}
		wPool := worker.New(pool, workerID, "", concurrency)
		go wPool.Run(context.Background())
		slog.Info("Embedded worker pool started in API container", "worker_id", workerID, "concurrency", concurrency)
	}

	router := api.NewRouter(pool)

	slog.Info("HTTP Server listening", "port", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, router); err != nil {
		slog.Error("HTTP server failed", "error", err)
		os.Exit(1)
	}
}
