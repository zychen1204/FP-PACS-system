# PACS 資料庫 Spec ↔ 實作 ↔ 實測 對照矩陣

本文件是「規範符合性」的單頁稽核紀錄。每行：

1. **Spec 條款**（FR / NFR 編號 + 摘要 / 來源）
2. **對應實作**（檔案 + migration 編號）
3. **驗證指令**（可重跑）
4. **實測結果**（實際輸出）
5. **階段**（P1 / P2 / 已落地 / deferred）

> **驗證環境**：`docker compose down -v && docker compose up -d --build`
> 後在 host 機 macOS（Darwin 25.x）跑下表中的 Bash 指令。
> 詳細步驟見 [`../TESTING.md`](../TESTING.md) 與
> [`PHASE2_VERIFICATION.md`](PHASE2_VERIFICATION.md)（19 個 section 完整劇本）。

> **狀態**：Phase 1 + Phase 2 全部已落地。本表反映當前 main 分支的狀態。

---

## 1. 功能性規範（FR）

### FR-3 拒絕原因回讀

| 項目 | 內容 |
|---|---|
| 規範 | 拒絕門禁時須回傳原因代碼（Anti-Passback、Offline 等） |
| 實作 | `access_events.reason TEXT DEFAULT ''`（migration `0001`） |
| 驗證 | `psql -c "SELECT reason, count(*) FROM access_events WHERE reason <> '' GROUP BY reason ORDER BY count(*) DESC;"` |
| 實測結果 | `reason` 欄位確實在儲存可變長度的拒絕/標註原因，至少出現 anti-passback (`same direction within 30s`) 與未註冊卡 (`unregistered badge`) 兩類語意 |
| 階段 | **P1 ✅** |

### FR-4 事件非同步持久化

| 項目 | 內容 |
|---|---|
| 規範 | DB 不在門禁決策 hot path；事件透過 MQ 緩衝 |
| 實作 | access-api 不接 DB；event-processor 從 Redis Streams `pacs:events` 拉取後寫入；Phase 2 加 DLQ (`pacs:events:dead`，3 次重試後)；docker-compose 中 event-processor `depends_on` migrate 完成 |
| 驗證 | 檢查 `backend/cmd/access-api/main.go` 的 import；觀察 `docker compose up` 啟動順序日誌 |
| 實測結果 | `access-api` import 區塊只有 `pacs/backend/internal/{cache,models,queue}` — **沒有 `internal/db`**。migrate 1~6 + 99 退出 0 後其餘 service 才啟動 |
| 階段 | **P1 ✅** + **P2 DLQ 加成** |

### FR-5 個人出勤紀錄

| 項目 | 內容 |
|---|---|
| 規範 | 每筆事件含 timestamp / gate / direction；當日在廠停留時數 |
| 實作 | `access_events` 的 `event_time / gate_id / direction` 欄位；`mv_daily_attendance` MV 預聚合（migration `0006` + `0105`）；reporting-api `QueryAttendance` 讀 MV |
| stay_hours 演進 | 0006 baseline = `last_out - first_in` (head-tail)。**0105 fix**：改用 LAG window function 配對 IN→OUT 累加，午餐 / 外出時段不算廠內 |
| 驗證 | `curl http://localhost:8081/v1/reports/attendance` |
| 預期 | 員工 8:00 IN / 12:00 OUT / 13:00 IN / 18:00 OUT — last_out - first_in = 10h，但 stay_hours = 9h（IN/OUT 累加扣午餐外出）|
| 階段 | **P1 ✅** + **P2 query 改寫命中索引 + 改讀 MV** + **0105 fix stay_hours 累加** |

### FR-6 階層式組織報表

