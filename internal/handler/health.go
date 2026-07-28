package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/plugin/dbresolver"
)

type HealthHandler struct {
	db *gorm.DB
}

func NewHealthHandler(db *gorm.DB) *HealthHandler {
	return &HealthHandler{db: db}
}

func (h *HealthHandler) Register(r gin.IRouter) {
	r.GET("/health/live", h.Live)
	r.GET("/health/ready", h.Ready)
}

// Live godoc
// @Summary Liveness probe
// @Tags health
// @Produce json
// @Success 200 {object} map[string]string
// @Router /health/live [get]
func (h *HealthHandler) Live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Ready godoc
// @Summary Readiness probe
// @Tags health
// @Produce json
// @Success 200 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /health/ready [get]
func (h *HealthHandler) Ready(c *gin.Context) {
	if h.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "database unavailable"})
		return
	}

	ctx := c.Request.Context()
	if err := pingDB(ctx, h.db, dbresolver.Write); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "database primary unavailable"})
		return
	}

	if err := pingDB(ctx, h.db, dbresolver.Read); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "database replica unavailable"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}

func pingDB(ctx context.Context, db *gorm.DB, resolver clause.Expression) error {
	var one int
	return db.Clauses(resolver).WithContext(ctx).Raw("SELECT 1").Scan(&one).Error
}
