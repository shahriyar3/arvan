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
	"github.com/shahriyar/arvan/internal/handler"
	"github.com/shahriyar/arvan/internal/middleware"
	"github.com/shahriyar/arvan/internal/observability"
	appredis "github.com/shahriyar/arvan/internal/redis"
	"github.com/shahriyar/arvan/internal/ratelimit"
	"github.com/shahriyar/arvan/internal/repository"
	"github.com/shahriyar/arvan/internal/service"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger := observability.NewLogger(cfg.App.LogLevel)
	slog.SetDefault(logger)

	db, err := repository.NewDB(cfg.Database)
	if err != nil {
		logger.Error("failed to connect database", "error", err)
		os.Exit(1)
	}

	accountRepo := repository.NewAccountRepository(db)
	ledgerRepo := repository.NewLedgerRepository(db)
	smsRepo := repository.NewSMSRepository(db)
	outboxRepo := repository.NewOutboxRepository(db)
	idempotencyRepo := repository.NewIdempotencyRepository(db)
	accountService := service.NewAccountService(accountRepo, ledgerRepo)
	smsService := service.NewSMSService(accountRepo, ledgerRepo, smsRepo, outboxRepo, idempotencyRepo)

	if cfg.App.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())

	handler.NewHealthHandler(db).Register(router)

	v1 := router.Group("/v1")
	v1.Use(middleware.AccountToken(accountRepo))
	if cfg.RateLimit.Enabled {
		redisClient := appredis.NewClient(cfg.Redis)
		defer func() {
			if err := redisClient.Close(); err != nil {
				logger.Warn("failed to close redis client", "error", err)
			}
		}()
		limiter := ratelimit.NewRedisLimiter(redisClient, ratelimit.RedisConfig{
			Window:    cfg.RateLimit.Window,
			Limit:     cfg.RateLimit.Limit,
			KeyPrefix: cfg.RateLimit.KeyPrefix,
		})
		v1.Use(middleware.RateLimit(limiter))
	}
	handler.NewAccountHandler(accountService).Register(v1)
	handler.NewSMSHandler(smsService).Register(v1, middleware.Idempotency(idempotencyRepo))

	server := &http.Server{
		Addr:         cfg.HTTP.Addr(),
		Handler:      router,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
	}

	go func() {
		logger.Info("api server starting", "addr", cfg.HTTP.Addr())
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("api server failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()

	logger.Info("api server shutting down")
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("api server shutdown failed", "error", err)
		os.Exit(1)
	}
}
