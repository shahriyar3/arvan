//go:build integration

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/domain"
	domainerrors "github.com/shahriyar/arvan/internal/domain/errors"
	"github.com/shahriyar/arvan/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSMSServiceSendIdempotencyConcurrency(t *testing.T) {
	db := repository.NewIntegrationDB(t)
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
	var inProgressCount int
	var firstMessageID uuid.UUID
	for i := 0; i < workers; i++ {
		select {
		case err := <-errs:
			if errors.Is(err, domainerrors.ErrIdempotencyInProgress) {
				inProgressCount++
				continue
			}
			require.Failf(t, "unexpected worker error", "%v", err)
		case result := <-results:
			successCount++
			if firstMessageID == uuid.Nil {
				firstMessageID = result.MessageID
			} else {
				assert.Equal(t, firstMessageID, result.MessageID)
			}
		}
	}
	require.GreaterOrEqual(t, successCount, 1, "expected at least one successful send")
	require.NotEqual(t, uuid.Nil, firstMessageID)
	t.Logf("success=%d in_progress=%d", successCount, inProgressCount)

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
	db := repository.NewIntegrationDB(t)
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

func TestSMSServiceSendExactSpendLimit(t *testing.T) {
	db := repository.NewIntegrationDB(t)
	accountRepo := repository.NewAccountRepository(db)
	ledgerRepo := repository.NewLedgerRepository(db)
	smsRepo := repository.NewSMSRepository(db)
	outboxRepo := repository.NewOutboxRepository(db)
	idempotencyRepo := repository.NewIdempotencyRepository(db)
	svc := NewSMSService(accountRepo, ledgerRepo, smsRepo, outboxRepo, idempotencyRepo)
	ctx := context.Background()

	account, err := accountRepo.UpsertByTokenHash(ctx, "sms-exact-spend-hash")
	require.NoError(t, err)

	accountSvc := NewAccountService(accountRepo, ledgerRepo)
	_, err = accountSvc.Topup(ctx, account.ID, 3)
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		_, err := svc.Send(ctx, account.ID, domain.SendSMSInput{
			To:          "+989121234567",
			Body:        "Hello",
			MessageType: domain.MessageTypeStandard,
		})
		require.NoError(t, err, "send %d should succeed", i+1)
	}

	balance, err := accountRepo.GetBalance(ctx, account.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), balance)

	_, err = svc.Send(ctx, account.ID, domain.SendSMSInput{
		To:          "+989121234567",
		Body:        "One more",
		MessageType: domain.MessageTypeStandard,
	})
	assert.ErrorIs(t, err, domainerrors.ErrInsufficientBalance)

	balance, err = accountRepo.GetBalance(ctx, account.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), balance)

	var smsCount int64
	require.NoError(t, db.Table("sms_messages").Where("account_id = ?", account.ID).Count(&smsCount).Error)
	assert.Equal(t, int64(3), smsCount)
}
