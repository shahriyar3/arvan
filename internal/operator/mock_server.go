package operator

import (
	"encoding/json"
	"log/slog"
	"math/rand"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/config"
)

type MockServer struct {
	cfg config.MockOperatorConfig
	rng *rand.Rand
}

func NewMockServer(cfg config.MockOperatorConfig) *MockServer {
	return &MockServer{
		cfg: cfg,
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

type mockSendRequest struct {
	MessageID string `json:"message_id" binding:"required"`
	AccountID string `json:"account_id" binding:"required"`
	To        string `json:"to" binding:"required"`
	Body      string `json:"body" binding:"required"`
}

type mockSendResponse struct {
	OperatorRef string `json:"operator_ref"`
}

func (s *MockServer) Register(router *gin.Engine) {
	router.POST("/v1/sms", s.handleSend)
	router.GET("/health/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
}

func (s *MockServer) handleSend(c *gin.Context) {
	var req mockSendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	s.simulateLatency()

	if s.shouldFail() {
		slog.Warn("mock operator simulated failure",
			"message_id", req.MessageID,
			"to", req.To,
		)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "simulated operator failure"})
		return
	}

	slog.Info("mock operator accepted sms",
		"message_id", req.MessageID,
		"account_id", req.AccountID,
		"to", req.To,
	)

	c.JSON(http.StatusOK, mockSendResponse{
		OperatorRef: uuid.NewString(),
	})
}

func (s *MockServer) simulateLatency() {
	min := s.cfg.MinLatency
	max := s.cfg.MaxLatency
	if max < min {
		max = min
	}
	var delay time.Duration
	if max == min {
		delay = min
	} else {
		delta := max - min
		delay = min + time.Duration(s.rng.Int63n(int64(delta)+1))
	}
	time.Sleep(delay)
}

func (s *MockServer) shouldFail() bool {
	if s.cfg.FailureRate <= 0 {
		return false
	}
	if s.cfg.FailureRate >= 100 {
		return true
	}
	return s.rng.Float64()*100 < s.cfg.FailureRate
}

func (s *MockServer) ConfigSummary() string {
	summary, _ := json.Marshal(gin.H{
		"min_latency":  s.cfg.MinLatency.String(),
		"max_latency":  s.cfg.MaxLatency.String(),
		"failure_rate": s.cfg.FailureRate,
	})
	return string(summary)
}
