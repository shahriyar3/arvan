package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
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

	ctx := context.Background()
	if cfg.Telemetry.Enabled {
		shutdownTracer, err := observability.InitTracer(ctx, cfg.App.Name+"-outbox-relay", cfg.Telemetry.OTLPEndpoint)
		if err != nil {
			logger.Error("failed to init tracer", "error", err)
			os.Exit(1)
		}
		defer func() {
			if err := observability.ShutdownGracefully(shutdownTracer, 5*time.Second); err != nil {
				logger.Warn("tracer shutdown failed", "error", err)
			}
		}()
	}

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

	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if cfg.OutboxRelay.MetricsPort > 0 {
		go serveMetrics(cfg.OutboxRelay.MetricsPort)
	}

	relay.Start(runCtx)

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

func serveMetrics(port int) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	addr := fmt.Sprintf(":%d", port)
	slog.Info("outbox relay metrics server starting", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil && err != http.ErrServerClosed {
		slog.Error("outbox relay metrics server failed", "error", err)
	}
}