| 項目 | 內容 |
|---|---|
| 規範 | manager 看子樹 drill-down |
| 實作（DB 層）| `employees.org_path_ltree LTREE` + `idx_employees_org_path_gist`（migration `0003`） |
| 實作（API 層）| reporting-api `/v1/reports/manager-team`：`GetManagerScope` 取 caller scope（空回 403），`QueryManagerTeamAttendance` 用 `org_path_ltree <@ $scope::ltree` 命中 GiST |
| 驗證 | `curl 'http://localhost:8081/v1/reports/manager-team?as=B100'`（廠長視野）vs `?as=B011`（非主管 → 403） |
| 實測結果 | B100 (`scope=TSMC.Fab12`) 看到製造部 + 品保部子樹（5 員工）；B001 部主管看到單部門（3 員工）；B011 員工 → 403 |
| 階段 | **P1 path enum** → **P2 ✅ ltree + GiST**（對齊 HW2 §5.2/§5.3） |

### FR-7 出勤趨勢報表

| 項目 | 內容 |
|---|---|
| 規範 | 日 / 月 / 季 / 年趨勢；HW2 §5.3 選型 `mv_daily_attendance` 5 min refresh |
| 實作（DB）| `mv_daily_attendance` materialized view + UNIQUE index + GiST index（migration `0006`）|
| 實作（refresh）| 獨立 `mv-refresher` service：`REFRESH MATERIALIZED VIEW CONCURRENTLY mv_daily_attendance` 每 300 秒（env `REFRESH_INTERVAL_SECONDS`）|
| 實作（API）| reporting-api `/v1/reports/trend?period=day|week|month|quarter`：在 MV 上 `date_trunc` bucket，scope manager 自動限縮 |
| 驗證 | `curl 'http://localhost:8081/v1/reports/trend?as=B100&period=day'`；觀察 `mv-refresher` logs |
| 實測結果 | day/week/month/quarter 4 種 bucket 都正確聚合；`mv-refresher` log 顯示 `[REFRESH] mv_daily_attendance (22.061353ms)` |
| 階段 | **P2 ✅ 已落地** |

### FR-9 階層式資料權限

| 項目 | 內容 |
|---|---|
| 規範 | manager 只看 org tree 子節點；跨部門查詢回 403 |
| 實作（DB 層）| 提供 `org_path_ltree` (migration `0003`) + `job_level` VARCHAR + CHECK (migration `0102`，取代早期 `is_manager` BOOLEAN) — schema 完備且支援多階主管 |
| 實作（API 層）| reporting-api 採 pattern a 兩段式：`GetManagerScope(badge)` 空回 403，否則 `<@` filter；JWT middleware (`internal/auth`) 提供 badge_id |
| Backend 實作範本（pattern a）| ```sql<br>-- Step 1: 驗證 caller 是主管 + 取 scope<br>SELECT org_path_ltree::text FROM employees<br>WHERE badge_id = $1 AND job_level <> 'STAFF' AND is_active = TRUE;<br>-- 若回傳空 → backend 回 403 Forbidden<br><br>-- Step 2: 用 Step 1 的 path 過濾子樹（命中 GiST）<br>SELECT ... FROM mv_daily_attendance<br>WHERE org_path_ltree <@ $2::ltree;<br>``` |
| 驗證 | `curl -w 'http=%{http_code}\n' 'http://localhost:8081/v1/reports/manager-team?as=B011'` |
| 實測結果 | B011 非主管 → `http=403 {"badge_id":"B011","error":"not a manager"}` |
| 為何 DB 層不直接 enforce | DB 沒有「session 身份」概念（pacs_reporter 是共用 read role）；身份驗證屬 API 層責任（JWT / OIDC），DB 層只能提供 schema 與資料 |
| 階段 | **P2 ✅ DB + API 全落地** |

### FR-11 異常進出警報

