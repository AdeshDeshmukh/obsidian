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
)

func main() {
	slog.Info("Starting API Server...")

	cfg := config.Load()
	middleware.InitJWTSecret(cfg.JWTSecret)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

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

	router := api.NewRouter(pool)

	slog.Info("HTTP Server listening", "port", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, router); err != nil {
		slog.Error("HTTP server failed", "error", err)
		os.Exit(1)
	}
}
