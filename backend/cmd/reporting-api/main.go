package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"pacs/backend/internal/auth"
	"pacs/backend/internal/db"
	"pacs/backend/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
)

var (
	database  Reporter // *db.PostgresDB in production; mockDB in tests
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
	defer func() { _ = database.Close() }()
	fmt.Println("✅ PostgreSQL connected")

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery(), corsMiddleware())

	router.GET("/healthz", healthCheck)
	router.GET("/readyz", readinessCheck)

	// FR-10 dev IdP：以 badge_id 發 HS256 JWT；正式環境應換真 OIDC provider
	router.POST("/v1/dev/login", devLogin)

	// FR-10 protected routes：套 JWT middleware。
	// dev/demo 環境設 DEV_AUTH_BYPASS=1（在 docker-compose），讓 frontend 免帶 token；
	// curl 模式可不設 env，改帶 `Authorization: Bearer <jwt>` 演示完整流程。
	authed := router.Group("/", auth.Middleware())
	authed.GET("/v1/reports/attendance", getAttendanceReport)
	authed.GET("/v1/audit", getAuditTrail)
	// FR-6 / FR-9 階層團隊報表 + 階層資料權限
	authed.GET("/v1/reports/manager-team", getManagerTeamReport)
	// FR-7 出勤趨勢報表（讀 mv_daily_attendance）
	authed.GET("/v1/reports/trend", getAttendanceTrend)
	// FR-8 Excel 匯出（PDF 延後）
	authed.GET("/v1/reports/attendance/export", exportAttendance)
	// FR-11 警報列表
	authed.GET("/v1/alerts", listAlerts)

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
	if err := srv.Shutdown(ctx); err != nil {
		fmt.Printf("❌ Shutdown error: %v\n", err)
	}
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

// getManagerTeamReport — FR-6 / FR-9 pattern a：
// (1) ?as=<badge_id> → lookup manager scope；空結果 → 403。
// (2) 用 scope ltree `<@` 過濾 mv_daily_attendance 取子樹。
func getManagerTeamReport(c *gin.Context) {
	badgeID := c.Query("as")
	if badgeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "as (manager badge_id) is required"})
		return
	}
	scope, err := database.GetManagerScope(c.Request.Context(), badgeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("scope lookup failed: %v", err)})
		return
	}
	if scope == "" {
		c.JSON(http.StatusForbidden, gin.H{"error": "not a manager", "badge_id": badgeID})
		return
	}
	date := c.Query("date")
	reports, err := database.QueryManagerTeamAttendance(c.Request.Context(), scope, date)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("query failed: %v", err)})
		return
	}
	if reports == nil {
		reports = []models.AttendanceReport{}
	}
	c.JSON(http.StatusOK, gin.H{"manager_scope": scope, "reports": reports})
}

// getAttendanceTrend — FR-7：日/週/月/季彙總，讀 mv_daily_attendance。
// ?as=<badge_id> 若該 badge 是 manager，限縮在其 org scope；否則不限制。
func getAttendanceTrend(c *gin.Context) {
	period := c.DefaultQuery("period", "day")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	var scope string
	if asID := c.Query("as"); asID != "" {
		if s, err := database.GetManagerScope(c.Request.Context(), asID); err == nil && s != "" {
			scope = s
		}
	}

	trends, err := database.QueryAttendanceTrend(c.Request.Context(), period, scope, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("trend query failed: %v", err)})
		return
	}
	if trends == nil {
		trends = []models.AttendanceTrend{}
	}
	c.JSON(http.StatusOK, gin.H{"scope": scope, "trends": trends})
}

// exportAttendance — FR-8：產出 Excel（xlsx），PDF 延後。
// 重用 QueryAttendance 結果；header 設 Content-Disposition: attachment。
func exportAttendance(c *gin.Context) {
	format := c.DefaultQuery("format", "excel")
	if format != "excel" && format != "xlsx" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only format=excel supported in this phase"})
		return
	}
	date := c.Query("date")
	reports, err := database.QueryAttendance(c.Request.Context(), date)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("query failed: %v", err)})
		return
	}

	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	sheet := "Attendance"
	idx, _ := f.NewSheet(sheet)
	f.SetActiveSheet(idx)
	_ = f.DeleteSheet("Sheet1")

	headers := []string{"Employee ID", "Name", "Org Path", "Work Date", "First In", "Last Out", "Swipes", "Stay Hours"}
	for col, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}
	for row, r := range reports {
		rowNum := row + 2
		firstIn := ""
		lastOut := ""
		if r.FirstIn != nil {
			firstIn = r.FirstIn.Format(time.RFC3339)
		}
		if r.LastOut != nil {
			lastOut = r.LastOut.Format(time.RFC3339)
		}
		vals := []interface{}{r.EmployeeID, r.Name, r.OrgPath, r.WorkDate, firstIn, lastOut, r.SwipeCount, r.StayHours}
		for col, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(col+1, rowNum)
			_ = f.SetCellValue(sheet, cell, v)
		}
	}

	filename := fmt.Sprintf("attendance-%s.xlsx", time.Now().Format("20060102-150405"))
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	if err := f.Write(c.Writer); err != nil {
		fmt.Printf("[ERROR] Excel write failed: %v\n", err)
	}
}

// listAlerts — FR-11 read side。?open=true 只列未處理；?limit=N 限制筆數。
func listAlerts(c *gin.Context) {
	openOnly := c.Query("open") == "true"
	limit := 100
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	alerts, err := database.QueryAlerts(c.Request.Context(), openOnly, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("alerts query failed: %v", err)})
		return
	}
	if alerts == nil {
		alerts = []models.Alert{}
	}
	c.JSON(http.StatusOK, alerts)
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

// devLogin issues an HS256 JWT for the given badge_id (dev IdP mock).
// 正式環境應改 OIDC provider redirect / callback。
func devLogin(c *gin.Context) {
	var req struct {
		BadgeID string `json:"badge_id" form:"badge_id"`
	}
	// 支援 JSON body 與 query string 兩種以方便 demo
	_ = c.ShouldBind(&req)
	if req.BadgeID == "" {
		req.BadgeID = c.Query("badge_id")
	}
	if req.BadgeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "badge_id is required"})
		return
	}
	token, err := auth.Issue(req.BadgeID, 24*time.Hour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"access_token": token, "token_type": "Bearer", "expires_in": 86400})
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
