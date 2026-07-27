package repository

import (
	"time"

	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/domain"
)

type idempotencyKeyModel struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey"`
	AccountID        uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_idempotency_account_key,priority:1"`
	IdempotencyKey   string    `gorm:"column:idempotency_key;size:255;not null;uniqueIndex:idx_idempotency_account_key,priority:2"`
	ResponseSnapshot []byte    `gorm:"type:jsonb;not null"`
	CreatedAt        time.Time `gorm:"not null;autoCreateTime"`
}

func (idempotencyKeyModel) TableName() string {
	return "idempotency_keys"
}

func toDomainIdempotency(m idempotencyKeyModel) domain.IdempotencyRecord {
	return domain.IdempotencyRecord{
		ID:               m.ID,
		AccountID:        m.AccountID,
		Key:              m.IdempotencyKey,
		ResponseSnapshot: m.ResponseSnapshot,
		CreatedAt:        m.CreatedAt,
	}
}