| 項目 | 內容 |
|---|---|
| 規範 | Anti-Passback、非工時進入等異常，30 秒內推送（App + Email） |
| 實作（DB 層）| `alerts` 表（migration `0004`），含 `alert_type` CHECK 列舉、`severity`、`details JSONB`、`occurred_at` / `resolved_at` |
| 實作（規則引擎）| `anomaly-detector` service（`backend/cmd/anomaly-detector/`）獨立 consumer group `anomaly-detectors` 消費 `pacs:events`，3 條規則：(a) `OFF_HOURS_ENTRY` 台北 22:00~06:00 SUCCESS IN (b) `APB_BURST` 同 badge 30 分鐘內 REJECTED_APB ≥ 3 (c) `TAILGATING` 同 gate 5 秒內 SUCCESS IN ≥ 3 |
| 實作（讀路徑）| reporting-api `/v1/alerts?open=true&limit=N` |
| 驗證 | 連刷 4 次 same direction 觸發 APB_BURST：`for i in 1 2 3 4; do curl -X POST -d ... http://localhost:8080/v1/swipe; done; sleep 3; curl http://localhost:8081/v1/alerts` |
| 實測結果 | alert 寫入 `alerts` 表（id=7, alert_type=APB_BURST, severity=HIGH, badge=V_ALERT）；anomaly-detector log 顯示 `[ALERT] APB_BURST severity=HIGH badge=V_ALERT`；reporting-api 即時讀到 |
| 已知簡化 | `STAT_OUTLIER` 規則保留 enum 但未實作；推播管道（webhook / email）未實作（HW2 規範說 30s App + Email，本實作只到 DB 寫入）|
| 階段 | **P2 ✅ 規則 + 警報持久化** ／ **App/Email 推播 deferred** |

### FR-12 不可變更稽核（**雙層保護**）

| 項目 | 內容 |
|---|---|
| 規範 | append-only；DB 層 REVOKE UPDATE/DELETE |
| 實作 | (a) `REVOKE UPDATE, DELETE ON access_events FROM pacs_user` (b) `BEFORE UPDATE OR DELETE` trigger (c) `BEFORE TRUNCATE` trigger。Phase 2 partition swap 後 trigger 重掛到 partition root |
| 驗證 | `psql -c "DELETE FROM access_events WHERE id=1;"` 應失敗；UPDATE / TRUNCATE 同理 |
| 實測結果（DELETE） | `ERROR:  Updates and deletes are not allowed on the access_events table (FR-12 compliance)` |
| 實測結果（UPDATE） | 同上 |
| 實測結果（TRUNCATE） | 同上 |
| 階段 | **P1 ✅✅**（雙層保護）+ **P2 partition root 重掛 trigger 驗證仍生效** |

### FR-13 Audit 查詢（badge × 日期範圍）

| 項目 | 內容 |
|---|---|
| 規範 | 員工 ID + 日期範圍，10s 內回傳 |
| 實作 | `idx_events_badge_eventdate (badge_id, event_date DESC)` 在每個 partition 上自動傳播；Phase 2 改 `WHERE event_date BETWEEN` 直接命中 |
| 驗證 | `EXPLAIN ANALYZE SELECT * FROM access_events WHERE badge_id='B001' AND event_date BETWEEN ... ORDER BY event_time DESC LIMIT 100;` |
| 實測結果（10k rows fixture） | `Bitmap Index Scan on access_events_y2026m05_badge_id_event_time_idx`、`Subplans Removed: 35`（partition pruning）、Execution Time **0.331 ms** |
| 階段 | **P1 ✅** + **P2 query 改寫直接命中索引** |

---

## 2. 非功能性規範（NFR）

### NFR-1 寫入 P99 < 50 ms（門禁決策）

| 項目 | 內容 |
|---|---|
| 規範 | access-api swipe P99 < 50 ms |
| 實作 | access-api 完全不打 DB；只讀 Redis cache + 寫 Redis Streams |
| 驗證 (1) — 微量 baseline | 20 次 swipe + 計算 avg latency |
| 實測結果 (1) | avg 1.55 ms / min 0.99 ms / max 5.44 ms（30× margin） |
| 驗證 (2) — 換班 burst | `scripts/k6-load-test/shift_burst.js`（HW2 §4.2 換班尖峰，5→100 QPS ramp + plateau）|
| Threshold (k6) | `http_req_duration{endpoint:swipe} p(99)<50` 由 k6 自動斷言 pass/fail |
| 階段 | **P1 ✅** + **k6 持續驗證** |

### NFR-2 報表 P95 < 200 ms

