package queue

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"pacs/backend/internal/models"
)

// newTestStream starts a miniredis instance, wires the env vars that
// NewRedisStream reads, and returns a connected RedisStream + a plain
// redis.Client for inspection (e.g. XLEN checks).
func newTestStream(t *testing.T) (*RedisStream, *redis.Client) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)

	t.Setenv("REDIS_HOST", mr.Host())
	t.Setenv("REDIS_PORT", mr.Port())

	s, err := NewRedisStream()
	if err != nil {
		t.Fatalf("NewRedisStream: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })

	return s, rc
}

func testEvent(badgeID string) models.AccessEvent {
	return models.AccessEvent{
		BadgeID:   badgeID,
		SiteID:    "Site-A",
		GateID:    "G1",
		Direction: "IN",
		Status:    "SUCCESS",
		Timestamp: time.Now().UTC(),
	}
}

// ── PublishEvent ───────────────────────────────────────────────────────────

// FR-4: 每次 publish 都應讓 stream 長度 +1
func TestPublishEvent_IncreasesStreamLength(t *testing.T) {
	s, rc := newTestStream(t)
	ctx := context.Background()

	before, _ := rc.XLen(ctx, StreamName).Result()

	if err := s.PublishEvent(ctx, testEvent("PUB_TEST")); err != nil {
		t.Fatalf("PublishEvent: %v", err)
	}

	after, _ := rc.XLen(ctx, StreamName).Result()
	if after != before+1 {
		t.Errorf("stream length: before=%d after=%d, expected +1", before, after)
	}
}

// FR-4: 連續發布多筆事件，每筆都進 stream
func TestPublishEvent_MultipleEvents(t *testing.T) {
	s, rc := newTestStream(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if err := s.PublishEvent(ctx, testEvent(fmt.Sprintf("MULTI_%d", i))); err != nil {
			t.Fatalf("PublishEvent[%d]: %v", i, err)
		}
	}
	n, _ := rc.XLen(ctx, StreamName).Result()
	if n < 5 {
		t.Errorf("expected at least 5 messages in stream, got %d", n)
	}
}

// ── CreateConsumerGroup ────────────────────────────────────────────────────

// 重複建立同名 group 不應回錯（BUSYGROUP 已吞掉）
func TestCreateConsumerGroup_Idempotent(t *testing.T) {
	s, _ := newTestStream(t)
	ctx := context.Background()

	if err := s.CreateConsumerGroup(ctx); err != nil {
		t.Fatalf("first CreateConsumerGroup: %v", err)
	}
	if err := s.CreateConsumerGroup(ctx); err != nil {
		t.Fatalf("second CreateConsumerGroup (idempotent): %v", err)
	}
}

func TestCreateNamedConsumerGroup_Idempotent(t *testing.T) {
	s, _ := newTestStream(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := s.CreateNamedConsumerGroup(ctx, "my-group"); err != nil {
			t.Fatalf("CreateNamedConsumerGroup call %d: %v", i+1, err)
		}
	}
}

// ── ConsumeEventsWithGroup ─────────────────────────────────────────────────

// FR-4: 消費者能正確接收並反序列化事件
func TestConsumeEventsWithGroup_ProcessesEvent(t *testing.T) {
	s, _ := newTestStream(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	group := "proc-group"
	if err := s.CreateNamedConsumerGroup(ctx, group); err != nil {
		t.Fatalf("CreateNamedConsumerGroup: %v", err)
	}

	// publish first so the first XReadGroup returns immediately
	if err := s.PublishEvent(ctx, testEvent("CONSUME_B001")); err != nil {
		t.Fatalf("PublishEvent: %v", err)
	}

	received := make(chan models.AccessEvent, 1)
	go func() {
		_ = s.ConsumeEventsWithGroup(ctx, group, "consumer-1", func(e models.AccessEvent) error {
			received <- e
			return nil
		})
	}()

	select {
	case e := <-received:
		if e.BadgeID != "CONSUME_B001" {
			t.Errorf("badge_id got=%q want=CONSUME_B001", e.BadgeID)
		}
		if e.SiteID != "Site-A" {
			t.Errorf("site_id got=%q want=Site-A", e.SiteID)
		}
	case <-ctx.Done():
		t.Fatal("timed out: event was not consumed within 4s")
	}
}

// ── DLQ (Dead-Letter Queue) ────────────────────────────────────────────────

// 處理器連續失敗 MaxRetries 次後，訊息應移到 pacs:events:dead
func TestConsumeEventsWithGroup_DLQ_OnExhaustedRetries(t *testing.T) {
	s, rc := newTestStream(t)
	// allow enough time for MaxRetries * 500ms + overhead
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	group := "dlq-group"
	if err := s.CreateNamedConsumerGroup(ctx, group); err != nil {
		t.Fatalf("CreateNamedConsumerGroup: %v", err)
	}

	if err := s.PublishEvent(ctx, testEvent("DLQ_BADGE")); err != nil {
		t.Fatalf("PublishEvent: %v", err)
	}

	before, _ := rc.XLen(ctx, DeadStreamName).Result()

	go func() {
		_ = s.ConsumeEventsWithGroup(ctx, group, "consumer-dlq", func(_ models.AccessEvent) error {
			return fmt.Errorf("handler always fails")
		})
	}()

	// Poll until DLQ receives the message or timeout
	deadline := time.Now().Add(7 * time.Second)
	var after int64
	for time.Now().Before(deadline) {
		after, _ = rc.XLen(ctx, DeadStreamName).Result()
		if after > before {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if after <= before {
		t.Errorf("DLQ should have received the failed message after %d retries: before=%d after=%d",
			MaxRetries, before, after)
	}
}

// 成功的訊息不應進 DLQ
func TestConsumeEventsWithGroup_SuccessfulMessage_NotInDLQ(t *testing.T) {
	s, rc := newTestStream(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	group := "success-group"
	_ = s.CreateNamedConsumerGroup(ctx, group)
	_ = s.PublishEvent(ctx, testEvent("SUCCESS_BADGE"))

	dlqBefore, _ := rc.XLen(ctx, DeadStreamName).Result()

	done := make(chan struct{}, 1)
	go func() {
		_ = s.ConsumeEventsWithGroup(ctx, group, "consumer-ok", func(_ models.AccessEvent) error {
			done <- struct{}{}
			return nil // success
		})
	}()

	// Wait for handler to be called
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("handler was not called within timeout")
	}

	// Give a moment for any async DLQ write that shouldn't happen
	time.Sleep(100 * time.Millisecond)
	dlqAfter, _ := rc.XLen(ctx, DeadStreamName).Result()
	if dlqAfter > dlqBefore {
		t.Error("successful message should NOT be sent to DLQ")
	}
}
