package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"
)

// ============================================================
// 真實月度門禁模擬器 — 產 SQL 種子用，不做 HTTP 重播
// 壓測請改用 scripts/k6-load-test/
// ============================================================

type SwipeEvent struct {
	BadgeID   string
	SiteID    string
	GateID    string
	Direction string
	EventTime time.Time
	IsAnomaly bool
}

type DayStats struct {
	Date  string
	Total int64
	IN    int64
	OUT   int64
}

var (
	sites       = []string{"FAB12A", "FAB12B", "FAB15", "FAB18A", "FAB18B"}
	gatesByType = map[string][]string{
		"MAIN":      {"G-01", "G-02", "G-03", "G-04"},
		"CLEANROOM": {"CR-01", "CR-02", "CR-03"},
		"OFFICE":    {"OFF-01", "OFF-02"},
		"EQUIPMENT": {"EQ-01", "EQ-02"},
	}
	gateTypes = []string{"MAIN", "CLEANROOM", "OFFICE", "EQUIPMENT"}

	taiwanHolidays = map[string]bool{
		"0101": true, "0128": true, "0129": true, "0130": true, "0131": true,
		"0201": true, "0202": true, "0228": true, "0404": true, "0501": true,
		"0619": true, "0920": true, "1010": true,
	}
)

func RunMonthlySimulation(cfg Config, startDate time.Time) {
	fmt.Println("🔄 Phase 1: 預先生成所有打卡事件...")
	events := generateAllEvents(cfg, startDate)

	// 🏁 最終全局排序：確保所有人的事件按時間先後排列
	sort.Slice(events, func(i, j int) bool {
		return events[i].EventTime.Before(events[j].EventTime)
	})

	dayMap := buildDayMap(events, startDate, cfg.Days)
	fmt.Printf("✅ 生成完畢：總事件 %d 筆\n", len(events))

	// 生成 SQL 種子檔以保留時間戳（解決進入/離開時間一樣的問題）
	sqlFile := "seed_history_events.sql"
	fmt.Printf("💾 Phase 2: 正在產出 SQL 種子檔 (%s)...\n", sqlFile)
	generateSQLFile(events, sqlFile, cfg)
	fmt.Println("✅ SQL 產出成功！請執行以下命令匯入以保留真實時間戳：")
	fmt.Println("   docker-compose exec -T postgres psql -U pacs_user -d pacs_db < " + sqlFile)

	if cfg.DryRun {
		return
	}

	fmt.Println("\n📊 Phase 3: 執行報表查詢驗證（讀路徑健康檢查）...")
	runReportValidation(cfg, startDate, dayMap)
}

func generateAllEvents(cfg Config, startDate time.Time) []SwipeEvent {
	var mu sync.Mutex
	var allEvents []SwipeEvent
	var wg sync.WaitGroup
	batchSize := 500
	processed := 0

	for i := 1; i <= cfg.Employees; i += batchSize {
		end := i + batchSize - 1
		if end > cfg.Employees {
			end = cfg.Employees
		}
		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()
			var local []SwipeEvent
			for empID := start; empID <= end; empID++ {
				badgeID := fmt.Sprintf("B-%06d", empID)
				isManager := empID <= cfg.Managers
				for d := 0; d < cfg.Days; d++ {
					date := startDate.AddDate(0, 0, d)
					local = append(local, generateEmployeeDayEvents(badgeID, isManager, date)...)
				}
			}
			mu.Lock()
			allEvents = append(allEvents, local...)
			processed += (end - start + 1)
			fmt.Printf("   進度: %d/%d\r", processed, cfg.Employees)
			mu.Unlock()
		}(i, end)
	}
	wg.Wait()
	fmt.Println()
	return allEvents
}

func generateEmployeeDayEvents(badgeID string, isManager bool, dayDate time.Time) []SwipeEvent {
	var evts []SwipeEvent
	wd := dayDate.Weekday()
	if wd == time.Saturday || wd == time.Sunday || taiwanHolidays[dayDate.Format("0102")] {
		if rand.Intn(100) >= 5 {
			return nil
		}
		// 假日加班
		in := dayDate.Add(time.Duration(rand.Intn(120)+480) * time.Minute) // 8-10am
		out := in.Add(time.Duration(rand.Intn(240)+120) * time.Minute)     // 2-6 hrs later
		return []SwipeEvent{newEvent(badgeID, "IN", in, false), newEvent(badgeID, "OUT", out, false)}
	}

	// 平日
	if rand.Intn(100) < 5 {
		return nil // 曠職
	}

	// 進入時間
	arrival := dayDate.Add(time.Duration(gaussianMinutes(480, 30)) * time.Minute)
	// 離開時間
	stayMins := 540 + rand.Intn(120) // 9-11 小時
	if isManager {
		stayMins += 60 // 管理者多待一小時
	}
	departure := arrival.Add(time.Duration(stayMins) * time.Minute)

	evts = append(evts, newEvent(badgeID, "IN", arrival, false))

	// 午休
	if rand.Intn(100) < 80 {
		lOut := dayDate.Add(time.Duration(720+rand.Intn(30)) * time.Minute)
		lIn := lOut.Add(time.Duration(40+rand.Intn(20)) * time.Minute)
		if lOut.After(arrival) && lIn.Before(departure) {
			evts = append(evts, newEvent(badgeID, "OUT", lOut, false), newEvent(badgeID, "IN", lIn, false))
		}
	}

	evts = append(evts, newEvent(badgeID, "OUT", departure, false))

	sort.Slice(evts, func(i, j int) bool { return evts[i].EventTime.Before(evts[j].EventTime) })
	return evts
}

