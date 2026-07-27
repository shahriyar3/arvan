//go:build integration

package service

import (
	"context"
	"sync"
	"testing"

	"github.com/shahriyar/arvan/internal/domain"
	"github.com/shahriyar/arvan/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccountServiceTopupConcurrency(t *testing.T) {
	db := repository.NewIntegrationDB(t)
	accountRepo := repository.NewAccountRepository(db)
	ledgerRepo := repository.NewLedgerRepository(db)
	svc := NewAccountService(accountRepo, ledgerRepo)
	ctx := context.Background()

	account, err := accountRepo.UpsertByTokenHash(ctx, "topup-concurrency-hash")
	require.NoError(t, err)

	const workers = 100
	const amount = int64(1)
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, topupErr := svc.Topup(ctx, account.ID, amount)
			errs <- topupErr
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}

	balance, err := accountRepo.GetBalance(ctx, account.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(workers)*amount, balance)

	var topupLedgerCount int64
	require.NoError(t, db.Table("account_ledger").
		Where("account_id = ? AND reason = ?", account.ID, domain.LedgerReasonTopup).
		Count(&topupLedgerCount).Error)
	assert.Equal(t, int64(workers), topupLedgerCount)
}
