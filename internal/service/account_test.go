package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/domain"
	domainerrors "github.com/shahriyar/arvan/internal/domain/errors"
	"github.com/shahriyar/arvan/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccountServiceTopup(t *testing.T) {
	db := repository.NewTestDB(t)
	accountRepo := repository.NewAccountRepository(db)
	ledgerRepo := repository.NewLedgerRepository(db)
	svc := NewAccountService(accountRepo, ledgerRepo)
	ctx := context.Background()

	account, err := accountRepo.UpsertByTokenHash(ctx, "service-topup-hash")
	require.NoError(t, err)
	accountID := account.ID

	t.Run("topup updates balance and writes ledger", func(t *testing.T) {
		balance, err := svc.Topup(ctx, accountID, 50)
		require.NoError(t, err)
		assert.Equal(t, int64(50), balance)

		entries, err := ledgerRepo.ListByAccount(ctx, accountID, 10, nil)
		require.NoError(t, err)
		require.Len(t, entries, 1)
		assert.Equal(t, int64(50), entries[0].Delta)
		assert.Equal(t, domain.LedgerReasonTopup, entries[0].Reason)
	})

	t.Run("rejects non-positive amount", func(t *testing.T) {
		_, err := svc.Topup(ctx, accountID, 0)
		assert.ErrorIs(t, err, domainerrors.ErrInvalidAmount)

		_, err = svc.Topup(ctx, accountID, -5)
		assert.ErrorIs(t, err, domainerrors.ErrInvalidAmount)
	})

	t.Run("returns not found for missing account", func(t *testing.T) {
		_, err := svc.Topup(ctx, uuid.New(), 10)
		assert.ErrorIs(t, err, domainerrors.ErrNotFound)
	})
}

func TestAccountServiceListLedgerInvalidCursor(t *testing.T) {
	db := repository.NewTestDB(t)
	accountRepo := repository.NewAccountRepository(db)
	ledgerRepo := repository.NewLedgerRepository(db)
	svc := NewAccountService(accountRepo, ledgerRepo)
	ctx := context.Background()

	account, err := accountRepo.UpsertByTokenHash(ctx, "service-ledger-hash")
	require.NoError(t, err)

	unknown := uuid.New()
	_, err = svc.ListLedger(ctx, account.ID, 10, &unknown)
	assert.ErrorIs(t, err, domainerrors.ErrInvalidCursor)
}
