package repository

import (
	"time"

	"github.com/google/uuid"
)

type outboxEventModel struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey"`
	AggregateID  uuid.UUID  `gorm:"type:uuid;not null"`
	EventType    string     `gorm:"size:100;not null"`
	Payload      []byte     `gorm:"type:jsonb;not null"`
	Status       string     `gorm:"size:20;not null;default:pending"`
	LockedUntil  *time.Time
	PublishedAt  *time.Time
	RetryCount   int        `gorm:"not null;default:0"`
	CreatedAt    time.Time  `gorm:"not null;autoCreateTime"`
}

func (outboxEventModel) TableName() string {
	return "outbox_events"
}
