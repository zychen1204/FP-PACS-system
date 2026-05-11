package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"pacs/backend/internal/db"
	"pacs/backend/internal/models"
	"pacs/backend/internal/queue"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// anomaly-detector — FR-11
//
// 用獨立 consumer group `anomaly-detectors` 消費 pacs:events stream，
// 規則式判定後寫 alerts 表（reporting-api 讀）。
//
// 規則（demo 簡化版）：
//   1. OFF_HOURS_ENTRY  — Asia/Taipei 22:00~06:00 SUCCESS IN
//   2. APB_BURST        — 同 badge 30 分鐘內 REJECTED_APB 累計 ≥ 3 次
//   3. TAILGATING       — 同 gate 5 秒內 IN ≥ 3 次（簡化）
//
// 真實環境會再加：3σ 偏差（用 mv_daily_attendance 歷史平均）、跨日 missing OUT 等。

const consumerGroup = "anomaly-detectors"

var (
	database    *db.PostgresDB
	eventStream *queue.RedisStream
	startTime   = time.Now()
	processed   int64
	alertsRaised int64

	// in-memory state for sliding-window rules
	apbState      = make(map[string]*counter) // badge → counter
	apbStateMu    sync.Mutex
	tailgateState = make(map[string]*counter) // gate → counter
	tailgateMu    sync.Mutex
)

type counter struct {
	count    int
	windowAt time.Time
}

func main() {
	fmt.Println(`
  ╔═══════════════════════════════════════════════════╗
  ║  PACS Anomaly Detector (FR-11)                    ║
  ║  Redis Stream → rule engine → alerts table        ║
  ╚═══════════════════════════════════════════════════╝`)

	var err error
	database, err = db.NewPostgresDB()
	if err != nil {
		fmt.Printf("❌ PostgreSQL connection failed: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()
	fmt.Println("✅ PostgreSQL connected")

	eventStream, err = queue.NewRedisStream()
	if err != nil {
		fmt.Printf("❌ Redis Stream connection failed: %v\n", err)
		os.Exit(1)
	}
	defer eventStream.Close()
	fmt.Println("✅ Redis Stream connected")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := eventStream.CreateNamedConsumerGroup(ctx, consumerGroup); err != nil {
		fmt.Printf("⚠️ Consumer group warning: %v\n", err)
	}

	go runHealthServer()

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		fmt.Println("\n🛑 Shutting down Anomaly Detector...")
		cancel()
	}()

	hostname, _ := os.Hostname()
	fmt.Println("🔍 Listening for events on stream 'pacs:events'...")
	err = eventStream.ConsumeEventsWithGroup(ctx, consumerGroup, fmt.Sprintf("detector-%s", hostname),
		func(event models.AccessEvent) error {
			atomic.AddInt64(&processed, 1)
			detect(ctx, event)
			return nil
		})
	if err != nil && err != context.Canceled && err != redis.ErrClosed {
		fmt.Printf("❌ Consumer error: %v\n", err)
	}
}

// detect runs all rules against an event.
func detect(ctx context.Context, e models.AccessEvent) {
	if isOffHoursEntry(e) {
		raise(ctx, "OFF_HOURS_ENTRY", "MEDIUM", &e.BadgeID, &e.SiteID, &e.GateID,
			map[string]interface{}{"event_time": e.Timestamp.UTC(), "direction": e.Direction})
	}
	if isApbBurst(e) {
		raise(ctx, "APB_BURST", "HIGH", &e.BadgeID, &e.SiteID, &e.GateID,
			map[string]interface{}{"count_window_minutes": 30})
	}
	if isTailgating(e) {
		raise(ctx, "TAILGATING", "HIGH", nil, &e.SiteID, &e.GateID,
			map[string]interface{}{"count_window_seconds": 5})
	}
}

func isOffHoursEntry(e models.AccessEvent) bool {
	if e.Status != "SUCCESS" || e.Direction != "IN" {
		return false
	}
	tpe, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		return false
	}
	hour := e.Timestamp.In(tpe).Hour()
	return hour >= 22 || hour < 6
}

func isApbBurst(e models.AccessEvent) bool {
	if e.Status != "REJECTED_APB" {
		return false
	}
	apbStateMu.Lock()
	defer apbStateMu.Unlock()
	c, ok := apbState[e.BadgeID]
	now := time.Now()
	if !ok || now.Sub(c.windowAt) > 30*time.Minute {
		apbState[e.BadgeID] = &counter{count: 1, windowAt: now}
		return false
	}
	c.count++
	if c.count >= 3 {
		// reset to avoid duplicate alerts in same window
		delete(apbState, e.BadgeID)
		return true
	}
	return false
}

func isTailgating(e models.AccessEvent) bool {
	if e.Status != "SUCCESS" || e.Direction != "IN" {
		return false
	}
	key := e.SiteID + "/" + e.GateID
	tailgateMu.Lock()
	defer tailgateMu.Unlock()
	c, ok := tailgateState[key]
	now := time.Now()
	if !ok || now.Sub(c.windowAt) > 5*time.Second {
		tailgateState[key] = &counter{count: 1, windowAt: now}
		return false
	}
	c.count++
	if c.count >= 3 {
		delete(tailgateState, key)
		return true
	}
	return false
}

func raise(ctx context.Context, typ, sev string, badgeID, siteID, gateID *string, detail map[string]interface{}) {
	b, _ := json.Marshal(detail)
	if err := database.InsertAlert(ctx, typ, sev, badgeID, siteID, gateID, string(b)); err != nil {
		fmt.Printf("[ALERT-ERR] %s: %v\n", typ, err)
		return
	}
	atomic.AddInt64(&alertsRaised, 1)
	bid := ""
	if badgeID != nil {
		bid = *badgeID
	}
	fmt.Printf("[ALERT] %s severity=%s badge=%s\n", typ, sev, bid)
}

func runHealthServer() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":         "healthy",
			"service":        "anomaly-detector",
			"processed":      atomic.LoadInt64(&processed),
			"alerts_raised":  atomic.LoadInt64(&alertsRaised),
			"uptime":         time.Since(startTime).String(),
		})
	})
	port := envOrDefault("HEALTH_PORT", "8083")
	fmt.Printf("📡 Health endpoint on :%s\n", port)
	_ = r.Run(":" + port)
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
