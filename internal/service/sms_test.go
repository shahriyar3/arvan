package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/domain"
	domainerrors "github.com/shahriyar/arvan/internal/domain/errors"
	"github.com/shahriyar/arvan/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSMSServiceSend(t *testing.T) {
	db := repository.NewTestDB(t)
	accountRepo := repository.NewAccountRepository(db)
	ledgerRepo := repository.NewLedgerRepository(db)
	smsRepo := repository.NewSMSRepository(db)
	outboxRepo := repository.NewOutboxRepository(db)
	idempotencyRepo := repository.NewIdempotencyRepository(db)
	svc := NewSMSService(accountRepo, ledgerRepo, smsRepo, outboxRepo, idempotencyRepo)
	ctx := context.Background()

	account, err := accountRepo.UpsertByTokenHash(ctx, "sms-send-hash")
	require.NoError(t, err)

	accountSvc := NewAccountService(accountRepo, ledgerRepo)
	_, err = accountSvc.Topup(ctx, account.ID, 10)
	require.NoError(t, err)

	t.Run("deducts balance and writes sms ledger and outbox in one tx", func(t *testing.T) {
		result, err := svc.Send(ctx, account.ID, domain.SendSMSInput{
			To:          "+989121234567",
			Body:        "Hello",
			MessageType: domain.MessageTypeStandard,
		})
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, result.MessageID)
		assert.Equal(t, domain.SMSStatusAccepted, result.Status)

		balance, err := accountRepo.GetBalance(ctx, account.ID)
		require.NoError(t, err)
		assert.Equal(t, int64(9), balance)

		entries, err := ledgerRepo.ListByAccount(ctx, account.ID, 10, nil)
		require.NoError(t, err)
		require.NotEmpty(t, entries)
		assert.Equal(t, int64(-1), entries[0].Delta)
		assert.Equal(t, domain.LedgerReasonSend, entries[0].Reason)
		require.NotNil(t, entries[0].RefID)
		assert.Equal(t, result.MessageID, *entries[0].RefID)

		var smsCount int64
		require.NoError(t, db.Table("sms_messages").Where("id = ?", result.MessageID).Count(&smsCount).Error)
		assert.Equal(t, int64(1), smsCount)

		var outbox struct {
			EventType string
			Status    string
			Payload   []byte
		}
		require.NoError(t, db.Table("outbox_events").Where("aggregate_id = ?", result.MessageID).First(&outbox).Error)
		assert.Equal(t, domain.OutboxEventTypeSMSSendRequested, outbox.EventType)
		assert.Equal(t, domain.OutboxStatusPending, outbox.Status)

		var payload map[string]string
		require.NoError(t, json.Unmarshal(outbox.Payload, &payload))
		assert.Equal(t, result.MessageID.String(), payload["message_id"])
		assert.Equal(t, account.ID.String(), payload["account_id"])
		assert.Equal(t, "+989121234567", payload["to"])
		assert.Equal(t, "Hello", payload["body"])
		assert.Equal(t, domain.MessageTypeStandard, payload["message_type"])
	})

	t.Run("returns insufficient balance when cost exceeds balance", func(t *testing.T) {
		emptyAccount, err := accountRepo.UpsertByTokenHash(ctx, "sms-empty-hash")
		require.NoError(t, err)

		_, err = svc.Send(ctx, emptyAccount.ID, domain.SendSMSInput{
			To:          "+989121234567",
			Body:        "Hello",
			MessageType: domain.MessageTypeStandard,
		})
		assert.ErrorIs(t, err, domainerrors.ErrInsufficientBalance)

		balance, err := accountRepo.GetBalance(ctx, emptyAccount.ID)
		require.NoError(t, err)
		assert.Equal(t, int64(0), balance)
	})

	t.Run("rejects invalid message type", func(t *testing.T) {
		_, err := svc.Send(ctx, account.ID, domain.SendSMSInput{
			To:          "+989121234567",
			Body:        "Hello",
			MessageType: "urgent",
		})
		assert.ErrorIs(t, err, domainerrors.ErrInvalidMessageType)
	})

	t.Run("rejects empty to or body", func(t *testing.T) {
		_, err := svc.Send(ctx, account.ID, domain.SendSMSInput{
			To:          "  ",
			Body:        "Hello",
			MessageType: domain.MessageTypeStandard,
		})
		assert.ErrorIs(t, err, domainerrors.ErrInvalidRequest)
	})

	t.Run("rejects invalid phone number", func(t *testing.T) {
		_, err := svc.Send(ctx, account.ID, domain.SendSMSInput{
			To:          "09121234567",
			Body:        "Hello",
			MessageType: domain.MessageTypeStandard,
		})
		assert.ErrorIs(t, err, domainerrors.ErrInvalidPhoneNumber)
	})

	t.Run("rejects message over single-page limit", func(t *testing.T) {
		_, err := svc.Send(ctx, account.ID, domain.SendSMSInput{
			To:          "+989121234567",
			Body:        strings.Repeat("a", domain.GSM7MaxLength+1),
			MessageType: domain.MessageTypeStandard,
		})
		assert.ErrorIs(t, err, domainerrors.ErrMessageTooLong)
	})
}

