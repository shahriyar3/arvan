package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/domain"
	domainerrors "github.com/shahriyar/arvan/internal/domain/errors"
	"gorm.io/gorm"
)

type SMSRepository struct {
	db *gorm.DB
}

func NewSMSRepository(db *gorm.DB) *SMSRepository {
	return &SMSRepository{db: db}
}

func (r *SMSRepository) Create(ctx context.Context, tx *gorm.DB, msg domain.SMSMessage) error {
	model := smsMessageModel{
		ID:             msg.ID,
		AccountID:      msg.AccountID,
		ToNumber:       msg.ToNumber,
		Body:           msg.Body,
		Encoding:       msg.Encoding,
		MessageType:    msg.MessageType,
		Status:         msg.Status,
		Cost:           msg.Cost,
		IdempotencyKey: msg.IdempotencyKey,
	}

	db := tx
	if db == nil {
		db = writeDB(r.db)
	}

	if err := db.WithContext(ctx).Create(&model).Error; err != nil {
		return fmt.Errorf("create sms message: %w", err)
	}

	return nil
}

func (r *SMSRepository) GetByAccountAndID(ctx context.Context, accountID, messageID uuid.UUID) (*domain.SMSMessage, error) {
	return r.getByAccountAndID(ctx, readDB(r.db), accountID, messageID)
}

func (r *SMSRepository) GetByAccountAndIDTx(ctx context.Context, tx *gorm.DB, accountID, messageID uuid.UUID) (*domain.SMSMessage, error) {
	db := tx
	if db == nil {
		db = writeDB(r.db)
	}
	return r.getByAccountAndID(ctx, db, accountID, messageID)
}

func (r *SMSRepository) getByAccountAndID(ctx context.Context, db *gorm.DB, accountID, messageID uuid.UUID) (*domain.SMSMessage, error) {
	var model smsMessageModel
	err := db.WithContext(ctx).
		Where("id = ? AND account_id = ?", messageID, accountID).
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainerrors.ErrNotFound
		}
		return nil, fmt.Errorf("get sms message: %w", err)
	}

	msg := toDomainSMS(model)
	return &msg, nil
}

func (r *SMSRepository) ListByAccount(
	ctx context.Context,
	accountID uuid.UUID,
	limit int,
	cursor *uuid.UUID,
) ([]domain.SMSMessage, error) {
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
		var anchor smsMessageModel
		if err := readDB(r.db).WithContext(ctx).
			Where("id = ? AND account_id = ?", *cursor, accountID).
			First(&anchor).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, domainerrors.ErrInvalidCursor
			}
			return nil, fmt.Errorf("resolve sms cursor: %w", err)
		}
		query = query.Where(
			"(created_at < ?) OR (created_at = ? AND id < ?)",
			anchor.CreatedAt, anchor.CreatedAt, anchor.ID,
		)
	}

	var models []smsMessageModel
	if err := query.Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list sms messages: %w", err)
	}

	messages := make([]domain.SMSMessage, len(models))
	for i, model := range models {
		messages[i] = toDomainSMS(model)
	}

	return messages, nil
}

func (r *SMSRepository) MarkSentIfAccepted(ctx context.Context, tx *gorm.DB, accountID, messageID uuid.UUID) (bool, error) {
	now := time.Now().UTC()
	db := tx
	if db == nil {
		db = writeDB(r.db)
	}

	result := db.WithContext(ctx).
		Model(&smsMessageModel{}).
		Where("id = ? AND account_id = ? AND status = ?", messageID, accountID, domain.SMSStatusAccepted).
		Updates(map[string]any{
			"status":  domain.SMSStatusSent,
			"sent_at": now,
		})
	if result.Error != nil {
		return false, fmt.Errorf("mark sms sent: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

func (r *SMSRepository) MarkFailedIfAccepted(ctx context.Context, tx *gorm.DB, accountID, messageID uuid.UUID) (bool, error) {
	db := tx
	if db == nil {
		db = writeDB(r.db)
	}

	result := db.WithContext(ctx).
		Model(&smsMessageModel{}).
		Where("id = ? AND account_id = ? AND status = ?", messageID, accountID, domain.SMSStatusAccepted).
		Update("status", domain.SMSStatusFailed)
	if result.Error != nil {
		return false, fmt.Errorf("mark sms failed: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}
