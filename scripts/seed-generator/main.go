package main

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"time"
)

// ============================================================
// PACS seed-generator — 歷史 demo 資料產生器
//
// 用途：產出 SQL 種子檔，灌入 access_events 讓 dashboard 有畫面、
//       讓 reporting EXPLAIN ANALYZE 能看到 partition + index 效益。
//
// 與壓力測試的分工：
//   - seed-generator (本工具)：產 SQL → psql 直灌 DB，一次性
//   - k6-load-test         ：即時 HTTP POST access-api，驗 NFR-1/2 threshold
//
// 使用方式：
//   go run . --mode local            # 1,000 員工 (Phase 1)
//   go run . --mode fab              # 30,000 員工 (HW2 Phase 2)
//   go run . --mode cloud            # 90,000 員工 (Phase 3)
//   go run . --employees 5000 --days 7
//   go run . --mode local --days 30 --clear
//   go run . --mode local --start-date 2025-06-01 --clear   # 絕對日期區間
// ============================================================

func printHelp() {
	fmt.Print(`
╔═══════════════════════════════════════════════════════════════╗
║          PACS seed-generator — 歷史資料 SQL 種子              ║
╠═══════════════════════════════════════════════════════════════╣
║ 使用方式:                                                     ║
║   go run . [options]                                          ║
║                                                               ║
║ 模式 (擇一)：                                                 ║
║   --mode   local|fab|cloud  規模 preset                       ║
║             local = 1,000   (Phase 1 試點)                    ║
║             fab   = 30,000  (HW2 Phase 2 全廠)                ║
║             cloud = 90,000  (Phase 3 全球)                    ║
║                                                               ║
║ 細部覆寫 (可選；指定後忽略 mode preset)：                     ║
║   --employees N              員工總數                         ║
║   --managers-l2 N            二級主管數量                     ║
║                                                               ║
║ 其他選項：                                                    ║
║   --days N                   模擬天數 (預設 30)               ║
║   --start-date YYYY-MM-DD    起始日 (inclusive)，覆寫 --days  ║
║   --end-date   YYYY-MM-DD    結束日 (exclusive, ≤today)       ║
║   --clear                    匯入前 TRUNCATE 舊資料           ║
║   --api    URL               Access API (報表驗證用)          ║
║   --report URL               Reporting API (報表驗證用)       ║
║   --dry-run                  只統計不產 SQL                   ║
║                                                               ║
║ 範例 (1 年歷史)：                                             ║
║   go run . --mode local --start-date 2025-06-01 --clear       ║
║                                                               ║
║ 產出：seed_history_events.sql                                 ║
║   docker compose exec -T postgres psql -U pacs_user \         ║
║     -d pacs_db < seed_history_events.sql                      ║
╚═══════════════════════════════════════════════════════════════╝
`)
}

// Config 執行配置
type Config struct {
	Mode      string // local | fab | cloud
	Days      int
	StartDate string // YYYY-MM-DD inclusive; overrides Days when set
	EndDate   string // YYYY-MM-DD exclusive; defaults to today, must be <= today
	AccessAPI string
	ReportAPI string
	DryRun    bool
	Clear     bool
	Employees int // 員工總數（由 Mode 決定或 --employees 覆寫）
	Managers  int // 二級主管數
}

func modePreset(mode string) (employees, managers int, ok bool) {
	switch mode {
	case "local":
		return 1000, 11, true // 1 L1 + 10 L2
	case "fab":
		return 30000, 51, true // 1 L1 + 50 L2，對應 HW2 Phase 2
	case "cloud":
		return 90000, 151, true // 1 L1 + 150 L2，對應 Phase 3
	}
	return 0, 0, false
}

