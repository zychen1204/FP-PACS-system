# PACS 開發待辦事項 (TODO)



## 1. 前端 (Frontend)
- [ ] **詳細歷程 Modal**：點擊報表 Row 彈出垂直當日刷卡歷史。


## 2. 後端 (Backend)
- [ ] **停留時數修正 (0105)**：將目前「頭尾相減」邏輯改為「當日所有廠內時間累加」。


- [ ] **Audit Trail API (歷程查詢)**：
  - **功能**：依據資料庫存儲的刷卡歷史，回傳特定員工當日的完整歷程給前端。
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
- [ ] **大規模模擬樣本 (0104)**：可參考 [SimulationGuide.md](SimulationGuide.md) 擴充，產出滿足 HW2 規格一個本地端跑的、一個上雲端的規格 大量刷卡紀錄（需包含單日多次進出與符合 APB 邏輯）。

- [ ] **新增全日刷卡歷史資料表**：將原本`access_events` 完整記錄前端傳入的每一筆刷卡紀錄，並作為報表查詢的唯一來源給後端使用。


## 4. K8s GKE
- [ ] **部署所有專案上 GKE**
---

