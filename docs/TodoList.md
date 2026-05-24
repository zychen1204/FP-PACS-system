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
- [❌] **~~支援模擬時間戳 (0103)~~**：主動放棄 — 接受 client 端時間戳會破壞 audit trail 可信度（見 PR #16 RexLeee 評估）。壓測改用 k6 即時 HTTP + seed-generator SQL 直灌。



## 3. 資料庫 (Database)

- [✅] **k6 壓測腳本** — `scripts/k6-load-test/` 已建立，含三個場景：
  - `shift_burst.js`：HW2 §4.2 換班尖峰，驗 NFR-1 `p(99)<50ms`
  - `steady_baseline.js`：常態 QPS 對照組
  - `mixed_read_write.js`：write + read 並行，同時驗 NFR-1 + NFR-2 `p(95)<200ms`

- [ ] **執行雲端 90k seed (`0104_cloud_seed`)** — `scripts/cloud_migrations/0104_cloud_seed.up.sql` 是 Phase 3 規模播種，**手動執行**：
  ```bash
  gcloud sql connect <INSTANCE> --user=pacs_user --database=pacs_db \
    < scripts/cloud_migrations/0104_cloud_seed.up.sql
  ```
  執行完跑 k6 `shift_burst.js`（BADGE_COUNT=90000）驗 NFR-1。

- [ ] **k6 壓測整合 Prometheus remote-write**：目前 k6 用 console summary（已能 pass/fail thresholds）。進階：讓 k6 metrics 也 push 到 `monitoring/prometheus`，在 Grafana 看 P99 趨勢線。


## 4. K8s GKE

- [ ] **部署所有專案上 GKE**
- [ ] **跑 `k8s/07-k6-load-test.yaml` 驗 HPA** — `kubectl apply` 後 `kubectl get hpa access-api -w`，預期 60 秒內 replicas 擴展（NFR-4）。

---
