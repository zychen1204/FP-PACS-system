package cache

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// newTestCache creates a RedisCache backed by an in-process miniredis server.
// The server and client are automatically cleaned up when the test ends.
func newTestCache(t *testing.T) *RedisCache {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	return &RedisCache{client: client}
}

// ── CheckAntiPassback ──────────────────────────────────────────────────────

// FR-2: 首次刷卡（無狀態）應被允許
func TestCheckAntiPassback_FirstSwipe_Allowed(t *testing.T) {
	c := newTestCache(t)
	ok, err := c.CheckAntiPassback(context.Background(), "Site-A", "B001", "IN")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("first swipe with no prior state should be allowed")
	}
}

// FR-2: 相同方向應被 Anti-Passback 拒絕
func TestCheckAntiPassback_SameDirection_Rejected(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	_ = c.SetDirection(ctx, "Site-A", "B002", "IN")

	ok, err := c.CheckAntiPassback(ctx, "Site-A", "B002", "IN")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("same direction should be rejected (APB violation)")
	}
}

// FR-2: 不同方向應被允許（IN→OUT）
func TestCheckAntiPassback_OppositeDirection_Allowed(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	_ = c.SetDirection(ctx, "Site-A", "B003", "IN")

	ok, err := c.CheckAntiPassback(ctx, "Site-A", "B003", "OUT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("opposite direction (IN→OUT) should be allowed")
	}
}

// FR-2: OUT→IN 也應允許
func TestCheckAntiPassback_OutThenIn_Allowed(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	_ = c.SetDirection(ctx, "Site-A", "B004", "OUT")

	ok, _ := c.CheckAntiPassback(ctx, "Site-A", "B004", "IN")
	if !ok {
		t.Error("OUT→IN should be allowed")
	}
}

// FR-2: 不同 site 的 APB 狀態互相獨立
func TestCheckAntiPassback_DifferentSite_Independent(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	_ = c.SetDirection(ctx, "Site-A", "B005", "IN")

	// Site-B 的狀態應獨立，首次刷卡應允許
	ok, _ := c.CheckAntiPassback(ctx, "Site-B", "B005", "IN")
	if !ok {
		t.Error("APB state should be per site; Site-B should be independent from Site-A")
	}
}

// ── SetDirection ───────────────────────────────────────────────────────────

// FR-2: SetDirection 寫入後，同方向 check 應回 false
func TestSetDirection_Persists(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	if err := c.SetDirection(ctx, "Site-A", "B006", "OUT"); err != nil {
		t.Fatalf("SetDirection: %v", err)
	}

	ok, _ := c.CheckAntiPassback(ctx, "Site-A", "B006", "OUT")
	if ok {
		t.Error("SetDirection should persist: same direction should now be rejected")
	}
}

// FR-2: SetDirection 可覆蓋先前狀態
func TestSetDirection_Overwrites_PreviousState(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	_ = c.SetDirection(ctx, "Site-A", "B007", "IN")
	_ = c.SetDirection(ctx, "Site-A", "B007", "OUT") // overwrite

	// last direction = OUT, so another OUT should be rejected
	ok, _ := c.CheckAntiPassback(ctx, "Site-A", "B007", "OUT")
	if ok {
		t.Error("SetDirection should overwrite: OUT should now be rejected after OUT→OUT update")
	}

	// but IN should be allowed
	ok, _ = c.CheckAntiPassback(ctx, "Site-A", "B007", "IN")
	if !ok {
		t.Error("IN should be allowed after last direction = OUT")
	}
}