| 項目 | 內容 |
|---|---|
| 規範 | 報表查詢 P95 < 200 ms |
| 實作（索引）| partial index `idx_events_status_date (event_date, badge_id) WHERE status='SUCCESS'`（attendance）、`idx_events_badge_eventdate (badge_id, event_date DESC)`（audit） |
| 實作（partition）| 按月 partition + automatic pruning |
| 實作（MV）| `mv_daily_attendance` 預聚合（含 0105 stay_hours fix）；`QueryAttendance` 與 `QueryManagerTeamAttendance` 皆讀 MV |
| 驗證 (1) — query plan | 載入 10k fixture → `EXPLAIN ANALYZE` 兩條 query |
| 實測結果（attendance）| `Bitmap Index Scan on access_events_y2026m05_event_date_badge_id_idx` + `Subplans Removed: 35` + Execution Time **2.564 ms**（78× margin） |
| 實測結果（audit）| `Bitmap Index Scan on access_events_y2026m05_badge_id_event_time_idx` + Execution Time **0.331 ms**（600× margin） |
| 驗證 (2) — write + read 並行 | `scripts/k6-load-test/mixed_read_write.js`（swipe burst + report constant rate 同時跑）|
| Threshold (k6) | `http_req_duration{endpoint:report} p(95)<200` 由 k6 自動斷言 |
| 階段 | **P1 索引** + **P2 partition + MV** + **k6 持續驗證** 全落地 |

### NFR-5 DB 失效時事件不可丟

| 項目 | 內容 |
|---|---|
| 規範 | DB 故障時事件 buffer 在 MQ |
| 實作 | Redis Streams `pacs:events` 在 DB 之前；event-processor 拉不到 DB 時事件留 stream；Phase 2 加 DLQ `pacs:events:dead` 在重試 3 次後失敗事件不卡 main stream |
| 驗證 | `docker compose stop postgres` 後 swipe 仍 200；`docker compose start postgres` 後 stream 內事件補進 DB |
| 實測結果 | DB stop 期間 swipe 仍 200、Redis stream 長度從 28 → 29；DB 復原 12s 後 BUFFER01 row 出現在 `access_events` |
| 階段 | **P1 ✅** + **P2 DLQ 加成** |

### NFR-7 Observability

