package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	DatabaseURL       string
	JWTSecret         string
	Port              string
	WorkerID          string
	QueueID           string
	Concurrency       int
	LeaseDuration     time.Duration
	HeartbeatInterval time.Duration
	PollInterval      time.Duration
}

// Load populates the Config struct from environment variables with fallbacks
func Load() Config {
	return Config{
		DatabaseURL:       getEnv("DATABASE_URL", "postgres://postgres:obsidian@localhost:5432/obsidian?sslmode=disable"),
		JWTSecret:         getEnv("JWT_SECRET", "obsidian-secret-key-change-in-prod"),
		Port:              getEnv("PORT", "8080"),
		WorkerID:          getEnv("WORKER_ID", ""),
		QueueID:           getEnv("QUEUE_ID", ""),
		Concurrency:       getEnvInt("CONCURRENCY", 5),
		LeaseDuration:     getEnvDuration("LEASE_DURATION_SECONDS", 30) * time.Second,
		HeartbeatInterval: getEnvDuration("HEARTBEAT_INTERVAL_SECONDS", 10) * time.Second,
		PollInterval:      getEnvDuration("POLL_INTERVAL_SECONDS", 2) * time.Second,
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		if val, err := strconv.Atoi(value); err == nil && val > 0 {
			return val
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback int64) time.Duration {
	if value, ok := os.LookupEnv(key); ok {
		if val, err := strconv.ParseInt(value, 10, 64); err == nil && val > 0 {
			return time.Duration(val)
		}
	}
	return time.Duration(fallback)
}
