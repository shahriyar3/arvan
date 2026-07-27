package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisLimiterAllowsUnderLimit(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	limiter := NewRedisLimiter(client, RedisConfig{
		Window: time.Second,
		Limit:  3,
	})

	accountID := uuid.New()
	for i := 0; i < 3; i++ {
		result, err := limiter.Allow(context.Background(), accountID)
		require.NoError(t, err)
		assert.True(t, result.Allowed)
	}
}

func TestRedisLimiterBlocksOverLimit(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	limiter := NewRedisLimiter(client, RedisConfig{
		Window: time.Second,
		Limit:  2,
	})

	accountID := uuid.New()
	for i := 0; i < 2; i++ {
		result, err := limiter.Allow(context.Background(), accountID)
		require.NoError(t, err)
		assert.True(t, result.Allowed)
	}

	result, err := limiter.Allow(context.Background(), accountID)
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Greater(t, result.RetryAfter, time.Duration(0))
}

func TestRedisLimiterIsolatedPerAccount(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	limiter := NewRedisLimiter(client, RedisConfig{
		Window: time.Second,
		Limit:  1,
	})

	accountA := uuid.New()
	accountB := uuid.New()

	result, err := limiter.Allow(context.Background(), accountA)
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	result, err = limiter.Allow(context.Background(), accountB)
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	result, err = limiter.Allow(context.Background(), accountA)
	require.NoError(t, err)
	assert.False(t, result.Allowed)
}
