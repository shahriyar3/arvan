package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/ratelimit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubLimiter struct {
	allowed bool
	err     error
	calls   int
	lastID  uuid.UUID
}

func (s *stubLimiter) Allow(_ context.Context, accountID uuid.UUID) (ratelimit.AllowResult, error) {
	s.calls++
	s.lastID = accountID
	if s.err != nil {
		return ratelimit.AllowResult{}, s.err
	}
	if s.allowed {
		return ratelimit.AllowResult{Allowed: true}, nil
	}
	return ratelimit.AllowResult{Allowed: false}, nil
}

func TestRateLimitMiddlewareAllows(t *testing.T) {
	gin.SetMode(gin.TestMode)

	accountID := uuid.New()
	limiter := &stubLimiter{allowed: true}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(AccountIDKey, accountID)
		c.Next()
	})
	router.Use(RateLimit(limiter))
	router.GET("/v1/ping", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, limiter.calls)
	assert.Equal(t, accountID, limiter.lastID)
}

func TestRateLimitMiddlewareBlocks(t *testing.T) {
	gin.SetMode(gin.TestMode)

	limiter := &stubLimiter{allowed: false}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(AccountIDKey, uuid.New())
		c.Next()
	})
	router.Use(RateLimit(limiter))
	router.GET("/v1/ping", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
}

func TestRateLimitMiddlewareRedisError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	limiter := &stubLimiter{err: errors.New("redis down")}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(AccountIDKey, uuid.New())
		c.Next()
	})
	router.Use(RateLimit(limiter))
	router.GET("/v1/ping", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
}
