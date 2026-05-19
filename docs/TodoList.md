# PACS 開發待辦事項 (TODO)



## 1. 前端 (Frontend)

- [ ] **移除「主管視野」並整合至「出席報表」**：
  
  - **出席報表整合**：
    - **權限與角色自適應 (Dynamic Role Adaptation)**：
      - 當系統檢測到目前模擬登入者為一般員工時，只顯示該員工個人的當日出席摘要。
      - 當登入者為主管時（如 `is_manager` 為 true），「出席報表」介面將動態升級為「主管級別出席報表」，並顯示「部門查詢範圍」（如 `管理部門: 部門名稱`）。
- [ ] **詳細歷程 Modal（點擊 Row 彈出垂直員工當日刷卡歷史）**：
  - **觸發與互動機制**：
    - 在「出席報表」的表格中，將每一列的 `員工 ID` 或新增的 `操作` 欄位設計為可點擊的連結/按鈕（配有 🔍 圖示與懸停微動畫效果），或者整行 Row 支援點擊。
    - 點擊後，發送 API 請求 `GET /v1/audit?badge_id={id}&date={date}` 獲取當日刷卡明細，在加載期間顯示精美的半透明 Loading 骨架屏或 Spinner 載入動畫。



## 2. 後端 (Backend)
- [ ] **停留時數修正 (0105)**：將目前「頭尾相減」邏輯改為「當日所有廠內時間累加」。
- [ ] **支援模擬時間戳 (0103)**：更改後端新增支援 HTTP POST 的`event_time` 欄位(用做模擬時間戳壓力測試)，讓資料庫組員可利用此功能生成大規模的 API HTTP 壓力測試。


- [ ] **Audit Trail API (歷程查詢)**：
  - **功能**：依據資料庫存儲的員工刷卡歷史，回傳特定員工當日的完整歷程給前端。
  - **端點**：`GET /v1/audit?badge_id={id}&date={date}`
  - **回傳格式**：
    ```json
    {
      "badge_id": "B001",
      "total_stay_hours": 8.5,
      "history": [
        { "timestamp": "08:00:00", "direction": "IN", "gate_id": "Gate-1A", "status": "SUCCESS" },
        { "timestamp": "12:00:00", "direction": "OUT", "gate_id": "Gate-1A", "status": "SUCCESS" }
      ]
    }
    ```


## 3. 資料庫 (Database)
- [ ] **大規模模擬樣本 (0104)**：生成大量刷卡紀錄，產出滿足 HW2 規格（本地/雲端）。可參考 [SimulationGuide.md](SimulationGuide.md) 但必須改成使用 **HTTP POST** 方式進行模擬(利用後端更改支援模擬時間戳 (0103)內容)以測試真實 API 壓力，也可再加上K6做補充純負載測試。

- [ ] **新增員工刷卡歷史資料表**：將原本`access_events` 完整記錄前端傳入的每一筆刷卡紀錄(依照員工ID分開)，並作為後端報表查詢的來源。


## 4. K8s GKE
- [ ] **部署所有專案上 GKE**
---

