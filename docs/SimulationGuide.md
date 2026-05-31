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

**推薦方式：一鍵 reset 腳本**

```bash
# 預設 7 天 / local 規模（1,000 人）
./scripts/demo-reset.sh

# 30 天 + Phase 2 規模（30,000 人）
./scripts/demo-reset.sh 30 fab

# 絕對日期：2025-06-01 → 昨天（~1 年歷史，自動帶 --clear）
./scripts/demo-reset.sh 2025-06-01

# 絕對日期 + Phase 2 規模
./scripts/demo-reset.sh 2025-06-01 fab
```

`scripts/demo-reset.sh` 會 down -v → 起服務 → 跑 migration（0001~0106）→ 跑 seed-generator → 灌 SQL → REFRESH MV → 驗證沒有未來時間事件。
**參數 1 自動偵測格式**：`YYYY-MM-DD` 切絕對日期模式（自動帶 `--clear`）、整數切相對天數模式。

**手動步驟（debug 用）**

```bash
# 1. 啟服務（自動跑 migrations 0001~0103 + 0105 stay_hours + 0106 MV future guard）
docker compose down -v && docker compose up -d

# 2. 產過去 30 天 SQL 種子（不含今天，1,000 人 = Phase 1）
cd scripts/seed-generator
go run . --mode local --days 30

# 3. 灌進 DB
docker compose exec -T postgres psql -U pacs_user -d pacs_db < seed_history_events.sql

# 4. REFRESH MV（必要：seed 灌完後 MV 還是空）
docker compose exec postgres psql -U pacs_user -d pacs_db -c \
  "REFRESH MATERIALIZED VIEW mv_daily_attendance;"

# 5. 確認沒有未來時間事件
docker compose exec postgres psql -U pacs_user -d pacs_db -c \
  "SELECT COUNT(*) AS future_events FROM access_events WHERE event_time > NOW();"

# 6. 開前端看報表（員工 ID 範圍 B-000001 ~ B-001000）
open http://localhost/
```

> **時間軸契約（重要）**：seed-generator 產出的事件**只涵蓋 [today - N, yesterday]**，今天完全不種——今天的資料留給 access-api 即時 swipe 產生（demo 場上點點看就能展示 CQRS write path）。
> 0099 dev_seed 同理：5 個 demo 主管的 ~45 筆事件全部落在過去 3 天。
> 任何時候 `SELECT COUNT(*) FROM access_events WHERE event_time > NOW()` 都應該回傳 0。

> **Phase 2 規模（30k）**：`go run . --mode fab --days 30` — 約 1–3 分鐘產 SQL；匯入需 5–10 分鐘。
> **Phase 3 規模（90k）**：建議跑雲端 `0104_cloud_seed`，seed-generator 在 90k 規模 SQL 檔超過 1GB。


## 雲端 reset (GKE / Cloud SQL)

`scripts/cloud-reset.sh` 把雲端 Cloud SQL 的 `access_events` 一鍵清空、灌入新的歷史資料；**employees / alerts / MV 定義都不動**。第一參數同樣自動偵測 `YYYY-MM-DD`（絕對日期）或整數（相對天數）。

```bash
# 切到雲端 cluster
gcloud container clusters get-credentials pacs-cluster \
  --location=asia-east1 --project=extreme-water-497313-j8

# 7 天 × local（預設）
./scripts/cloud-reset.sh

# 絕對日期：2025-06-01 → 昨天（1 年 × ~87 萬筆）
./scripts/cloud-reset.sh 2025-06-01

# 只看會送什麼 SQL，不真的灌
./scripts/cloud-reset.sh --dry-run 2025-06-01
```

**與本地版本的差異**：Cloud SQL 不給 superuser 權限，無法 `SET session_replication_role`，所以腳本改用 `ALTER TABLE ... DISABLE/ENABLE TRIGGER USER` 對 parent 與所有 37 個 partition toggle FR-12 trigger，再跑 TRUNCATE / INSERT / REFRESH MV。

**前置條件**：
- 已 `kubectl config use-context` 切到 `pacs-cluster`（腳本會檢查，否則 fail-fast）
- 雲端的 `employees` 表已含 90K 員工（由 `0104_cloud_seed.up.sql` 種好）

**典型耗時**：本地產 SQL 30-60 秒 + cloud upload 30-60 秒 + Cloud SQL 灌入 1-2 分鐘 + REFRESH MV 5-10 秒 ≈ **3-5 分鐘**。


## 絕對日期區間（demo 用 1 年歷史）

`--days N` 是「從今天往前推」的相對時間，demo 需要跨多月份的趨勢資料時不夠用。改用 `--start-date` / `--end-date` 可指定絕對區間，**`--end-date` 永遠不能超過今天**，否則 fail-fast。

```bash
# 灌入 2025-06-01 → 昨天（約 1 年、~87 萬筆事件、SQL ~92 MB）
cd scripts/seed-generator
go run . --mode local --start-date 2025-06-01 --clear

# 灌進 DB（~1-2 分鐘）
docker compose exec -T postgres psql -U pacs_user -d pacs_db < seed_history_events.sql
docker compose exec postgres psql -U pacs_user -d pacs_db -c \
  "REFRESH MATERIALIZED VIEW mv_daily_attendance;"
```

**flag 優先序**：`--start-date` / `--end-date` 任一指定就覆寫 `--days`；同時指定就用絕對區間並重算天數。

> **已知限制 — 農曆假日跨年誤差**：`realistic-simulator.go` 的台灣假日 calendar 用 `MMDD` 為 key、跨年共用，但農曆假日（除夕初一）2025/2026 實際日期不同（2025 春節 1/28–2/2、2026 春節 2/15–2/19）。對日報/月報趨勢影響 < 1%（只影響 5%/95% 出勤機率切換），demo 階段可接受。


## CLI 選項

```text
--mode   local|fab|cloud   規模 preset
--employees N              員工總數（覆寫 mode preset）
--managers-l2 N            二級主管數量（覆寫 mode preset）
--days N                   模擬天數（預設 30，含週末 / 假日 / 出缺席 / 午休邏輯）
--start-date YYYY-MM-DD    起始日期 (inclusive, Asia/Taipei)，覆寫 --days
--end-date   YYYY-MM-DD    結束日期 (exclusive, ≤ today)，預設今天
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
