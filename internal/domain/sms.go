package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	SMSCostPerMessage = int64(1)

	MessageTypeStandard = "standard"
	MessageTypeExpress  = "express"

	EncodingGSM7  = "gsm7"
	EncodingUCS2  = "ucs2"

	SMSStatusAccepted = "accepted"

	OutboxEventTypeSMSSendRequested = "sms.send_requested"
	OutboxStatusPending             = "pending"
)

type SMSMessage struct {
	ID             uuid.UUID
	AccountID      uuid.UUID
	ToNumber       string
	Body           string
	Encoding       string
	MessageType    string
	Status         string
	Cost           int64
	IdempotencyKey *string
	CreatedAt      time.Time
	SentAt         *time.Time
}

type SendSMSInput struct {
	To             string
	Body           string
	MessageType    string
	IdempotencyKey *string
}

type SendSMSResult struct {
	MessageID uuid.UUID
	Status    string
}

type OutboxEvent struct {
	ID          uuid.UUID
	AggregateID uuid.UUID
	EventType   string
	Payload     []byte
	Status      string
}
