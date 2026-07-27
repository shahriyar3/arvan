package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/domain"
	domainerrors "github.com/shahriyar/arvan/internal/domain/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubIdempotencyLookup struct {
	record *domain.IdempotencyRecord
	err    error
}

func (s stubIdempotencyLookup) FindByAccountAndKey(_ context.Context, _ uuid.UUID, _ string) (*domain.IdempotencyRecord, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.record, nil
}

func TestIdempotencyMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	accountID := uuid.New()
	messageID := uuid.New()

	t.Run("passes through when header is absent", func(t *testing.T) {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set(AccountIDKey, accountID)
			c.Next()
		})
		router.POST("/send", Idempotency(stubIdempotencyLookup{}), func(c *gin.Context) {
			c.Status(http.StatusCreated)
		})

		req := httptest.NewRequest(http.MethodPost, "/send", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusCreated, rec.Code)
	})

	t.Run("rejects invalid uuid header", func(t *testing.T) {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set(AccountIDKey, accountID)
			c.Next()
		})
		router.POST("/send", Idempotency(stubIdempotencyLookup{}), func(c *gin.Context) {
			c.Status(http.StatusCreated)
		})

		req := httptest.NewRequest(http.MethodPost, "/send", nil)
		req.Header.Set(idempotencyHeader, "not-a-uuid")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("returns cached response on fast path", func(t *testing.T) {
		snapshot, err := json.Marshal(domain.IdempotencyResponse{
			MessageID: messageID.String(),
			Status:    domain.SMSStatusAccepted,
		})
		require.NoError(t, err)

		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set(AccountIDKey, accountID)
			c.Next()
		})
		key := uuid.New().String()
		router.POST("/send", Idempotency(stubIdempotencyLookup{
			record: &domain.IdempotencyRecord{
				AccountID:        accountID,
				Key:              key,
				ResponseSnapshot: snapshot,
			},
		}), func(c *gin.Context) {
			c.Status(http.StatusCreated)
		})

		req := httptest.NewRequest(http.MethodPost, "/send", nil)
		req.Header.Set(idempotencyHeader, key)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		require.Equal(t, http.StatusAccepted, rec.Code)
		assert.Contains(t, rec.Body.String(), messageID.String())
	})

	t.Run("returns conflict when snapshot is still pending", func(t *testing.T) {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set(AccountIDKey, accountID)
			c.Next()
		})
		key := uuid.New().String()
		router.POST("/send", Idempotency(stubIdempotencyLookup{
			record: &domain.IdempotencyRecord{
				AccountID:        accountID,
				Key:              key,
				ResponseSnapshot: []byte("{}"),
			},
		}), func(c *gin.Context) {
			c.Status(http.StatusCreated)
		})

		req := httptest.NewRequest(http.MethodPost, "/send", nil)
		req.Header.Set(idempotencyHeader, key)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusConflict, rec.Code)
	})

	t.Run("continues when key is not found", func(t *testing.T) {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set(AccountIDKey, accountID)
			c.Next()
		})
		key := uuid.New().String()
		router.POST("/send", Idempotency(stubIdempotencyLookup{
			err: domainerrors.ErrNotFound,
		}), func(c *gin.Context) {
			got, ok := IdempotencyKey(c)
			assert.True(t, ok)
			assert.Equal(t, key, got)
			c.Status(http.StatusCreated)
		})

		req := httptest.NewRequest(http.MethodPost, "/send", nil)
		req.Header.Set(idempotencyHeader, key)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusCreated, rec.Code)
	})
}
