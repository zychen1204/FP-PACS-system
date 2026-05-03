# 🎨 前端架構

## 📁 目錄結構

```
frontend/
├── index.html       主頁面 - 3 個功能標籤
├── app.js           應用邏輯 (~500 行)
├── style.css        響應式樣式 (~400 行)
└── README.md        本文件
```

## 🏗️ 核心組件

### index.html (~500 行)
包含 3 個功能標籤：

1. **🔑 刷卡模擬** (Tab 1)
   - 輸入徽章 ID
   - 選擇工地、門禁、方向（IN/OUT）
   - 提交刷卡請求
   - 顯示實時響應和歷史記錄

2. **📊 出席報表** (Tab 2)
   - 輸入 JWT 令牌（可選）
   - 提交查詢
   - 顯示員工出席統計
   - 按日期排序

3. **⚙️ 設定** (Tab 3)
   - 配置 Access API URL
   - 配置 Reporting API URL
   - 測試連線按鈕
   - 清空歷史記錄
   - 導出 CSV

### app.js (~500 行)
主要函數：

| 函數 | 用途 |
|------|------|
| `sendSwipe()` | POST /v1/swipe |
| `fetchReport()` | GET /v1/reports/attendance |
| `displaySwipeResponse()` | 更新 UI 響應 |
| `addToSwipeHistory()` | 管理歷史記錄 |
| `testServerConnection()` | 驗證後端連接 |
| `exportToCSV()` | 導出報表 |

### style.css (~400 行)
- 響應式設計（Mobile First）
- CSS 變數主題系統
- 動畫狀態指示器
- Grid/Flex 佈局
- 暗色主題支持

## 🌐 API 通信

### 技術棧
- **無框架**：使用原生 fetch() API
- **無依賴**：純 JavaScript + HTML5 + CSS3
- **存儲**：localStorage 本地持久化
- **CORS**：已在後端啟用

### 請求格式

**Access API (POST /v1/swipe)**
```json
{
  "badge_id": "E001",
  "site_id": "Site-A",
  "gate_id": "Gate-01",
  "direction": "IN"
}
```

**Reporting API (GET /v1/reports/attendance)**
```
?since=2026-05-01&jwt=token...
```

## 📱 響應式設計

- **桌面** (≥1024px)：3 欄佈局
- **平板** (768-1023px)：2 欄佈局
- **手機** (<768px)：1 欄佈局

## 🔧 開發與測試

### 打開前端
1. 在 VS Code 中右鍵點擊 `index.html` → "Open with Live Server"
2. 或在瀏覽器手動打開（file:// 協議）
3. 或用 Python 內建伺服器：
   ```bash
   python -m http.server 8000
   ```
   然後訪問 `http://localhost:8000`

詳細測試步驟請參考 [../TESTING.md](../TESTING.md)

開啟 **3 個新的終端機視窗**，分別執行以下命令（每個視窗一個）：

**終端機 1 - Access API（刷卡端點）：**
```bash
cd pacs
go run ./cmd/access-api/main.go
```

你應該看到：
```
2026/05/03 21:12:00 Starting Access API on :8080
```

**終端機 2 - Event Processor（事件處理）：**
```bash
cd pacs
go run ./cmd/event-processor/main.go
```

**終端機 3 - Reporting API（報表查詢）：**
```bash
cd pacs
go run ./cmd/reporting-api/main.go
```

你應該看到：
```
2026/05/03 21:12:05 Starting Reporting API on :8081
```

### 第 3 步：開啟前端應用

在瀏覽器中開啟前端應用：
```
file:///C:/Users/atat9/OneDrive/Desktop/cloud%20native%20project/pacs/frontend/index.html
```

或者直接在 VS Code 中右鍵 `index.html` → **Open with Live Server**（如果已安裝 Live Server 擴充功能）。

---

## 💻 前端應用使用教學

### 1️⃣ **刷卡模擬器標籤**

#### 模擬刷卡流程：

1. **輸入員工證件 ID**  
   例如：`B001`（系統預設）

2. **選擇地點（Site）**  
   - Site-A（台積電 Fab12）
   - Site-B（台積電 Fab15）
   - Site-C（總部）

3. **選擇閘門（Gate）**  
   - Gate-1, Gate-2, 或 Gate-3

4. **選擇方向**  
   - ➡️ **進入（IN）**：員工進入工作區
   - ⬅️ **離開（OUT）**：員工離開工作區

