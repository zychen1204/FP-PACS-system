# PACS 資料庫 Spec ↔ 實作 ↔ 實測 對照矩陣

本文件是「規範符合性」的單頁稽核紀錄。每行：

1. **Spec 條款**（FR / NFR 編號 + 摘要 / 來源）
2. **對應實作**（檔案 + migration 編號）
3. **驗證指令**（可重跑）
4. **實測結果**（實際輸出，T2 階段填入）
5. **階段**（P1 已實作 ／ P2 deferred）

> **驗證環境**：`docker compose down -v && docker compose up -d --build`
> 後在 host 機 macOS（Darwin 25.3.0）跑下表中的 Bash 指令。
> 詳細步驟見 [`../TESTING.md`](../TESTING.md)。

---

## 1. 功能性規範（FR）

### FR-3 拒絕原因回讀

| 項目 | 內容 |
|---|---|
| 規範 | 拒絕門禁時須回傳原因代碼（Anti-Passback、Offline 等） |
| 實作 | `access_events.reason TEXT DEFAULT ''`（migration 0001） |
| 驗證 | `psql -c "SELECT reason, count(*) FROM access_events WHERE reason <> '' GROUP BY reason ORDER BY count(*) DESC;"` |
| 實測結果 | <pre>                reason                \| count<br>--------------------------------------+-------<br> [LOAD_TEST]                          \| 10000<br> [DEV_SEED]                           \|    43<br> [DEV_SEED] same direction within 30s \|     1<br> [DEV_SEED] unregistered badge        \|     1</pre>reason 欄位確實在儲存可變長度的拒絕/標註原因，且至少出現 anti-passback (`same direction within 30s`) 與未註冊卡 (`unregistered badge`) 兩類語意 |
| 階段 | **P1 ✅** |

### FR-4 事件非同步持久化

| 項目 | 內容 |
|---|---|
| 規範 | DB 不在門禁決策 hot path；事件透過 MQ 緩衝 |
| 實作 | access-api 不接 DB；event-processor 從 Redis Streams `pacs:events` 拉取後寫入；docker-compose 中 `event-processor depends_on { redis: healthy, postgres: healthy, migrate: completed }` |
| 驗證 | 檢查 `backend/cmd/access-api/main.go` 的 import；觀察 `docker compose up` 啟動順序日誌 |
| 實測結果 | `access-api` import 區塊只有 `pacs/backend/internal/{cache,models,queue}` — **沒有 `internal/db`**。<br>啟動順序日誌：<pre>migrate-1  \| 1/u init_schema (28.6ms)<br>migrate-1  \| 2/u event_date_indexes (47.0ms)<br>migrate-1  \| 3/u employee_audit_cols (53.8ms)<br>migrate-1  \| 4/u read_only_role (64.4ms)<br>migrate-1  \| 99/u dev_seed (76.9ms)<br>...<br>migrate-1 Exited (0)<br>reporting-api-1 Started<br>event-processor-1 Started</pre> 證明 migrate 退出後才啟動依賴 DB 的服務 |
| 階段 | **P1 ✅** |

### FR-5 個人出勤紀錄

| 項目 | 內容 |
|---|---|
| 規範 | 每筆事件含 timestamp / gate / direction；stay_hours 由報表計算 |
| 實作 | `access_events` 的 `event_time / gate_id / direction` 欄位；reporting-api `QueryAttendance` 用 `MIN(IN) / MAX(OUT)` 配對 |
| 驗證 | `curl http://localhost:8081/v1/reports/attendance` |
| 實測結果 | reporting-api 回 5 筆出勤紀錄（節錄）：<pre>[<br>  {"employee_id":"B001","name":"王小明",<br>   "org_path":"TSMC.Fab12.製造部","work_date":"2026-05-05",<br>   "first_in":"2026-05-05T00:07:47.862Z",<br>   "last_out":"2026-05-05T18:00:00Z",<br>   "swipe_count":116,"stay_hours":17.87},<br>  {"employee_id":"B002","name":"李大華",<br>   "swipe_count":4,"stay_hours":9},<br>  ...<br>]</pre>欄位 first_in/last_out/swipe_count/stay_hours 全部填妥 |
| 階段 | **P1 ✅** |

