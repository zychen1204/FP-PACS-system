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
	defer redisCache.Close()
	fmt.Println("✅ Redis cache connected")

	eventStream, err = queue.NewRedisStream()
	if err != nil {
		fmt.Printf("❌ Redis Stream connection failed: %v\n", err)
		os.Exit(1)
	}
	defer eventStream.Close()
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
	srv.Shutdown(ctx)
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

	ctx := c.Request.Context()

	// Check anti-passback via Redis cache (FR2)
	allowed, err := redisCache.CheckAntiPassback(ctx, req.SiteID, req.BadgeID, req.Direction)
	if err != nil {
		fmt.Printf("[WARN] Redis error, fail-open: %v\n", err)
		allowed = true // Fail-open for availability (NFR3)
	}

	event := models.AccessEvent{
		BadgeID:   req.BadgeID,
		SiteID:    req.SiteID,
		GateID:    req.GateID,
		Direction: req.Direction,
		Timestamp: time.Now().UTC(),
	}

	if !allowed {
		event.Status = "REJECTED_APB"
		event.Reason = "Anti-Passback Violation"
		atomic.AddInt64(&swipeFail, 1)

		// Async persist via Message Queue (FR4)
		go eventStream.PublishEvent(context.Background(), event)

		fmt.Printf("[REJECTED] APB violation: %s at %s\n", req.BadgeID, req.SiteID)
		c.JSON(http.StatusForbidden, models.SwipeResponse{
			Status:    "REJECTED_APB",
			Message:   "Anti-Passback Violation",
			ErrorCode: "ERR_ANTI_PASSBACK",
		})
		return
	}

	// Update APB state in cache
	if err := redisCache.SetDirection(ctx, req.SiteID, req.BadgeID, req.Direction); err != nil {
		fmt.Printf("[WARN] Failed to update APB state: %v\n", err)
	}

	event.Status = "SUCCESS"
	atomic.AddInt64(&swipeOK, 1)

	// Async persist via Message Queue (FR4)
	go eventStream.PublishEvent(context.Background(), event)

	fmt.Printf("[SUCCESS] %s %s at %s/%s\n", req.BadgeID, req.Direction, req.SiteID, req.GateID)
	c.JSON(http.StatusOK, models.SwipeResponse{
		Status:  "SUCCESS",
		Message: "Access granted",
	})
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
