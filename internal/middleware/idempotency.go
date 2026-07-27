package middleware

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/domain"
	domainerrors "github.com/shahriyar/arvan/internal/domain/errors"
)

const IdempotencyKeyContextKey = "idempotency_key"

const idempotencyHeader = "Idempotency-Key"

type IdempotencyLookup interface {
	FindByAccountAndKey(ctx context.Context, accountID uuid.UUID, key string) (*domain.IdempotencyRecord, error)
}

// Idempotency validates the header, performs a fast-path lookup, and stores the key in context.
func Idempotency(lookup IdempotencyLookup) gin.HandlerFunc {
	return func(c *gin.Context) {
		rawKey := c.GetHeader(idempotencyHeader)
		if rawKey == "" {
			c.Next()
			return
		}

		if _, err := uuid.Parse(rawKey); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": gin.H{
					"code":    "validation_error",
					"message": domainerrors.ErrInvalidIdempotencyKey.Error(),
				},
			})
			return
		}

		c.Set(IdempotencyKeyContextKey, rawKey)

		accountID, ok := AccountID(c)
		if !ok {
			c.Next()
			return
		}

		record, err := lookup.FindByAccountAndKey(c.Request.Context(), accountID, rawKey)
		if err != nil {
			if errors.Is(err, domainerrors.ErrNotFound) {
				c.Next()
				return
			}
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": gin.H{
					"code":    "internal_error",
					"message": "failed to check idempotency key",
				},
			})
			return
		}

		resp, ok := domain.ParseIdempotencyResponse(record.ResponseSnapshot)
		if !ok {
			c.AbortWithStatusJSON(http.StatusConflict, gin.H{
				"error": gin.H{
					"code":    "idempotency_in_progress",
					"message": domainerrors.ErrIdempotencyInProgress.Error(),
				},
			})
			return
		}

		c.AbortWithStatusJSON(http.StatusAccepted, gin.H{
			"message_id": resp.MessageID,
			"status":     resp.Status,
		})
	}
}

func IdempotencyKey(c *gin.Context) (string, bool) {
	value, ok := c.Get(IdempotencyKeyContextKey)
	if !ok {
		return "", false
	}

	key, ok := value.(string)
	return key, ok
}
