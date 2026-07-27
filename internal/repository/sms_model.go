package repository

import (
	"time"

	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/domain"
)

type smsMessageModel struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey"`
	AccountID      uuid.UUID  `gorm:"type:uuid;not null;index:idx_sms_messages_account_created,priority:1"`
	ToNumber       string     `gorm:"column:to_number;size:20;not null"`
	Body           string     `gorm:"not null"`
	Encoding       string     `gorm:"size:10;not null"`
	MessageType    string     `gorm:"size:20;not null;default:standard"`
	Status         string     `gorm:"size:30;not null;default:accepted"`
	Cost           int64      `gorm:"not null"`
	IdempotencyKey *string    `gorm:"size:255"`
	CreatedAt      time.Time  `gorm:"not null;autoCreateTime;index:idx_sms_messages_account_created,priority:2,sort:desc"`
	SentAt         *time.Time
}

func (smsMessageModel) TableName() string {
	return "sms_messages"
}

func toDomainSMS(m smsMessageModel) domain.SMSMessage {
	return domain.SMSMessage{
		ID:             m.ID,
		AccountID:      m.AccountID,
		ToNumber:       m.ToNumber,
		Body:           m.Body,
		Encoding:       m.Encoding,
		MessageType:    m.MessageType,
		Status:         m.Status,
		Cost:           m.Cost,
		IdempotencyKey: m.IdempotencyKey,
		CreatedAt:      m.CreatedAt,
		SentAt:         m.SentAt,
	}
}