### FR-6 階層式組織報表

| 項目 | 內容 |
|---|---|
| 規範 | manager 看子樹 drill-down |
| 實作（P1） | `employees.org_path VARCHAR(255)` 字串路徑（如 `TSMC.Fab12.製造部`），可用 `LIKE 'TSMC.Fab12%'` 前綴查詢 |
| 實作（P2 預留） | closure table `org_relations(ancestor_id, descendant_id, distance)` |
| 驗證 | `psql -c "SELECT badge_id, name, org_path FROM employees ORDER BY badge_id;"` |
| 實測結果 | <pre> badge_id \|  name  \|     org_path<br>----------+--------+-------------------<br> B001     \| 王小明 \| TSMC.Fab12.製造部<br> B002     \| 李大華 \| TSMC.Fab12.品保部<br> B003     \| 張美玲 \| TSMC.Fab15.研發部<br> B004     \| 陳志偉 \| TSMC.Fab15.設備部<br> B005     \| 林雅婷 \| TSMC.總部.人資部</pre>三層 dot-separated path（公司.廠區.部門）落地，可用 `org_path LIKE 'TSMC.Fab12%'` 做前綴 drill-down |
| 階段 | **P1 ✅**（字串版） ／ **P2 deferred**（closure table） |

### FR-7 出勤趨勢報表

| 項目 | 內容 |
|---|---|
| 規範 | 日 / 月 / 季 / 年趨勢；HW2 選型 `mv_daily_attendance` 5 min refresh |
| 實作（P1） | reporting-api 即時 `GROUP BY` 聚合（資料量小，可接受） |
| 實作（P2 預留） | `CREATE MATERIALIZED VIEW mv_daily_attendance` |
| 驗證 | `curl http://localhost:8081/v1/reports/attendance` 回非空 |
| 實測結果 | 同 FR-5 實測結果，5 員工出勤紀錄即時聚合產出，回應 < 1 s（10k+ rows fixture 已載入） |
| 階段 | **P1 ⚠️**（即時聚合） ／ **P2 deferred**（MV） |

### FR-9 階層式資料權限

| 項目 | 內容 |
|---|---|
| 規範 | manager 只看 org tree 子節點 |
| 實作 | DB 層提供 `org_path`；filter 由 reporting-api 處理 |
| 驗證 | 不在 DB 層；spec 出處：HW2 §FR-9 標註 "API 過濾" |
| 實測結果 | n/a — 不在 DB 範圍 |
| 階段 | **DB 層支援 ✅**（提供 `org_path`）／ API filter 由 backend 負責 |

### FR-12 不可變更稽核（**雙層保護**）

| 項目 | 內容 |
|---|---|
| 規範 | append-only；DB 層 REVOKE UPDATE/DELETE |
| 實作 | 全部在 baseline migration `0001`：(a) `REVOKE UPDATE, DELETE ON access_events FROM pacs_user` (b) `BEFORE UPDATE OR DELETE` trigger (c) `BEFORE TRUNCATE` trigger（補 row-level trigger 不會觸發 TRUNCATE 的旁路） |
| 驗證 | TESTING Step 9.1–9.3（DELETE / UPDATE / TRUNCATE 各跑一次） |
| 實測結果（DELETE） | <pre>$ psql -U pacs_user ... -c "DELETE FROM access_events WHERE id=1;"<br>ERROR:  Updates and deletes are not allowed on the access_events<br>        table (FR-12 compliance)<br>CONTEXT:  PL/pgSQL function protect_audit_log() line 3 at RAISE</pre> |
| 實測結果（UPDATE） | <pre>$ psql -U pacs_user ... -c "UPDATE access_events SET status='X' WHERE id=1;"<br>ERROR:  Updates and deletes are not allowed on the access_events<br>        table (FR-12 compliance)<br>CONTEXT:  PL/pgSQL function protect_audit_log() line 3 at RAISE</pre> |
| 實測結果（TRUNCATE） | <pre>$ psql -U pacs_user ... -c "TRUNCATE access_events;"<br>ERROR:  Updates and deletes are not allowed on the access_events<br>        table (FR-12 compliance)<br>CONTEXT:  PL/pgSQL function protect_audit_log() line 3 at RAISE</pre> 三項全擋，雙層保護生效 |
| 階段 | **P1 ✅✅**（超出 spec：spec 只要 REVOKE，本實作另加 trigger 雙保險，並蓋住 TRUNCATE 旁路） |

