package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/domain"
	domainerrors "github.com/shahriyar/arvan/internal/domain/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AccountRepository struct {
	db *gorm.DB
}

func NewAccountRepository(db *gorm.DB) *AccountRepository {
	return &AccountRepository{db: db}
}

func (r *AccountRepository) FindByTokenHash(ctx context.Context, tokenHash string) (*domain.Account, error) {
	var model accountModel
	err := writeDB(r.db).WithContext(ctx).
		Where("token_hash = ?", tokenHash).
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainerrors.ErrNotFound
		}
		return nil, fmt.Errorf("find account by token hash: %w", err)
	}

	account := toDomainAccount(model)
	return &account, nil
}

func (r *AccountRepository) UpsertByTokenHash(ctx context.Context, tokenHash string) (*domain.Account, error) {
	var model accountModel
	err := writeDB(r.db).WithContext(ctx).
		Where("token_hash = ?", tokenHash).
		First(&model).Error
	if err == nil {
		account := toDomainAccount(model)
		return &account, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("find account for upsert: %w", err)
	}

	model = accountModel{ID: uuid.New(), TokenHash: tokenHash}
	if err := writeDB(r.db).WithContext(ctx).Create(&model).Error; err != nil {
		return nil, fmt.Errorf("create account: %w", err)
	}

	account := toDomainAccount(model)
	return &account, nil
}

func (r *AccountRepository) GetBalance(ctx context.Context, accountID uuid.UUID) (int64, error) {
	var model accountModel
	err := readDB(r.db).WithContext(ctx).
		Select("balance").
		Where("id = ?", accountID).
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, domainerrors.ErrNotFound
		}
		return 0, fmt.Errorf("get balance: %w", err)
	}

	return model.Balance, nil
}

func (r *AccountRepository) LockAndAddBalance(
	ctx context.Context,
	tx *gorm.DB,
	accountID uuid.UUID,
	amount int64,
) (int64, error) {
	var model accountModel
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", accountID).
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, domainerrors.ErrNotFound
		}
		return 0, fmt.Errorf("lock account: %w", err)
	}

	model.Balance += amount
	if err := tx.WithContext(ctx).Save(&model).Error; err != nil {
		return 0, fmt.Errorf("update balance: %w", err)
	}

	return model.Balance, nil
}

func (r *AccountRepository) LockAccountForUpdate(
	ctx context.Context,
	tx *gorm.DB,
	accountID uuid.UUID,
) (int64, error) {
	var model accountModel
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", accountID).
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, domainerrors.ErrNotFound
		}
		return 0, fmt.Errorf("lock account: %w", err)
	}

	return model.Balance, nil
}

func (r *AccountRepository) DeductLockedBalance(
	ctx context.Context,
	tx *gorm.DB,
	accountID uuid.UUID,
	cost int64,
) (int64, error) {
	var model accountModel
	err := tx.WithContext(ctx).
		Where("id = ?", accountID).
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, domainerrors.ErrNotFound
		}
		return 0, fmt.Errorf("load locked account: %w", err)
	}

	if model.Balance < cost {
		return 0, domainerrors.ErrInsufficientBalance
	}

	model.Balance -= cost
	if err := tx.WithContext(ctx).Save(&model).Error; err != nil {
		return 0, fmt.Errorf("deduct balance: %w", err)
	}

	return model.Balance, nil
}

func (r *AccountRepository) LockAndDeductBalance(
	ctx context.Context,
	tx *gorm.DB,
	accountID uuid.UUID,
	cost int64,
) (int64, error) {
	if _, err := r.LockAccountForUpdate(ctx, tx, accountID); err != nil {
		return 0, err
	}
	return r.DeductLockedBalance(ctx, tx, accountID, cost)
}

func (r *AccountRepository) DB() *gorm.DB {
	return r.db
}
