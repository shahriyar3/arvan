package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	LedgerReasonTopup  = "topup"
	LedgerReasonSend   = "send"
	LedgerReasonRefund = "refund"
)

type LedgerEntry struct {
	ID        uuid.UUID
	AccountID uuid.UUID
	Delta     int64
	Reason    string
	RefID     *uuid.UUID
	CreatedAt time.Time
}
