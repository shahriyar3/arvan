package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/domain"
	domainerrors "github.com/shahriyar/arvan/internal/domain/errors"
	"github.com/shahriyar/arvan/internal/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubSMSService struct {
	result  domain.SendSMSResult
	err     error
	getMsg  domain.SMSMessage
	getErr  error
	list    []domain.SMSMessage
	listErr error
}

func (s stubSMSService) Send(_ context.Context, _ uuid.UUID, _ domain.SendSMSInput) (domain.SendSMSResult, error) {
	if s.err != nil {
		return domain.SendSMSResult{}, s.err
	}
	return s.result, nil
}

func (s stubSMSService) Get(_ context.Context, _, _ uuid.UUID) (domain.SMSMessage, error) {
	if s.getErr != nil {
		return domain.SMSMessage{}, s.getErr
	}
	return s.getMsg, nil
}

func (s stubSMSService) List(_ context.Context, _ uuid.UUID, limit int, _ *uuid.UUID) ([]domain.SMSMessage, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	if limit > 0 && limit < len(s.list) {
		return s.list[:limit], nil
	}
	return s.list, nil
}

func newSMSTestRouter(accountID uuid.UUID, svc smsService) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.AccountIDKey, accountID)
		c.Next()
	})
	NewSMSHandler(svc).Register(router.Group("/v1"), func(c *gin.Context) { c.Next() })
	return router
}

func TestSMSHandlerSend(t *testing.T) {
	gin.SetMode(gin.TestMode)
	accountID := uuid.New()
	messageID := uuid.New()

	router := newSMSTestRouter(accountID, stubSMSService{
		result: domain.SendSMSResult{
			MessageID: messageID,
			Status:    domain.SMSStatusAccepted,
		},
	})

	body, err := json.Marshal(map[string]string{
		"to":           "+989121234567",
		"body":         "Hello",
		"message_type": "standard",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/sms/send", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusAccepted, rec.Code)
	assert.Contains(t, rec.Body.String(), messageID.String())
	assert.Contains(t, rec.Body.String(), `"status":"accepted"`)
}

func TestSMSHandlerSendErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	accountID := uuid.New()

	t.Run("invalid body returns 400", func(t *testing.T) {
		router := newSMSTestRouter(accountID, stubSMSService{})
		req := httptest.NewRequest(http.MethodPost, "/v1/sms/send", bytes.NewReader([]byte("{}")))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("insufficient balance returns 402", func(t *testing.T) {
		router := newSMSTestRouter(accountID, stubSMSService{
			err: domainerrors.ErrInsufficientBalance,
		})
		body, err := json.Marshal(map[string]string{
			"to":           "+989121234567",
			"body":         "Hello",
			"message_type": "standard",
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/v1/sms/send", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusPaymentRequired, rec.Code)
	})

	t.Run("invalid message type returns 400", func(t *testing.T) {
		router := newSMSTestRouter(accountID, stubSMSService{
			err: domainerrors.ErrInvalidMessageType,
		})
		body, err := json.Marshal(map[string]string{
			"to":           "+989121234567",
			"body":         "Hello",
			"message_type": "urgent",
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/v1/sms/send", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("idempotency in progress returns 409", func(t *testing.T) {
		router := newSMSTestRouter(accountID, stubSMSService{
			err: domainerrors.ErrIdempotencyInProgress,
		})
		body, err := json.Marshal(map[string]string{
			"to":           "+989121234567",
			"body":         "Hello",
			"message_type": "standard",
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/v1/sms/send", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusConflict, rec.Code)
	})
}

func TestSMSHandlerListAndGet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	accountID := uuid.New()
	messageID := uuid.New()
	now := time.Now()

	router := newSMSTestRouter(accountID, stubSMSService{
		list: []domain.SMSMessage{
			{
				ID:          messageID,
				ToNumber:    "+989121234567",
				Body:        "Hello",
				Encoding:    domain.EncodingGSM7,
				MessageType: domain.MessageTypeStandard,
				Status:      domain.SMSStatusAccepted,
				Cost:        1,
				CreatedAt:   now,
			},
		},
		getMsg: domain.SMSMessage{
			ID:          messageID,
			ToNumber:    "+989121234567",
			Body:        "Hello",
			Encoding:    domain.EncodingGSM7,
			MessageType: domain.MessageTypeStandard,
			Status:      domain.SMSStatusAccepted,
			Cost:        1,
			CreatedAt:   now,
		},
	})

	t.Run("list returns items", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/sms", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), messageID.String())
	})

	t.Run("get returns message", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/sms/"+messageID.String(), nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"status":"accepted"`)
	})

	t.Run("get not found returns 404", func(t *testing.T) {
		router := newSMSTestRouter(accountID, stubSMSService{
			getErr: domainerrors.ErrNotFound,
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/sms/"+uuid.New().String(), nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}
