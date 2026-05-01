package app

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/CookSleep/lillian_backend/internal/config"
	"github.com/CookSleep/lillian_backend/internal/db"
	"github.com/CookSleep/lillian_backend/internal/httpapi"
	"github.com/CookSleep/lillian_backend/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

type App struct {
	cfg        config.Config
	logger     *log.Logger
	db         *pgxpool.Pool
	httpServer *http.Server
}

func New(ctx context.Context, cfg config.Config, logger *log.Logger) (*App, error) {
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	if cfg.AutoMigrate {
		if err := db.ApplyMigrations(ctx, pool, cfg.MigrationsDir); err != nil {
			if pool != nil {
				pool.Close()
			}
			return nil, err
		}
	}

	objectStore, err := storage.NewS3Store(ctx, cfg.Storage)
	if err != nil {
		if pool != nil {
			pool.Close()
		}
		return nil, err
	}

	api := httpapi.New(cfg, pool, objectStore, logger)
	return &App{
		cfg:    cfg,
		logger: logger,
		db:     pool,
		httpServer: &http.Server{
			Addr:              cfg.ListenAddr,
			Handler:           api.Handler(),
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       120 * time.Second,
		},
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	errs := make(chan error, 1)
	go func() {
		a.logger.Printf("listening on %s", a.cfg.ListenAddr)
		errs <- a.httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return a.httpServer.Shutdown(shutdownCtx)
	case err := <-errs:
		return err
	}
}

func (a *App) Close() {
	if a.db != nil {
		a.db.Close()
	}
}