| 項目 | 內容 |
|---|---|
| 規範 | 監控 query latency 與異常 |
| 實作 | postgres `command:` 啟用 `pg_stat_statements`、`log_min_duration_statement = 100ms`、`log_line_prefix = '%t [%p]: db=%d,user=%u '`；baseline migration `0001` 內含 `CREATE EXTENSION pg_stat_statements`；PR #3 加 Prometheus + Grafana |
| 驗證 | `psql -c "SELECT count(*) FROM pg_stat_statements;"` |
| 實測結果 | 已追蹤 98 條 query；max calls 29 |
| 階段 | **P1 ✅** + **P2 (PR #3) Prometheus + Grafana** |

### NFR-8 Immutable audit

| 項目 | 內容 |
|---|---|
| 規範 | append-only |
| 實作 | 同 FR-12 |
| 驗證 | 同 FR-12 |
| 實測結果 | 見 FR-12 |
| 階段 | **P1 ✅✅** |

---

## 3. 角色與最小權限

### 角色分離

| 項目 | 內容 |
|---|---|
| 設計 | 寫入角色（`pacs_user`，含 event-processor / anomaly-detector / mv-refresher / org-sync）vs 唯讀角色（`pacs_reporter`，reporting-api） |
| 實作 | baseline migration `0001` 內建 `pacs_reporter` 角色；migration `0004` 加 alerts 表 grants；docker-compose `reporting-api.environment.DB_USER=pacs_reporter` |
| 驗證 | `psql -U pacs_reporter` 跑 SELECT / INSERT 比對 |
| 實測結果（reporter SELECT） | `SELECT count(*) FROM access_events;` 回 count（正常） |
| 實測結果（reporter INSERT 應 denied） | `ERROR:  permission denied for table access_events` |
| 階段 | **P1 ✅** + **P2 alerts/MV 表也 grant 對應角色** |

---

## 4. Phase 2 升級項目落地狀態

| 項目 | 規範來源 | 落實位置 | 狀態 |
|---|---|---|:---:|
| 按月 partitioning | HW2 §5.3 | migration `0005` | ✅ |
| `mv_daily_attendance` materialized view | HW2 §5.3 | migration `0006` + `mv-refresher` service | ✅ |
| ltree + GiST index | HW2 §5.2/§5.3 | migration `0003` + `trg_sync_org_path_ltree` | ✅ |
| `alerts` 表 + anomaly-detector | FR-11 / HW2 §5.3 | migration `0004` + `backend/cmd/anomaly-detector/` | ✅ |
| MQ DLQ (`pacs:events:dead`) | HW2 §5.3 Pub/Sub+DLQ | `backend/internal/queue/stream.go` | ✅ |
| Read replica（demo 簡化）| HW2 §5.3 | docker network alias `postgres-replica` | ⚠️ demo 用 alias；正式環境換真 streaming replica |
| `job_level` 多階主管 | FR-6/9 進化 | migration `0102`（取代 `is_manager` BOOLEAN）| ✅ |
| Phase 1 baseline seed (1k) | HW2 §4.1 | migration `0103_seed_local` (auto) | ✅ |
| Phase 3 cloud seed (90k) | HW2 §4.3 | `cloud_migrations/0104_cloud_seed`（手動） | ✅ 檔案就緒 |
| stay_hours IN/OUT 累加 | FR-5 嚴謹語意 | migration `0105` 改 MV 定義 | ✅ |
| seed-generator (Go) 動態規模 | HW2 §4 三個 Phase | `scripts/seed-generator/` `--mode local\|fab\|cloud` | ✅ |
| k6 shift-burst 壓測 | HW2 §4.2 + spec「Shift Change spike」+ NFR-1/2/4 | `scripts/k6-load-test/*.js` + `k8s/07-k6-load-test.yaml` | ✅ |
| HA / 99.9% (NFR-3) | HW2 §5.3 | 本機 demo 不適用 | ⏸ Phase 3 |
| Encryption at rest (NFR-6) | HW2 §5.3 | production deployment | ⏸ infra 層 |

---

## 4.5 Schema gap closure — FR-6 / FR-9 演進

PR #1 baseline 落地後深度核對 spec 發現，`org_path` VARCHAR 只能描述員工
歸屬部門，但無法 (a) 識別主管、(b) 高效做子樹查詢。修補分三段：

1. **migration `0002`**：加 `is_manager BOOLEAN`，補 3 個 demo 員工讓階層查詢有實質範圍
2. **migration `0003` (Phase 2)**：加 `org_path_ltree LTREE` + GiST index 對齊 HW2 §5.2/§5.3，
   API 改用 `<@` ancestor 命中 GiST
3. **migration `0102`**：以 `job_level VARCHAR(20) CHECK IN ('STAFF','MANAGER_L1','MANAGER_L2')`
   取代 `is_manager`，讓「一級主管 vs. 二級主管」可在 DB 層識別；scope 語意不變

### 評估過但未採用的方案

| 方案 | 為何不採 |
|---|---|
| `parent_badge_id` 自參照 adjacency list | recursive CTE 計畫不穩；HW2 也沒選 |
| 純 path enumeration（VARCHAR LIKE prefix） | Phase 1 baseline 為求快採用過；Phase 2 對齊 HW2 規格升 ltree |
| Closure table | 過度設計；Phase 1 + 2 規模下 ltree GiST 足夠 |

### 採用方案的成本

- 雙欄位並存（`org_path` VARCHAR + `org_path_ltree` LTREE）— 多 1 個 LTREE 欄 + 1 個 GiST index
- 1 個 BEFORE INSERT/UPDATE trigger 自動同步
- ltree label 需要 PG 16 + UTF-8 locale 才接受中文（已升）

---

## 5. 已知 follow-up（不在本資料庫範圍）

- ✅ ~~`backend/internal/db/postgres.go` 的 `QueryAttendance` / `QueryAuditTrail` 使用 `event_time::date`~~ — **已在 PR #2 修，改用 `event_date` 命中索引**
- ⏸ FR-11 推播管道（webhook / email）：HW2 規範說 30s App + Email；本實作只到 DB 寫 `alerts`。前端可從 `/v1/alerts` 拉，正式推播待後續
- ⏸ FR-8 PDF 匯出：本階段只交 Excel（excelize）；PDF 需要 gofpdf + Noto Sans CJK TTF
- ⏸ Read Replica 真 streaming replication：目前是 docker network alias 指同 DB
- ⏸ `org-sync` 接真 LDAP/AD：目前是 mock 靜態資料
- ⏸ pg_partman 自動預建下個月 partition：目前預建到 2027-12

---

## 6. 驗證再現性

完整驗證流程：

```bash
cd /path/to/FP-PACS-system
docker compose down -v
docker compose up -d --build
sleep 25

# Step 1: 9 service 健康
docker compose ps

# Step 4: 既有 schema
docker compose exec postgres psql -U pacs_user -d pacs_db -c "SELECT count(*) FROM access_events;"
docker compose exec postgres psql -U pacs_user -d pacs_db -c "SELECT count(*) FROM alerts;"
docker compose exec postgres psql -U pacs_user -d pacs_db -c "SELECT count(*) FROM mv_daily_attendance;"

# Step 9: FR-12 immutability (DELETE / UPDATE / TRUNCATE 三擋)
docker compose exec postgres psql -U pacs_user -d pacs_db -c "DELETE FROM access_events WHERE id=1;"
docker compose exec postgres psql -U pacs_user -d pacs_db -c "UPDATE access_events SET status='X' WHERE id=1;"
docker compose exec postgres psql -U pacs_user -d pacs_db -c "TRUNCATE access_events;"

# Step 10: 角色最小權限
docker compose exec postgres psql -U pacs_reporter -d pacs_db -c "SELECT count(*) FROM access_events;"
docker compose exec postgres psql -U pacs_reporter -d pacs_db \
  -c "INSERT INTO access_events (badge_id, site_id, gate_id, direction, status, event_time, event_date) \
      VALUES ('B999','S','G','IN','SUCCESS',NOW(),CURRENT_DATE);"

# Step 11: NFR-2 EXPLAIN (需要先載 10k fixture 才看得到 index 勝出)
docker compose exec -T postgres psql -U pacs_user -d pacs_db <<'SQL'
INSERT INTO access_events (badge_id, site_id, gate_id, direction, status, reason, event_time, event_date)
SELECT 'L' || lpad((i % 50)::text, 4, '0'), 'Site-A', 'Gate-1',
       CASE WHEN i % 2 = 0 THEN 'IN' ELSE 'OUT' END,
       'SUCCESS', '[LOAD_TEST]',
       ((CURRENT_DATE - (i % 7) + ((i % 24) || ' hours')::interval) AT TIME ZONE 'Asia/Taipei')::timestamptz,
       (CURRENT_DATE - (i % 7))
FROM generate_series(1, 10000) AS i;
ANALYZE access_events;
SQL
docker compose exec postgres psql -U pacs_user -d pacs_db -c \
  "EXPLAIN ANALYZE SELECT badge_id, count(*) FROM access_events
   WHERE event_date = CURRENT_DATE AND status='SUCCESS' GROUP BY badge_id;"
docker compose exec postgres psql -U pacs_user -d pacs_db -c \
  "EXPLAIN ANALYZE SELECT * FROM access_events
   WHERE badge_id='L0001' AND event_date BETWEEN CURRENT_DATE - 7 AND CURRENT_DATE
   ORDER BY event_time DESC LIMIT 100;"

# Step 12: FR-11 anomaly trigger
for i in 1 2 3 4; do
  curl -sX POST -H "Content-Type: application/json" \
    -d '{"badge_id":"DEMO","site_id":"Site-A","gate_id":"Gate-1","direction":"IN"}' \
    http://localhost:8080/v1/swipe > /dev/null
done
sleep 2
curl -s http://localhost:8081/v1/alerts | jq 'length'   # 應該 ≥ 1
```

完整 19 個 section 驗收劇本見 [`PHASE2_VERIFICATION.md`](PHASE2_VERIFICATION.md)。
