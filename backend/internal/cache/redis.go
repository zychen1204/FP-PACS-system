package cache

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache handles anti-passback state via Redis
type RedisCache struct {
	client *redis.Client
}

// NewRedisCache creates a new Redis cache connection
func NewRedisCache() (*RedisCache, error) {
	host := os.Getenv("REDIS_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("REDIS_PORT")
	if port == "" {
		port = "6379"
	}

	client := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%s", host, port),
		DB:           0,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})

	// Retry connection
	var err error
	for i := 0; i < 15; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err = client.Ping(ctx).Err()
		cancel()
		if err == nil {
			break
		}
		fmt.Printf("[CACHE] Waiting for Redis... (%d/15)\n", i+1)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	return &RedisCache{client: client}, nil
}

// CheckAntiPassback checks if a badge swipe violates anti-passback rules.
// Returns true if the swipe is allowed, false if it violates APB.
func (r *RedisCache) CheckAntiPassback(ctx context.Context, siteID, badgeID, direction string) (bool, error) {
	key := fmt.Sprintf("apb:%s:%s", siteID, badgeID)

	lastDir, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return true, nil // No previous record → allow
	}
	if err != nil {
		return false, err
	}

	if lastDir == direction {
		return false, nil // Same direction → APB violation
	}

	return true, nil
}

// SetDirection updates the last known direction for anti-passback
func (r *RedisCache) SetDirection(ctx context.Context, siteID, badgeID, direction string) error {
	key := fmt.Sprintf("apb:%s:%s", siteID, badgeID)
	return r.client.Set(ctx, key, direction, 24*time.Hour).Err()
}

// Close closes the Redis connection
func (r *RedisCache) Close() error {
	return r.client.Close()
}
