package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"pacs/backend/internal/db"
	"pacs/backend/internal/models"

	"github.com/gin-gonic/gin"
)

var (
	database  *db.PostgresDB
	startTime = time.Now()
)

func main() {
	fmt.Println(`
  ╔═══════════════════════════════════════════════════╗
  ║  PACS Reporting API (Read Plane)                  ║
  ║  Attendance Reports & Audit Trail                 ║
  ╚═══════════════════════════════════════════════════╝`)

	var err error

	database, err = db.NewPostgresDB()
	if err != nil {
		fmt.Printf("❌ PostgreSQL connection failed: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()
	fmt.Println("✅ PostgreSQL connected")

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery(), corsMiddleware())

	router.GET("/healthz", healthCheck)
	router.GET("/readyz", readinessCheck)
	router.GET("/v1/reports/attendance", getAttendanceReport)
	router.GET("/v1/audit", getAuditTrail)

	port := envOrDefault("PORT", "8081")

	srv := &http.Server{Addr: ":" + port, Handler: router}

	go func() {
		fmt.Printf("📊 Reporting API listening on :%s\n", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("❌ Server error: %v\n", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	fmt.Println("\n🛑 Shutting down Reporting API...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}

func getAttendanceReport(c *gin.Context) {
	date := c.Query("date") // optional: ?date=2026-05-03

	reports, err := database.QueryAttendance(c.Request.Context(), date)
	if err != nil {
		fmt.Printf("[ERROR] Query failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query attendance"})
		return
	}

	if reports == nil {
		reports = []models.AttendanceReport{}
	}

	fmt.Printf("[REPORT] Generated %d records\n", len(reports))
	c.JSON(http.StatusOK, reports)
}

func getAuditTrail(c *gin.Context) {
	badgeID := c.Query("badge_id")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	if badgeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "badge_id is required"})
		return
	}
	if startDate == "" {
		startDate = time.Now().Format("2006-01-02")
	}
	if endDate == "" {
		endDate = time.Now().Format("2006-01-02")
	}

	events, err := database.QueryAuditTrail(c.Request.Context(), badgeID, startDate, endDate)
	if err != nil {
		fmt.Printf("[ERROR] Audit query failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query audit trail"})
		return
	}

	if events == nil {
		events = []models.AccessEvent{}
	}

	c.JSON(http.StatusOK, events)
}

func healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy", "service": "reporting-api",
		"uptime": time.Since(startTime).String(),
	})
}

func readinessCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ready": true})
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
