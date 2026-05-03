# 🎯 PACS 系統完整測試指南

## 📋 項目說明

**PACS = Physical Access Control System（物理訪問控制系統）**

一個員工刷卡管理系統：
- 📊 追蹤員工進出（進入/離開）
- 🔐 防止反尾隨（同方向連續刷卡被拒）
- 📈 生成出席報表

---

## 🏗️ 項目結構

```
pacs/
├── frontend/              ← 前端 Web 應用（HTML/CSS/JS）
│   ├── TESTING.md        （前端測試指南）
│   ├── index.html        （主頁面）
│   ├── app.js            （應用邏輯）
│   └── style.css         （樣式）
│
└── backend/              ← 後端 Go 微服務
    ├── TESTING.md        （後端測試指南）
    ├── main.go           （程序入口）
    ├── go.mod            （依賴管理）
    ├── handlers/         （API 處理程序）
    └── models/           （數據結構）
```

---

## 🚀 快速開始

### 步驟 1: 啟動後端

**Windows PowerShell:**
```powershell
cd backend
$env:Path += ";C:\Program Files\Go\bin"
go run main.go
```

**確認後端運行:**
```
✓ Access API:       http://localhost:8080/v1/swipe
✓ Reporting API:    http://localhost:8081/v1/reports/attendance
🔐 Starting Access API on port 8080...
📊 Starting Reporting API on port 8081...
```

### 步驟 2: 打開前端

在瀏覽器中打開 `frontend/index.html`：

**方式 1（推薦）：** 在 VS Code 中右鍵點擊 `frontend/index.html` → "Open with Live Server"

**方式 2：** 手動打開，在瀏覽器網址欄輸入：
```
file:///<your-project-path>/pacs/frontend/index.html
```

**方式 3（簡單）：** 在 PowerShell 中執行：
```powershell
Start-Process "$PWD/frontend/index.html"
```

### 步驟 3: 測試系統

1. 切換到「⚙️ 設定」標籤
2. 點擊「🔗 測試連線」
3. 應看到：`✓ Access API 連線成功` 和 `✓ Reporting API 連線成功`

---

## 📊 測試流程

### 測試 1: 正常刷卡

1. 切換到「🔑 刷卡模擬」標籤
2. 輸入:
   - 員工 ID: `E001`
   - 廠區: `Site-A`
   - 門閘: `Gate-1`
   - 方向: `進入 (IN)`
3. 點擊「送出刷卡請求」
4. ✓ 顯示「進入成功」(200)

### 測試 2: 反尾隨機制

1. 再次送出相同刷卡（還是進入 IN）
2. ✗ 應顯示「反尾隨被拒」(403)

### 測試 3: 改變方向

1. 改為「離開 (OUT)」
2. ✓ 應成功（200）

### 測試 4: 再次進入

1. 改為「進入 (IN)」
2. ✓ 應成功（200）- 因為上次是 OUT

### 測試 5: 查看報表

1. 切換到「📊 出席報表」標籤
2. 點擊「取得報表」
3. ✓ 應顯示員工 E001 的進出記錄

---

## 🔄 系統數據流

```
前端 (HTML/JS)
    ↓
[POST /v1/swipe]  或  [GET /v1/reports/attendance]
    ↓
後端 API (Go)
    ├── Access API (Port 8080)
    │   ├── 驗證刷卡請求
    │   ├── 檢查反尾隨
    │   └── 記錄訪問日誌
    │
    └── Reporting API (Port 8081)
        ├── 讀取訪問日誌
        ├── 按員工分組
        └── 生成報表
    ↓
共享狀態 (メモリ中)
    ├── AccessLog []
    └── AntiPassback map
    ↓
前端渲染結果
```

---

## 📝 詳細測試指南

### 前端測試
👉 詳見: [frontend/TESTING.md](frontend/TESTING.md)
- UI 元件測試
- localStorage 測試
- 連線測試

### 後端測試
👉 詳見: [backend/TESTING.md](backend/TESTING.md)
- API 端點測試
- 反尾隨邏輯驗證
- 報表生成驗證

---

## ✅ 驗證清單

### 後端驗證
- [ ] Go 已安裝 (`go version`)
- [ ] 後端啟動無錯誤
- [ ] Port 8080 可訪問
- [ ] Port 8081 可訪問
- [ ] Access API 回應 200
- [ ] Reporting API 回應 200

### 前端驗證
- [ ] 前端頁面打開正常
- [ ] 三個標籤可正常切換
- [ ] 測試連線成功
- [ ] 可發送刷卡請求
- [ ] 可查看報表
- [ ] 設定可保存

### 功能驗證
- [ ] 首次刷卡成功
- [ ] 反尾隨被正確拒絕
- [ ] 改變方向後可進行
- [ ] 報表正確顯示數據
- [ ] CORS 正常工作

---

## 🐛 故障排查

### 後端無法啟動

**症狀:** `go: command not found`

**解決:**
```powershell
$env:Path += ";C:\Program Files\Go\bin"
go version  # 驗證
go run main.go
```

### 前端無法連接後端

**症狀:** 「🔗 測試連線」顯示紅色 ✗

**檢查:**
1. 後端是否運行？
2. Port 是否正確？
3. 防火牆設定？
4. 在「⚙️ 設定」修改 API 地址

### Port 被占用

**症狀:** `address already in use`

**解決:**
1. 找出占用進程: `netstat -ano | findstr :8080`
2. 關閉進程或修改 main.go 中的端口

---

## 📚 相關文檔

- [frontend/README.md](frontend/README.md) - 前端架構說明
- [backend/README.md](backend/README.md) - 後端架構說明
- [frontend/TESTING.md](frontend/TESTING.md) - 前端詳細測試指南
- [backend/TESTING.md](backend/TESTING.md) - 後端詳細測試指南

---

## 🎯 測試順序建議

1. **確認後端運行**
   ```
   cd backend
   $env:Path += ";C:\Program Files\Go\bin"
   go run main.go
   ```

2. **打開前端頁面**
   
   在瀏覽器打開 `frontend/index.html`（具體路徑取決於你的項目位置）

3. **測試連線**
   - ⚙️ 設定 → 🔗 測試連線

4. **進行功能測試**
   - 刷卡模擬
   - 反尾隨機制
   - 報表查詢

5. **使用 curl 進行 API 測試（可選）**
   ```bash
   curl http://localhost:8080/healthz
   curl http://localhost:8081/healthz
   ```

