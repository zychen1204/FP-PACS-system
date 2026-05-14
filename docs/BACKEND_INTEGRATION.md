# PACS 前端升級 v2.0 — 後端整合規格 (極簡版)

本文件定義前端與後端通訊的標準格式與處理邏輯。

---

## 1. 門禁刷卡 (Access API)

### POST `/v1/swipe`
*   **前端傳輸格式 (Input)**
    ```json
    {
      "badge_id": "B001",
      "gate_id": "1-A",        // 格式：[層級]-[編號]
      "direction": "IN"        // IN (進入) 或 OUT (離開)
    }
    ```
*   **後端應該做什麼 (Logic)**
    1.  **階層驗證**：依照「外層 (Tier 1) -> 緩衝區 -> 內層 (Tier 2)」邏輯。必須先 IN 刷過外層，才能 IN 刷內層。
    2.  **Anti-Passback**：防止連續兩次同方向刷卡。
    3.  **架構參考**：
        ```text
                          外部區域 (Outside Area)
               +---------------[門 1-A]---------------+
               |          中間緩衝區 (Buffer)         |
           [門 1-B]                                [門 1-C]  <-- 外層 (Tier 1)
               |       +----------------------+       |
               |       |     內部核心區域     |       |
               |   [門 2-A]  (Inside Area)  [門 2-C]  |  <-- 內層 (Tier 2)
               |       |                      |       |
               |       +-------[門 2-B]-------+       |
               |                                      |
               +--------------------------------------+
        ```
*   **後端應傳輸格式 (Output)**
    *   **成功 (200)**: `{"status":"SUCCESS"}`
    *   **失敗 (403)**: `{"status":"REJECTED_APB", "reason":"未進入外層閘門"}`

---

## 2. 數據報表 (Reporting API)

### 2.1 主管團隊報表: `GET /v1/reports/manager-team`
*   **前端傳輸格式 (Input)**
    *   `as`: 主管 Badge ID (例: `B100`)
    *   `date`: 查詢日期 (例: `2026-05-14`)
*   **後端應該做什麼 (Logic)**
    1.  驗證 `as` 是否具備主管權限 (`is_manager=true`)，若無則回傳 **403**。
    2.  根據主管的 `org_path` 查詢其下屬所有層級員工的出席紀錄。
*   **後端應傳輸格式 (Output)**
    ```json
    {
      "manager_scope": "TSMC.Fab12.MFG",
      "reports": [
        {
          "employee_id": "B001",
          "name": "王小明",
          "is_manager": false,
          "org_path": "TSMC.Fab12.MFG.P1",
          "work_date": "2026-05-14",
          "first_in": "2026-05-14T08:00:00Z",
          "last_out": "2026-05-14T17:30:00Z",
          "swipe_count": 4,
          "stay_hours": 9.5
        }
      ]
    }
    ```

### 2.2 出勤趨勢分析: `GET /v1/reports/trend`
*   **前端傳輸格式 (Input)**
    *   `period`: `day`, `week`, `month`, `quarter`
    *   `as`: 查詢範圍 ID (主管 Badge ID)
    *   `start_date` / `end_date`: 日期區間
*   **後端應該做什麼 (Logic)**
    1.  對 `mv_daily_attendance` 視圖進行時間聚合。
    2.  計算期間內的平均工時、總人數與總刷卡次數。
*   **後端應傳輸格式 (Output)**
    ```json
    {
      "scope": "TSMC.Fab12",
      "trends": [
        { 
          "bucket": "2026-05-11", 
          "head_count": 42, 
          "avg_stay_hrs": 8.2, 
          "total_swipes": 156 
        }
      ]
    }
    ```

---

## 3. 系統警報 (Alerts API)

### GET `/v1/alerts`
*   **後端應該做什麼 (Logic)**：回傳系統偵測到的異常（如：APB 違規、停留時間過長）。
*   **後端應傳輸格式 (Output)**
    ```json
    [
      {
        "id": 101,
        "severity": "CRITICAL",
        "message": "員工 B005 違規進入內層 (未經過外層)",
        "gate_id": "2-A",
        "timestamp": "2026-05-14T10:00:00Z"
      }
    ]
    ```
