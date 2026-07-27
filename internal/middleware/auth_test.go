package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/domain"
	domainerrors "github.com/shahriyar/arvan/internal/domain/errors"
	"github.com/shahriyar/arvan/internal/seed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubAccountLookup struct {
	accounts map[string]domain.Account
	err      error
}

func (s stubAccountLookup) FindByTokenHash(_ context.Context, tokenHash string) (*domain.Account, error) {
	if s.err != nil {
		return nil, s.err
	}

	account, ok := s.accounts[tokenHash]
	if !ok {
		return nil, domainerrors.ErrNotFound
	}
	return &account, nil
}

func TestAccountTokenMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	accountID := uuid.New()
	lookup := stubAccountLookup{
		accounts: map[string]domain.Account{
			seed.TokenHash(seed.AccountAToken): {ID: accountID},
		},
	}

	router := gin.New()
	router.Use(AccountToken(lookup))
	router.GET("/v1/ping", func(c *gin.Context) {
		id, ok := AccountID(c)
		require.True(t, ok)
		c.JSON(http.StatusOK, gin.H{"account_id": id.String()})
	})

	t.Run("missing token returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("invalid token returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
		req.Header.Set("X-Account-Token", "invalid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("valid token sets account id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
		req.Header.Set("X-Account-Token", seed.AccountAToken)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), accountID.String())
	})
}
