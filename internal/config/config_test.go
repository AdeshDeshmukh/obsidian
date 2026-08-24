package config_test

import (
	"os"
	"testing"
	"time"

	"obsidian/internal/config"
)

func TestConfigDefaults(t *testing.T) {
	// Clean environment
	os.Unsetenv("PORT")
	os.Unsetenv("CONCURRENCY")
	os.Unsetenv("POLL_INTERVAL_SECONDS")
	os.Unsetenv("LEASE_DURATION_SECONDS")
	os.Unsetenv("HEARTBEAT_INTERVAL_SECONDS")

	cfg := config.Load()

	if cfg.Port != "8080" {
		t.Errorf("expected default port 8080, got %s", cfg.Port)
	}
	if cfg.Concurrency != 5 {
		t.Errorf("expected default concurrency 5, got %d", cfg.Concurrency)
	}
	if cfg.PollInterval != 2*time.Second {
		t.Errorf("expected default poll interval 2s, got %v", cfg.PollInterval)
	}
	if cfg.LeaseDuration != 30*time.Second {
		t.Errorf("expected default lease duration 30s, got %v", cfg.LeaseDuration)
	}
	if cfg.HeartbeatInterval != 10*time.Second {
		t.Errorf("expected default heartbeat interval 10s, got %v", cfg.HeartbeatInterval)
	}
}

func TestConfigEnvOverrides(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("CONCURRENCY", "12")
	t.Setenv("POLL_INTERVAL_SECONDS", "5")
	t.Setenv("LEASE_DURATION_SECONDS", "45")

	cfg := config.Load()

	if cfg.Port != "9090" {
		t.Errorf("expected overridden port 9090, got %s", cfg.Port)
	}
	if cfg.Concurrency != 12 {
		t.Errorf("expected overridden concurrency 12, got %d", cfg.Concurrency)
	}
	if cfg.PollInterval != 5*time.Second {
		t.Errorf("expected overridden poll interval 5s, got %v", cfg.PollInterval)
	}
	if cfg.LeaseDuration != 45*time.Second {
		t.Errorf("expected overridden lease duration 45s, got %v", cfg.LeaseDuration)
	}
}
