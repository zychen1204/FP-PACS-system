# 🎯 PACS - Physical Access Control System

**員工刷卡管理系統** - 簡潔高效的前後端應用

| 功能 | 說明 |
|------|------|
| 📊 刷卡追蹤 | 記錄員工進出（IN/OUT） |
| 🔐 反尾隨機制 | 防止同方向連續刷卡 |
| 📈 出席報表 | 按員工/日期統計 |
| 🌐 Web 介面 | 現代化響應式設計 |

---

## 📁 項目結構

```
pacs/
├── frontend/           ← Web 應用（HTML/CSS/JS）
│   ├── index.html      主頁面 + 3 個功能頁簽
│   ├── app.js          應用邏輯 (~500 行)
│   └── style.css       響應式樣式 (~400 行)
│
├── backend/            ← Go 微服務
│   ├── main.go         程序入口（雙端口）
│   ├── handlers/       API 處理器
│   │   ├── access.go   刷卡 API
│   │   ├── reporting.go出席 API
│   │   └── state.go    共享狀態
│   ├── models/         數據結構
│   └── go.mod          依賴管理
│
└── TESTING.md          📖 完整測試指南
```

---

## 🚀 快速開始

### 啟動後端

**PowerShell:**
```powershell
cd backend
$env:Path += ";C:\Program Files\Go\bin"
go run main.go
```

**確認運行:**
```
✓ Access API:       http://localhost:8080/v1/swipe
✓ Reporting API:    http://localhost:8081/v1/reports/attendance
```

### 打開前端

在瀏覽器打開 `frontend/index.html`：
- 右鍵點擊文件 → "Open with Live Server"
- 或在瀏覽器網址欄輸入：`file:///<your-project-path>/pacs/frontend/index.html`

---

## 📖 文檔

| 文檔 | 內容 |
|------|------|
| **[TESTING.md](./TESTING.md)** | 📋 完整測試指南（推薦開始） |
| **[backend/README.md](./backend/README.md)** | 🔧 後端架構 |
| **[frontend/README.md](./frontend/README.md)** | 🎨 前端架構 |
```bash
docker build --build-arg SERVICE=access-api -t gcr.io/your-project/access-api .
docker build --build-arg SERVICE=event-processor -t gcr.io/your-project/event-processor .
# ... Repeat for other services
```

### 2. Apply Kubernetes Manifests
```bash
kubectl apply -f k8s/base/config.yaml
kubectl apply -f k8s/apps/access-api.yaml
kubectl apply -f k8s/apps/workers.yaml
```

## 🧪 Verification
Run the enhanced test script to verify JWT logic and metrics:
```powershell
.\scripts\test_pacs.ps1
```
*(Note: For the professional version, ensure you provide a valid JWT in the Authorization header for reporting calls.)*
