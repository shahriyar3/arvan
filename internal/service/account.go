package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/domain"
	domainerrors "github.com/shahriyar/arvan/internal/domain/errors"
	"github.com/shahriyar/arvan/internal/repository"
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

type AccountService struct {
	accounts *repository.AccountRepository
	ledger   *repository.LedgerRepository
}

func NewAccountService(accounts *repository.AccountRepository, ledger *repository.LedgerRepository) *AccountService {
	return &AccountService{
		accounts: accounts,
		ledger:   ledger,
	}
}

func (s *AccountService) Topup(ctx context.Context, accountID uuid.UUID, amount int64) (int64, error) {
	if amount <= 0 {
		return 0, domainerrors.ErrInvalidAmount
	}

	var newBalance int64
	err := s.accounts.DB().Clauses(dbresolver.Write).WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		balance, err := s.accounts.LockAndAddBalance(ctx, tx, accountID, amount)
		if err != nil {
			return err
		}

		if err := s.ledger.Create(ctx, tx, domain.LedgerEntry{
			AccountID: accountID,
			Delta:     amount,
			Reason:    domain.LedgerReasonTopup,
		}); err != nil {
			return err
		}

		newBalance = balance
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("topup account: %w", err)
	}

	return newBalance, nil
}

func (s *AccountService) GetBalance(ctx context.Context, accountID uuid.UUID) (int64, error) {
	balance, err := s.accounts.GetBalance(ctx, accountID)
	if err != nil {
		return 0, fmt.Errorf("get balance: %w", err)
	}
	return balance, nil
}

func (s *AccountService) ListLedger(
	ctx context.Context,
	accountID uuid.UUID,
	limit int,
	cursor *uuid.UUID,
) ([]domain.LedgerEntry, error) {
	entries, err := s.ledger.ListByAccount(ctx, accountID, limit, cursor)
	if err != nil {
		return nil, fmt.Errorf("list ledger: %w", err)
	}
	return entries, nil
}
