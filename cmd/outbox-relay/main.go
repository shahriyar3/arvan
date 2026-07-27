package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/shahriyar/arvan/internal/config"
	"github.com/shahriyar/arvan/internal/observability"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger := observability.NewLogger(cfg.App.LogLevel)
	slog.SetDefault(logger)
	logger.Info("outbox relay starting")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	logger.Info("outbox relay shutting down")
}
