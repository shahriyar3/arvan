package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/shahriyar/arvan/internal/config"
	"github.com/shahriyar/arvan/internal/observability"
	"github.com/shahriyar/arvan/internal/operator"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger := observability.NewLogger(cfg.App.LogLevel)
	slog.SetDefault(logger)

	if cfg.App.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	mock := operator.NewMockServer(cfg.MockOp)
	router := gin.New()
	router.Use(gin.Recovery())
	mock.Register(router)

	server := &http.Server{
		Addr:         cfg.MockOp.Addr(),
		Handler:      router,
		ReadTimeout:  cfg.MockOp.ReadTimeout,
		WriteTimeout: cfg.MockOp.WriteTimeout,
	}

	go func() {
		logger.Info("mock operator starting",
			"addr", cfg.MockOp.Addr(),
			"config", mock.ConfigSummary(),
		)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("mock operator failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()

	logger.Info("mock operator shutting down")
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("mock operator shutdown failed", "error", err)
		os.Exit(1)
	}
}
