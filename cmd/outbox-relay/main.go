package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/shahriyar/arvan/internal/broker"
	"github.com/shahriyar/arvan/internal/config"
	"github.com/shahriyar/arvan/internal/observability"
	"github.com/shahriyar/arvan/internal/outbox"
	"github.com/shahriyar/arvan/internal/repository"
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

	db, err := repository.NewDB(cfg.Database)
	if err != nil {
		logger.Error("failed to connect database", "error", err)
		os.Exit(1)
	}

	rmq, err := broker.Connect(cfg.RabbitMQ.URL)
	if err != nil {
		logger.Error("failed to connect rabbitmq", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := rmq.Close(); err != nil {
			logger.Error("failed to close rabbitmq", "error", err)
		}
	}()

	outboxRepo := repository.NewOutboxRepository(db)
	relay := outbox.NewRelay(outboxRepo, rmq, cfg.OutboxRelay)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	relay.Start(ctx)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	logger.Info("outbox relay shutting down")
	relay.Stop()
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.OutboxRelay.ShutdownTimeout)
	defer shutdownCancel()

	done := make(chan struct{})
	go func() {
		relay.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-shutdownCtx.Done():
		logger.Warn("outbox relay shutdown timed out")
	}

	logger.Info("outbox relay stopped")
}
