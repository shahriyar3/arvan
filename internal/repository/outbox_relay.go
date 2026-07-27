package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/domain"
	"gorm.io/gorm"
)

func (r *OutboxRepository) ClaimPendingBatch(
	ctx context.Context,
	limit int,
	lockDuration time.Duration,
) ([]domain.OutboxEventRecord, error) {
	if limit <= 0 {
		limit = 50
	}

	var claimed []domain.OutboxEventRecord
	err := writeDB(r.db).WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var models []outboxEventModel
		query := `
			SELECT id, aggregate_id, event_type, payload, status, locked_until, published_at, retry_count, created_at
			FROM outbox_events
			WHERE status = ?
			  AND (locked_until IS NULL OR locked_until < NOW())
			ORDER BY created_at
			LIMIT ?
			FOR UPDATE SKIP LOCKED`
		if err := tx.Raw(query, domain.OutboxStatusPending, limit).Scan(&models).Error; err != nil {
			return fmt.Errorf("claim pending outbox events: %w", err)
		}
		if len(models) == 0 {
			return nil
		}

		lockedUntil := time.Now().UTC().Add(lockDuration)
		ids := make([]uuid.UUID, len(models))
		for i, model := range models {
			ids[i] = model.ID
		}

		if err := tx.Model(&outboxEventModel{}).
			Where("id IN ?", ids).
			Update("locked_until", lockedUntil).Error; err != nil {
			return fmt.Errorf("lock outbox events: %w", err)
		}

		claimed = make([]domain.OutboxEventRecord, len(models))
		for i, model := range models {
			claimed[i] = toDomainOutbox(model)
			until := lockedUntil
			claimed[i].LockedUntil = &until
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return claimed, nil
}

func (r *OutboxRepository) MarkPublished(ctx context.Context, eventID uuid.UUID) error {
	now := time.Now().UTC()
	result := writeDB(r.db).WithContext(ctx).
		Model(&outboxEventModel{}).
		Where("id = ? AND status = ?", eventID, domain.OutboxStatusPending).
		Updates(map[string]any{
			"status":       domain.OutboxStatusPublished,
			"published_at": now,
			"locked_until": nil,
		})
	if result.Error != nil {
		return fmt.Errorf("mark outbox published: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("mark outbox published: event %s not pending", eventID)
	}
	return nil
}

func (r *OutboxRepository) RecordPublishFailure(ctx context.Context, eventID uuid.UUID) error {
	result := writeDB(r.db).WithContext(ctx).
		Model(&outboxEventModel{}).
		Where("id = ? AND status = ?", eventID, domain.OutboxStatusPending).
		Updates(map[string]any{
			"retry_count":  gorm.Expr("retry_count + 1"),
			"locked_until": nil,
		})
	if result.Error != nil {
		return fmt.Errorf("record outbox publish failure: %w", result.Error)
	}
	return nil
}

func toDomainOutbox(m outboxEventModel) domain.OutboxEventRecord {
	return domain.OutboxEventRecord{
		ID:          m.ID,
		AggregateID: m.AggregateID,
		EventType:   m.EventType,
		Payload:     m.Payload,
		Status:      m.Status,
		LockedUntil: m.LockedUntil,
		PublishedAt: m.PublishedAt,
		RetryCount:  m.RetryCount,
		CreatedAt:   m.CreatedAt,
	}
}