### FR-13 Audit 查詢（badge × 日期範圍）

| 項目 | 內容 |
|---|---|
| 規範 | 員工 ID + 日期範圍，10s 內回傳 |
| 實作 | `idx_events_badge_eventdate (badge_id, event_date DESC)`（baseline migration `0001`） |
| 驗證 | TESTING Step 11.3：`EXPLAIN ANALYZE SELECT * FROM access_events WHERE badge_id='B001' AND event_date BETWEEN ... ORDER BY event_time DESC LIMIT 100;` |
| 實測結果 | <pre>Limit  (cost=0.29..32.06 rows=100)<br>       (actual time=0.022..0.032 rows=100)<br>  -> Index Scan Backward using<br>     idx_events_badge_date on access_events<br>     Index Cond: badge_id = 'B001'<br>     Filter: event_date BETWEEN CURRENT_DATE-7 AND CURRENT_DATE<br>Execution Time: 0.047 ms</pre>**註**：對於「最近 N 筆 ORDER BY event_time DESC LIMIT 100」這種 pattern，optimizer 選擇 `idx_events_badge_date`（排序鍵就是 event_time）比走 `idx_events_badge_eventdate` 後再排序更便宜。兩個索引都在 baseline `0001` 中建立。0.047 ms 遠低於 NFR 預算；`idx_events_badge_eventdate` 為純按日期過濾的 query 預留 |
| 階段 | **P1 ✅**（DB 索引就位；backend `event_time::date` 查詢未直接命中新索引 — 見「已知 follow-up」） |

---

## 2. 非功能性規範（NFR）

### NFR-1 寫入 P99 < 50 ms（門禁決策）

| 項目 | 內容 |
|---|---|
| 規範 | access-api swipe P99 < 50 ms |
| 實作 | access-api 完全不打 DB；只讀 Redis cache + 寫 Redis Streams |
| 驗證 | 檢視 `backend/cmd/access-api/main.go` 的 import 區塊 |
| 實測結果 | <pre>import (<br>    "context"<br>    "fmt"<br>    "net/http"<br>    ...<br>    "pacs/backend/internal/cache"   ← Redis<br>    "pacs/backend/internal/models"<br>    "pacs/backend/internal/queue"   ← Redis Streams<br>    "github.com/gin-gonic/gin"<br>)</pre>**沒有 `pacs/backend/internal/db`** — 編譯期就保證 access-api 不可能 reach PostgreSQL；對比 reporting-api / event-processor main.go 都有 `internal/db` import |
| 階段 | **P1 ✅** |

### NFR-2 報表 P95 < 200 ms

