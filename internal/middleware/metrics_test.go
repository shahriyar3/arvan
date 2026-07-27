package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestSMSAcceptMetricsRecordsSendRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	before := counterValue("sms_accept_total", "202")

	r := gin.New()
	v1 := r.Group("/v1")
	v1.Use(SMSAcceptMetrics())
	v1.POST("/sms/send", func(c *gin.Context) {
		c.Status(http.StatusAccepted)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/sms/send", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusAccepted, w.Code)

	after := counterValue("sms_accept_total", "202")
	require.Equal(t, before+1, after)
}

func counterValue(name, status string) float64 {
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return 0
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			if len(m.GetLabel()) == 1 &&
				m.GetLabel()[0].GetName() == "status" &&
				m.GetLabel()[0].GetValue() == status {
				return m.GetCounter().GetValue()
			}
		}
	}
	return 0
}
