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

// org-sync — HW2 §5.3：模擬 LDAP/AD → DB 組織樹同步。
// Phase 2 是 CronJob，定期 upsert employees。本實作以 Go ticker 模擬 cron。
// 真實環境會：
//   1. 連 LDAP（gopkg.in/ldap.v3）抓 OU 樹
//   2. 計算 employee diff，upsert 進 employees
//   3. 失效員工 set is_active=FALSE（不刪除，FR-9 與審計需要保留歷史）
//
// 本 demo 版本以靜態固定資料 upsert，證明 cron + DB 寫入 flow 通暢。

var (
	syncCount int64
	startTime = time.Now()
)

type orgRecord struct {
	badgeID   string
	name      string
	orgPath   string
	isManager bool
}

func main() {
	fmt.Println(`
  ╔═══════════════════════════════════════════════════╗
  ║  PACS Org-Sync (HW2 §5.3 LDAP→DB CronJob mock)    ║
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

	intervalStr := envOrDefault("SYNC_INTERVAL_SECONDS", "1800")
	intervalSec, _ := time.ParseDuration(intervalStr + "s")
	if intervalSec < 30*time.Second {
		intervalSec = 30 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runHealthServer()
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		fmt.Println("\n🛑 Shutting down Org-Sync...")
		cancel()
	}()

	// 啟動時做一次完整 sync，之後依 interval 跑
	sync(ctx, db)

	t := time.NewTicker(intervalSec)
	defer t.Stop()
	fmt.Printf("⏱ syncing every %s\n", intervalSec)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			sync(ctx, db)
		}
	}
}

// mockLDAP returns the simulated org records.
// 真實環境改連 LDAP/AD 抓 OU。
func mockLDAP() []orgRecord {
	return []orgRecord{
		{"B001", "王小明", "TSMC.Fab12.製造部", true},
		{"B002", "李大華", "TSMC.Fab12.品保部", true},
		{"B003", "張美玲", "TSMC.Fab15.研發部", true},
		{"B004", "陳志偉", "TSMC.Fab15.設備部", true},
		{"B005", "林雅婷", "TSMC.總部.人資部", true},
		{"B011", "林員工", "TSMC.Fab12.製造部", false},
		{"B012", "趙員工", "TSMC.Fab12.製造部", false},
		{"B100", "黃廠長", "TSMC.Fab12", true},
		// 模擬 LDAP 新增一個員工
		{"B013", "鄭新進", "TSMC.Fab12.製造部", false},
	}
}

func sync(ctx context.Context, db *sql.DB) {
	records := mockLDAP()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		fmt.Printf("[SYNC-ERR] begin: %v\n", err)
		return
	}
	defer tx.Rollback()

	// UPSERT 進 employees（trg_sync_org_path_ltree 會自動同步 ltree 欄位）
	stmt := `
		INSERT INTO employees (badge_id, name, org_path, is_manager, is_active, updated_at)
		VALUES ($1, $2, $3, $4, TRUE, NOW())
		ON CONFLICT (badge_id) DO UPDATE
		SET name       = EXCLUDED.name,
		    org_path   = EXCLUDED.org_path,
		    is_manager = EXCLUDED.is_manager,
		    is_active  = TRUE,
		    updated_at = NOW()
	`
	for _, r := range records {
		if _, err := tx.ExecContext(ctx, stmt, r.badgeID, r.name, r.orgPath, r.isManager); err != nil {
			fmt.Printf("[SYNC-ERR] upsert %s: %v\n", r.badgeID, err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		fmt.Printf("[SYNC-ERR] commit: %v\n", err)
		return
	}
	atomic.AddInt64(&syncCount, 1)
	fmt.Printf("[SYNC] upserted %d employees\n", len(records))
}

func runHealthServer() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":     "healthy",
			"service":    "org-sync",
			"sync_count": atomic.LoadInt64(&syncCount),
			"uptime":     time.Since(startTime).String(),
		})
	})
	port := envOrDefault("HEALTH_PORT", "8085")
	_ = r.Run(":" + port)
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