| 項目 | 內容 |
|---|---|
| 規範 | 報表查詢 P95 < 200 ms |
| 實作 | partial index `idx_events_status_date (event_date, badge_id) WHERE status='SUCCESS'`（attendance）、`idx_events_badge_eventdate (badge_id, event_date DESC)`（audit）— 均在 baseline migration `0001` |
| 驗證 | TESTING Step 11：載入 fixture 10k → `EXPLAIN ANALYZE` 兩條 query；plan 須見 `Index Scan using ...` |
| 實測結果（attendance EXPLAIN） | fixture 載入後 `access_events` 共 10,045 筆。<pre>EXPLAIN ANALYZE SELECT badge_id, count(*) FROM access_events<br>  WHERE event_date = CURRENT_DATE AND status='SUCCESS' GROUP BY badge_id;<br><br>HashAggregate  (cost=137.22..137.28 rows=6)<br>  -> Bitmap Heap Scan on access_events<br>     Recheck Cond: event_date = CURRENT_DATE<br>                   AND status='SUCCESS'<br>     -> Bitmap Index Scan on idx_events_status_date<br>        Index Cond: event_date = CURRENT_DATE<br>Execution Time: 0.087 ms</pre>✅ 走 partial index `idx_events_status_date`，0.087 ms ≪ 200 ms |
| 實測結果（audit EXPLAIN） | <pre>EXPLAIN ANALYZE SELECT * FROM access_events<br>  WHERE badge_id='B001' AND event_date BETWEEN<br>        CURRENT_DATE-7 AND CURRENT_DATE<br>  ORDER BY event_time DESC LIMIT 100;<br><br>Limit (rows=100, actual time=0.022..0.032)<br>  -> Index Scan Backward using idx_events_badge_date<br>Execution Time: 0.047 ms</pre>✅ 走 `idx_events_badge_date`（見 FR-13 註：optimizer 對含 `ORDER BY event_time DESC LIMIT N` 的 query 偏好排序鍵為 event_time 的索引），0.047 ms ≪ 200 ms |
| 已知 follow-up | backend `QueryAttendance` / `QueryAuditTrail` 仍用 `event_time::date` 不命中新索引 — README「Backend follow-up TODO」段已列出，由 backend owner 處理 |
| 階段 | **DB 層 P1 ✅** ／ **backend query 改寫為 follow-up** |

### NFR-5 DB 失效時事件不可丟

| 項目 | 內容 |
|---|---|
| 規範 | DB 故障時事件 buffer 在 MQ |
| 實作 | Redis Streams `pacs:events` 在 DB 之前；event-processor 拉不到 DB 時事件留 stream，恢復後 catch-up |
| 驗證 | `docker compose stop postgres` 後刷卡仍 200；`docker compose start postgres` 後 stream 內事件補進 DB |
| 實測結果 | _不在本輪驗證範圍_（需要長時間實驗）；此項符合性靠架構保證 |
| 階段 | **P1 ✅**（架構保證） |

### NFR-7 Observability

| 項目 | 內容 |
|---|---|
| 規範 | 監控 query latency 與異常 |
| 實作 | postgres `command:` 啟用 `pg_stat_statements`、`log_min_duration_statement = 100ms`、`log_line_prefix = '%t [%p]: db=%d,user=%u '`；baseline migration `0001` 內含 `CREATE EXTENSION pg_stat_statements` |
| 驗證 | `psql -c "SELECT count(*) FROM pg_stat_statements;"` 不報錯 |
| 實測結果 | <pre>SELECT count(*) AS tracked_queries, max(calls) AS max_calls<br>FROM pg_stat_statements;<br><br> tracked_queries \| max_calls<br>-----------------+-----------<br>              51 \|        10</pre>extension 啟用、已追蹤 51 條 query；最高 mean 為 fixture INSERT 的 53.55 ms（單筆 10k bulk insert，正常） |
| 階段 | **P1 ✅** |

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
| 設計 | event-processor 用 `pacs_user`（write）；reporting-api 用 `pacs_reporter`（read-only） |
| 實作 | baseline migration `0001` 內建 `pacs_reporter` 角色與 grants；docker-compose `reporting-api.environment.DB_USER=pacs_reporter` |
| 驗證 | TESTING Step 10：以 `pacs_reporter` 跑 SELECT 與 INSERT |
| 實測結果（reporter SELECT） | <pre>$ psql -U pacs_reporter -d pacs_db -c<br>  "SELECT count(*) FROM access_events;"<br><br> count<br>-------<br>    45</pre>✅ 唯讀帳號可正常 SELECT |
| 實測結果（reporter INSERT 應 denied） | <pre>$ psql -U pacs_reporter -d pacs_db -c<br>  "INSERT INTO access_events (badge_id,site_id,gate_id,<br>     direction,status) VALUES ('B999','S','G','IN','SUCCESS');"<br><br>ERROR:  permission denied for table access_events</pre>✅ 唯讀帳號 INSERT 被權限阻擋，最小權限生效 |
| 階段 | **P1 ✅** |

