package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PoolOptions struct {
	MaxConns int
	MinConns int
}

func Open(ctx context.Context, databaseURL string, options PoolOptions) (*pgxpool.Pool, error) {
	if databaseURL == "" {
		return nil, nil
	}

	cfg, err := poolConfig(ctx, databaseURL, options)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return pool, nil
}

func poolConfig(ctx context.Context, databaseURL string, options PoolOptions) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	if options.MaxConns > 0 {
		cfg.MaxConns = int32(options.MaxConns)
	}
	if options.MinConns > 0 {
		cfg.MinConns = int32(options.MinConns)
	}
	if cfg.MinConns > cfg.MaxConns {
		cfg.MinConns = cfg.MaxConns
	}
	return cfg, nil
}
