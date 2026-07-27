package middleware

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	domainerrors "github.com/shahriyar/arvan/internal/domain/errors"
	"github.com/shahriyar/arvan/internal/domain"
)

const AccountIDKey = "account_id"

const accountTokenHeader = "X-Account-Token"

type AccountLookup interface {
	FindByTokenHash(ctx context.Context, tokenHash string) (*domain.Account, error)
}

func AccountToken(lookup AccountLookup) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader(accountTokenHeader)
		if token == "" {
			writeUnauthorized(c, "missing X-Account-Token header")
			return
		}

		account, err := lookup.FindByTokenHash(c.Request.Context(), domain.HashToken(token))
		if err != nil {
			if err == domainerrors.ErrNotFound {
				writeUnauthorized(c, "invalid account token")
				return
			}
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": gin.H{
					"code":    "internal_error",
					"message": "failed to resolve account",
				},
			})
			return
		}

		c.Set(AccountIDKey, account.ID)
		c.Next()
	}
}

func AccountID(c *gin.Context) (uuid.UUID, bool) {
	value, ok := c.Get(AccountIDKey)
	if !ok {
		return uuid.Nil, false
	}

	accountID, ok := value.(uuid.UUID)
	return accountID, ok
}

func writeUnauthorized(c *gin.Context, message string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"error": gin.H{
			"code":    "unauthorized",
			"message": message,
		},
	})
}
