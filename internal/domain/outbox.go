package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	OutboxStatusPublished = "published"
	OutboxStatusFailed    = "failed"
)

type OutboxEventRecord struct {
	ID          uuid.UUID
	AggregateID uuid.UUID
	EventType   string
	Payload     []byte
	Status      string
	LockedUntil *time.Time
	PublishedAt *time.Time
	RetryCount  int
	CreatedAt   time.Time
}

type SMSSendPayload struct {
	MessageID    string            `json:"message_id"`
	AccountID    string            `json:"account_id"`
	To           string            `json:"to"`
	Body         string            `json:"body"`
	MessageType  string            `json:"message_type"`
	TraceContext map[string]string `json:"trace_context,omitempty"`
}
