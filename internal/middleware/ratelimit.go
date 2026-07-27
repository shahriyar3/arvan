package middleware

import (
	"math"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/shahriyar/arvan/internal/ratelimit"
)

func RateLimit(limiter ratelimit.Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		accountID, ok := AccountID(c)
		if !ok {
			c.Next()
			return
		}

		result, err := limiter.Allow(c.Request.Context(), accountID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": gin.H{
					"code":    "internal_error",
					"message": "rate limit check failed",
				},
			})
			return
		}

		if !result.Allowed {
			seconds := int(math.Ceil(result.RetryAfter.Seconds()))
			if seconds < 1 {
				seconds = 1
			}
			c.Header("Retry-After", strconv.Itoa(seconds))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": gin.H{
					"code":    "rate_limit_exceeded",
					"message": "too many requests",
				},
			})
			return
		}

		c.Next()
	}
}
