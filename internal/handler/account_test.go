package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/domain"
	"github.com/shahriyar/arvan/internal/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubAccountService struct {
	topupBalance int64
	topupErr     error
	balance      int64
	balanceErr   error
}

func (s stubAccountService) Topup(_ context.Context, _ uuid.UUID, amount int64) (int64, error) {
	if s.topupErr != nil {
		return 0, s.topupErr
	}
	if s.topupBalance == 0 {
		return amount, nil
	}
	return s.topupBalance, nil
}

func (s stubAccountService) GetBalance(_ context.Context, _ uuid.UUID) (int64, error) {
	if s.balanceErr != nil {
		return 0, s.balanceErr
	}
	return s.balance, nil
}

func (s stubAccountService) ListLedger(_ context.Context, _ uuid.UUID, _ int, _ *uuid.UUID) ([]domain.LedgerEntry, error) {
	return nil, nil
}

func TestAccountHandlerTopupAndBalance(t *testing.T) {
	gin.SetMode(gin.TestMode)

	accountID := uuid.New()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.AccountIDKey, accountID)
		c.Next()
	})

	handler := &AccountHandler{accounts: stubAccountService{balance: 500, topupBalance: 1500}}
	handler.Register(router.Group("/v1"))

	t.Run("topup returns balance", func(t *testing.T) {
		body, err := json.Marshal(map[string]int64{"amount": 1000})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/v1/account/topup", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"balance":1500`)
	})

	t.Run("balance returns current balance", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/account/balance", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"balance":500`)
	})
}
