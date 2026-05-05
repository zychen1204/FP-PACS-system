# PACS 完整執行與測試流程

## 前置需求

- [Docker Desktop](https://www.docker.com/products/docker-desktop/) 已安裝並啟動
- 確保 Port 80, 5432, 6379, 8080, 8081 未被佔用

---

## Step 1：啟動所有服務

```bash
cd "FP-PACS-system"
docker-compose up --build
```

預期輸出（等待所有服務啟動完成）：

```
✅ PostgreSQL connected
✅ Redis cache connected
✅ Redis Stream connected
🔐 Access API listening on :8080
📊 Reporting API listening on :8081
🔄 Listening for events on stream 'pacs:events'...
```

> 如果想在背景執行，使用 `docker-compose up --build -d`

---

## Step 2：確認服務健康狀態

```bash
# 檢查所有容器狀態
docker-compose ps

# Access API 健康檢查
curl http://localhost:8080/healthz

# Reporting API 健康檢查
curl http://localhost:8081/healthz
```

預期回應：
```json
{"service":"access-api","status":"healthy","uptime":"..."}
```

---

## Step 3：測試刷卡功能 (FR1 - InOut Operation)

### 3.1 正常刷卡進入

```bash
curl -X POST http://localhost:8080/v1/swipe ^
  -H "Content-Type: application/json" ^
  -d "{\"badge_id\":\"B001\",\"site_id\":\"Site-A\",\"gate_id\":\"Gate-1\",\"direction\":\"IN\"}"
```

預期回應：
```json
{"status":"SUCCESS","message":"Access granted"}
```

### 3.2 正常刷卡離開

```bash
curl -X POST http://localhost:8080/v1/swipe ^
  -H "Content-Type: application/json" ^
  -d "{\"badge_id\":\"B001\",\"site_id\":\"Site-A\",\"gate_id\":\"Gate-1\",\"direction\":\"OUT\"}"
```

預期回應：
```json
{"status":"SUCCESS","message":"Access granted"}
```

### 3.3 Anti-Passback 測試 (FR2)

連續刷同方向兩次（先執行 3.1 後再執行一次 IN）：

```bash
curl -X POST http://localhost:8080/v1/swipe ^
  -H "Content-Type: application/json" ^
  -d "{\"badge_id\":\"B001\",\"site_id\":\"Site-A\",\"gate_id\":\"Gate-1\",\"direction\":\"IN\"}"
```

預期回應（HTTP 403）：
```json
{"status":"REJECTED_APB","message":"Anti-Passback Violation","error_code":"ERR_ANTI_PASSBACK"}
```

### 3.4 多員工批次刷卡

```bash
curl -X POST http://localhost:8080/v1/swipe -H "Content-Type: application/json" -d "{\"badge_id\":\"B002\",\"site_id\":\"Site-A\",\"gate_id\":\"Gate-1\",\"direction\":\"IN\"}"
curl -X POST http://localhost:8080/v1/swipe -H "Content-Type: application/json" -d "{\"badge_id\":\"B003\",\"site_id\":\"Site-B\",\"gate_id\":\"Gate-2\",\"direction\":\"IN\"}"
curl -X POST http://localhost:8080/v1/swipe -H "Content-Type: application/json" -d "{\"badge_id\":\"B002\",\"site_id\":\"Site-A\",\"gate_id\":\"Gate-1\",\"direction\":\"OUT\"}"
curl -X POST http://localhost:8080/v1/swipe -H "Content-Type: application/json" -d "{\"badge_id\":\"B003\",\"site_id\":\"Site-B\",\"gate_id\":\"Gate-2\",\"direction\":\"OUT\"}"
```

---

## Step 4：測試事件持久化 (FR4)

驗證事件已寫入 PostgreSQL：

```bash
docker-compose exec postgres psql -U pacs_user -d pacs_db -c "SELECT * FROM access_events ORDER BY event_time DESC LIMIT 10;"
```

預期看到所有刷卡紀錄（包含 SUCCESS 和 REJECTED_APB）。

---

## Step 5：測試出席報表 (FR5)

```bash
# 取得所有出席報表
curl http://localhost:8081/v1/reports/attendance

# 依日期查詢（替換為今天的日期）
curl "http://localhost:8081/v1/reports/attendance?date=2026-05-03"
```

預期回應：
```json
[
  {
    "employee_id": "B001",
    "name": "王小明",
    "org_path": "TSMC.Fab12.製造部",
    "work_date": "2026-05-03",
    "first_in": "...",
    "last_out": "...",
    "swipe_count": 4,
    "stay_hours": 0.5
  }
]
```

---

## Step 6：測試稽核查詢 (FR13)

```bash
curl "http://localhost:8081/v1/audit?badge_id=B001&start_date=2026-05-01&end_date=2026-05-31"
```

預期回應：該員工在指定日期範圍的完整事件列表（包含被拒絕的）。

---

## Step 7：測試 Metrics (NFR7)

```bash
curl http://localhost:8080/metrics
```

預期回應：
```
# HELP pacs_swipe_total Total badge swipes
# TYPE pacs_swipe_total counter
pacs_swipe_total{status="success"} 6
pacs_swipe_total{status="rejected"} 1
```

---

## Step 8：測試前端介面

1. 開啟瀏覽器前往 **http://localhost**
2. 在「刷卡模擬」頁面：
   - 輸入 Badge ID（例如 B001）
   - 選擇地點和閘門
   - 點擊進入或離開
   - 確認回應顯示 SUCCESS 或 REJECTED_APB
3. 在「出席報表」頁面：
   - 點擊「取得報表」
   - 確認顯示出席紀錄，包含停留時數
4. 在「設定」頁面：
   - 點擊「測試連線」確認 Access API 和 Reporting API 都顯示 ✓

---

## Step 9：驗證不可變更稽核 (FR12)

`access_events` 採雙層保護：`REVOKE UPDATE/DELETE` 角色權限 + `BEFORE UPDATE/DELETE/TRUNCATE` trigger。下列三項都應失敗：

```bash
# 9.1 DELETE 應失敗（角色 REVOKE）
docker compose exec postgres psql -U pacs_user -d pacs_db -c "DELETE FROM access_events WHERE id = 1;"
# 預期：ERROR: permission denied for table access_events

# 9.2 UPDATE 應失敗（角色 REVOKE 或 trigger）
docker compose exec postgres psql -U pacs_user -d pacs_db -c "UPDATE access_events SET status='SUCCESS' WHERE id = 1;"
# 預期：ERROR: permission denied for table access_events

# 9.3 TRUNCATE 應失敗（statement-level trigger）
docker compose exec postgres psql -U pacs_user -d pacs_db -c "TRUNCATE access_events;"
# 預期：ERROR: Updates and deletes are not allowed on the access_events table (FR-12 compliance)
```

---

## Step 10：驗證角色分離（最小權限）

`pacs_reporter` 是 read-only role，提供給 reporting-api 使用：

```bash
# 10.1 reporter 可以 SELECT
docker compose exec postgres psql -U pacs_reporter -d pacs_db -c "SELECT count(*) FROM access_events;"
# 預期：count > 0

# 10.2 reporter 不能 INSERT
docker compose exec postgres psql -U pacs_reporter -d pacs_db \
  -c "INSERT INTO access_events (badge_id, site_id, gate_id, direction, status) VALUES ('B999','S','G','IN','SUCCESS');"
# 預期：ERROR: permission denied for table access_events
```

---

## Step 11：驗證報表效能 (NFR-2 P95 < 200 ms)

載入 fixture（10k 筆事件），用 EXPLAIN ANALYZE 確認 baseline `0001` 中為 NFR-2 設計的索引有被命中：

```bash
# 11.1 載入 fixture
docker compose exec -T postgres psql -U pacs_user -d pacs_db < scripts/fixtures/load_test.sql

# 11.2 attendance 查詢應走 idx_events_status_date
docker compose exec postgres psql -U pacs_user -d pacs_db -c \
  "EXPLAIN ANALYZE SELECT badge_id, count(*) FROM access_events
   WHERE event_date = CURRENT_DATE AND status = 'SUCCESS' GROUP BY badge_id;"
# 預期：plan 含 'Index Scan using idx_events_status_date'，total time < 200ms

# 11.3 audit_trail 查詢應走 idx_events_badge_eventdate
docker compose exec postgres psql -U pacs_user -d pacs_db -c \
  "EXPLAIN ANALYZE SELECT * FROM access_events
   WHERE badge_id='B001' AND event_date BETWEEN CURRENT_DATE - 7 AND CURRENT_DATE
   ORDER BY event_time DESC LIMIT 100;"
# 預期：plan 含 'Index Scan using idx_events_badge_eventdate'

# 11.4 用 pg_stat_statements 看歷史 query 平均時間
docker compose exec postgres psql -U pacs_user -d pacs_db -c \
  "SELECT substring(query,1,80) AS q, calls, round(mean_exec_time::numeric,2) AS mean_ms
   FROM pg_stat_statements WHERE query ILIKE 'SELECT%access_events%'
   ORDER BY mean_exec_time DESC LIMIT 5;"
```

> 註：目前 `backend/internal/db/postgres.go` 的 query 仍用 `event_time::date`，
> 不會直接命中新索引；待 backend owner 改寫為 `event_date` 後此 11.2 / 11.3
> 的 EXPLAIN 會在現實 query 上呈現相同結果。

---

## Step 12：驗證 FR-6 / FR-9 階層查詢（DB 層）

`employees.is_manager` 旗標 + `org_path` LIKE prefix 是 FR-6（階層團隊報表）
與 FR-9（階層資料權限）的 DB 層支援。三個 sub-step 對應「看員工樹 →
廠長視野（跨部門）→ 部主管視野（單一部門）」。

```bash
# 12.1 看員工樹與 manager 標記
docker compose exec postgres psql -U pacs_user -d pacs_db -c "
  SELECT badge_id, name, org_path, is_manager
  FROM employees
  ORDER BY org_path, badge_id;"
# 預期：8 員工；B100/B001-B005 is_manager=t；B011/B012 為 f

# 12.2 廠長 B100 視野（FR-6 跨部門 drill-down）— pattern a 兩段式
docker compose exec -T postgres psql -U pacs_user -d pacs_db <<'EOF'
\set caller 'B100'
SELECT org_path FROM employees
  WHERE badge_id = :'caller' AND is_manager = TRUE \gset
\echo Manager scope: :org_path
SELECT e.badge_id, e.name, e.org_path,
       COUNT(ae.id) FILTER (WHERE ae.status='SUCCESS') AS swipes
FROM employees e
LEFT JOIN access_events ae ON ae.badge_id = e.badge_id
WHERE e.org_path = :'org_path' OR e.org_path LIKE :'org_path' || '.%'
GROUP BY e.badge_id, e.name, e.org_path
ORDER BY e.org_path, e.badge_id;
EOF
# 預期：5 列 — B100 / B002 / B001 / B011 / B012；
#       B011/B012 swipes=0（剛入職還沒打卡是合理場景）

# 12.3 部主管 B001 視野
docker compose exec -T postgres psql -U pacs_user -d pacs_db <<'EOF'
\set caller 'B001'
SELECT org_path FROM employees
  WHERE badge_id = :'caller' AND is_manager = TRUE \gset
\echo Manager scope: :org_path
SELECT e.badge_id, e.name, e.org_path
FROM employees e
WHERE e.org_path = :'org_path' OR e.org_path LIKE :'org_path' || '.%'
ORDER BY e.badge_id;
EOF
# 預期：3 列 — B001 / B011 / B012
```

> 註 1（pattern a 兩段式）：上面 `\gset` 把第 1 段（取 manager scope）
> 的結果綁到 psql 變數 `:org_path`，讓第 2 段直接帶入。Backend 實作時
> 第 1 段回空表示「caller 不是主管」，應回 HTTP 403（FR-9 negative case）。
>
> 註 2（FR-9 negative）：`SELECT org_path FROM employees WHERE badge_id='B011' AND is_manager=TRUE`
> 應回 0 列；DB 不擋查詢，由 backend 處理 403 邏輯。

---

## Step 13：停止服務

```bash
# 停止所有服務
docker-compose down

# 停止並清除資料庫資料
docker-compose down -v
```

---

## 常見問題

| 問題 | 解決方案 |
|------|----------|
| Port 已被佔用 | 停止佔用 port 的程式，或在 docker-compose.yml 中修改 port mapping |
| PostgreSQL 連線失敗 | 等待 health check 完成，event-processor 會自動重試 30 次 |
| 前端無法連線後端 | 確認 Nginx 容器正常運行，API 透過反向代理存取 |
| go mod tidy 本地失敗 | 不影響 Docker 建置，依賴在容器內解析 |

---

## 服務架構一覽

| 服務 | Port | 角色 | 依賴 |
|------|------|------|------|
| frontend | 80 | Nginx 靜態網頁 + 反向代理 | access-api, reporting-api |
| access-api | 8080 | 刷卡決策 (Write Plane) | Redis |
| event-processor | 8082 (health) | 事件持久化 | Redis, PostgreSQL |
| reporting-api | 8081 | 報表查詢 (Read Plane) | PostgreSQL |
| postgres | 5432 | 主資料庫 | - |
| redis | 6379 | 快取 + 訊息佇列 | - |
