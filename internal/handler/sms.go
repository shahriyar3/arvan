package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/domain"
	domainerrors "github.com/shahriyar/arvan/internal/domain/errors"
	"github.com/shahriyar/arvan/internal/middleware"
)

type smsService interface {
	Send(ctx context.Context, accountID uuid.UUID, input domain.SendSMSInput) (domain.SendSMSResult, error)
	Get(ctx context.Context, accountID, messageID uuid.UUID) (domain.SMSMessage, error)
	List(ctx context.Context, accountID uuid.UUID, limit int, cursor *uuid.UUID) ([]domain.SMSMessage, error)
}

type SMSHandler struct {
	sms smsService
}

func NewSMSHandler(sms smsService) *SMSHandler {
	return &SMSHandler{sms: sms}
}

func (h *SMSHandler) Register(r gin.IRouter, idempotency gin.HandlerFunc) {
	r.POST("/sms/send", idempotency, h.Send)
	r.GET("/sms", h.List)
	r.GET("/sms/:id", h.Get)
}

type sendSMSRequest struct {
	To          string `json:"to" binding:"required"`
	Body        string `json:"body" binding:"required"`
	MessageType string `json:"message_type" binding:"required"`
}

func (h *SMSHandler) Send(c *gin.Context) {
	accountID, ok := middleware.AccountID(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "missing account context")
		return
	}

	var req sendSMSRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "validation_error", "invalid request body")
		return
	}

	result, err := h.sms.Send(c.Request.Context(), accountID, domain.SendSMSInput{
		To:          req.To,
		Body:        req.Body,
		MessageType: req.MessageType,
		IdempotencyKey: idempotencyKeyPtr(c),
	})
	if err != nil {
		switch {
		case errors.Is(err, domainerrors.ErrInvalidRequest):
			writeError(c, http.StatusBadRequest, "validation_error", "to and body are required")
		case errors.Is(err, domainerrors.ErrInvalidMessageType):
			writeError(c, http.StatusBadRequest, "validation_error", "message_type must be standard or express")
		case errors.Is(err, domainerrors.ErrInvalidPhoneNumber):
			writeError(c, http.StatusBadRequest, "validation_error", "to must be a valid E.164 phone number")
		case errors.Is(err, domainerrors.ErrMessageTooLong):
			writeError(c, http.StatusBadRequest, "validation_error", "body exceeds single-page SMS limit")
		case errors.Is(err, domainerrors.ErrInsufficientBalance):
			writeError(c, http.StatusPaymentRequired, "insufficient_balance", "insufficient balance")
		case errors.Is(err, domainerrors.ErrIdempotencyInProgress):
			writeError(c, http.StatusConflict, "idempotency_in_progress", "request with this idempotency key is still in progress; retry later")
		case errors.Is(err, domainerrors.ErrNotFound):
			writeError(c, http.StatusNotFound, "not_found", "account not found")
		default:
			writeError(c, http.StatusInternalServerError, "internal_error", "failed to send sms")
		}
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message_id": result.MessageID.String(),
		"status":     result.Status,
	})
}

func idempotencyKeyPtr(c *gin.Context) *string {
	key, ok := middleware.IdempotencyKey(c)
	if !ok {
		return nil
	}
	return &key
}

func (h *SMSHandler) List(c *gin.Context) {
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

	messages, err := h.sms.List(c.Request.Context(), accountID, limit, cursor)
	if err != nil {
		if errors.Is(err, domainerrors.ErrInvalidCursor) {
			writeError(c, http.StatusBadRequest, "validation_error", "invalid cursor")
			return
		}
		writeError(c, http.StatusInternalServerError, "internal_error", "failed to list sms messages")
		return
	}

	items := make([]gin.H, len(messages))
	for i, msg := range messages {
		items[i] = smsMessageResponse(msg)
	}

	response := gin.H{"items": items}
	if len(messages) == limit {
		response["next_cursor"] = messages[len(messages)-1].ID.String()
	}

	c.JSON(http.StatusOK, response)
}

func (h *SMSHandler) Get(c *gin.Context) {
	accountID, ok := middleware.AccountID(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "missing account context")
		return
	}

	messageID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeError(c, http.StatusBadRequest, "validation_error", "invalid message id")
		return
	}

	msg, err := h.sms.Get(c.Request.Context(), accountID, messageID)
	if err != nil {
		if errors.Is(err, domainerrors.ErrNotFound) {
			writeError(c, http.StatusNotFound, "not_found", "message not found")
			return
		}
		writeError(c, http.StatusInternalServerError, "internal_error", "failed to get sms message")
		return
	}

	c.JSON(http.StatusOK, smsMessageResponse(msg))
}

func smsMessageResponse(msg domain.SMSMessage) gin.H {
	item := gin.H{
		"id":           msg.ID.String(),
		"to":           msg.ToNumber,
		"body":         msg.Body,
		"encoding":     msg.Encoding,
		"message_type": msg.MessageType,
		"status":       msg.Status,
		"cost":         msg.Cost,
		"created_at":   msg.CreatedAt,
	}
	if msg.SentAt != nil {
		item["sent_at"] = msg.SentAt
	}
	return item
}
