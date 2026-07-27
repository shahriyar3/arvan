package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/shahriyar/arvan/internal/broker"
	"github.com/shahriyar/arvan/internal/config"
	"github.com/shahriyar/arvan/internal/observability"
	"github.com/shahriyar/arvan/internal/operator"
	"github.com/shahriyar/arvan/internal/repository"
	"github.com/shahriyar/arvan/internal/worker"
	"github.com/sony/gobreaker"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger := observability.NewLogger(cfg.App.LogLevel)
	slog.SetDefault(logger)
	logger.Info("worker starting")

	ctx := context.Background()
	if cfg.Telemetry.Enabled {
		shutdownTracer, err := observability.InitTracer(ctx, cfg.App.Name+"-worker", cfg.Telemetry.OTLPEndpoint)
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

	smsRepo := repository.NewSMSRepository(db)
	processedRepo := repository.NewProcessedConsumerRepository(db)

	cbClient := operator.NewCircuitBreakerClient(
		operator.NewHTTPClient(cfg.Operator.BaseURL, cfg.Operator.Timeout),
		circuitBreakerSettings(cfg.Operator.CircuitBreaker),
	)

	processor := worker.NewProcessor(db, smsRepo, processedRepo, cbClient, cfg.Worker)
	processor.SetDLQPublisher(rmq)

	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if cfg.Worker.MetricsPort > 0 {
		go serveMetrics(cfg.Worker.MetricsPort)
	}

	go reportCircuitBreakerState(runCtx, cbClient)

	var wg sync.WaitGroup
	queues := []string{broker.QueueExpress, broker.QueueStandard}
	for _, queue := range queues {
		queue := queue
		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.Info("worker consumer started", "queue", queue)
			if err := processor.RunConsumer(runCtx, rmq, queue); err != nil {
				logger.Error("worker consumer stopped", "queue", queue, "error", err)
			}
		}()
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	logger.Info("worker shutting down")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Worker.ShutdownTimeout)
	defer shutdownCancel()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		processor.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-shutdownCtx.Done():
		logger.Warn("worker shutdown timed out with in-flight messages")
	}

	logger.Info("worker stopped")
}

func circuitBreakerSettings(cfg config.CircuitBreakerConfig) gobreaker.Settings {
	threshold := cfg.ReadyToTrip
	if threshold == 0 {
		threshold = 5
	}
	return gobreaker.Settings{
		Name:        "operator",
		MaxRequests: cfg.MaxRequests,
		Interval:    cfg.Interval,
		Timeout:     cfg.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= threshold
		},
	}
}

func serveMetrics(port int) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	addr := fmt.Sprintf(":%d", port)
	slog.Info("worker metrics server starting", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil && err != http.ErrServerClosed {
		slog.Error("metrics server failed", "error", err)
	}
}

func reportCircuitBreakerState(ctx context.Context, client *operator.CircuitBreakerClient) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		observability.RecordCircuitBreakerState("operator", client.State())
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
