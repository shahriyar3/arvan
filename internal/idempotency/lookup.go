package idempotency

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/domain"
)

type DBLookup interface {
	FindByAccountAndKey(ctx context.Context, accountID uuid.UUID, key string) (*domain.IdempotencyRecord, error)
}

// CompositeLookup checks Redis first, then falls back to the database (source of truth).
type CompositeLookup struct {
	db    DBLookup
	cache ResponseCache
}

func NewCompositeLookup(db DBLookup, cache ResponseCache) *CompositeLookup {
	return &CompositeLookup{db: db, cache: cache}
}

func (l *CompositeLookup) FindByAccountAndKey(
	ctx context.Context,
	accountID uuid.UUID,
	key string,
) (*domain.IdempotencyRecord, error) {
	if l.cache != nil {
		resp, ok, err := l.cache.Get(ctx, accountID, key)
		if err == nil && ok {
			snapshot, marshalErr := json.Marshal(resp)
			if marshalErr != nil {
				return nil, marshalErr
			}
			return &domain.IdempotencyRecord{
				AccountID:        accountID,
				Key:              key,
				ResponseSnapshot: snapshot,
			}, nil
		}
	}

	return l.db.FindByAccountAndKey(ctx, accountID, key)
}
