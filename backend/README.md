# 🔧 後端架構

## 📁 目錄結構

```
backend/
├── main.go              主程序入口 - 啟動兩個 Gin 服務器
├── go.mod              依賴管理（Gin 框架）
├── go.sum              依賴鎖定
├── handlers/
│   ├── access.go       Access API (Port 8080)
│   ├── reporting.go    Reporting API (Port 8081)
│   └── state.go        共享狀態管理 (RWMutex)
└── models/
    └── types.go        數據結構定義
```


## 🏗️ 核心組件

### main.go
- 初始化 `SharedState` 共享狀態
- 啟動 Access API (Goroutine @ Port 8080)
- 啟動 Reporting API (Main @ Port 8081)
- 實現 CORS 中間件

### handlers/access.go
- **HandleSwipe()**: 處理 POST /v1/swipe
  - 驗證請求
  - 檢查反尾隨機制
  - 記錄訪問日誌
  - 返回 HTTP 200/403
- **GetMetrics()**: Prometheus 格式指標
- **HealthCheck()**: 健康狀態端點

### handlers/reporting.go
- **GetAttendanceReport()**: 處理 GET /v1/reports/attendance
  - 按員工/日期分組訪問日誌
  - 計算 first_in、last_out、swipe_count
  - 返回 JSON 格式報表

### handlers/state.go
- `SharedState` 結構體
  - `Mu sync.RWMutex` - 線程安全鎖
  - `AccessLog []models.AccessLog` - 訪問日誌
  - `AntiPassback map[string]string` - 反尾隨狀態

### models/types.go
- `AccessLog` - 單次刷卡記錄
- `SwipeRequest` - 刷卡請求 JSON
- `SwipeResponse` - 刷卡響應 JSON
- `AttendanceReport` - 出席報表數據

## 🌐 API 端點

### Access API (Port 8080)
| 方法 | 端點 | 功能 |
|------|------|------|
| POST | /v1/swipe | 提交刷卡 |
| GET | /healthz | 活動探測 |
| GET | /metrics | Prometheus 指標 |

### Reporting API (Port 8081)
| 方法 | 端點 | 功能 |
|------|------|------|
| GET | /v1/reports/attendance | 出席報表 |
| GET | /healthz | 活動探測 |
| GET | /readyz | 就緒探測 |

---

## ℹ️ 注意事項

- 依賴：**Gin Web Framework v1.9.1**
- Go 版本：**1.21+**
- 數據存儲：**內存**（演示用，重啟後數據清空）
- 端口衝突：確保 8080/8081 未被占用

詳細測試步驟請參考 [../TESTING.md](../TESTING.md)

### 反尾隨機制 (Anti-Passback)
- 同一員工連續刷入或刷出會被拒絕
- 正常流程：進入 → 進入 (✓) → 進入 (✗ 被拒) → 離開 (✓) → 進入 (✓)

### 數據共享
- 使用 Go 的全局變量和 mutex 在兩個 API 之間共享訪問日誌
- Access API 寫入日誌
- Reporting API 讀取日誌生成報表

### CORS 支援
- 所有 API 都支援跨域請求
- 前端可以從任何來源發送請求

---

## 🛠️ 開發注意事項

### 依賴
- **Gin Web Framework** - Go 的輕量級 Web 框架
- 自動下載依賴：`go mod download`

### 編譯為可執行檔
\`\`\`bash
go build -o pacs-backend.exe main.go
./pacs-backend.exe
\`\`\`

### 編譯為不同平台
\`\`\`bash
# Linux
GOOS=linux GOARCH=amd64 go build -o pacs-backend main.go

# macOS
GOOS=darwin GOARCH=amd64 go build -o pacs-backend main.go

# Windows
go build -o pacs-backend.exe main.go
\`\`\`

---

## 📊 數據流

\`\`\`
前端 (HTML/JS)
    ↓
[POST /v1/swipe]  或  [GET /v1/reports/attendance]
    ↓
Access API (Port 8080)  或  Reporting API (Port 8081)
    ↓
共享的 Go 全局狀態 (accessLog, antiPassback)
    ↓
後端處理邏輯
    ↓
返回 JSON 響應
    ↓
前端渲染結果
\`\`\`

---

## ❓ 常見問題

**Q: 如何停止伺服器？**
A: 在終端按 Ctrl+C

**Q: 如何清除訪問日誌？**
A: 重新啟動後端服務（日誌存儲在內存中）

**Q: 如何連接到遠程後端？**
A: 在前端「⚙️ 設定」中修改 API 地址

**Q: 如何部署到 Kubernetes？**
A: 編譯 Go 程序，建立 Docker 映像，使用 k8s/ 目錄中的配置文件

---

希望這個結構清晰且易於理解！
