package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestHealthEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	NewHealthHandler().Register(router)

	for _, tc := range []struct {
		path     string
		contains string
	}{
		{"/health/live", "ok"},
		{"/health/ready", "ready"},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), tc.contains)
	}
}