func TestSMSServiceSendIdempotency(t *testing.T) {
	db := repository.NewTestDB(t)
	accountRepo := repository.NewAccountRepository(db)
	ledgerRepo := repository.NewLedgerRepository(db)
	smsRepo := repository.NewSMSRepository(db)
	outboxRepo := repository.NewOutboxRepository(db)
	idempotencyRepo := repository.NewIdempotencyRepository(db)
	svc := NewSMSService(accountRepo, ledgerRepo, smsRepo, outboxRepo, idempotencyRepo)
	ctx := context.Background()

	account, err := accountRepo.UpsertByTokenHash(ctx, "sms-idempotency-hash")
	require.NoError(t, err)

	accountSvc := NewAccountService(accountRepo, ledgerRepo)
	_, err = accountSvc.Topup(ctx, account.ID, 5)
	require.NoError(t, err)

	idempotencyKey := uuid.New().String()
	input := domain.SendSMSInput{
		To:             "+989121234567",
		Body:           "Hello",
		MessageType:    domain.MessageTypeStandard,
		IdempotencyKey: &idempotencyKey,
	}

	first, err := svc.Send(ctx, account.ID, input)
	require.NoError(t, err)

	second, err := svc.Send(ctx, account.ID, input)
	require.NoError(t, err)
	assert.Equal(t, first.MessageID, second.MessageID)
	assert.Equal(t, first.Status, second.Status)

	balance, err := accountRepo.GetBalance(ctx, account.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(4), balance)

	var smsCount int64
	require.NoError(t, db.Table("sms_messages").Where("account_id = ?", account.ID).Count(&smsCount).Error)
	assert.Equal(t, int64(1), smsCount)

	var sendLedgerCount int64
	require.NoError(t, db.Table("account_ledger").
		Where("account_id = ? AND reason = ?", account.ID, domain.LedgerReasonSend).
		Count(&sendLedgerCount).Error)
	assert.Equal(t, int64(1), sendLedgerCount)

	otherKey := uuid.New().String()
	thirdInput := input
	thirdInput.IdempotencyKey = &otherKey
	third, err := svc.Send(ctx, account.ID, thirdInput)
	require.NoError(t, err)
	assert.NotEqual(t, first.MessageID, third.MessageID)

	balance, err = accountRepo.GetBalance(ctx, account.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), balance)

	var totalSMS int64
	require.NoError(t, db.Table("sms_messages").Where("account_id = ?", account.ID).Count(&totalSMS).Error)
	assert.Equal(t, int64(2), totalSMS)
}

func TestSMSServiceSendIdempotencyInProgress(t *testing.T) {
	db := repository.NewTestDB(t)
	accountRepo := repository.NewAccountRepository(db)
	ledgerRepo := repository.NewLedgerRepository(db)
	smsRepo := repository.NewSMSRepository(db)
	outboxRepo := repository.NewOutboxRepository(db)
	idempotencyRepo := repository.NewIdempotencyRepository(db)
	svc := NewSMSService(accountRepo, ledgerRepo, smsRepo, outboxRepo, idempotencyRepo)
	ctx := context.Background()

	account, err := accountRepo.UpsertByTokenHash(ctx, "sms-idempotency-in-progress-hash")
	require.NoError(t, err)

	accountSvc := NewAccountService(accountRepo, ledgerRepo)
	_, err = accountSvc.Topup(ctx, account.ID, 5)
	require.NoError(t, err)

	idempotencyKey := uuid.New().String()
	require.NoError(t, db.Exec(
		`INSERT INTO idempotency_keys (id, account_id, idempotency_key, response_snapshot, created_at) VALUES (?, ?, ?, ?, datetime('now'))`,
		uuid.New(), account.ID, idempotencyKey, []byte("{}"),
	).Error)

	_, err = svc.Send(ctx, account.ID, domain.SendSMSInput{
		To:             "+989121234567",
		Body:           "Hello",
		MessageType:    domain.MessageTypeStandard,
		IdempotencyKey: &idempotencyKey,
	})
	assert.ErrorIs(t, err, domainerrors.ErrIdempotencyInProgress)

	balance, err := accountRepo.GetBalance(ctx, account.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(5), balance)
}

