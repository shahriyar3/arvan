package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/shahriyar/arvan/internal/repository"
	"github.com/stretchr/testify/require"
)

func TestHealthEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("live always ok", func(t *testing.T) {
		router := gin.New()
		NewHealthHandler(nil).Register(router)

		req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), "ok")
	})

	t.Run("ready unavailable without database", func(t *testing.T) {
		router := gin.New()
		NewHealthHandler(nil).Register(router)

		req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	})

	t.Run("ready ok when primary and replica respond", func(t *testing.T) {
		db := repository.NewTestDB(t)

		router := gin.New()
		NewHealthHandler(db).Register(router)

		req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), "ready")
	})
}
