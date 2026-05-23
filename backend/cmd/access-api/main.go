package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"strings"

	"pacs/backend/internal/cache"
	"pacs/backend/internal/models"
	"pacs/backend/internal/queue"

	"github.com/gin-gonic/gin"
)

var (
	redisCache  *cache.RedisCache
	eventStream *queue.RedisStream
	startTime   = time.Now()
	swipeOK     int64
	swipeFail   int64
)

func main() {
	fmt.Println(`
  ╔═══════════════════════════════════════════════════╗
  ║  PACS Access API (Write Plane)                    ║
  ║  Cloud-Native Physical Access Control System      ║
  ╚═══════════════════════════════════════════════════╝`)

	var err error

	redisCache, err = cache.NewRedisCache()
	if err != nil {
		fmt.Printf("❌ Redis connection failed: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = redisCache.Close() }()
	fmt.Println("✅ Redis cache connected")

	eventStream, err = queue.NewRedisStream()
	if err != nil {
		fmt.Printf("❌ Redis Stream connection failed: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = eventStream.Close() }()
	fmt.Println("✅ Redis Stream connected")

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery(), corsMiddleware())

	router.GET("/healthz", healthCheck)
	router.GET("/readyz", readinessCheck)
	router.GET("/metrics", getMetrics)
	router.POST("/v1/swipe", handleSwipe)

	port := envOrDefault("PORT", "8080")

	srv := &http.Server{Addr: ":" + port, Handler: router}

	go func() {
		fmt.Printf("🔐 Access API listening on :%s\n", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("❌ Server error: %v\n", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	fmt.Println("\n🛑 Shutting down Access API...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		fmt.Printf("❌ Shutdown error: %v\n", err)
	}
}

// gateTier extracts the tier digit from a gate_id.
// Supports two formats used in the system:
//   - spec format  "1-A", "2-B"  → first part before '-'
//   - HTML format  "Gate-1A", "Gate-2A" → first digit found after '-'
func gateTier(gateID string) string {
	for _, part := range strings.SplitN(gateID, "-", 2) {
		if len(part) > 0 && part[0] >= '1' && part[0] <= '9' {
			return string(part[0])
		}
	}
	return "1" // default to tier 1
}

func handleSwipe(c *gin.Context) {
	var req models.SwipeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.SwipeResponse{
			Status:    "ERROR",
			Message:   "Invalid request body",
			ErrorCode: "ERR_INVALID_REQUEST",
		})
		return
	}

	// 0103: 客戶端可送 event_time 覆寫 server time（壓測/批次回放歷史事件用）。
	// 解析失敗一律回 400 — 不靜默 fallback，否則壓測會誤以為時間有生效。
	eventTS := time.Now().UTC()
	if req.EventTime != "" {
		ts, err := time.Parse(time.RFC3339, req.EventTime)
		if err != nil {
			c.JSON(http.StatusBadRequest, models.SwipeResponse{
				Status:    "ERROR",
				Message:   "event_time must be RFC3339, e.g. 2026-05-23T08:30:00Z",
				ErrorCode: "ERR_INVALID_EVENT_TIME",
			})
			return
		}
		eventTS = ts.UTC()
	}

	ctx := c.Request.Context()
	tier := gateTier(req.GateID)
	siteKey := req.SiteID
	if siteKey == "" {
		siteKey = "global"
	}
	t1Scope := siteKey + ":1" // e.g. "FAB12-A:1"
	t2Scope := siteKey + ":2"
	apbScope := siteKey + ":" + tier

	// Cross-site exclusivity: a badge may only be physically inside one site at a time.
	// Checked only on Tier-1 IN, which is the entry point of every site.
	if req.Direction == "IN" && tier == "1" {
		activeSite, err := redisCache.GetActiveSite(ctx, req.BadgeID)
		if err != nil {
			fmt.Printf("[WARN] Redis unavailable (cross-site check), fail-closed: %v\n", err)
			c.JSON(http.StatusServiceUnavailable, models.SwipeResponse{
				Status:    "ERROR",
				Message:   "Access control unavailable, please retry",
				ErrorCode: "ERR_SYSTEM_UNAVAILABLE",
			})
			return
		}
		if activeSite != "" && activeSite != siteKey {
			atomic.AddInt64(&swipeFail, 1)
			reason := fmt.Sprintf("已在廠區 %s 內，請先刷出", activeSite)
			if !publishSwipeEvent(c, models.AccessEvent{
				BadgeID: req.BadgeID, SiteID: req.SiteID, GateID: req.GateID,
				Direction: req.Direction, Status: "REJECTED_APB",
				Reason: reason, Timestamp: eventTS,
			}) {
				return
			}
			fmt.Printf("[REJECTED] Cross-site: %s still inside %s, blocked at %s\n", req.BadgeID, activeSite, siteKey)
			c.JSON(http.StatusForbidden, models.SwipeResponse{
				Status:    "REJECTED_APB",
				Reason:    reason,
				Message:   "Badge already checked in at another site",
				ErrorCode: "ERR_CROSS_SITE",
			})
			return
		}
	}

	// Strict tier hierarchy (no cascade):
	// IN:  Tier-2 requires Tier-1 already IN (same site).
	// OUT: Tier-1 requires Tier-2 already OUT or never entered (same site).
	if req.Direction == "IN" && tier == "2" {
		state, err := redisCache.GetState(ctx, t1Scope, req.BadgeID)
		if err != nil {
			fmt.Printf("[WARN] Redis unavailable (tier check), fail-closed: %v\n", err)
			c.JSON(http.StatusServiceUnavailable, models.SwipeResponse{
				Status:    "ERROR",
				Message:   "Access control unavailable, please retry",
				ErrorCode: "ERR_SYSTEM_UNAVAILABLE",
			})
			return
		}
		if state != "IN" {
			atomic.AddInt64(&swipeFail, 1)
			if !publishSwipeEvent(c, models.AccessEvent{
				BadgeID: req.BadgeID, SiteID: req.SiteID, GateID: req.GateID,
				Direction: req.Direction, Status: "REJECTED_APB",
				Reason: "未進入外層閘門", Timestamp: eventTS,
			}) {
				return
			}
			fmt.Printf("[REJECTED] Tier-2 IN without Tier-1: %s at %s\n", req.BadgeID, req.GateID)
			c.JSON(http.StatusForbidden, models.SwipeResponse{
				Status:    "REJECTED_APB",
				Reason:    "未進入外層閘門",
				Message:   "Must enter outer gate first",
				ErrorCode: "ERR_TIER_VIOLATION",
			})
			return
		}
	}
	if req.Direction == "OUT" && tier == "1" {
		state, err := redisCache.GetState(ctx, t2Scope, req.BadgeID)
		if err != nil {
			fmt.Printf("[WARN] Redis unavailable (tier check), fail-closed: %v\n", err)
			c.JSON(http.StatusServiceUnavailable, models.SwipeResponse{
				Status:    "ERROR",
				Message:   "Access control unavailable, please retry",
				ErrorCode: "ERR_SYSTEM_UNAVAILABLE",
			})
			return
		}
		if state == "IN" {
			atomic.AddInt64(&swipeFail, 1)
			if !publishSwipeEvent(c, models.AccessEvent{
				BadgeID: req.BadgeID, SiteID: req.SiteID, GateID: req.GateID,
				Direction: req.Direction, Status: "REJECTED_APB",
				Reason: "請先刷出內層閘門", Timestamp: eventTS,
			}) {
				return
			}
			fmt.Printf("[REJECTED] Tier-1 OUT while Tier-2 still IN: %s at %s\n", req.BadgeID, req.GateID)
			c.JSON(http.StatusForbidden, models.SwipeResponse{
				Status:    "REJECTED_APB",
				Reason:    "請先刷出內層閘門",
				Message:   "Must exit inner gate first",
				ErrorCode: "ERR_TIER_VIOLATION",
			})
			return
		}
	}

	// Check anti-passback via Redis cache (FR-2).
	// Fail-closed per design doc §5.2: Redis unavailable → 503, never bypass APB.
	allowed, err := redisCache.CheckAntiPassback(ctx, apbScope, req.BadgeID, req.Direction)
	if err != nil {
		fmt.Printf("[WARN] Redis unavailable, fail-closed: %v\n", err)
		c.JSON(http.StatusServiceUnavailable, models.SwipeResponse{
			Status:    "ERROR",
			Message:   "Access control unavailable, please retry",
			ErrorCode: "ERR_SYSTEM_UNAVAILABLE",
		})
		return
	}

	event := models.AccessEvent{
		BadgeID:   req.BadgeID,
		SiteID:    req.SiteID,
		GateID:    req.GateID,
		Direction: req.Direction,
		Timestamp: eventTS,
	}

	if !allowed {
		event.Status = "REJECTED_APB"
		event.Reason = "Anti-Passback Violation"
		atomic.AddInt64(&swipeFail, 1)

		if !publishSwipeEvent(c, event) {
			return
		}

		fmt.Printf("[REJECTED] APB violation: %s at %s\n", req.BadgeID, req.GateID)
		c.JSON(http.StatusForbidden, models.SwipeResponse{
			Status:    "REJECTED_APB",
			Reason:    "Anti-Passback Violation",
			Message:   "Anti-Passback Violation",
			ErrorCode: "ERR_ANTI_PASSBACK",
		})
		return
	}

	event.Status = "SUCCESS"

	if !commitSuccessfulSwipe(c, apbScope, event) {
		return
	}
	atomic.AddInt64(&swipeOK, 1)

	// Keep active_site in sync with Tier-1 state.
	if tier == "1" {
		if req.Direction == "IN" {
			_ = redisCache.SetActiveSite(ctx, req.BadgeID, siteKey)
		} else {
			_ = redisCache.ClearActiveSite(ctx, req.BadgeID)
		}
	}

	fmt.Printf("[SUCCESS] %s %s at %s/%s\n", req.BadgeID, req.Direction, siteKey, req.GateID)
	c.JSON(http.StatusOK, models.SwipeResponse{
		Status:  "SUCCESS",
		Message: "Access granted",
	})
}

func publishSwipeEvent(c *gin.Context, event models.AccessEvent) bool {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	if err := eventStream.PublishEvent(ctx, event); err != nil {
		fmt.Printf("[WARN] PublishEvent failed, fail-closed: %v\n", err)
		c.JSON(http.StatusServiceUnavailable, models.SwipeResponse{
			Status:    "ERROR",
			Message:   "Access control unavailable, please retry",
			ErrorCode: "ERR_SYSTEM_UNAVAILABLE",
		})
		return false
	}
	return true
}

func commitSuccessfulSwipe(c *gin.Context, scope string, event models.AccessEvent) bool {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	if err := redisCache.SetDirectionAndPublishEvent(ctx, scope, event.BadgeID, event.Direction, queue.StreamName, event); err != nil {
		fmt.Printf("[WARN] Failed to commit APB state and event, fail-closed: %v\n", err)
		c.JSON(http.StatusServiceUnavailable, models.SwipeResponse{
			Status:    "ERROR",
			Message:   "Access control unavailable, please retry",
			ErrorCode: "ERR_SYSTEM_UNAVAILABLE",
		})
		return false
	}
	return true
}

func healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy", "service": "access-api",
		"uptime": time.Since(startTime).String(),
	})
}

func readinessCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ready": true})
}

func getMetrics(c *gin.Context) {
	ok := atomic.LoadInt64(&swipeOK)
	fail := atomic.LoadInt64(&swipeFail)
	m := fmt.Sprintf(`# HELP pacs_swipe_total Total badge swipes
# TYPE pacs_swipe_total counter
pacs_swipe_total{status="success"} %d
pacs_swipe_total{status="rejected"} %d
# HELP pacs_uptime_seconds Uptime
# TYPE pacs_uptime_seconds gauge
pacs_uptime_seconds %f
`, ok, fail, time.Since(startTime).Seconds())
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(m))
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
