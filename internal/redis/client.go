package redis

import (
	"github.com/redis/go-redis/v9"
	"github.com/shahriyar/arvan/internal/config"
)

func NewClient(cfg config.RedisConfig) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
}