func parseArgs() Config {
	cfg := Config{
		Days:      30,
		AccessAPI: "http://localhost:8080",
		ReportAPI: "http://localhost:8081",
	}

	var employeesSet, managersSet bool
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
		case "--start-date":
			if i+1 < len(args) {
				cfg.StartDate = args[i+1]
				i++
			}
		case "--end-date":
			if i+1 < len(args) {
				cfg.EndDate = args[i+1]
				i++
			}
		case "--employees":
			if i+1 < len(args) {
				if v, err := strconv.Atoi(args[i+1]); err == nil {
					cfg.Employees = v
					employeesSet = true
				}
				i++
			}
		case "--managers-l2":
			if i+1 < len(args) {
				if v, err := strconv.Atoi(args[i+1]); err == nil {
					cfg.Managers = v
					managersSet = true
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

	// 套用 mode preset（除非 --employees / --managers-l2 已覆寫）
	if cfg.Mode != "" {
		emp, mgr, ok := modePreset(cfg.Mode)
		if !ok {
			fmt.Printf("❌ 未知 mode: %s（合法值：local | fab | cloud）\n", cfg.Mode)
			printHelp()
			os.Exit(1)
		}
		if !employeesSet {
			cfg.Employees = emp
		}
		if !managersSet {
			cfg.Managers = mgr
		}
	}

	if cfg.Employees <= 0 {
		fmt.Println("❌ 必須指定 --mode 或 --employees N")
		printHelp()
		os.Exit(1)
	}
	if cfg.Managers <= 0 {
		// 沒指定主管數時，給 1 L1 + employees/600 L2 的合理預設
		cfg.Managers = 1 + cfg.Employees/600
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

	// Resolve simulation window: defaults to [today - Days, today), can be
	// overridden by --start-date / --end-date. endDate is always <= today so
	// we never emit future timestamps that would pollute reports.
	loc, _ := time.LoadLocation("Asia/Taipei")
	if loc == nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	endDate := today
	if cfg.EndDate != "" {
		parsed, err := time.ParseInLocation("2006-01-02", cfg.EndDate, loc)
		if err != nil {
			fmt.Printf("❌ --end-date 格式錯誤 (YYYY-MM-DD)：%v\n", err)
			os.Exit(1)
		}
		if parsed.After(today) {
			fmt.Printf("❌ --end-date %s 不能超過今天 %s（避免產出未來時間）\n",
				parsed.Format("2006-01-02"), today.Format("2006-01-02"))
			os.Exit(1)
		}
		endDate = parsed
	}

	var startDate time.Time
	if cfg.StartDate != "" {
		parsed, err := time.ParseInLocation("2006-01-02", cfg.StartDate, loc)
		if err != nil {
			fmt.Printf("❌ --start-date 格式錯誤 (YYYY-MM-DD)：%v\n", err)
			os.Exit(1)
		}
		if !parsed.Before(endDate) {
			fmt.Printf("❌ --start-date %s 必須早於 --end-date %s\n",
				parsed.Format("2006-01-02"), endDate.Format("2006-01-02"))
			os.Exit(1)
		}
		startDate = parsed
		cfg.Days = int(endDate.Sub(startDate).Hours() / 24)
	} else {
		startDate = endDate.AddDate(0, 0, -cfg.Days)
	}

	fmt.Printf(`
╔═══════════════════════════════════════════════════════════════╗
║              PACS seed-generator 啟動                         ║
╠═══════════════════════════════════════════════════════════════╣
║  模式     : %-51s║
║  員工數   : %-51s║
║  模擬區間 : %-51s║
║  Access   : %-51s║
║  Dry-Run  : %-51s║
╚═══════════════════════════════════════════════════════════════╝
`,
		fmt.Sprintf("%s (employees=%d, managers_l2=%d)", cfg.Mode, cfg.Employees, cfg.Managers),
		fmt.Sprintf("%d 人", cfg.Employees),
		fmt.Sprintf("%s → %s (%d 天)",
			startDate.Format("2006-01-02"),
			endDate.AddDate(0, 0, -1).Format("2006-01-02"),
			cfg.Days),
		cfg.AccessAPI,
		fmt.Sprintf("%v", cfg.DryRun),
	)

	fmt.Printf("\n📅 模擬區間：%s → %s（不含 %s，留給即時 swipe）\n\n",
		startDate.Format("2006-01-02"),
		endDate.AddDate(0, 0, -1).Format("2006-01-02"),
		endDate.Format("2006-01-02"),
	)

	RunMonthlySimulation(cfg, startDate)
}
