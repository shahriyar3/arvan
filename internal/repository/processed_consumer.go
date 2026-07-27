package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type processedConsumerEventModel struct {
	MessageID   uuid.UUID `gorm:"type:uuid;primaryKey"`
	ProcessedAt time.Time `gorm:"not null;autoCreateTime"`
}

func (processedConsumerEventModel) TableName() string {
	return "processed_consumer_events"
}

type ProcessedConsumerRepository struct {
	db *gorm.DB
}

func NewProcessedConsumerRepository(db *gorm.DB) *ProcessedConsumerRepository {
	return &ProcessedConsumerRepository{db: db}
}

func (r *ProcessedConsumerRepository) MarkProcessedIfNew(ctx context.Context, tx *gorm.DB, messageID uuid.UUID) (bool, error) {
	model := processedConsumerEventModel{MessageID: messageID}
	db := tx
	if db == nil {
		db = writeDB(r.db)
	}

	result := db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&model)
	if result.Error != nil {
		return false, fmt.Errorf("mark processed consumer event: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

func (r *ProcessedConsumerRepository) Exists(ctx context.Context, messageID uuid.UUID) (bool, error) {
	var count int64
	err := writeDB(r.db).WithContext(ctx).
		Model(&processedConsumerEventModel{}).
		Where("message_id = ?", messageID).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("check processed consumer event: %w", err)
	}
	return count > 0, nil
}

func (r *ProcessedConsumerRepository) DeleteClaim(ctx context.Context, tx *gorm.DB, messageID uuid.UUID) error {
	db := tx
	if db == nil {
		db = writeDB(r.db)
	}

	result := db.WithContext(ctx).
		Where("message_id = ?", messageID).
		Delete(&processedConsumerEventModel{})
	if result.Error != nil {
		return fmt.Errorf("delete processed consumer claim: %w", result.Error)
	}
	return nil
}
