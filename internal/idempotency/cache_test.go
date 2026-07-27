package idempotency

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shahriyar/arvan/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisResponseCacheSetGet(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := NewRedisResponseCache(client, RedisCacheConfig{TTL: time.Hour})

	accountID := uuid.New()
	key := uuid.New().String()
	resp := domain.IdempotencyResponse{
		MessageID: uuid.New().String(),
		Status:    domain.SMSStatusAccepted,
	}

	require.NoError(t, cache.Set(context.Background(), accountID, key, resp))

	got, ok, err := cache.Get(context.Background(), accountID, key)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, resp, got)
}

func TestCompositeLookupUsesCacheBeforeDB(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := NewRedisResponseCache(client, RedisCacheConfig{TTL: time.Hour})

	accountID := uuid.New()
	key := uuid.New().String()
	messageID := uuid.New().String()
	require.NoError(t, cache.Set(context.Background(), accountID, key, domain.IdempotencyResponse{
		MessageID: messageID,
		Status:    domain.SMSStatusAccepted,
	}))

	lookup := NewCompositeLookup(stubDBLookup{err: errors.New("db should not be called")}, cache)
	record, err := lookup.FindByAccountAndKey(context.Background(), accountID, key)
	require.NoError(t, err)

	parsed, ok := domain.ParseIdempotencyResponse(record.ResponseSnapshot)
	require.True(t, ok)
	assert.Equal(t, messageID, parsed.MessageID)
}

type stubDBLookup struct {
	err error
}

func (s stubDBLookup) FindByAccountAndKey(context.Context, uuid.UUID, string) (*domain.IdempotencyRecord, error) {
	return nil, s.err
}
