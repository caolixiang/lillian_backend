package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("HOST", "")
	t.Setenv("PORT", "")
	t.Setenv("UPSTREAM_TIMEOUT_SECONDS", "")

	cfg, err := Load("1.2.3")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ListenAddr != "0.0.0.0:8787" {
		t.Fatalf("ListenAddr = %q", cfg.ListenAddr)
	}
	if cfg.Version != "1.2.3" {
		t.Fatalf("Version = %q", cfg.Version)
	}
	if cfg.PublicAPIBaseURL != "" {
		t.Fatalf("PublicAPIBaseURL = %q", cfg.PublicAPIBaseURL)
	}
	if cfg.CORSOrigin != "*" {
		t.Fatalf("CORSOrigin = %q", cfg.CORSOrigin)
	}
	if cfg.UpstreamTimeout != 10*time.Minute {
		t.Fatalf("UpstreamTimeout = %s", cfg.UpstreamTimeout)
	}
	if !cfg.AutoMigrate {
		t.Fatalf("AutoMigrate should default to true")
	}
}

func TestLoadRuntimeValues(t *testing.T) {
	t.Setenv("HOST", "127.0.0.1")
	t.Setenv("PORT", "9000")
	t.Setenv("UPSTREAM_TIMEOUT_SECONDS", "600")
	t.Setenv("TASK_WORKER_CONCURRENCY", "6")
	t.Setenv("DB_POOL_MAX_CONNS", "32")
	t.Setenv("DB_POOL_MIN_CONNS", "4")
	t.Setenv("S3_FORCE_PATH_STYLE", "false")
	t.Setenv("EPUSDT_BASE_URL", "https://pay.example.com")
	t.Setenv("EPUSDT_PID", "merchant-1")
	t.Setenv("EPUSDT_SECRET_KEY", "secret-1")

	cfg, err := Load("dev")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ListenAddr != "127.0.0.1:9000" {
		t.Fatalf("ListenAddr = %q", cfg.ListenAddr)
	}
	if cfg.UpstreamTimeout != 600*time.Second {
		t.Fatalf("UpstreamTimeout = %s", cfg.UpstreamTimeout)
	}
	if cfg.TaskWorkerConcurrency != 6 {
		t.Fatalf("TaskWorkerConcurrency = %d", cfg.TaskWorkerConcurrency)
	}
	if cfg.Database.PoolMaxConns != 32 {
		t.Fatalf("Database.PoolMaxConns = %d", cfg.Database.PoolMaxConns)
	}
	if cfg.Database.PoolMinConns != 4 {
		t.Fatalf("Database.PoolMinConns = %d", cfg.Database.PoolMinConns)
	}
	if cfg.Storage.ForcePathStyle {
		t.Fatalf("ForcePathStyle should be false")
	}
	if cfg.EPUSDT.BaseURL != "https://pay.example.com" {
		t.Fatalf("EPUSDT.BaseURL = %q", cfg.EPUSDT.BaseURL)
	}
	if cfg.EPUSDT.PID != "merchant-1" {
		t.Fatalf("EPUSDT.PID = %q", cfg.EPUSDT.PID)
	}
	if cfg.EPUSDT.SecretKey != "secret-1" {
		t.Fatalf("EPUSDT.SecretKey = %q", cfg.EPUSDT.SecretKey)
	}
	if cfg.EPUSDT.Currency != "USDT" || cfg.EPUSDT.Token != "USDT" || cfg.EPUSDT.Network != "TRON" {
		t.Fatalf("EPUSDT asset = %s/%s/%s", cfg.EPUSDT.Currency, cfg.EPUSDT.Token, cfg.EPUSDT.Network)
	}
}
