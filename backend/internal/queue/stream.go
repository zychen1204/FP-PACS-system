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
	StreamName     = "pacs:events"
	GroupName      = "event-processors"
	DeadStreamName = "pacs:events:dead"
	// MaxRetries: handler 連續失敗達此次數後該則消息進 DLQ 並 XACK 主 stream。
	// HW2 §5.3 列為 Pub/Sub + DLQ Phase 2 升級項。
	MaxRetries = 3
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

// CreateConsumerGroup creates the default consumer group (idempotent)
func (s *RedisStream) CreateConsumerGroup(ctx context.Context) error {
	return s.CreateNamedConsumerGroup(ctx, GroupName)
}

// CreateNamedConsumerGroup creates an arbitrary consumer group (idempotent).
// 每個獨立服務（event-processor / anomaly-detector）用自己的 group。
func (s *RedisStream) CreateNamedConsumerGroup(ctx context.Context, group string) error {
	err := s.client.XGroupCreateMkStream(ctx, StreamName, group, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return err
	}
	return nil
}

// ConsumeEvents reads with default group; preserves original signature for event-processor.
func (s *RedisStream) ConsumeEvents(ctx context.Context, consumerName string, handler func(event models.AccessEvent) error) error {
	return s.ConsumeEventsWithGroup(ctx, GroupName, consumerName, handler)
}

// ConsumeEventsWithGroup is the general-purpose loop with explicit group name + DLQ.
//
// 失敗處理：對同一 msg.ID 累計重試 MaxRetries 次；超過則把 message 推到
// pacs:events:dead（含 original_id / error / consumer / failed_at）並 XACK
// 主 stream，避免無限重試卡住消費。
//
// 重試計數採用 in-memory map（key=msg.ID）。XREADGROUP `>` 只回覆未投遞給該
// consumer 的訊息；若 handler 回 error 不 XACK，下一輪 `>` 不會再回同一則，
// 必須走 XAUTOCLAIM/XPENDING 才會重投遞。本實作對 retry 採「同一輪 in-place
// retry up to MaxRetries 次」的簡化版（與真實 Pub/Sub backoff 不同，但對 demo
// 足以驗證 DLQ 行為）。
func (s *RedisStream) ConsumeEventsWithGroup(ctx context.Context, group, consumerName string, handler func(event models.AccessEvent) error) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		streams, err := s.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    group,
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
					fmt.Printf("[STREAM] Unmarshal error: %v — sending to DLQ\n", err)
					s.toDLQ(ctx, msg.ID, data, err, consumerName)
					s.client.XAck(ctx, StreamName, group, msg.ID)
					continue
				}

				var lastErr error
				for attempt := 0; attempt < MaxRetries; attempt++ {
					if err := handler(event); err == nil {
						lastErr = nil
						break
					} else {
						lastErr = err
						fmt.Printf("[STREAM] Handler error attempt %d/%d: %v\n", attempt+1, MaxRetries, err)
						time.Sleep(500 * time.Millisecond)
					}
				}

				if lastErr != nil {
					fmt.Printf("[STREAM] Exhausted retries; sending to DLQ\n")
					s.toDLQ(ctx, msg.ID, data, lastErr, consumerName)
				}
				// 不論 success 或 DLQ 都 ACK 主 stream，避免無限重投
				s.client.XAck(ctx, StreamName, group, msg.ID)
			}
		}
	}
}

// toDLQ pushes a failed message to pacs:events:dead with diagnostic metadata.
func (s *RedisStream) toDLQ(ctx context.Context, originalID, data string, cause error, consumer string) {
	err := s.client.XAdd(ctx, &redis.XAddArgs{
		Stream: DeadStreamName,
		Values: map[string]interface{}{
			"data":        data,
			"original_id": originalID,
			"error":       cause.Error(),
			"consumer":    consumer,
			"failed_at":   time.Now().UTC().Format(time.RFC3339Nano),
		},
	}).Err()
	if err != nil {
		fmt.Printf("[DLQ-ERR] failed to push to %s: %v\n", DeadStreamName, err)
	}
}

// Close closes the Redis connection
func (s *RedisStream) Close() error {
	return s.client.Close()
}
