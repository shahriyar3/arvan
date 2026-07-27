package domain

import (
	"time"

	"github.com/google/uuid"
)

type Account struct {
	ID        uuid.UUID
	TokenHash string
	Balance   int64
	CreatedAt time.Time
}
