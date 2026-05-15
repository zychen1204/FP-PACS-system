package main

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"time"
)

// ============================================================
// PACS 門禁壓力測試 Load Generator
//
// 使用方式：
//   go run . --mode local --days 30
//   go run . --mode cloud --days 30
//   go run . --mode local --days 7  --workers 10
//   go run . --mode cloud --days 30 --workers 50 --qps-scale 1.0
//
// 策略：預先生成所有虛擬打卡事件（帶真實時間戳），
//       再以批次方式快速重播，不需真實等待時間。
// ============================================================

func printHelp() {
	fmt.Print(`
╔═══════════════════════════════════════════════════════════════╗
║            PACS 門禁打卡模擬器 - 真實月度壓力測試            ║
╠═══════════════════════════════════════════════════════════════╣
║ 使用方式:                                                     ║
║   go run . [options]                                          ║
║                                                               ║
║ 選項:                                                         ║
║   --mode   local|cloud   規模模式（必填）                     ║
║             local = 1,000 人；cloud = 90,000 人              ║
║   --days   N             模擬天數（預設 30）                  ║
║   --workers N            並行發送 goroutine 數                ║
║             local 預設 20；cloud 預設 100                     ║
║   --qps-scale F          QPS 縮放係數（預設 1.0）             ║
║             local 建議 0.5（避免本地過載）                    ║
║   --api    URL           Access API（預設 localhost:8080）    ║
║   --report URL           Report API（預設 localhost:8081）    ║
║   --dry-run              只生成事件，不實際送出（顯示統計）   ║
║                                                               ║
║ 範例:                                                         ║
║   # 本地測試（1,000 人，30 天模擬）                           ║
║   go run . --mode local --days 30                            ║
║                                                               ║
║   # 雲端壓力測試（90,000 人，30 天，100 workers）             ║
║   go run . --mode cloud --days 30 --workers 100              ║
║                                                               ║
║   # 只看統計不送出                                            ║
║   go run . --mode local --days 30 --dry-run                  ║
╚═══════════════════════════════════════════════════════════════╝
`)
}

// Config 執行配置
type Config struct {
	Mode      string  // local | cloud
	Days      int     // 模擬天數
	Workers   int     // 並行 goroutine 數
	QPSScale  float64 // QPS 縮放係數
	AccessAPI string  // Access API URL
	ReportAPI string  // Reporting API URL
	DryRun    bool    // 只生成不送出
	Clear     bool    // 是否在匯入前清空舊資料
	Employees int     // 員工總數（由 Mode 決定）
	Managers  int     // 管理者數量
}

func parseArgs() Config {
	cfg := Config{
		Mode:      "",
		Days:      30,
		Workers:   0, // 0 = 由 Mode 決定
		QPSScale:  1.0,
		AccessAPI: "http://localhost:8080",
		ReportAPI: "http://localhost:8081",
		DryRun:    false,
	}

	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--mode":
			if i+1 < len(args) {
				cfg.Mode = args[i+1]
				i++
			}
		case "--days":
			if i+1 < len(args) {
				if v, err := strconv.Atoi(args[i+1]); err == nil {
					cfg.Days = v
				}
				i++
			}
		case "--workers":
			if i+1 < len(args) {
				if v, err := strconv.Atoi(args[i+1]); err == nil {
					cfg.Workers = v
				}
				i++
			}
		case "--qps-scale":
			if i+1 < len(args) {
				if v, err := strconv.ParseFloat(args[i+1], 64); err == nil {
					cfg.QPSScale = v
				}
				i++
			}
		case "--api":
			if i+1 < len(args) {
				cfg.AccessAPI = args[i+1]
				i++
			}
		case "--report":
			if i+1 < len(args) {
				cfg.ReportAPI = args[i+1]
				i++
			}
		case "--dry-run":
			cfg.DryRun = true
		case "--clear":
			cfg.Clear = true
		case "--help", "-h":
			printHelp()
			os.Exit(0)
		}
	}

	// 驗證 mode
	if cfg.Mode != "local" && cfg.Mode != "cloud" {
		fmt.Println("❌ 必須指定 --mode local 或 --mode cloud")
		printHelp()
		os.Exit(1)
	}

	// 依 mode 設定規模
	if cfg.Mode == "local" {
		cfg.Employees = 1000
		cfg.Managers = 11 // 1 L1 + 10 L2
		if cfg.Workers == 0 {
			cfg.Workers = 20
		}
		if cfg.QPSScale == 1.0 {
			cfg.QPSScale = 0.5 // 本地預設縮小
		}
	} else {
		cfg.Employees = 90000
		cfg.Managers = 151 // 1 L1 + 150 L2
		if cfg.Workers == 0 {
			cfg.Workers = 100
		}
	}

	return cfg
}

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}

	rand.Seed(time.Now().UnixNano())
	cfg := parseArgs()

	fmt.Printf(`
╔═══════════════════════════════════════════════════════════════╗
║                  PACS 月度壓力測試啟動                       ║
╠═══════════════════════════════════════════════════════════════╣
║  模式     : %-51s║
║  員工數   : %-51s║
║  模擬天數 : %-51s║
║  Workers  : %-51s║
║  QPS縮放  : %-51s║
║  Access   : %-51s║
║  Dry-Run  : %-51s║
╚═══════════════════════════════════════════════════════════════╝
`,
		cfg.Mode,
		fmt.Sprintf("%d 人（管理者 %d 人）", cfg.Employees, cfg.Managers),
		fmt.Sprintf("%d 天（含週末/假日/出缺席模擬）", cfg.Days),
		fmt.Sprintf("%d goroutines", cfg.Workers),
		fmt.Sprintf("%.2fx", cfg.QPSScale),
		cfg.AccessAPI,
		fmt.Sprintf("%v", cfg.DryRun),
	)

	// 設定模擬起始日期（今天開始往後 30 天，嚴格對齊台北時間凌晨 0 點）
	loc, _ := time.LoadLocation("Asia/Taipei")
	if loc == nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	now := time.Now().In(loc)
	startDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	fmt.Printf("\n📅 模擬區間：%s → %s\n\n",
		startDate.Format("2006-01-02"),
		startDate.AddDate(0, 0, cfg.Days-1).Format("2006-01-02"),
	)

	// 執行模擬
	RunMonthlySimulation(cfg, startDate)
}
