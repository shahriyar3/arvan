//go:build integration

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/domain"
	"github.com/shahriyar/arvan/internal/middleware"
	"github.com/shahriyar/arvan/internal/repository"
	"github.com/shahriyar/arvan/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSMSHandlerSendValidationIntegration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := repository.NewIntegrationDB(t)
	accountRepo := repository.NewAccountRepository(db)
	ledgerRepo := repository.NewLedgerRepository(db)
	smsRepo := repository.NewSMSRepository(db)
	outboxRepo := repository.NewOutboxRepository(db)
	idempotencyRepo := repository.NewIdempotencyRepository(db)
	accountSvc := service.NewAccountService(accountRepo, ledgerRepo)
	smsSvc := service.NewSMSService(accountRepo, ledgerRepo, smsRepo, outboxRepo, idempotencyRepo)
	ctx := context.Background()

	account, err := accountRepo.UpsertByTokenHash(ctx, "handler-validation-integration")
	require.NoError(t, err)
	_, err = accountSvc.Topup(ctx, account.ID, 10)
	require.NoError(t, err)

	router := gin.New()
	v1 := router.Group("/v1")
	v1.Use(func(c *gin.Context) {
		c.Set(middleware.AccountIDKey, account.ID)
		c.Next()
	})
	NewSMSHandler(smsSvc).Register(v1, middleware.Idempotency(idempotencyRepo))

	send := func(body map[string]string) int {
		payload, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/v1/sms/send", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec.Code
	}

	t.Run("rejects gsm7 over 160 chars", func(t *testing.T) {
		assert.Equal(t, http.StatusBadRequest, send(map[string]string{
			"to":           "+989121234567",
			"body":         strings.Repeat("a", domain.GSM7MaxLength+1),
			"message_type": "standard",
		}))
	})

	t.Run("rejects ucs2 over 70 chars", func(t *testing.T) {
		assert.Equal(t, http.StatusBadRequest, send(map[string]string{
			"to":           "+989121234567",
			"body":         strings.Repeat("س", domain.UCS2MaxLength+1),
			"message_type": "standard",
		}))
	})
}

func TestSMSHandlerSendIdempotencyIntegration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := repository.NewIntegrationDB(t)
	accountRepo := repository.NewAccountRepository(db)
	ledgerRepo := repository.NewLedgerRepository(db)
	smsRepo := repository.NewSMSRepository(db)
	outboxRepo := repository.NewOutboxRepository(db)
	idempotencyRepo := repository.NewIdempotencyRepository(db)
	accountSvc := service.NewAccountService(accountRepo, ledgerRepo)
	smsSvc := service.NewSMSService(accountRepo, ledgerRepo, smsRepo, outboxRepo, idempotencyRepo)
	ctx := context.Background()

	account, err := accountRepo.UpsertByTokenHash(ctx, "handler-idempotency-integration")
	require.NoError(t, err)
	_, err = accountSvc.Topup(ctx, account.ID, 10)
	require.NoError(t, err)

	router := gin.New()
	v1 := router.Group("/v1")
	v1.Use(func(c *gin.Context) {
		c.Set(middleware.AccountIDKey, account.ID)
		c.Next()
	})
	NewSMSHandler(smsSvc).Register(v1, middleware.Idempotency(idempotencyRepo))

	idempotencyKey := uuid.New().String()
	body, err := json.Marshal(map[string]string{
		"to":           "+989121234567",
		"body":         "Hello",
		"message_type": "standard",
	})
	require.NoError(t, err)

	send := func() (int, map[string]string) {
		req := httptest.NewRequest(http.MethodPost, "/v1/sms/send", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", idempotencyKey)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		var resp map[string]string
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		return rec.Code, resp
	}

	status1, resp1 := send()
	require.Equal(t, http.StatusAccepted, status1)
	require.NotEmpty(t, resp1["message_id"])
	assert.Equal(t, domain.SMSStatusAccepted, resp1["status"])

	status2, resp2 := send()
	require.Equal(t, http.StatusAccepted, status2)
	assert.Equal(t, resp1["message_id"], resp2["message_id"])
	assert.Equal(t, resp1["status"], resp2["status"])

	balance, err := accountRepo.GetBalance(ctx, account.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(9), balance)

	var smsCount int64
	require.NoError(t, db.Table("sms_messages").Where("account_id = ?", account.ID).Count(&smsCount).Error)
	assert.Equal(t, int64(1), smsCount)
}

func TestSMSHandlerSendInsufficientBalanceIntegration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := repository.NewIntegrationDB(t)
	accountRepo := repository.NewAccountRepository(db)
	ledgerRepo := repository.NewLedgerRepository(db)
	smsRepo := repository.NewSMSRepository(db)
	outboxRepo := repository.NewOutboxRepository(db)
	idempotencyRepo := repository.NewIdempotencyRepository(db)
	smsSvc := service.NewSMSService(accountRepo, ledgerRepo, smsRepo, outboxRepo, idempotencyRepo)
	ctx := context.Background()

	account, err := accountRepo.UpsertByTokenHash(ctx, "handler-402-integration")
	require.NoError(t, err)

	router := gin.New()
	v1 := router.Group("/v1")
	v1.Use(func(c *gin.Context) {
		c.Set(middleware.AccountIDKey, account.ID)
		c.Next()
	})
	NewSMSHandler(smsSvc).Register(v1, middleware.Idempotency(idempotencyRepo))

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
}