5. **送出刷卡請求**  
   點擊「🔄 送出刷卡請求」按鈕

#### 預期行為：

**首次刷卡（IN）** → ✓ 成功  
```
{"status": "SUCCESS", "message": "Access granted"}
```

**立即再次刷卡（IN）** → ✗ 防尾隨機制攔截  
```
{"status": "REJECTED_APB", "message": "Anti-Passback Violation"}
```
（防止同一人連續進入兩次，確保實體安全）

**先進後出** → ✓ 成功  
- 刷卡 IN → 成功
- 刷卡 OUT → 成功
- 再次刷卡 IN → 成功

#### 刷卡紀錄

所有刷卡操作會自動保存在表格中，包括：
- 時間、證件 ID、地點、方向、狀態

紀錄會自動保存在瀏覽器的 `localStorage` 中，重新整理頁面後仍會保留。

---

### 2️⃣ **出席報表標籤**

#### 取得出席報表：

1. **粘貼 JWT Token**  
   出於演示目的，你可以暫時跳過 Token 驗證（見下方「無 Token 測試」）

2. **點擊「📥 取得報表」**

3. **查看報表結果**  
   系統會顯示：
   - 📈 統計信息（總紀錄數、員工人數、總刷卡次數）
   - 📋 詳細表格（員工 ID、姓名、組織、進出時間、刷卡次數）

#### 無 Token 測試（開發模式）

如果後端尚未設置完整的 JWT 驗證，可以：
1. 先在「⚙️ 設定」標籤確認 API 地址正確
2. 直接點擊「📥 取得報表」（空著 Token）
3. 查看是否返回報表資料

---

### 3️⃣ **設定標籤**

#### API 連線設定

**Access API 網址**
- 預設：`http://localhost:8080`
- 修改此地址以連接到不同的 Access API 實例

**Reporting API 網址**
- 預設：`http://localhost:8081`
- 修改此地址以連接到不同的 Reporting API 實例

#### 測試連線

點擊「🔗 測試連線」按鈕驗證後端服務是否可連接。

結果會顯示：
- ✓ Access API 連線成功
- ✓ Reporting API 連線成功

#### 快速操作

- **🗑️ 清除刷卡紀錄**：刪除所有本機刷卡歷史（不影響後端資料庫）
- **📥 匯出紀錄 (CSV)**：將刷卡紀錄下載為 CSV 檔案

---

## 🧪 完整測試場景

### 場景 1：正常進出（反尾隨檢查）

```
操作流程：
1. 刷卡 IN (B001, Site-A)        → ✓ SUCCESS
2. 刷卡 IN (B001, Site-A)        → ✗ REJECTED_APB (防尾隨)
3. 刷卡 OUT (B001, Site-A)       → ✓ SUCCESS
4. 刷卡 IN (B001, Site-A)        → ✓ SUCCESS (允許再次進入)
```

### 場景 2：多位員工

```
1. 刷卡 IN (B001, Site-A, Gate-1)  → ✓ SUCCESS
2. 刷卡 IN (B002, Site-A, Gate-2)  → ✓ SUCCESS (不同員工)
3. 刷卡 IN (B003, Site-B, Gate-1)  → ✓ SUCCESS (不同地點)
```

### 場景 3：查詢報表

```
1. 點擊「📊 出席報表」標籤
2. 點擊「📥 取得報表」
3. 檢查是否看到：
   - B001 在 Site-A 的出入紀錄
   - B002 在 Site-A 的出入紀錄
   - B003 在 Site-B 的出入紀錄
```

---

## 🔍 查看系統日誌

### Access API 日誌
檢查終端機視窗 1 的輸出，查看刷卡請求的日誌：
```
2026-05-03T21:12:20+08:00 INFO Swipe request received badge_id=B001 site_id=Site-A gate_id=Gate-1
2026-05-03T21:12:21+08:00 WARN Access Rejected: APB Violation badge_id=B001 site_id=Site-A
```

### Event Processor 日誌
檢查終端機視窗 2，查看事件處理情況：
```
2026-05-03T21:12:20+08:00 INFO Processing access event badge_id=B001 status=SUCCESS
2026-05-03T21:12:21+08:00 INFO Persisting to database...
```

### 資料庫查詢
連接到 PostgreSQL 查看實際存儲的資料：
```bash
# 查看訪問事件
psql -h localhost -U pacs_user -d pacs_db -c "SELECT * FROM access_events ORDER BY timestamp DESC LIMIT 10;"

# 查看每日出席統計
psql -h localhost -U pacs_user -d pacs_db -c "SELECT * FROM mv_daily_attendance LIMIT 10;"
```

