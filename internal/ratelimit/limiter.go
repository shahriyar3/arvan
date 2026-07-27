package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type AllowResult struct {
	Allowed    bool
	RetryAfter time.Duration
}

type Limiter interface {
	Allow(ctx context.Context, accountID uuid.UUID) (AllowResult, error)
}

type RedisConfig struct {
	Window   time.Duration
	Limit    int64
	KeyPrefix string
}

type RedisLimiter struct {
	client *redis.Client
	cfg    RedisConfig
}

func NewRedisLimiter(client *redis.Client, cfg RedisConfig) *RedisLimiter {
	if cfg.Window <= 0 {
		cfg.Window = time.Second
	}
	if cfg.Limit <= 0 {
		cfg.Limit = 100
	}
	if cfg.KeyPrefix == "" {
		cfg.KeyPrefix = "ratelimit"
	}
	return &RedisLimiter{client: client, cfg: cfg}
}

func (l *RedisLimiter) Allow(ctx context.Context, accountID uuid.UUID) (AllowResult, error) {
	key := fmt.Sprintf("%s:%s", l.cfg.KeyPrefix, accountID.String())
	now := time.Now()
	windowStart := now.Add(-l.cfg.Window)

	pipe := l.client.Pipeline()
	pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart.UnixNano()))
	add := pipe.ZAdd(ctx, key, redis.Z{Score: float64(now.UnixNano()), Member: now.UnixNano()})
	pipe.Expire(ctx, key, l.cfg.Window+time.Second)
	countCmd := pipe.ZCard(ctx, key)

	if _, err := pipe.Exec(ctx); err != nil {
		return AllowResult{}, fmt.Errorf("rate limit redis: %w", err)
	}
	if err := add.Err(); err != nil {
		return AllowResult{}, fmt.Errorf("rate limit redis add: %w", err)
	}

	count := countCmd.Val()
	if count <= l.cfg.Limit {
		return AllowResult{Allowed: true}, nil
	}

	_ = l.client.ZRem(ctx, key, now.UnixNano())

	oldest, err := l.client.ZRangeWithScores(ctx, key, 0, 0).Result()
	if err != nil {
		return AllowResult{Allowed: false, RetryAfter: l.cfg.Window}, nil
	}
	if len(oldest) == 0 {
		return AllowResult{Allowed: false, RetryAfter: l.cfg.Window}, nil
	}

	retryAfter := time.Until(time.Unix(0, int64(oldest[0].Score)).Add(l.cfg.Window))
	if retryAfter < time.Second {
		retryAfter = time.Second
	}

	return AllowResult{Allowed: false, RetryAfter: retryAfter}, nil
}
