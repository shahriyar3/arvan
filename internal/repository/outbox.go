package repository

import (
	"context"
	"fmt"

	"github.com/shahriyar/arvan/internal/domain"
	"gorm.io/gorm"
)

type OutboxRepository struct {
	db *gorm.DB
}

func NewOutboxRepository(db *gorm.DB) *OutboxRepository {
	return &OutboxRepository{db: db}
}

func (r *OutboxRepository) Create(ctx context.Context, tx *gorm.DB, event domain.OutboxEvent) error {
	model := outboxEventModel{
		ID:          event.ID,
		AggregateID: event.AggregateID,
		EventType:   event.EventType,
		Payload:     event.Payload,
		Status:      event.Status,
	}

	db := tx
	if db == nil {
		db = writeDB(r.db)
	}

	if err := db.WithContext(ctx).Create(&model).Error; err != nil {
		return fmt.Errorf("create outbox event: %w", err)
	}

	return nil
}
