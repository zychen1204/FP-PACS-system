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

```bash
docker-compose exec postgres psql -U pacs_user -d pacs_db -c "DELETE FROM access_events WHERE id = 1;"
```

預期回應（應該失敗）：
```
ERROR: permission denied for table access_events
```

---

## Step 10：停止服務

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