---

## 4. P2 deferred 項目（架構規劃中、本階段不實作）

| 項目 | 觸發條件 | 文件位置 |
|---|---|---|
| 按月 partitioning | `access_events` > 5 GB | [scripts/README.md](../scripts/README.md) §"Phase 2 partitioning playbook" |
| Closure table 取代 `org_path` | 組織深度 > 5 / hierarchical query 大量出現 | [database-erd.md](database-erd.md) §8 |
| `mv_daily_attendance` materialized view | reporting P95 接近 200 ms | [database-erd.md](database-erd.md) §8 |
| Read replica | 報表 QPS 干擾寫入 | infra 層 |
| HA / 99.9% (NFR-3) | Phase 2 起 | infra 層 |
| Encryption at rest (NFR-6) | production deployment | cloud provider |

---

## 5. 已知 follow-up（不在本資料庫範圍）

- `backend/internal/db/postgres.go` 的 `QueryAttendance` / `QueryAuditTrail` 使用 `event_time::date`，
  雖然功能正確但無法直接命中 `event_date` 上的新索引。建議 backend owner 在後續 PR 改寫為
  `WHERE event_date = $1` / `WHERE event_date BETWEEN $1 AND $2`。
- 此 follow-up 已在根 [`README.md`](../README.md) 「Backend follow-up TODO」段列出。

---

## 6. 驗證再現性

完整驗證流程：

```bash
cd "/path/to/final_project"
docker compose down -v
docker compose up -d --build
# Step 1
docker compose ps
# Step 4
docker compose exec postgres psql -U pacs_user -d pacs_db -c "SELECT count(*) FROM access_events;"
# Step 9.1 / 9.2 / 9.3
docker compose exec postgres psql -U pacs_user -d pacs_db -c "DELETE FROM access_events WHERE id=1;"
docker compose exec postgres psql -U pacs_user -d pacs_db -c "UPDATE access_events SET status='X' WHERE id=1;"
docker compose exec postgres psql -U pacs_user -d pacs_db -c "TRUNCATE access_events;"
# Step 10
docker compose exec postgres psql -U pacs_reporter -d pacs_db -c "SELECT count(*) FROM access_events;"
docker compose exec postgres psql -U pacs_reporter -d pacs_db \
  -c "INSERT INTO access_events (badge_id, site_id, gate_id, direction, status) VALUES ('B999','S','G','IN','SUCCESS');"
# Step 11
docker compose exec -T postgres psql -U pacs_user -d pacs_db < scripts/fixtures/load_test.sql
docker compose exec postgres psql -U pacs_user -d pacs_db -c \
  "EXPLAIN ANALYZE SELECT badge_id, count(*) FROM access_events
   WHERE event_date = CURRENT_DATE AND status='SUCCESS' GROUP BY badge_id;"
docker compose exec postgres psql -U pacs_user -d pacs_db -c \
  "EXPLAIN ANALYZE SELECT * FROM access_events
   WHERE badge_id='B001' AND event_date BETWEEN CURRENT_DATE - 7 AND CURRENT_DATE
   ORDER BY event_time DESC LIMIT 100;"
```

每段輸出貼到本文件對應 row 的「實測結果」欄。
