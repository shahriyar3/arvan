package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/shahriyar/arvan/internal/broker"
	"github.com/shahriyar/arvan/internal/config"
	"github.com/shahriyar/arvan/internal/observability"
	"github.com/shahriyar/arvan/internal/operator"
	"github.com/shahriyar/arvan/internal/repository"
	"github.com/shahriyar/arvan/internal/worker"
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
	op := operator.NewHTTPClient(cfg.Operator.BaseURL, cfg.Operator.Timeout)
	processor := worker.NewProcessor(db, smsRepo, processedRepo, op, cfg.Worker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	queues := []string{broker.QueueExpress, broker.QueueStandard}
	for _, queue := range queues {
		queue := queue
		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.Info("worker consumer started", "queue", queue)
			if err := processor.RunConsumer(ctx, rmq, queue); err != nil {
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
