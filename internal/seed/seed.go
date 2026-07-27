package seed

import (
	"context"

	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/repository"
	"github.com/shahriyar/arvan/internal/service"
)

type AccountService struct {
	svc *service.AccountService
}

func NewAccountService(accounts *repository.AccountRepository, ledger *repository.LedgerRepository) *AccountService {
	return &AccountService{svc: service.NewAccountService(accounts, ledger)}
}

// EnsureBalance tops up the account when current balance is below target.
func (s *AccountService) EnsureBalance(ctx context.Context, accountID uuid.UUID, target int64) (int64, error) {
	balance, err := s.svc.GetBalance(ctx, accountID)
	if err != nil {
		return 0, err
	}
	if balance >= target {
		return balance, nil
	}
	return s.svc.Topup(ctx, accountID, target-balance)
}
