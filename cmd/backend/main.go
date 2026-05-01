package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/CookSleep/lillian_backend/internal/app"
	"github.com/CookSleep/lillian_backend/internal/config"
)

var version = "dev"

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.LUTC)

	cfg, err := config.Load(version)
	if err != nil {
		logger.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	application, err := app.New(ctx, cfg, logger)
	if err != nil {
		logger.Fatal(err)
	}
	defer application.Close()

	if err := application.Run(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatal(err)
	}
}
