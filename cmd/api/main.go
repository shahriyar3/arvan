// Package main SMS Gateway HTTP API.
//
// @title           SMS Gateway API
// @version         1.0
// @description     Prepaid SMS Gateway REST API for ArvanCloud challenge.
// @host            localhost:8080
// @BasePath        /
//
// @securityDefinitions.apikey AccountToken
// @in header
// @name X-Account-Token
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	_ "github.com/shahriyar/arvan/api/openapi"
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

	ctx := context.Background()
	var shutdownTracer func(context.Context) error
	if cfg.Telemetry.Enabled {
		endpoint := cfg.Telemetry.OTLPEndpoint
		shutdownTracer, err = observability.InitTracer(ctx, cfg.App.Name, endpoint)
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

	accountRepo := repository.NewAccountRepository(db)
	ledgerRepo := repository.NewLedgerRepository(db)
	smsRepo := repository.NewSMSRepository(db)
	outboxRepo := repository.NewOutboxRepository(db)
	idempotencyRepo := repository.NewIdempotencyRepository(db)
	accountService := service.NewAccountService(accountRepo, ledgerRepo)
	smsService := service.NewSMSService(accountRepo, ledgerRepo, smsRepo, outboxRepo, idempotencyRepo)

	metricsCtx, metricsCancel := context.WithCancel(context.Background())
	defer metricsCancel()
	go observability.NewOutboxMetricsReporter(outboxRepo, 15*time.Second).Run(metricsCtx)

	if cfg.App.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())
	if cfg.Telemetry.Enabled {
		router.Use(otelgin.Middleware(cfg.App.Name))
	}

	handler.NewHealthHandler(db).Register(router)
	handler.RegisterMetrics(router)
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	v1 := router.Group("/v1")
	v1.Use(middleware.AccountToken(accountRepo))
	v1.Use(middleware.SMSAcceptMetrics())
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

	metricsCancel()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()

	logger.Info("api server shutting down")
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("api server shutdown failed", "error", err)
		os.Exit(1)
	}
}