func TestSMSServiceSendIdempotencyConcurrency(t *testing.T) {
	db := repository.NewTestDB(t)
	if repository.UsesSQLite(db) {
		t.Skip("concurrent idempotency stress requires PostgreSQL")
	}
	accountRepo := repository.NewAccountRepository(db)
	ledgerRepo := repository.NewLedgerRepository(db)
	smsRepo := repository.NewSMSRepository(db)
	outboxRepo := repository.NewOutboxRepository(db)
	idempotencyRepo := repository.NewIdempotencyRepository(db)
	svc := NewSMSService(accountRepo, ledgerRepo, smsRepo, outboxRepo, idempotencyRepo)
	ctx := context.Background()

	account, err := accountRepo.UpsertByTokenHash(ctx, "sms-idempotency-concurrency-hash")
	require.NoError(t, err)

	accountSvc := NewAccountService(accountRepo, ledgerRepo)
	_, err = accountSvc.Topup(ctx, account.ID, 1)
	require.NoError(t, err)

	idempotencyKey := uuid.New().String()
	input := domain.SendSMSInput{
		To:             "+989121234567",
		Body:           "Hello",
		MessageType:    domain.MessageTypeStandard,
		IdempotencyKey: &idempotencyKey,
	}

	const workers = 20
	results := make(chan domain.SendSMSResult, workers)
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			result, sendErr := svc.Send(ctx, account.ID, input)
			if sendErr != nil {
				errs <- sendErr
				return
			}
			results <- result
		}()
	}

	var successCount int
	var firstMessageID uuid.UUID
	for i := 0; i < workers; i++ {
		select {
		case err := <-errs:
			t.Logf("worker error: %v", err)
		case result := <-results:
			successCount++
			if firstMessageID == uuid.Nil {
				firstMessageID = result.MessageID
			} else {
				assert.Equal(t, firstMessageID, result.MessageID)
			}
		}
	}
	require.GreaterOrEqual(t, successCount, 1)
	require.NotEqual(t, uuid.Nil, firstMessageID)

	balance, err := accountRepo.GetBalance(ctx, account.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), balance)

	var smsCount int64
	require.NoError(t, db.Table("sms_messages").Where("account_id = ?", account.ID).Count(&smsCount).Error)
	assert.Equal(t, int64(1), smsCount)

	var sendLedgerCount int64
	require.NoError(t, db.Table("account_ledger").
		Where("account_id = ? AND reason = ?", account.ID, domain.LedgerReasonSend).
		Count(&sendLedgerCount).Error)
	assert.Equal(t, int64(1), sendLedgerCount)
}

func TestSMSServiceSendConcurrency(t *testing.T) {
	db := repository.NewTestDB(t)
	if repository.UsesSQLite(db) {
		t.Skip("concurrency stress requires PostgreSQL; skeleton for integration tests")
	}
	accountRepo := repository.NewAccountRepository(db)
	ledgerRepo := repository.NewLedgerRepository(db)
	smsRepo := repository.NewSMSRepository(db)
	outboxRepo := repository.NewOutboxRepository(db)
	idempotencyRepo := repository.NewIdempotencyRepository(db)
	svc := NewSMSService(accountRepo, ledgerRepo, smsRepo, outboxRepo, idempotencyRepo)
	ctx := context.Background()

	account, err := accountRepo.UpsertByTokenHash(ctx, "sms-concurrency-hash")
	require.NoError(t, err)

	accountSvc := NewAccountService(accountRepo, ledgerRepo)
	_, err = accountSvc.Topup(ctx, account.ID, 100)
	require.NoError(t, err)

	const workers = 100
	results := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			_, sendErr := svc.Send(ctx, account.ID, domain.SendSMSInput{
				To:          "+989121234567",
				Body:        "Hello",
				MessageType: domain.MessageTypeStandard,
			})
			results <- sendErr
		}()
	}

	for i := 0; i < workers; i++ {
		require.NoError(t, <-results, "worker %d failed", i)
	}

	balance, err := accountRepo.GetBalance(ctx, account.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), balance)

	var smsCount int64
	require.NoError(t, db.Table("sms_messages").Where("account_id = ?", account.ID).Count(&smsCount).Error)
	assert.Equal(t, int64(workers), smsCount)

	var sendLedgerCount int64
	require.NoError(t, db.Table("account_ledger").
		Where("account_id = ? AND reason = ?", account.ID, domain.LedgerReasonSend).
		Count(&sendLedgerCount).Error)
	assert.Equal(t, int64(workers), sendLedgerCount)

	_, err = svc.Send(ctx, account.ID, domain.SendSMSInput{
		To:          "+989121234567",
		Body:        "One more",
		MessageType: domain.MessageTypeStandard,
	})
	assert.ErrorIs(t, err, domainerrors.ErrInsufficientBalance)
}
