package repository

import (
	"time"

	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/domain"
)

type accountModel struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	TokenHash string    `gorm:"column:token_hash;size:255;not null;uniqueIndex"`
	Balance   int64     `gorm:"not null;default:0"`
	CreatedAt time.Time `gorm:"not null;autoCreateTime"`
}

func (accountModel) TableName() string {
	return "accounts"
}

func toDomainAccount(m accountModel) domain.Account {
	return domain.Account{
		ID:        m.ID,
		TokenHash: m.TokenHash,
		Balance:   m.Balance,
		CreatedAt: m.CreatedAt,
	}
}
