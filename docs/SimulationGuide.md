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


## 0103 — `POST /v1/swipe` 支援 `event_time` 注入

刷卡 API 加了 optional `event_time` 欄位（RFC3339）給壓測 / 批次回放使用。
留空維持原行為（server time），格式錯誤直接回 400。

| 欄位 | 必填 | 範例 |
|---|---|---|
| `badge_id` | ✅ | `B-000123` |
| `gate_id` | ✅ | `1-A` / `Gate-2B` |
| `direction` | ✅ | `IN` / `OUT` |
| `site_id` | 否（預設 `global`） | `FAB12-A` |
| `event_time` | **否（0103 新增）** | `2026-03-15T09:00:00Z` |

請求範例：

```bash
# 回放 2026-03-15 早上 9 點的 IN
curl -X POST http://localhost:8080/v1/swipe \
  -H 'Content-Type: application/json' \
  -d '{
    "badge_id":"B-000123",
    "site_id":"FAB12-A",
    "gate_id":"1-A",
    "direction":"IN",
    "event_time":"2026-03-15T09:00:00Z"
  }'
```

回傳：

| 情境 | HTTP | `error_code` |
|---|---|---|
| 接受並寫進 stream | 200 | — |
| `event_time` 不是 RFC3339 | 400 | `ERR_INVALID_EVENT_TIME` |
| 缺 `badge_id` / `direction` / `gate_id` | 400 | `ERR_INVALID_REQUEST` |
| 違反 APB / Tier / Cross-site | 403 | `ERR_ANTI_PASSBACK` / `ERR_TIER_VIOLATION` / `ERR_CROSS_SITE` |

### 注意事項

1. **時區**：handler 收到 `event_time` 後一律 normalize 成 UTC。`event_date`
   partition key 由 `(event_time AT TIME ZONE 'Asia/Taipei')::date` 決定，
   想灌到「Taipei 某日」就把 UTC 時間往前 8 小時。
2. **分區範圍**：預建月份 partition 是 2025-01 ~ 2027-12；超出範圍會落入 default partition。
3. **APB 仍會作用**：模擬時若同 badge_id 連續同向，會被 Redis state 擋下回 403。
   壓測腳本可用以下任一策略迴避：
   - 每次用不同 `badge_id`
   - 交替 `IN` / `OUT`
   - 模擬不同 `site_id` 之間切換（記得先做完一邊 IN→OUT 才能跨）
4. **0104 大規模模擬建議用法**：load-generator 改成跑 goroutine pool 對 `POST /v1/swipe`
   發 request，`event_time` 自己 sliding window 推算，可同時測 API 吞吐 + DB 寫入。


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
