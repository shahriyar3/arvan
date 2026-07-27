package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shahriyar/arvan/internal/observability"
)

func SMSAcceptMetrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != "POST" || c.FullPath() != "/v1/sms/send" {
			c.Next()
			return
		}

		start := time.Now()
		c.Next()

		status := strconv.Itoa(c.Writer.Status())
		observability.RecordSMSAccept(status, time.Since(start).Seconds())
	}
}
