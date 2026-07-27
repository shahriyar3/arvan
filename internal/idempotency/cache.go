package idempotency

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shahriyar/arvan/internal/domain"
)

// ResponseCache stores completed idempotency responses for fast retries.
type ResponseCache interface {
	Get(ctx context.Context, accountID uuid.UUID, key string) (domain.IdempotencyResponse, bool, error)
	Set(ctx context.Context, accountID uuid.UUID, key string, resp domain.IdempotencyResponse) error
}

type RedisCacheConfig struct {
	TTL       time.Duration
	KeyPrefix string
}

type RedisResponseCache struct {
	client *redis.Client
	cfg    RedisCacheConfig
}

func NewRedisResponseCache(client *redis.Client, cfg RedisCacheConfig) *RedisResponseCache {
	if cfg.TTL <= 0 {
		cfg.TTL = 24 * time.Hour
	}
	if cfg.KeyPrefix == "" {
		cfg.KeyPrefix = "idempotency"
	}
	return &RedisResponseCache{client: client, cfg: cfg}
}

func (c *RedisResponseCache) cacheKey(accountID uuid.UUID, key string) string {
	return fmt.Sprintf("%s:%s:%s", c.cfg.KeyPrefix, accountID.String(), key)
}

func (c *RedisResponseCache) Get(ctx context.Context, accountID uuid.UUID, key string) (domain.IdempotencyResponse, bool, error) {
	raw, err := c.client.Get(ctx, c.cacheKey(accountID, key)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return domain.IdempotencyResponse{}, false, nil
		}
		return domain.IdempotencyResponse{}, false, fmt.Errorf("get idempotency cache: %w", err)
	}

	resp, ok := domain.ParseIdempotencyResponse(raw)
	if !ok {
		return domain.IdempotencyResponse{}, false, nil
	}
	return resp, true, nil
}

func (c *RedisResponseCache) Set(ctx context.Context, accountID uuid.UUID, key string, resp domain.IdempotencyResponse) error {
	raw, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshal idempotency cache: %w", err)
	}
	if err := c.client.Set(ctx, c.cacheKey(accountID, key), raw, c.cfg.TTL).Err(); err != nil {
		return fmt.Errorf("set idempotency cache: %w", err)
	}
	return nil
}
