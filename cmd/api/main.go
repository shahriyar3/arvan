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
// @description Demo token after `make seed`: demo-token-account-a
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
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	_ "github.com/shahriyar/arvan/api/openapi"
	"github.com/shahriyar/arvan/internal/config"
	"github.com/shahriyar/arvan/internal/handler"
	"github.com/shahriyar/arvan/internal/idempotency"
	"github.com/shahriyar/arvan/internal/middleware"
	"github.com/shahriyar/arvan/internal/observability"
	"github.com/shahriyar/arvan/internal/ratelimit"
	appredis "github.com/shahriyar/arvan/internal/redis"
	"github.com/shahriyar/arvan/internal/repository"
	"github.com/shahriyar/arvan/internal/service"
	"github.com/shahriyar/arvan/internal/swaggerui"
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

	var idempotencyCache idempotency.ResponseCache
	var redisClient *redis.Client
	if cfg.RateLimit.Enabled || cfg.Idempotency.CacheEnabled {
		redisClient = appredis.NewClient(cfg.Redis)
		defer func() {
			if err := redisClient.Close(); err != nil {
				logger.Warn("failed to close redis client", "error", err)
			}
		}()
	}
	if cfg.Idempotency.CacheEnabled && redisClient != nil {
		idempotencyCache = idempotency.NewRedisResponseCache(redisClient, idempotency.RedisCacheConfig{
			TTL:       cfg.Idempotency.CacheTTL,
			KeyPrefix: cfg.Idempotency.KeyPrefix,
		})
	}

	smsService := service.NewSMSServiceWithCache(
		accountRepo, ledgerRepo, smsRepo, outboxRepo, idempotencyRepo, idempotencyCache,
	)
	idempotencyLookup := idempotency.NewCompositeLookup(idempotencyRepo, idempotencyCache)

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
	swaggerui.Register(router)

	v1 := router.Group("/v1")
	v1.Use(middleware.AccountToken(accountRepo))
	v1.Use(middleware.SMSAcceptMetrics())
	if cfg.RateLimit.Enabled {
		if redisClient == nil {
			logger.Error("rate limit enabled but redis client is unavailable")
			os.Exit(1)
		}
		limiter := ratelimit.NewRedisLimiter(redisClient, ratelimit.RedisConfig{
			Window:    cfg.RateLimit.Window,
			Limit:     cfg.RateLimit.Limit,
			KeyPrefix: cfg.RateLimit.KeyPrefix,
		})
		v1.Use(middleware.RateLimit(limiter))
	}
	handler.NewAccountHandler(accountService).Register(v1)
	handler.NewSMSHandler(smsService).Register(v1, middleware.Idempotency(idempotencyLookup))

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