PostgreSQL 認證資訊（來自 docker-compose.yml）：
- 用戶：`pacs_user`
- 密碼：`pacs_password`
- 資料庫：`pacs_db`
- 主機：`localhost`
- Port：`5432`

---

## 🛠️ 故障排除

### ❌ 前端無法連接到後端

**症狀：**
- 前端顯示「⚙️ 設定」中的狀態為「❌ 離線」
- 發送刷卡請求時出現 CORS 錯誤

**解決方案：**
1. 確認後端微服務已啟動（檢查終端機視窗）
2. 確認 API 地址正確（「⚙️ 設定」中）
3. 點擊「🔗 測試連線」驗證
4. 如果仍不行，檢查防火牆設定或 localhost 解析

### ❌ Docker 容器啟動失敗

**症狀：**
```
unable to get image 'redis:7-alpine': error during connect
```

**解決方案：**
1. 啟動 Docker Desktop
2. 執行 `docker-compose down` 清理
3. 重新執行 `docker-compose up -d`

### ❌ Go 編譯錯誤

**症狀：**
```
cannot find module
```

**解決方案：**
```bash
cd pacs
go mod download
go mod tidy
go run ./cmd/access-api/main.go
```

### ❌ 報表查詢返回 401 Unauthorized

**症狀：**
```json
{"error": "Invalid Identity"}
```

**原因：** JWT 驗證失敗

**臨時解決方案（開發環境）：**
後端程式碼可能需要調整以支援無 Token 測試模式。編輯 `pkg/auth/auth.go`。

---

## 📚 API 參考

### Access API

#### 刷卡請求
```http
POST http://localhost:8080/v1/swipe
Content-Type: application/json

{
  "badge_id": "B001",
  "site_id": "Site-A",
  "gate_id": "Gate-1",
  "direction": "IN"
}
```

**成功回應 (200)**
```json
{
  "status": "SUCCESS",
  "message": "Access granted"
}
```

**被拒回應 (403)**
```json
{
  "status": "REJECTED_APB",
  "message": "Anti-Passback Violation"
}
```

---

### Reporting API

#### 取得出席報表
```http
GET http://localhost:8081/v1/reports/attendance
Authorization: Bearer <JWT_TOKEN>
```

**成功回應 (200)**
```json
[
  {
    "employee_id": "E001",
    "name": "Alice Chen",
    "org_path": "TSMC.Fab12",
    "work_date": "2026-05-03",
    "first_in": "2026-05-03T08:30:00Z",
    "last_out": "2026-05-03T18:00:00Z",
    "swipe_count": 2
  },
  ...
]
```

---

## 🎯 進階使用

### 1. 連接到遠端後端

假設後端部署在伺服器 `203.0.113.42`：

1. 開啟「⚙️ 設定」標籤
2. 修改 **Access API 網址** 為 `http://203.0.113.42:8080`
3. 修改 **Reporting API 網址** 為 `http://203.0.113.42:8081`
4. 點擊「🔗 測試連線」

### 2. 使用 Kubernetes 部署

部署到 K8s 集群：
```bash
kubectl apply -f k8s/base/config.yaml
kubectl apply -f k8s/apps/access-api.yaml
kubectl apply -f k8s/apps/workers.yaml
```

然後在設定中使用對應的 K8s 服務 DNS：
```
http://access-api.default.svc.cluster.local:8080
http://reporting-api.default.svc.cluster.local:8081
```

### 3. 性能測試

使用前端多次發送刷卡請求，同時監控：
- Prometheus 指標（`/metrics` 端點）
- 資料庫查詢性能
- Redis 快取命中率

```bash
# 查看 Access API 的 Prometheus 指標
curl http://localhost:8080/metrics
```

---

## 📝 開發注意事項

### 前端檔案結構
```
frontend/
├── index.html          # 主頁面
├── style.css           # 樣式表
└── app.js              # 應用邏輯
```

### 資料持久化
- 刷卡紀錄存儲在瀏覽器 `localStorage` 中
- API 設定也存儲在 `localStorage` 中
- 自動保存，無需手動操作

### CORS 政策
如果前端和後端不在同一來源，確保後端啟用了 CORS：
```go
w.Header().Set("Access-Control-Allow-Origin", "*")
w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
```

