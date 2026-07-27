package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/domain"
	domainerrors "github.com/shahriyar/arvan/internal/domain/errors"
	"gorm.io/gorm"
)

type IdempotencyRepository struct {
	db *gorm.DB
}

func NewIdempotencyRepository(db *gorm.DB) *IdempotencyRepository {
	return &IdempotencyRepository{db: db}
}

func (r *IdempotencyRepository) FindByAccountAndKey(
	ctx context.Context,
	accountID uuid.UUID,
	key string,
) (*domain.IdempotencyRecord, error) {
	var model idempotencyKeyModel
	err := writeDB(r.db).WithContext(ctx).
		Where("account_id = ? AND idempotency_key = ?", accountID, key).
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainerrors.ErrNotFound
		}
		return nil, fmt.Errorf("find idempotency key: %w", err)
	}

	record := toDomainIdempotency(model)
	return &record, nil
}

func (r *IdempotencyRepository) FindByAccountAndKeyTx(
	ctx context.Context,
	tx *gorm.DB,
	accountID uuid.UUID,
	key string,
) (*domain.IdempotencyRecord, error) {
	var model idempotencyKeyModel
	err := tx.WithContext(ctx).
		Where("account_id = ? AND idempotency_key = ?", accountID, key).
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainerrors.ErrNotFound
		}
		return nil, fmt.Errorf("find idempotency key in tx: %w", err)
	}

	record := toDomainIdempotency(model)
	return &record, nil
}

const idempotencyClaimSavepoint = "idempotency_claim"

// ClaimOrGet inserts a placeholder row for the idempotency key or returns an existing record on conflict.
// Uses a savepoint so a unique-violation on PostgreSQL does not abort the outer transaction.
func (r *IdempotencyRepository) ClaimOrGet(
	ctx context.Context,
	tx *gorm.DB,
	accountID uuid.UUID,
	key string,
) (*domain.IdempotencyRecord, bool, error) {
	model := idempotencyKeyModel{
		ID:               uuid.New(),
		AccountID:        accountID,
		IdempotencyKey:   key,
		ResponseSnapshot: []byte("{}"),
	}

	if err := tx.SavePoint(idempotencyClaimSavepoint).Error; err != nil {
		return nil, false, fmt.Errorf("idempotency savepoint: %w", err)
	}

	if err := tx.WithContext(ctx).Create(&model).Error; err != nil {
		if isUniqueViolation(err) {
			if err := tx.RollbackTo(idempotencyClaimSavepoint).Error; err != nil {
				return nil, false, fmt.Errorf("idempotency rollback savepoint: %w", err)
			}
			existing, findErr := r.FindByAccountAndKeyTx(ctx, tx, accountID, key)
			if findErr != nil {
				return nil, false, findErr
			}
			return existing, false, nil
		}
		return nil, false, fmt.Errorf("claim idempotency key: %w", err)
	}

	record := toDomainIdempotency(model)
	return &record, true, nil
}

func (r *IdempotencyRepository) UpdateSnapshot(
	ctx context.Context,
	tx *gorm.DB,
	accountID uuid.UUID,
	key string,
	snapshot []byte,
) error {
	result := tx.WithContext(ctx).
		Model(&idempotencyKeyModel{}).
		Where("account_id = ? AND idempotency_key = ?", accountID, key).
		Update("response_snapshot", snapshot)
	if result.Error != nil {
		return fmt.Errorf("update idempotency snapshot: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domainerrors.ErrNotFound
	}
	return nil
}

func isUniqueViolation(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") || strings.Contains(msg, "duplicate key")
}
