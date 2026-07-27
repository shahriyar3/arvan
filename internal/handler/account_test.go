package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/domain"
	domainerrors "github.com/shahriyar/arvan/internal/domain/errors"
	"github.com/shahriyar/arvan/internal/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubAccountService struct {
	topupBalance int64
	topupErr     error
	balance      int64
	balanceErr   error
	ledger       []domain.LedgerEntry
	ledgerErr    error
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

func (s stubAccountService) ListLedger(_ context.Context, _ uuid.UUID, limit int, _ *uuid.UUID) ([]domain.LedgerEntry, error) {
	if s.ledgerErr != nil {
		return nil, s.ledgerErr
	}
	if limit > 0 && limit < len(s.ledger) {
		return s.ledger[:limit], nil
	}
	return s.ledger, nil
}

func newAccountTestRouter(accountID uuid.UUID, svc accountService) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.AccountIDKey, accountID)
		c.Next()
	})
	NewAccountHandler(svc).Register(router.Group("/v1"))
	return router
}

func TestAccountHandlerTopupAndBalance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	accountID := uuid.New()
	router := newAccountTestRouter(accountID, stubAccountService{balance: 500, topupBalance: 1500})

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

func TestAccountHandlerTopupErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	accountID := uuid.New()

	t.Run("invalid body returns 400", func(t *testing.T) {
		router := newAccountTestRouter(accountID, stubAccountService{})
		req := httptest.NewRequest(http.MethodPost, "/v1/account/topup", bytes.NewReader([]byte("{")))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("non-positive amount returns 400", func(t *testing.T) {
		router := newAccountTestRouter(accountID, stubAccountService{
			topupErr: domainerrors.ErrInvalidAmount,
		})
		body, err := json.Marshal(map[string]int64{"amount": -10})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/v1/account/topup", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("missing account returns 404", func(t *testing.T) {
		router := newAccountTestRouter(accountID, stubAccountService{
			topupErr: domainerrors.ErrNotFound,
		})
		body, err := json.Marshal(map[string]int64{"amount": 10})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/v1/account/topup", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestAccountHandlerGetBalanceNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newAccountTestRouter(uuid.New(), stubAccountService{
		balanceErr: domainerrors.ErrNotFound,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/account/balance", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestAccountHandlerListLedger(t *testing.T) {
	gin.SetMode(gin.TestMode)
	accountID := uuid.New()
	entry1 := uuid.New()
	entry2 := uuid.New()
	now := time.Now()

	router := newAccountTestRouter(accountID, stubAccountService{
		ledger: []domain.LedgerEntry{
			{ID: entry1, Delta: 100, Reason: domain.LedgerReasonTopup, CreatedAt: now},
			{ID: entry2, Delta: 50, Reason: domain.LedgerReasonTopup, CreatedAt: now.Add(-time.Minute)},
		},
	})

	t.Run("returns items without next cursor when page incomplete", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/account/ledger?limit=10", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), entry1.String())
		assert.NotContains(t, rec.Body.String(), "next_cursor")
	})

	t.Run("returns next cursor when page is full", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/account/ledger?limit=1", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"next_cursor":"`+entry1.String()+`"`)
	})

	t.Run("invalid cursor uuid returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/account/ledger?cursor=not-a-uuid", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("invalid cursor from service returns 400", func(t *testing.T) {
		router := newAccountTestRouter(accountID, stubAccountService{
			ledgerErr: domainerrors.ErrInvalidCursor,
		})
		cursor := uuid.New()
		req := httptest.NewRequest(http.MethodGet, "/v1/account/ledger?cursor="+cursor.String(), nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestAccountHandlerLedgerIsolation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	accountA := uuid.New()
	accountB := uuid.New()
	otherAccountEntry := uuid.New()

	routerB := newAccountTestRouter(accountB, stubAccountService{
		ledgerErr: domainerrors.ErrInvalidCursor,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/account/ledger?cursor="+otherAccountEntry.String(), nil)
	rec := httptest.NewRecorder()
	routerB.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	routerA := newAccountTestRouter(accountA, stubAccountService{
		ledger: []domain.LedgerEntry{
			{ID: otherAccountEntry, Delta: 100, Reason: domain.LedgerReasonTopup, CreatedAt: time.Now()},
		},
	})
	req = httptest.NewRequest(http.MethodGet, "/v1/account/ledger", nil)
	rec = httptest.NewRecorder()
	routerA.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestParseLimit(t *testing.T) {
	assert.Equal(t, 20, parseLimit(""))
	assert.Equal(t, 20, parseLimit("0"))
	assert.Equal(t, 20, parseLimit("abc"))
	assert.Equal(t, 50, parseLimit("50"))
	assert.Equal(t, maxLedgerLimit, parseLimit("500"))
}
