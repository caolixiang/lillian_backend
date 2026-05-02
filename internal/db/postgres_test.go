package db

import (
	"context"
	"testing"
)

func TestPoolConfigAppliesExplicitConnectionLimits(t *testing.T) {
	cfg, err := poolConfig(context.Background(), "postgres://user:pass@example.com:5432/app", PoolOptions{
		MaxConns: 32,
		MinConns: 4,
	})
	if err != nil {
		t.Fatalf("poolConfig() error = %v", err)
	}
	if cfg.MaxConns != 32 {
		t.Fatalf("MaxConns = %d", cfg.MaxConns)
	}
	if cfg.MinConns != 4 {
		t.Fatalf("MinConns = %d", cfg.MinConns)
	}
}
