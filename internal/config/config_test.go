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
	t.Setenv("S3_FORCE_PATH_STYLE", "false")

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
	if cfg.Storage.ForcePathStyle {
		t.Fatalf("ForcePathStyle should be false")
	}
}
