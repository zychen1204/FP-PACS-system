package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"pacs/backend/internal/models"

	"github.com/redis/go-redis/v9"
)

const (
	StreamName = "pacs:events"
	GroupName  = "event-processors"
)

// RedisStream handles event publishing and consuming via Redis Streams
type RedisStream struct {
	client *redis.Client
}

// NewRedisStream creates a new Redis Streams connection
func NewRedisStream() (*RedisStream, error) {
	host := os.Getenv("REDIS_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("REDIS_PORT")
	if port == "" {
		port = "6379"
	}

	client := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%s", host, port),
		DB:   0,
	})

	var err error
	for i := 0; i < 15; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err = client.Ping(ctx).Err()
		cancel()
		if err == nil {
			break
		}
		fmt.Printf("[STREAM] Waiting for Redis... (%d/15)\n", i+1)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		return nil, fmt.Errorf("redis stream connection failed: %w", err)
	}

	return &RedisStream{client: client}, nil
}

// PublishEvent publishes an access event to the Redis Stream
func (s *RedisStream) PublishEvent(ctx context.Context, event models.AccessEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return s.client.XAdd(ctx, &redis.XAddArgs{
		Stream: StreamName,
		Values: map[string]interface{}{"data": string(data)},
	}).Err()
}

// CreateConsumerGroup creates the consumer group (idempotent)
func (s *RedisStream) CreateConsumerGroup(ctx context.Context) error {
	err := s.client.XGroupCreateMkStream(ctx, StreamName, GroupName, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return err
	}
	return nil
}

// ConsumeEvents reads and processes events from the Redis Stream
func (s *RedisStream) ConsumeEvents(ctx context.Context, consumerName string, handler func(event models.AccessEvent) error) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		streams, err := s.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    GroupName,
			Consumer: consumerName,
			Streams:  []string{StreamName, ">"},
			Count:    10,
			Block:    1 * time.Second,
		}).Result()

		if err != nil {
			if err == redis.Nil || ctx.Err() != nil {
				continue
			}
			fmt.Printf("[STREAM] Read error: %v\n", err)
			time.Sleep(1 * time.Second)
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				data, ok := msg.Values["data"].(string)
				if !ok {
					continue
				}

				var event models.AccessEvent
				if err := json.Unmarshal([]byte(data), &event); err != nil {
					fmt.Printf("[STREAM] Unmarshal error: %v\n", err)
					continue
				}

				if err := handler(event); err != nil {
					fmt.Printf("[STREAM] Handler error: %v, will retry\n", err)
					time.Sleep(500 * time.Millisecond)
					continue
				}

				s.client.XAck(ctx, StreamName, GroupName, msg.ID)
			}
		}
	}
}

// Close closes the Redis connection
func (s *RedisStream) Close() error {
	return s.client.Close()
}
