package api

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisRateLimiter mengimplementasikan RateLimiter menggunakan Redis SET NX EX,
// aman untuk deployment multi-instance (cooldown terbagi lintas replica).
type RedisRateLimiter struct {
	client *redis.Client
}

// NewRedisRateLimiter membuat limiter dari REDIS_URL (mis. redis://host:6379/0).
func NewRedisRateLimiter(redisURL string) (*RedisRateLimiter, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	return &RedisRateLimiter{client: redis.NewClient(opt)}, nil
}

// Ping memverifikasi konektivitas Redis (dipakai saat bootstrap).
func (r *RedisRateLimiter) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

// Close menutup koneksi Redis.
func (r *RedisRateLimiter) Close() error { return r.client.Close() }

// Allow melakukan SET key placeholder NX EX window. Bila SET berhasil (true),
// aksi diizinkan; bila key sudah ada (false), masih dalam cooldown.
func (r *RedisRateLimiter) Allow(ctx context.Context, key string, window time.Duration) (bool, error) {
	ok, err := r.client.SetNX(ctx, "ratelimit:"+key, 1, window).Result()
	if err != nil {
		return false, fmt.Errorf("redis setnx: %w", err)
	}
	return ok, nil
}