func newEvent(badgeID, dir string, t time.Time, anomaly bool) SwipeEvent {
	gt := gateTypes[rand.Intn(len(gateTypes))]
	return SwipeEvent{BadgeID: badgeID, Direction: dir, EventTime: t, IsAnomaly: anomaly, SiteID: sites[rand.Intn(len(sites))], GateID: gatesByType[gt][rand.Intn(len(gatesByType[gt]))]}
}

func generateSQLFile(events []SwipeEvent, filename string, cfg Config) {
	f, _ := os.Create(filename)
	defer f.Close()

	fmt.Fprintln(f, "SET session_replication_role = 'replica';")
	if cfg.Clear {
		fmt.Fprintln(f, "TRUNCATE TABLE access_events CASCADE;")
		fmt.Fprintln(f, "TRUNCATE TABLE employees CASCADE;")
		generateEmployeeSeedsSQL(f, cfg)
	} else {
		fmt.Fprintln(f, "DELETE FROM access_events WHERE reason = '[STRESS_TEST]';")
	}

	batchSize := 1000
	for i := 0; i < len(events); i += batchSize {
		end := i + batchSize
		if end > len(events) {
			end = len(events)
		}

		fmt.Fprintln(f, "INSERT INTO access_events (badge_id, site_id, gate_id, direction, status, reason, event_time, event_date) VALUES")
		for j := i; j < end; j++ {
			e := events[j]
			status := "SUCCESS"
			reason := "[STRESS_TEST]"
			if e.IsAnomaly {
				status = "REJECTED_APB"
			}
			sep := ","
			if j == end-1 {
				sep = ";"
			}
			fmt.Fprintf(f, "('%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s')%s\n",
				e.BadgeID, e.SiteID, e.GateID, e.Direction, status, reason, e.EventTime.Format(time.RFC3339), e.EventTime.Format("2006-01-02"), sep)
		}
	}
	fmt.Fprintln(f, "SET session_replication_role = 'origin';")
	fmt.Fprintln(f, "REFRESH MATERIALIZED VIEW CONCURRENTLY mv_daily_attendance;")
}

func buildDayMap(events []SwipeEvent, start time.Time, days int) map[string]*DayStats {
	m := make(map[string]*DayStats)
	for i := 0; i < days; i++ {
		d := start.AddDate(0, 0, i).Format("2006-01-02")
		m[d] = &DayStats{Date: d}
	}
	for _, e := range events {
		d := e.EventTime.Format("2006-01-02")
		if s, ok := m[d]; ok {
			s.Total++
			if e.Direction == "IN" {
				s.IN++
			} else {
				s.OUT++
			}
		}
	}
	return m
}

func runReportValidation(cfg Config, startDate time.Time, dayMap map[string]*DayStats) {
	client := &http.Client{Timeout: 10 * time.Second}
	checkDate := startDate.Format("2006-01-02")

	queries := []struct {
		name string
		url  string
	}{
		{"出勤報表", fmt.Sprintf("%s/v1/reports/attendance?date=%s", cfg.ReportAPI, checkDate)},
		{"主管團隊報表", fmt.Sprintf("%s/v1/reports/manager-team?as=B-000001&date=%s", cfg.ReportAPI, checkDate)},
		{"趨勢分析", fmt.Sprintf("%s/v1/reports/trend?period=day&as=B-000001", cfg.ReportAPI)},
	}

	fmt.Println("\n🔍 驗證報表 API：")
	for _, q := range queries {
		resp, err := client.Get(q.url)
		if err != nil {
			fmt.Printf("   %-15s: ❌ 請求失敗: %v\n", q.name, err)
			continue
		}
		defer resp.Body.Close()
		fmt.Printf("   %-15s: %d\n", q.name, resp.StatusCode)
	}
}

func gaussianMinutes(mean, stddev int) int {
	return mean + (rand.Intn(stddev*4) - stddev*2)
}

func generateEmployeeSeedsSQL(f *os.File, cfg Config) {
	fmt.Fprintln(f, "-- ── 自動播種 1000 位員工 ──")
	fmt.Fprintln(f, "INSERT INTO employees (badge_id, name, job_level, org_path, org_path_ltree, is_active) VALUES")
	// 廠長
	fmt.Fprintln(f, "('B-000001', '廠長_總管', 'MANAGER_L1', 'TSMC', 'TSMC', TRUE),")
	// 經理與員工
	for i := 2; i <= cfg.Employees; i++ {
		badgeID := fmt.Sprintf("B-%06d", i)
		name := ""
		level := "STAFF"
		org := fmt.Sprintf("TSMC.製造部_%02d", (i-12)%10+1)
		if i <= cfg.Managers {
			level = "MANAGER_L2"
			name = fmt.Sprintf("部經理_%02d", i-1)
			org = fmt.Sprintf("TSMC.製造部_%02d", i-1)
		} else {
			name = fmt.Sprintf("員工_%06d", i)
		}
		sep := ","
		if i == cfg.Employees {
			sep = ";"
		}
		fmt.Fprintf(f, "('%s', '%s', '%s', '%s', '%s', TRUE)%s\n",
			badgeID, name, level, org, org, sep)
	}
}
