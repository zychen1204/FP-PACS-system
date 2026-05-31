package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"pacs/backend/internal/models"

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
		PoolSize:     50,
		MinIdleConns: 10,
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
// scope is a namespace for the APB key (e.g., gate tier "1" or "2").
// Returns true if the swipe is allowed, false if it violates APB.
func (r *RedisCache) CheckAntiPassback(ctx context.Context, scope, badgeID, direction string) (bool, error) {
	key := fmt.Sprintf("apb:%s:%s", scope, badgeID)

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

// GetState returns the last recorded direction for a badge in the given scope.
// Returns "" if no record exists. Used for tier hierarchy validation.
func (r *RedisCache) GetState(ctx context.Context, scope, badgeID string) (string, error) {
	key := fmt.Sprintf("apb:%s:%s", scope, badgeID)
	state, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return state, err
}

// SetDirection updates the last known direction for anti-passback
func (r *RedisCache) SetDirection(ctx context.Context, scope, badgeID, direction string) error {
	key := fmt.Sprintf("apb:%s:%s", scope, badgeID)
	return r.client.Set(ctx, key, direction, 24*time.Hour).Err()
}

// SetDirectionAndPublishEvent atomically updates APB state and appends the
// access event to the Redis Stream. This keeps the success response from
// observing a half-written Redis state.
func (r *RedisCache) SetDirectionAndPublishEvent(ctx context.Context, scope, badgeID, direction, streamName string, event models.AccessEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("apb:%s:%s", scope, badgeID)
	pipe := r.client.TxPipeline()
	pipe.Set(ctx, key, direction, 24*time.Hour)
	pipe.XAdd(ctx, &redis.XAddArgs{
		Stream: streamName,
		Values: map[string]interface{}{"data": string(data)},
	})
	_, err = pipe.Exec(ctx)
	return err
}

// GetActiveSite returns the site_id the badge is currently checked in to.
// Returns "" if the badge is not inside any site.
func (r *RedisCache) GetActiveSite(ctx context.Context, badgeID string) (string, error) {
	key := fmt.Sprintf("active_site:%s", badgeID)
	site, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return site, err
}

// SetActiveSite records that a badge has entered the given site (Tier-1 IN).
func (r *RedisCache) SetActiveSite(ctx context.Context, badgeID, siteID string) error {
	key := fmt.Sprintf("active_site:%s", badgeID)
	return r.client.Set(ctx, key, siteID, 24*time.Hour).Err()
}

// ClearActiveSite removes the active site record when a badge exits Tier-1.
func (r *RedisCache) ClearActiveSite(ctx context.Context, badgeID string) error {
	key := fmt.Sprintf("active_site:%s", badgeID)
	return r.client.Del(ctx, key).Err()
}

// Close closes the Redis connection
func (r *RedisCache) Close() error {
	return r.client.Close()
}
