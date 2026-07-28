package swaggerui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shahriyar/arvan/internal/seed"
)

func TestSwaggerInitializerPreauthorizesDemoToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	Register(router)

	req := httptest.NewRequest(http.MethodGet, "/swagger/swagger-initializer.js", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "persistAuthorization: true")
	assert.Contains(t, body, `preauthorizeApiKey("AccountToken", "`+seed.AccountAToken+`")`)
}

func TestSwaggerIndexServed(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	Register(router)

	req := httptest.NewRequest(http.MethodGet, "/swagger/index.html", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, strings.Contains(rec.Header().Get("Content-Type"), "text/html"))
}
