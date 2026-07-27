package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/domain"
	domainerrors "github.com/shahriyar/arvan/internal/domain/errors"
	"github.com/shahriyar/arvan/internal/middleware"
)

type accountService interface {
	Topup(ctx context.Context, accountID uuid.UUID, amount int64) (int64, error)
	GetBalance(ctx context.Context, accountID uuid.UUID) (int64, error)
	ListLedger(ctx context.Context, accountID uuid.UUID, limit int, cursor *uuid.UUID) ([]domain.LedgerEntry, error)
}

type AccountHandler struct {
	accounts accountService
}

func NewAccountHandler(accounts accountService) *AccountHandler {
	return &AccountHandler{accounts: accounts}
}

func (h *AccountHandler) Register(r gin.IRouter) {
	r.POST("/account/topup", h.Topup)
	r.GET("/account/balance", h.GetBalance)
	r.GET("/account/ledger", h.ListLedger)
}

type topupRequest struct {
	Amount int64 `json:"amount" binding:"required"`
}

func (h *AccountHandler) Topup(c *gin.Context) {
	accountID, ok := middleware.AccountID(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "missing account context")
		return
	}

	var req topupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "validation_error", "invalid request body")
		return
	}

	balance, err := h.accounts.Topup(c.Request.Context(), accountID, req.Amount)
	if err != nil {
		switch {
		case errors.Is(err, domainerrors.ErrInvalidAmount):
			writeError(c, http.StatusBadRequest, "validation_error", "amount must be positive")
		case errors.Is(err, domainerrors.ErrNotFound):
			writeError(c, http.StatusNotFound, "not_found", "account not found")
		default:
			writeError(c, http.StatusInternalServerError, "internal_error", "failed to top up account")
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"balance": balance})
}

func (h *AccountHandler) GetBalance(c *gin.Context) {
	accountID, ok := middleware.AccountID(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "missing account context")
		return
	}

	balance, err := h.accounts.GetBalance(c.Request.Context(), accountID)
	if err != nil {
		if errors.Is(err, domainerrors.ErrNotFound) {
			writeError(c, http.StatusNotFound, "not_found", "account not found")
			return
		}
		writeError(c, http.StatusInternalServerError, "internal_error", "failed to get balance")
		return
	}

	c.JSON(http.StatusOK, gin.H{"balance": balance})
}

func (h *AccountHandler) ListLedger(c *gin.Context) {
	accountID, ok := middleware.AccountID(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "missing account context")
		return
	}

	limit := parseLimit(c.Query("limit"))

	var cursor *uuid.UUID
	if rawCursor := c.Query("cursor"); rawCursor != "" {
		parsed, err := uuid.Parse(rawCursor)
		if err != nil {
			writeError(c, http.StatusBadRequest, "validation_error", "invalid cursor")
			return
		}
		cursor = &parsed
	}

	entries, err := h.accounts.ListLedger(c.Request.Context(), accountID, limit, cursor)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "failed to list ledger")
		return
	}

	items := make([]gin.H, len(entries))
	for i, entry := range entries {
		item := gin.H{
			"id":         entry.ID.String(),
			"delta":      entry.Delta,
			"reason":     entry.Reason,
			"created_at": entry.CreatedAt,
		}
		if entry.RefID != nil {
			item["ref_id"] = entry.RefID.String()
		}
		items[i] = item
	}

	c.JSON(http.StatusOK, gin.H{"items": items})
}

func parseLimit(raw string) int {
	if raw == "" {
		return 20
	}

	var limit int
	if _, err := fmt.Sscanf(raw, "%d", &limit); err != nil || limit <= 0 {
		return 20
	}
	return limit
}

func writeError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	})
}
