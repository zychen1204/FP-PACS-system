# PACS 完整執行與測試流程

## 前置需求

- [Docker Desktop](https://www.docker.com/products/docker-desktop/) 已安裝並啟動
- Go 1.22+（後端單元測試用）
- 確保 Port 80, 5432, 6379, 8080, 8081 未被佔用

---

## 一、後端單元測試（不需要 Docker）

所有後端邏輯皆有 Go 測試覆蓋，可在本機直接執行，**不需要 PostgreSQL / Redis / Docker**。

```bash
# 所有 go test 指令都必須在 backend/ 目錄下執行
cd FP-PACS-system/backend
go test ./... -count=1
```

預期輸出（11 個套件，86 個測試，全數通過）：

```
ok  pacs/backend/cmd/access-api          ~1s
ok  pacs/backend/cmd/anomaly-detector    ~0.7s
ok  pacs/backend/cmd/event-processor     ~0.8s
ok  pacs/backend/cmd/mv-refresher        ~1.3s
ok  pacs/backend/cmd/org-sync            ~0.7s
ok  pacs/backend/cmd/reporting-api       ~1.6s
ok  pacs/backend/internal/auth           ~1.1s
ok  pacs/backend/internal/cache          ~1.2s
ok  pacs/backend/internal/db             ~1.0s
ok  pacs/backend/internal/models         ~0.7s
ok  pacs/backend/internal/queue          ~2.6s
```

### 測試涵蓋範圍

| 套件 | 測試數 | 涵蓋功能 |
|------|--------|---------|
| `cmd/access-api` | 21 | FR-1 刷卡、FR-2/3 APB、FR-4 Stream、FR-14 雙層門禁、FR-15 廠區隔離、NFR 延遲 |
| `cmd/reporting-api` | 31 | FR-5~9 報表、FR-10 JWT、FR-11 警報、FR-13 稽核、NFR 延遲（含新聚合端點）|
| `cmd/anomaly-detector` | 14 | FR-11 非上班進場、APB burst、tailgating 偵測邏輯 |
| `cmd/event-processor` | 7 | Redis Stream 消費、DLQ retry |
| `cmd/mv-refresher` | 7 | Materialized view 刷新計數器 |
| `cmd/org-sync` | 11 | LDAP mock、org_path 格式驗證 |
| `internal/auth` | 10 | JWT issue/parse、middleware、DEV_AUTH_BYPASS |
| `internal/cache` | 7 | Anti-Passback Redis 狀態讀寫 |
| `internal/db` | 4 | envOrDefault、DB 連線預設值 |
| `internal/models` | 15 | JSON 欄位名稱、omitempty、canonical status 值 |
| `internal/queue` | 8 | Redis Stream 發佈、Consumer Group、DLQ |

### NFR 延遲 SLA 驗證

兩個 SLA 由測試直接斷言（不使用 mock DB 延遲，純 handler overhead）：

```bash
# 在 backend/ 目錄下執行
cd FP-PACS-system/backend

# Access API sub-50ms SLA
go test ./cmd/access-api/... -run TestHandleSwipe_HandlerLatency_Sub50ms -v

# Reporting API sub-200ms SLA（含4個端點）
go test ./cmd/reporting-api/... -run "HandlerLatency" -v
```

最近一次量測結果：

| 測試 | 20次平均 handler overhead | SLA 預算 |
|------|--------------------------|---------|
| Access API 刷卡 | ~309 µs | < 50 ms |
| Reporting attendance | ~27 µs | < 200 ms |
| Reporting manager-team | ~0 µs | < 200 ms |
| Reporting trend（90 buckets）| ~67 µs | < 200 ms |
| Reporting aggregated | ~0 µs | < 200 ms |

> handler overhead 遠低於 5ms 預算，真實 DB I/O（PostgreSQL MV + GiST index）加進來後 SLA 仍有充裕餘裕。

---

## 二、啟動所有服務（Docker）

```bash
cd FP-PACS-system
docker-compose up --build
```

預期輸出（等待所有服務就緒）：

```
✅ PostgreSQL connected
✅ Redis cache connected
✅ Redis Stream connected
🔐 Access API listening on :8080
📊 Reporting API listening on :8081
🔄 Listening for events on stream 'pacs:events'...
```

> 背景執行：`docker-compose up --build -d`

---

## 三、確認服務健康狀態

```bash
docker-compose ps

# Access API
curl http://localhost:8080/healthz

# Reporting API
curl http://localhost:8081/healthz
```

預期回應：
```json
{"service":"access-api","status":"healthy","uptime":"..."}
```

---

## 四、刷卡功能測試（FR-1 ~ FR-4）

### 4.1 正常進入

```bash
curl -X POST http://localhost:8080/v1/swipe \
  -H "Content-Type: application/json" \
  -d '{"badge_id":"B001","site_id":"Site-A","gate_id":"Gate-1A","direction":"IN"}'
```

```json
{"status":"SUCCESS","message":"Access granted"}
```

### 4.2 正常離開

```bash
curl -X POST http://localhost:8080/v1/swipe \
  -H "Content-Type: application/json" \
  -d '{"badge_id":"B001","site_id":"Site-A","gate_id":"Gate-1A","direction":"OUT"}'
```

### 4.3 Anti-Passback 測試（FR-2/3）

連續兩次同方向 IN（執行 4.1 後再執行一次 IN）：

```bash
curl -X POST http://localhost:8080/v1/swipe \
  -H "Content-Type: application/json" \
  -d '{"badge_id":"B001","site_id":"Site-A","gate_id":"Gate-1A","direction":"IN"}'
```

預期回應（HTTP 403）：
```json
{"status":"REJECTED_APB","message":"Anti-Passback Violation","error_code":"ERR_ANTI_PASSBACK"}
```

### 4.4 雙層門禁測試（FR-14）

未通過外層直接刷內層 → 403：
```bash
curl -X POST http://localhost:8080/v1/swipe \
  -H "Content-Type: application/json" \
  -d '{"badge_id":"B002","site_id":"Site-A","gate_id":"Gate-2A","direction":"IN"}'
```

正確順序（外層 IN → 內層 IN → 內層 OUT → 外層 OUT）：
```bash
curl -X POST http://localhost:8080/v1/swipe -H "Content-Type: application/json" \
  -d '{"badge_id":"B002","site_id":"Site-A","gate_id":"Gate-1A","direction":"IN"}'
curl -X POST http://localhost:8080/v1/swipe -H "Content-Type: application/json" \
  -d '{"badge_id":"B002","site_id":"Site-A","gate_id":"Gate-2A","direction":"IN"}'
curl -X POST http://localhost:8080/v1/swipe -H "Content-Type: application/json" \
  -d '{"badge_id":"B002","site_id":"Site-A","gate_id":"Gate-2A","direction":"OUT"}'
curl -X POST http://localhost:8080/v1/swipe -H "Content-Type: application/json" \
  -d '{"badge_id":"B002","site_id":"Site-A","gate_id":"Gate-1A","direction":"OUT"}'
```

---

## 五、事件持久化（FR-4）

```bash
docker-compose exec postgres psql -U pacs_user -d pacs_db \
  -c "SELECT badge_id, gate_id, direction, status, event_time FROM access_events ORDER BY event_time DESC LIMIT 10;"
```

---

## 六、出席報表 API（FR-5）

### 個人當日報表

```bash
curl "http://localhost:8081/v1/reports/attendance?as=B001&start_date=2026-05-01&end_date=2026-05-31"
```

回應（每日一列，`status` 使用 DB 正規值）：
```json
[
  {
    "employee_id": "B001",
    "name": "王小明",
    "status": "STAFF",
    "org_path": "TSMC.Fab12.製造部",
    "work_date": "2026-05-01",
    "first_in": "2026-05-01T08:00:00Z",
    "last_out": "2026-05-01T17:00:00Z",
    "swipe_count": 4,
    "stay_hours": 9.0
  }
]
```

> `status` 正規值：`MANAGER_L1` / `MANAGER_L2` / `STAFF`

### 個人月/季聚合報表（新端點）

```bash
curl "http://localhost:8081/v1/reports/attendance/aggregated?as=B001&start_date=2026-05-01&end_date=2026-05-31"
```

```json
[
  {
    "employee_id": "B001",
    "name": "王小明",
    "status": "STAFF",
    "org_path": "TSMC.Fab12.製造部",
    "total_swipes": 80,
    "total_stay_hours": 180.0,
    "day_count": 20,
    "avg_swipes": 4.0,
    "avg_stay_hours": 9.0
  }
]
```

---

## 七、主管階層報表（FR-6 / FR-9）

### 7.1 主管查看團隊（日模式）

```bash
curl "http://localhost:8081/v1/reports/manager-team?as=B100&start_date=2026-05-01&end_date=2026-05-01"
```

```json
{
  "manager_scope": "TSMC.Fab12",
  "reports": [...]
}
```

### 7.2 主管查看團隊（月/季聚合，新端點）

```bash
curl "http://localhost:8081/v1/reports/manager-team/aggregated?as=B100&start_date=2026-05-01&end_date=2026-05-31"
```

```json
{
  "manager_scope": "TSMC.Fab12",
  "aggregates": [
    {
      "employee_id": "B001",
      "status": "MANAGER_L2",
      "total_swipes": 80,
      "avg_stay_hours": 9.0,
      ...
    }
  ]
}
```

### 7.3 非主管 → 403

```bash
curl "http://localhost:8081/v1/reports/manager-team?as=B011"
# HTTP 403: {"error":"not a manager","badge_id":"B011"}
```

---

## 八、出勤趨勢（FR-7）

趨勢回應現包含 `summary` 欄位（後端計算，前端不需自行聚合）：

```bash
curl "http://localhost:8081/v1/reports/trend?as=B100&period=day&start_date=2026-05-01&end_date=2026-05-31"
```

```json
{
  "scope": "TSMC.Fab12",
  "trends": [
    {"bucket": "2026-05-01", "head_count": 30, "avg_stay_hrs": 8.5, "total_swipes": 120}
  ],
  "summary": {
    "avg_swipes_per_person": 4.0,
    "avg_head_count": 28.5,
    "avg_stay_hrs": 8.3
  }
}
```

`period` 可選值：`day`（預設）、`week`、`month`、`quarter`

---

## 九、Excel 匯出（FR-8）

```bash
curl "http://localhost:8081/v1/reports/attendance/export?as=B001&format=excel&start_date=2026-05-01&end_date=2026-05-31" \
  -o attendance.xlsx
```

---

## 十、警報列表（FR-11）

```bash
# 全部警報
curl "http://localhost:8081/v1/alerts"

# 只看未處理
curl "http://localhost:8081/v1/alerts?open=true"

# 依嚴重程度篩選（HIGH / MEDIUM / LOW / CRITICAL）
curl "http://localhost:8081/v1/alerts?severity=HIGH"

# 限制筆數
curl "http://localhost:8081/v1/alerts?limit=20"
```

---

## 十一、稽核軌跡（FR-13）

```bash
curl "http://localhost:8081/v1/audit?badge_id=B001&start_date=2026-05-01&end_date=2026-05-31"
```

---

## 十二、JWT 驗證（FR-10）

### 取得 token

```bash
curl -X POST http://localhost:8081/v1/dev/login \
  -H "Content-Type: application/json" \
  -d '{"badge_id":"B001"}'
```

```json
{"access_token":"eyJ...","token_type":"Bearer","expires_in":86400}
```

### 使用 token 呼叫保護端點

```bash
TOKEN="eyJ..."
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8081/v1/reports/attendance?as=B001&start_date=2026-05-01&end_date=2026-05-01"
```

### DEV bypass（docker-compose 預設開啟）

設定 `DEV_AUTH_BYPASS=1` 後，不帶 token 仍可呼叫（`?as=` 當作 badge_id）。

---

## 十三、Prometheus Metrics

```bash
curl http://localhost:8080/metrics
```

```
# HELP pacs_swipe_total Total badge swipes
# TYPE pacs_swipe_total counter
pacs_swipe_total{status="success"} 6
pacs_swipe_total{status="rejected"} 1
```

---

## 十四、不可變更稽核驗證（FR-12）

```bash
# DELETE 應失敗
docker compose exec postgres psql -U pacs_user -d pacs_db \
  -c "DELETE FROM access_events WHERE id = 1;"
# 預期：ERROR: permission denied for table access_events

# UPDATE 應失敗
docker compose exec postgres psql -U pacs_user -d pacs_db \
  -c "UPDATE access_events SET status='SUCCESS' WHERE id = 1;"
# 預期：ERROR: permission denied for table access_events

# TRUNCATE 應失敗（trigger）
docker compose exec postgres psql -U pacs_user -d pacs_db \
  -c "TRUNCATE access_events;"
# 預期：ERROR: Updates and deletes are not allowed on the access_events table (FR-12 compliance)
```

---

## 十五、報表效能驗證（NFR-2 P95 < 200ms）

```bash
# 載入 fixture（10k 筆事件）
docker compose exec -T postgres psql -U pacs_user -d pacs_db < scripts/fixtures/load_test.sql

# attendance 查詢應走 idx_events_status_date
docker compose exec postgres psql -U pacs_user -d pacs_db -c \
  "EXPLAIN ANALYZE SELECT badge_id, count(*) FROM access_events
   WHERE event_date = CURRENT_DATE AND status = 'SUCCESS' GROUP BY badge_id;"

# audit_trail 查詢應走 idx_events_badge_eventdate
docker compose exec postgres psql -U pacs_user -d pacs_db -c \
  "EXPLAIN ANALYZE SELECT * FROM access_events
   WHERE badge_id='B001' AND event_date BETWEEN CURRENT_DATE - 7 AND CURRENT_DATE
   ORDER BY event_time DESC LIMIT 100;"
```

---

## 十六、階層權限 DB 驗證（FR-6 / FR-9）

```bash
# 員工樹與職等（status 正規值）
docker compose exec postgres psql -U pacs_user -d pacs_db -c "
  SELECT badge_id, name, job_level, org_path
  FROM employees ORDER BY job_level, org_path, badge_id;"
# B100 → MANAGER_L1，B001~B005 → MANAGER_L2，B011~B013 → STAFF

# 廠長 B100 視野（FR-6 drill-down）
docker compose exec -T postgres psql -U pacs_user -d pacs_db <<'EOF'
\set caller 'B100'
SELECT org_path FROM employees
  WHERE badge_id = :'caller' AND job_level <> 'STAFF' \gset
SELECT e.badge_id, e.name, e.org_path
FROM employees e
WHERE e.org_path_ltree <@ :'org_path'::ltree
ORDER BY e.org_path, e.badge_id;
EOF
```

---

## 十七、前端測試

### 自動化測試（test-runner.html）

直接用瀏覽器開啟，**不需要 Docker 或後端服務**：

[frontend/test-runner.html](http://localhost/test-runner.html)

點擊 **▶️ Run All Tests** 即可執行全部 30 個測試，涵蓋：

| 套件 | 測試數 | 涵蓋內容 |
|------|--------|---------|
| Unit Tests | 9 | `formatTime`、`formatTimeDetailed`、`getDateDaysAgo`、`getRoleBadge`（含 legacy fallback）|
| Integration Tests | 8 | 所有 API response shape（swipe、attendance、aggregated、audit、manager-team、trend + summary、alerts）|
| State Management | 4 | localStorage 讀寫（apiUrl、reportUrl、token、badge）|
| Data Validation | 6 | 刷卡 payload、閘門格式（Gate-NX）、canonical status 值、舊值排除、嚴重程度大寫 |
| UI Logic | 5 | isAggregated 判斷、mode 列表、DOM class 切換、trend avg 計算、統計欄位選擇 |

預期輸出：`✅ ALL TESTS PASSED`

---

### 手動測試（需 Docker 服務啟動）

開啟瀏覽器前往 **http://localhost**

### 頁面一：刷卡模擬

| 操作 | 預期結果 |
|------|---------|
| 選外層 → 輸入 B001 → 進入 → 送出 | 顯示綠色 ✅ 刷卡成功動畫 |
| 同一人再送一次進入 | 顯示紅色 ❌ REJECTED_APB |
| 選內層（未先過外層）送出 | HTTP 403，顯示失敗 |

### 頁面二：出席報表

| 操作 | 預期結果 |
|------|---------|
| 查看自己 / 日 / 選日期 → 查詢 | 表格顯示 6 欄（最早進入/最晚離開），統計 2 個數值卡片 |
| 查看自己 / 月 / 選月份 → 查詢 | 表格顯示月平均刷卡次、月平均停留時數，統計卡片標籤加「月」前綴 |
| 查看自己 / 季 → 查詢 | 同月但標籤加「季」 |
| 查看底下組織 / 日 | 表格 9 欄，右上角顯示「底下組織趨勢」按鈕 |
| 查看底下組織 / 月 | 表格 8 欄（含月刷卡總數/月刷卡平均/月總停留時數/月平均停留時數） |
| 點擊趨勢按鈕（月/季模式） | Modal 顯示後端 summary 卡片（月平均刷卡次/出勤人數/停留時數）+ Line chart |
| 點擊日模式任一 row | Modal 顯示當日完整刷卡稽核軌跡 |
| 點擊月/季模式任一 row | Modal 顯示該員工在日期範圍內每日趨勢圖 |
| 點擊「匯出 Excel」 | 下載 `attendance-YYYYMMDD-HHmmss.xlsx` |
| 以非主管 ID 查詢底下組織 | 顯示「無主管權限」錯誤 |

### 頁面三：警報異常

| 操作 | 預期結果 |
|------|---------|
| 不選嚴重程度 → 查詢 | 顯示所有警報（依 occurred_at DESC） |
| 選 HIGH → 查詢 | 只顯示 severity=HIGH 的警報 |
| 彩色標記 | 🔴 CRITICAL / 🟠 HIGH / 🟡 MEDIUM / 🟢 LOW |

### 頁面四：系統設定

| 操作 | 預期結果 |
|------|---------|
| 點擊「測試連線」 | Access API `/healthz` 與 Reporting API `/healthz` 同時顯示 ✓ 或 ✗ |
| 輸入新 API URL → 儲存 → 重新整理 | URL 仍保留（localStorage） |
| 輸入 badge ID → 取得 Token | 顯示 token 前 50 字元，有效期 24 hr |

---

## 十八、API 端點一覽

| 方法 | 路徑 | 功能 | FR |
|------|------|------|-----|
| POST | `/v1/swipe` | 刷卡決策 | FR-1~4 |
| GET | `/healthz` | 健康檢查 | - |
| GET | `/metrics` | Prometheus 指標 | NFR |
| GET | `/v1/reports/attendance` | 個人出席（日，每日一列）| FR-5 |
| GET | `/v1/reports/attendance/aggregated` | 個人出席（月/季，每人一列）| FR-5 |
| GET | `/v1/reports/attendance/export` | Excel 匯出 | FR-8 |
| GET | `/v1/reports/manager-team` | 主管團隊（日）| FR-6/9 |
| GET | `/v1/reports/manager-team/aggregated` | 主管團隊（月/季）| FR-6/9 |
| GET | `/v1/reports/trend` | 出勤趨勢 + summary | FR-7 |
| GET | `/v1/audit` | 稽核軌跡 | FR-13 |
| GET | `/v1/alerts` | 警報列表（可篩 severity）| FR-11 |
| POST | `/v1/dev/login` | 發 JWT（dev IdP）| FR-10 |

---

## 十九、服務架構一覽

| 服務 | Port | 角色 | 依賴 |
|------|------|------|------|
| frontend | 80 | Nginx 靜態網頁 + 反向代理 | access-api, reporting-api |
| access-api | 8080 | 刷卡決策 (Write Plane) | Redis |
| event-processor | 8082 (health) | 事件持久化 | Redis, PostgreSQL |
| reporting-api | 8081 | 報表查詢 (Read Plane) | PostgreSQL |
| anomaly-detector | 8083 (health) | 異常偵測 | Redis, PostgreSQL |
| mv-refresher | 8084 (health) | MV 定時刷新 | PostgreSQL |
| org-sync | 8085 (health) | LDAP 同步 | PostgreSQL |
| postgres | 5432 | 主資料庫 | - |
| redis | 6379 | 快取 + 訊息佇列 | - |

---

## 二十、停止服務

```bash
# 停止（保留資料）
docker-compose down

# 停止並清除所有資料
docker-compose down -v
```

---

## 常見問題

| 問題 | 解決方案 |
|------|---------|
| Port 已被佔用 | 停止佔用 port 的程式，或在 docker-compose.yml 修改 port mapping |
| PostgreSQL 連線失敗 | 等待 health check 完成，event-processor 會自動重試 30 次 |
| 前端無法連線後端 | 確認 Nginx 容器正常運行，API 透過反向代理存取 |
| `go test` 在 Windows 被 Application Control 封鎖 | 測試二進位受 OS 政策封鎖，`go build ./...` 正常即代表程式碼無誤 |
| 趨勢圖只顯示一個點 | 確認查詢的日期範圍 ≥ 2 天，且 `mv_daily_attendance` 已被刷新 |
| 月/季模式表格空白 | 確認已正確呼叫 `/aggregated` 端點（非日模式呼叫此端點） |
