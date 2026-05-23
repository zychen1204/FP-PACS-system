# 0103 員工與刷卡紀錄模擬指南 (Simulation)

本功能用於在本地開發與測試環境中，快速生成大量的組織與員工資料（後續將支援刷卡紀錄生成）。

SQL模擬格式檔案位置

- `scripts/migrations/0103_seed_local.up.sql`
- `scripts/migrations/0103_seed_local.down.sql`

模擬程式

- `scripts/load-generator/`


## 做了什麼 (What it does)

1. **清除舊資料**：安全地清空 `employees` 與 `access_events` 內的測試髒資料。
2. **生成 1,000 名階層員工**：
   - 1 位廠長 (MANAGER_L1)
   - 10 位部門經理 (MANAGER_L2)
   - 989 位一般員工 (STAFF)
3. **建立查詢索引**：自動產生對應的 `org_path_ltree` (例如 `TSMC.製造部_01`) 以供主管視野權限查詢。
4. **刷新視圖**：自動呼叫更新 `mv_daily_attendance` 統計報表。
5. **(TODO) 刷卡紀錄模擬**：未來將擴充此腳本，為上述 1000 位員工自動生成「單日多次 IN/OUT」的刷卡配對，用以驗證停留時數累加與進行壓力測試。




## 如何模擬 (How to simulate)


### 1. 開啟docker compose
```bash
docker compose down -v

docker compose up -d
```

### 2. 動態打卡模擬 (Go Load Generator)

```bash
# 進入目錄
cd scripts/load-generator

# 執行模擬產出 SQL
go run . --mode local --days 30 --clear

**模擬器參數：**
- `--mode local`: 針對本地 1,000 人進行模擬。
- `--days N`: 模擬 N 天的打卡紀錄（預設 30）。
- `--clear`: 清除所有資料庫原本資料。

# 匯入產出的 SQL (Windows PowerShell 務必加cmd /c 在前面和雙引號)
docker compose exec -T postgres psql -U pacs_user -d pacs_db < seed_history_events.sql

```
### 3.前往前端介面 出席報表輸入ID看報表 (注意ID是 B-000001 ~ B-001000 )
http://localhost/


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


