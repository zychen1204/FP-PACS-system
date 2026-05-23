# 歷史資料模擬指南 (Seed Simulation)

> **本文件只談「灌歷史資料」** — 讓 dashboard 有畫面、讓 EXPLAIN ANALYZE 看得到 partition + index 效益。
> 「即時壓力測試」（NFR-1 / NFR-2 / NFR-4 驗證）請改看 [`LoadTestGuide.md`](LoadTestGuide.md)。

## 工具分工

| 工具 | 何時用 | 做什麼 |
|---|---|---|
| `scripts/seed-generator/`（本文件） | demo 前**跑一次** | 產 30 天 SQL 種子 → `psql` 直灌 `access_events` |
| `scripts/k6-load-test/` | 隨時可重跑 | 即時 HTTP POST 打 access-api，驗 NFR thresholds |

`seed-generator` 走 SQL 直灌，不經過 access-api / Redis Stream / event-processor，因此**保留真實時間戳**，讓報表的 stay_hours 等數字正確。要驗 access-api hot path，請用 k6。

## 相關檔案

- 員工 baseline：`scripts/migrations/0103_seed_local.up.sql`（docker compose 啟動時自動跑）
- 員工 30 天打卡：`scripts/seed-generator/`（本文件介紹）
- 雲端 90k 員工：`scripts/cloud_migrations/0104_cloud_seed.up.sql`（手動執行）


## 規模 preset 對應 HW2 三個 Phase

| `--mode` | Employees | L2 Managers | 對應 HW2 §4 Phase |
|---|---|---|---|
| `local` | 1,000 | 10 | Phase 1 試點（單棟） |
| `fab`   | 30,000 | 50 | Phase 2 全廠（單一 Fab） |
| `cloud` | 90,000 | 150 | Phase 3 全球 |

也可不用 mode preset，直接 `--employees N --managers-l2 N` 細調。


## 快速開始（本地）

```bash
# 1. 啟服務（自動跑 migrations 0001~0103 + 0105 stay_hours fix）
docker compose down -v && docker compose up -d

# 2. 產 30 天 SQL 種子（1,000 人 = Phase 1）
cd scripts/seed-generator
go run . --mode local --days 30

# 3. 灌進 DB
docker compose exec -T postgres psql -U pacs_user -d pacs_db < seed_history_events.sql

# 4. 確認 MV 有資料
docker compose exec postgres psql -U pacs_user -d pacs_db -c \
  "SELECT count(*) FROM mv_daily_attendance;"

# 5. 開前端看報表（員工 ID 範圍 B-000001 ~ B-001000）
open http://localhost/
```

> **Phase 2 規模（30k）**：`go run . --mode fab --days 30` — 約 1–3 分鐘產 SQL；匯入需 5–10 分鐘。
> **Phase 3 規模（90k）**：建議跑雲端 `0104_cloud_seed`，seed-generator 在 90k 規模 SQL 檔超過 1GB。


## CLI 選項

```text
--mode   local|fab|cloud   規模 preset
--employees N              員工總數（覆寫 mode preset）
--managers-l2 N            二級主管數量（覆寫 mode preset）
--days N                   模擬天數（預設 30，含週末 / 假日 / 出缺席 / 午休邏輯）
--clear                    匯入前 TRUNCATE 舊資料
--api URL                  Access API（Phase 3 報表驗證用，預設 localhost:8080）
--report URL               Reporting API（同上，預設 localhost:8081）
--dry-run                  只統計不產 SQL
```


## 產出細節

`seed_history_events.sql` 內容：
1. `SET session_replication_role = 'replica'`（暫停 trigger，避免 FR-12 immutability trigger 擋 INSERT — 此操作只在 seed 期間合法）
2. （可選）`TRUNCATE access_events / employees`（`--clear` 時）
3. 員工 SQL（廠長 L1 + L2 部經理 + STAFF）
4. `INSERT INTO access_events ...` 批次（1000 筆一批）— 含 IN/OUT 配對、午休、週末/假日邏輯
5. `SET session_replication_role = 'origin'` 還原
6. `REFRESH MATERIALIZED VIEW CONCURRENTLY mv_daily_attendance` 觸發 0105 IN/OUT 累加計算


## 驗證 0105 stay_hours 累加邏輯

灌完資料後跑：
```bash
docker compose exec postgres psql -U pacs_user -d pacs_db -c \
  "SELECT badge_id, first_in::time, last_out::time, swipe_count, stay_hours
   FROM mv_daily_attendance
   WHERE event_date = CURRENT_DATE
   ORDER BY badge_id LIMIT 5;"
```

預期：午餐外出 1h 的員工 `last_out - first_in ≈ 10h` 但 `stay_hours ≈ 9h`（IN/OUT 累加扣午餐外出）。
