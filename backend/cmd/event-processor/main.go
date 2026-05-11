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

	"pacs/backend/internal/db"
	"pacs/backend/internal/models"
	"pacs/backend/internal/queue"

	"github.com/gin-gonic/gin"
)

var (
	database    *db.PostgresDB
	eventStream *queue.RedisStream
	startTime   = time.Now()
	processed   int64
	errCount    int64
)

func main() {
	fmt.Println(`
  ╔═══════════════════════════════════════════════════╗
  ║  PACS Event Processor                             ║
  ║  Redis Stream → PostgreSQL Writer                 ║
  ╚═══════════════════════════════════════════════════╝`)

	var err error

	database, err = db.NewPostgresDB()
	if err != nil {
		fmt.Printf("❌ PostgreSQL connection failed: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = database.Close() }()
	fmt.Println("✅ PostgreSQL connected")

	eventStream, err = queue.NewRedisStream()
	if err != nil {
		fmt.Printf("❌ Redis Stream connection failed: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = eventStream.Close() }()
	fmt.Println("✅ Redis Stream connected")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if cerr := eventStream.CreateConsumerGroup(ctx); cerr != nil {
		fmt.Printf("⚠️ Consumer group warning: %v\n", cerr)
	}
	fmt.Println("✅ Consumer group ready")

	// Health check server
	go runHealthServer()

	// Graceful shutdown
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		fmt.Println("\n🛑 Shutting down Event Processor...")
		cancel()
	}()

	fmt.Println("🔄 Listening for events on stream 'pacs:events'...")

	hostname, _ := os.Hostname()
	err = eventStream.ConsumeEvents(ctx, fmt.Sprintf("processor-%s", hostname), func(event models.AccessEvent) error {
		if dbErr := database.InsertEvent(ctx, event); dbErr != nil {
			atomic.AddInt64(&errCount, 1)
			return fmt.Errorf("DB insert failed: %w", dbErr)
		}
		atomic.AddInt64(&processed, 1)
		fmt.Printf("[PERSISTED] %s %s at %s (%s)\n", event.BadgeID, event.Direction, event.SiteID, event.Status)
		return nil
	})

	if err != nil && err != context.Canceled {
		fmt.Printf("❌ Consumer error: %v\n", err)
	}
}

func runHealthServer() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"service":   "event-processor",
			"processed": atomic.LoadInt64(&processed),
			"errors":    atomic.LoadInt64(&errCount),
			"uptime":    time.Since(startTime).String(),
		})
	})

	port := envOrDefault("HEALTH_PORT", "8082")
	fmt.Printf("📡 Health endpoint on :%s\n", port)
	if err := r.Run(":" + port); err != nil {
		fmt.Printf("❌ Health server error: %v\n", err)
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
