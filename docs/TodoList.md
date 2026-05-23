# PACS 開發待辦事項 (TODO)



## 1. 前端 (Frontend)

- [✅] **移除「主管視野」並整合至「出席報表」**：
  
  - **出席報表整合**：
    - **權限與角色自適應 (Dynamic Role Adaptation)**：
      - 當系統檢測到目前模擬登入者為一般員工時，只顯示該員工個人的當日出席摘要。
- [✅] **詳細歷程 Modal（點擊 Row 彈出垂直員工當日刷卡歷史）**：
  - **觸發與互動機制**：
    - 在「出席報表」的表格中，將每一列的 `員工 ID` 或新增的 `操作` 欄位設計為可點擊的連結/按鈕（配有 🔍 圖示與懸停微動畫效果），或者整行 Row 支援點擊。
    - 點擊後，發送 API 請求 `GET /v1/audit?badge_id={id}&date={date}` 獲取當日刷卡明細，在加載期間顯示精美的半透明 Loading 骨架屏或 Spinner 載入動畫。



## 2. 後端 (Backend)
- [✅] **停留時數修正 (0105)**：migration `0105_fix_stay_hours_calc` 重建 `mv_daily_attendance`，stay_hours 改用「IN/OUT counter pairing + Asia/Taipei 00:00 切片」演算法。
  - 同日午休（IN→OUT→IN→OUT）正確扣除休息時間
  - 跨午夜（IN 23:00 → OUT 02:00）依 Taipei midnight 切分計入對應日期
  - Tier-1/Tier-2 巢狀（IN1, IN2, OUT2, OUT1）視為單一 visit
  - Orphan IN/OUT（未配對）自動丟棄
  - `QueryAttendance` 改讀 MV，與 manager-team / trend / aggregated 同源；代價：5min eventual consistency（demo 想即時可手動 REFRESH）
- [✅] **支援模擬時間戳 (0103)**：`POST /v1/swipe` 加 optional `event_time`（RFC3339）。空 → server time；畸形 → 400 `ERR_INVALID_EVENT_TIME`。InsertEvent 的 `event_time AT TIME ZONE 'Asia/Taipei'` 路由到對應月份 partition，事件可回放到 2025-01 ~ 2027-12 任一月。



## 3. 資料庫 (Database)
- [ ] **大規模模擬樣本 (0104)**：生成大量刷卡紀錄，產出滿足 HW2 規格（本地/雲端）。可參考 [SimulationGuide.md](SimulationGuide.md) 但必須改成使用 **HTTP POST** 方式進行模擬(利用後端更改支援模擬時間戳 (0103)內容)以測試真實 API 壓力，也可再加上K6做補充純負載測試。


## 4. K8s GKE
- [ ] **部署所有專案上 GKE**
---

