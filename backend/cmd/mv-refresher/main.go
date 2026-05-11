package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
)

// mv-refresher — FR-7 Phase 2 升級
// HW2 §5.3：mv_daily_attendance 每 5 min refresh。
// 本服務跑 ticker，呼叫 REFRESH MATERIALIZED VIEW CONCURRENTLY。
// CONCURRENTLY 需要 MV 上有 UNIQUE 索引（0006 migration 已建）。

var (
	refreshCount   int64
	refreshErrors  int64
	lastRefreshDur int64 // nanoseconds, atomic
	startTime      = time.Now()
)

func main() {
	fmt.Println(`
  ╔═══════════════════════════════════════════════════╗
  ║  PACS MV Refresher (FR-7)                         ║
  ║  Refresh mv_daily_attendance every 5 minutes      ║
  ╚═══════════════════════════════════════════════════╝`)

	host := envOrDefault("DB_HOST", "localhost")
	port := envOrDefault("DB_PORT", "5432")
	user := envOrDefault("DB_USER", "pacs_user")
	password := envOrDefault("DB_PASSWORD", "pacs_password")
	dbName := envOrDefault("DB_NAME", "pacs_db")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbName)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Printf("❌ DB open: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// retry connection
	for i := 0; i < 30; i++ {
		if err = db.Ping(); err == nil {
			break
		}
		fmt.Printf("[DB] Waiting for PostgreSQL... (%d/30)\n", i+1)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		fmt.Printf("❌ DB ping: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ PostgreSQL connected")

	intervalStr := envOrDefault("REFRESH_INTERVAL_SECONDS", "300")
	intervalSec, _ := time.ParseDuration(intervalStr + "s")
	if intervalSec < 10*time.Second {
		intervalSec = 10 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runHealthServer()
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		fmt.Println("\n🛑 Shutting down MV Refresher...")
		cancel()
	}()

	// 啟動時先 refresh 一次
	refresh(ctx, db)

	t := time.NewTicker(intervalSec)
	defer t.Stop()
	fmt.Printf("⏱ refreshing every %s\n", intervalSec)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			refresh(ctx, db)
		}
	}
}

func refresh(ctx context.Context, db *sql.DB) {
	start := time.Now()
	_, err := db.ExecContext(ctx, "REFRESH MATERIALIZED VIEW CONCURRENTLY mv_daily_attendance")
	dur := time.Since(start)
	atomic.StoreInt64(&lastRefreshDur, int64(dur))
	if err != nil {
		atomic.AddInt64(&refreshErrors, 1)
		fmt.Printf("[REFRESH-ERR] %v (took %v)\n", err, dur)
		return
	}
	atomic.AddInt64(&refreshCount, 1)
	fmt.Printf("[REFRESH] mv_daily_attendance (%v)\n", dur)
}

func runHealthServer() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":            "healthy",
			"service":           "mv-refresher",
			"refresh_count":     atomic.LoadInt64(&refreshCount),
			"refresh_errors":    atomic.LoadInt64(&refreshErrors),
			"last_refresh_ns":   atomic.LoadInt64(&lastRefreshDur),
			"uptime":            time.Since(startTime).String(),
		})
	})
	port := envOrDefault("HEALTH_PORT", "8084")
	_ = r.Run(":" + port)
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
