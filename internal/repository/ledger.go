package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/domain"
	domainerrors "github.com/shahriyar/arvan/internal/domain/errors"
	"gorm.io/gorm"
)

type LedgerRepository struct {
	db *gorm.DB
}

func NewLedgerRepository(db *gorm.DB) *LedgerRepository {
	return &LedgerRepository{db: db}
}

func (r *LedgerRepository) Create(ctx context.Context, tx *gorm.DB, entry domain.LedgerEntry) error {
	model := ledgerModel{
		AccountID: entry.AccountID,
		Delta:     entry.Delta,
		Reason:    entry.Reason,
		RefID:     entry.RefID,
	}
	if entry.ID != uuid.Nil {
		model.ID = entry.ID
	}

	db := tx
	if db == nil {
		db = writeDB(r.db)
	}

	if err := db.WithContext(ctx).Create(&model).Error; err != nil {
		return fmt.Errorf("create ledger entry: %w", err)
	}

	return nil
}

func (r *LedgerRepository) ListByAccount(
	ctx context.Context,
	accountID uuid.UUID,
	limit int,
	cursor *uuid.UUID,
) ([]domain.LedgerEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	query := readDB(r.db).WithContext(ctx).
		Where("account_id = ?", accountID).
		Order("created_at DESC, id DESC").
		Limit(limit)

	if cursor != nil {
		var anchor ledgerModel
		if err := readDB(r.db).WithContext(ctx).
			Where("id = ? AND account_id = ?", *cursor, accountID).
			First(&anchor).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, domainerrors.ErrInvalidCursor
			}
			return nil, fmt.Errorf("resolve ledger cursor: %w", err)
		}
		query = query.Where(
			"(created_at < ?) OR (created_at = ? AND id < ?)",
			anchor.CreatedAt, anchor.CreatedAt, anchor.ID,
		)
	}

	var models []ledgerModel
	if err := query.Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list ledger entries: %w", err)
	}

	entries := make([]domain.LedgerEntry, len(models))
	for i, model := range models {
		entries[i] = toDomainLedger(model)
	}

	return entries, nil
}

func toDomainLedger(model ledgerModel) domain.LedgerEntry {
	return domain.LedgerEntry{
		ID:        model.ID,
		AccountID: model.AccountID,
		Delta:     model.Delta,
		Reason:    model.Reason,
		RefID:     model.RefID,
		CreatedAt: model.CreatedAt,
	}
}
