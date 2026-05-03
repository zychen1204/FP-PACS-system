package main

import (
	"fmt"

	"pacs/backend/handlers"
	"pacs/backend/models"

	"github.com/gin-gonic/gin"
)

var state *handlers.SharedState

func main() {
	fmt.Println(`
    ╔════════════════════════════════════════════════════════╗
    ║  PACS Backend Server (Go)                              ║
    ║  Cloud-Native Physical Access Control System           ║
    ╚════════════════════════════════════════════════════════╝
    
    ✓ Access API:       http://localhost:8080/v1/swipe
    ✓ Reporting API:    http://localhost:8081/v1/reports/attendance
    ✓ Health Check:     http://localhost:8080/healthz
    ✓ Metrics:          http://localhost:8080/metrics
    `)

	// Initialize shared state
	state = &handlers.SharedState{
		AccessLog:    make([]models.AccessLog, 0, 1000),
		AntiPassback: make(map[string]string),
	}

	// Create Access API router (Port 8080)
	go runAccessAPI()

	// Create Reporting API router (Port 8081)
	runReportingAPI()
}

func runAccessAPI() {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	// Add CORS middleware
	router.Use(corsMiddleware())

	// Create handler with shared state
	accessHandler := handlers.NewAccessHandler(state)

	// Routes
	router.GET("/healthz", accessHandler.HealthCheck)
	router.POST("/v1/swipe", accessHandler.HandleSwipe)
	router.GET("/metrics", accessHandler.GetMetrics)

	fmt.Println("🔐 Starting Access API on port 8080...")
	if err := router.Run(":8080"); err != nil {
		fmt.Printf("Failed to start Access API: %v\n", err)
	}
}

func runReportingAPI() {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	// Add CORS middleware
	router.Use(corsMiddleware())

	// Create handler with shared state
	reportingHandler := handlers.NewReportingHandler(state)

	// Routes
	router.GET("/healthz", reportingHandler.HealthCheck)
	router.GET("/readyz", reportingHandler.ReadinessCheck)
	router.GET("/v1/reports/attendance", reportingHandler.GetAttendanceReport)

	fmt.Println("📊 Starting Reporting API on port 8081...")
	if err := router.Run(":8081"); err != nil {
		fmt.Printf("Failed to start Reporting API: %v\n", err)
	}
}

// corsMiddleware adds CORS headers to all responses
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
