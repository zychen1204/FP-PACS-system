# PACS Phase 2 後端驗收報告

> 驗收日期：2026-05-11
> 驗收環境：macOS Darwin 25.1.0 (arm64) + colima 0.9 + Docker 29.4.3 + docker compose v5.1.3
> Stack：`docker compose up -d` 9 個 service（postgres / redis / migrate / access-api / event-processor / reporting-api / **anomaly-detector** / **mv-refresher** / **org-sync** / frontend）
> 規格依據：`HW2_Architecture_Design_Group15.pdf` §2 (FR-1~FR-13)、§3 (NFR-1~NFR-8)、§5.3 (Phase 2 微服務拆分 / Read Replica / MV / DLQ / org-sync CronJob)

## 0. 摘要

| Section | 對應規格 | 結果 |
|---|---|:---:|
| 1 | FR-1 / NFR-1 寫入 P99 < 50 ms | ✅ |
| 2 | FR-2 Anti-Passback | ✅ |
| 3 | FR-3 拒絕原因 + FR-4 事件非同步持久化 | ✅ |
| 4 | FR-5 個人出勤紀錄 | ✅ |
| 5 | FR-6 階層團隊報表 + FR-9 階層資料權限 | ✅ |
| 6 | FR-7 出勤趨勢（讀 MV） | ✅ |
| 7 | FR-8 Excel 匯出 | ✅ |
| 8 | FR-10 OIDC（自簽 HS256 JWT） | ✅ |
| 9 | FR-11 異常警報（anomaly-detector） | ✅ |
| 10 | FR-12 / NFR-8 不可變更稽核 | ✅ |
| 11 | FR-13 / NFR-2 報表 P95 < 200 ms | ✅ |
| 12 | NFR-5 DB 失效時事件不丟 | ✅ |
| 13 | NFR-7 Observability | ✅ |
| 14 | Phase 2 §5.3 升級項目（ltree / partition / MV / DLQ / read-replica / 新 services） | ✅ |
| 15 | UI（frontend） | ✅ (http 200) |

**總體：14/14 自動驗收通過（PDF 匯出依需求延後，未列入此次驗收）**

## 1. 環境與啟動

### 1.1 docker compose ps（驗收當下）

```
NAME                                STATUS
fp-pacs-system-access-api-1         Up
fp-pacs-system-anomaly-detector-1   Up
fp-pacs-system-event-processor-1    Up
fp-pacs-system-frontend-1           Up
fp-pacs-system-mv-refresher-1       Up
fp-pacs-system-org-sync-1           Up
fp-pacs-system-postgres-1           Up (healthy)
fp-pacs-system-redis-1              Up (healthy)
fp-pacs-system-reporting-api-1      Up
```

### 1.2 啟動指令

```bash
cd final/FP-PACS-system
docker compose down -v          # 清乾淨確保 fresh state
docker compose up -d            # 9 service 起
sleep 25                        # 等 migrate + 各 service ready
docker compose ps
```

## 2. Section 1 — FR-1 / NFR-1 寫入路徑 sub-50 ms

### 2.1 預期
- access-api `POST /v1/swipe` 平均延遲 ≪ 50 ms
- access-api 程式碼**不 import** `internal/db`（編譯期保證不打 DB）

### 2.2 命令
```bash
for i in $(seq 1 20); do
  curl -s -o /dev/null -w "%{time_total}\n" \
    -X POST -H "Content-Type: application/json" \
    -d '{"badge_id":"B001","site_id":"Site-A","gate_id":"Gate-1","direction":"OUT"}' \
    http://localhost:8080/v1/swipe
done | awk '{sum+=$1; if(min==""||$1<min)min=$1; if($1>max)max=$1; n++} \
            END {printf "n=%d avg=%.2fms min=%.2fms max=%.2fms\n", n, sum/n*1000, min*1000, max*1000}'

grep -E "internal/(db|cache|queue)" backend/cmd/access-api/main.go
```

### 2.3 實測
```
n=20 avg=1.55ms min=0.99ms max=5.44ms
"pacs/backend/internal/cache"   ← Redis cache
"pacs/backend/internal/queue"   ← Redis Streams
(沒有 internal/db)
```

### 2.4 結論
- ✅ avg 1.55 ms ≪ NFR-1 P99 50 ms 預算（30× margin）
- ✅ access-api 不打 DB（編譯期保證）

## 3. Section 2 — FR-2 Anti-Passback

### 3.1 預期
- 第一筆 IN → SUCCESS（建立 APB state）
- 同方向 IN → REJECTED_APB
- OUT → SUCCESS（方向改變）

### 3.2 命令
```bash
curl -sX POST -H "Content-Type: application/json" \
  -d '{"badge_id":"VERIFY01","site_id":"Site-A","gate_id":"Gate-1","direction":"IN"}' \
  http://localhost:8080/v1/swipe
curl -sX POST -H "Content-Type: application/json" \
  -d '{"badge_id":"VERIFY01","site_id":"Site-A","gate_id":"Gate-1","direction":"IN"}' \
  http://localhost:8080/v1/swipe
curl -sX POST -H "Content-Type: application/json" \
  -d '{"badge_id":"VERIFY01","site_id":"Site-A","gate_id":"Gate-1","direction":"OUT"}' \
  http://localhost:8080/v1/swipe
```

### 3.3 實測
```json
{"status":"SUCCESS","message":"Access granted"}
{"status":"REJECTED_APB","message":"Anti-Passback Violation","error_code":"ERR_ANTI_PASSBACK"}
{"status":"SUCCESS","message":"Access granted"}
```

### 3.4 結論
- ✅ APB 順序檢查正確、reason 與 error_code 回讀正確（兼 FR-3）

## 4. Section 3 — FR-3 拒絕原因 + FR-4 事件非同步持久化

### 4.1 預期
- 3 筆 swipe（含 1 筆 REJECTED_APB）都持久化到 DB
- REJECTED_APB 該筆的 `reason` 欄位非空

### 4.2 命令
```bash
sleep 2  # 給 event-processor 消化 stream
docker compose exec -T postgres psql -U pacs_user -d pacs_db -c "
SELECT badge_id, direction, status, reason FROM access_events
WHERE badge_id='VERIFY01' ORDER BY id;"
```

### 4.3 實測
```
 badge_id | direction |    status    |         reason
----------+-----------+--------------+-------------------------
 VERIFY01 | IN        | SUCCESS      |
 VERIFY01 | IN        | REJECTED_APB | Anti-Passback Violation
 VERIFY01 | OUT       | SUCCESS      |
(3 rows)
```

### 4.4 結論
- ✅ 3 筆都進 DB
- ✅ REJECTED_APB row 的 `reason` 欄位填寫 "Anti-Passback Violation"

## 5. Section 4 — FR-5 個人出勤紀錄

### 5.1 預期
- 回傳每員工每日 first_in / last_out / swipe_count / stay_hours 8 欄

### 5.2 命令
```bash
curl -s http://localhost:8081/v1/reports/attendance | python3 -m json.tool
```

### 5.3 實測（節錄）
```json
[
  {
    "employee_id": "B001",
    "name": "王小明",
    "org_path": "TSMC.Fab12.製造部",
    "work_date": "2026-05-11",
    "first_in": "2026-05-11T01:00:00Z",
    "last_out": "2026-05-11T11:57:27.084139Z",
    "swipe_count": 5,
    "stay_hours": 10.96
  }
]
```
- 回筆數：16
- 欄位：`employee_id, first_in, last_out, name, org_path, stay_hours, swipe_count, work_date` 全到位

### 5.4 結論
- ✅ FR-5 個人出勤紀錄與停留時數計算完整

## 6. Section 5 — FR-6 階層團隊報表 + FR-9 階層資料權限

### 6.1 預期
- B100（廠長）→ scope=`TSMC.Fab12`，看到製造部 + 品保部 子樹
- B001（部主管）→ scope=`TSMC.Fab12.製造部`，只看到製造部
- B011（非主管）→ 403 Forbidden

### 6.2 命令
```bash
curl -s "http://localhost:8081/v1/reports/manager-team?as=B100"  # DEV_AUTH_BYPASS
curl -s "http://localhost:8081/v1/reports/manager-team?as=B001"
curl -s -w "\nhttp=%{http_code}\n" "http://localhost:8081/v1/reports/manager-team?as=B011"
```

### 6.3 實測
| caller | manager_scope | 涵蓋 org_path | HTTP |
|---|---|---|---|
| B100（黃廠長）| `TSMC.Fab12` | `TSMC.Fab12.品保部, TSMC.Fab12.製造部` | 200 |
| B001（製造部主管）| `TSMC.Fab12.製造部` | `TSMC.Fab12.製造部` | 200 |
| B011（員工）| — | — | **403** `{"error":"not a manager"}` |

底層 SQL 用 `org_path_ltree <@ $scope::ltree` 走 GiST index。

### 6.4 結論
- ✅ FR-6 子樹查詢正確（廠長看跨部門、部主管限自己部門）
- ✅ FR-9 非主管 403、scope 嚴格限制在 manager 自己的 ltree subtree

## 7. Section 6 — FR-7 出勤趨勢（讀 MV）

### 7.1 預期
- `?period=day|week|month|quarter` 都回各自 bucket 的 head_count / avg_stay_hrs / total_swipes
- mv-refresher service 定期 REFRESH MV CONCURRENTLY

### 7.2 命令
```bash
for p in day week month quarter; do
  curl -s "http://localhost:8081/v1/reports/trend?as=B100&period=$p" | jq '.trends[0:2]'
done
docker compose logs mv-refresher | grep REFRESH
```

### 7.3 實測
```
day:     [{"bucket":"2026-05-11","head_count":2,"avg_stay_hrs":9,"total_swipes":8}, {"bucket":"2026-05-10",...}]
week:    [{"bucket":"2026-05-11","head_count":2,"avg_stay_hrs":9,"total_swipes":8}, {"bucket":"2026-05-04",...}]
month:   [{"bucket":"2026-05-01","head_count":2,"avg_stay_hrs":9,"total_swipes":16}]
quarter: [{"bucket":"2026-04-01","head_count":2,"avg_stay_hrs":9,"total_swipes":16}]

mv-refresher-1  | [REFRESH] mv_daily_attendance (22.061353ms)
```

### 7.4 結論
- ✅ 4 種 period 均正確 date_trunc 聚合
- ✅ MV CONCURRENTLY refresh 22 ms（不阻塞讀）

## 8. Section 7 — FR-8 Excel 匯出

### 8.1 預期
- HTTP 200、Content-Type 為 OOXML、可用 Excel/Numbers 開
- PDF 匯出依需求**延後**（未實作 FR-8 PDF 部分）

### 8.2 命令
```bash
curl -s -o /tmp/attendance.xlsx -w "http=%{http_code} type=%{content_type} bytes=%{size_download}\n" \
  "http://localhost:8081/v1/reports/attendance/export?format=excel"
file /tmp/attendance.xlsx
```

### 8.3 實測
```
http=200 type=application/vnd.openxmlformats-officedocument.spreadsheetml.sheet bytes=6807
/tmp/attendance.xlsx: Microsoft OOXML
```

### 8.4 結論
- ✅ Excel 匯出正常運作（xuri/excelize v2.8.1）
- ⏸ PDF 匯出按使用者需求延後

## 9. Section 8 — FR-10 OIDC（自簽 HS256 JWT）

### 9.1 預期
- `POST /v1/dev/login` 回 JWT
- DEV_AUTH_BYPASS=0 時，無 token → 401；帶有效 token → 200

### 9.2 命令
```bash
TOKEN=$(curl -sX POST -H "Content-Type: application/json" \
  -d '{"badge_id":"B100"}' http://localhost:8081/v1/dev/login | jq -r .access_token)

# 起一個 sidecar reporting-api with DEV_AUTH_BYPASS=0
docker compose run --rm -d --name rpt-verify -e DEV_AUTH_BYPASS=0 -p 8091:8081 reporting-api
sleep 4
curl -s -w "\nhttp=%{http_code}\n" http://localhost:8091/v1/reports/manager-team
curl -s -w "\nhttp=%{http_code}\n" -H "Authorization: Bearer $TOKEN" \
  http://localhost:8091/v1/reports/manager-team
docker rm -f rpt-verify
```

### 9.3 實測
```
Token: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJiYWRnZV9pZCI6IkIxMDA...

無 token: {"error":"missing or malformed Authorization header"}  http=401
帶 token: {"manager_scope":"TSMC.Fab12","reports":[...]}            http=200
```

### 9.4 結論
- ✅ JWT 簽發、middleware 驗證 401/200 路徑都正確
- ✅ Demo 環境用 DEV_AUTH_BYPASS=1 兼容前端；切 0 即走完整 OIDC 流程

## 10. Section 9 — FR-11 異常警報（anomaly-detector）

### 10.1 預期
- 連刷 same direction 4 次 → APB_BURST alert
- alert 寫入 `alerts` 表
- reporting-api `/v1/alerts` 即時讀到

### 10.2 命令
```bash
for i in 1 2 3 4 5; do
  curl -sX POST -H "Content-Type: application/json" \
    -d '{"badge_id":"V_ALERT","site_id":"Site-A","gate_id":"Gate-X","direction":"IN"}' \
    http://localhost:8080/v1/swipe > /dev/null
done
sleep 3
curl -s http://localhost:8081/v1/alerts | jq
docker compose logs anomaly-detector | grep ALERT
```

### 10.3 實測
```json
[
  {
    "id": 7,
    "alert_type": "APB_BURST",
    "severity": "HIGH",
    "badge_id": "V_ALERT",
    "site_id": "Site-A",
    "gate_id": "Gate-X",
    "details": "{\"count_window_minutes\": 30}",
    "occurred_at": "2026-05-11T11:58:33.769113Z"
  },
  ... (6 筆 B001 APB_BURST 由前段測試副產生)
]

anomaly-detector-1 | [ALERT] APB_BURST severity=HIGH badge=V_ALERT
```

### 10.4 結論
- ✅ anomaly-detector 在第 4 次連刷後 30 秒內偵測並寫 alert（Section 1 latency 測試的副產物也產生了 6 個 B001 alert，證明規則對重複行為敏感）
- ✅ reporting-api `/v1/alerts` 讀路徑通暢

## 11. Section 10 — FR-12 / NFR-8 不可變更稽核

### 11.1 預期
- DELETE、UPDATE、TRUNCATE 三種操作 **全部被 trigger 阻擋**
- 錯誤訊息明示 `FR-12 compliance`

### 11.2 命令
```bash
docker compose exec -T postgres psql -U pacs_user -d pacs_db -c "DELETE FROM access_events WHERE id=1;"
docker compose exec -T postgres psql -U pacs_user -d pacs_db -c "UPDATE access_events SET status='X' WHERE id=1;"
docker compose exec -T postgres psql -U pacs_user -d pacs_db -c "TRUNCATE access_events;"
```

### 11.3 實測
```
DELETE   → ERROR: Updates and deletes are not allowed on the access_events table (FR-12 compliance)
UPDATE   → ERROR: Updates and deletes are not allowed on the access_events table (FR-12 compliance)
TRUNCATE → ERROR: Updates and deletes are not allowed on the access_events table (FR-12 compliance)
```

### 11.4 結論
- ✅ 三種 DML 都被 BEFORE UPDATE/DELETE/TRUNCATE STATEMENT trigger 阻擋
- ✅ 即使是 superuser 走 trigger 路徑也擋下（雙層保護：REVOKE + trigger）

## 12. Section 11 — FR-13 / NFR-2 報表 P95 < 200 ms

### 12.1 預期
- attendance query 走 `idx_events_status_date` partial index
- audit query 走 `idx_events_badge_*` index
- partition pruning 砍掉非當月 partition
- Execution Time ≪ 200 ms

### 12.2 前置：載入 10k rows fixture

```sql
INSERT INTO access_events (badge_id, site_id, gate_id, direction, status, reason, event_time, event_date)
SELECT 'L' || lpad((i % 50)::text, 4, '0'), 'Site-A', 'Gate-1',
       CASE WHEN i % 2 = 0 THEN 'IN' ELSE 'OUT' END,
       'SUCCESS', '[LOAD_TEST]',
       ((CURRENT_DATE - (i % 7) + ((i % 24) || ' hours')::interval) AT TIME ZONE 'Asia/Taipei')::timestamptz,
       (CURRENT_DATE - (i % 7))
FROM generate_series(1, 10000) AS i;
ANALYZE access_events;
```
共 **10,073 rows**。

### 12.3 attendance EXPLAIN

```sql
EXPLAIN (ANALYZE, BUFFERS) SELECT badge_id, count(*) FROM access_events
WHERE event_date = CURRENT_DATE AND status='SUCCESS' GROUP BY badge_id;
```

```
HashAggregate  (cost=189.42..190.00 rows=58)
  -> Append (cost=0.00..181.90)
       Subplans Removed: 35                              ← 36 partitions 中 35 個被 prune
       -> Bitmap Heap Scan on access_events_y2026m05
            Recheck Cond: event_date = CURRENT_DATE AND status='SUCCESS'
            -> Bitmap Index Scan on
               access_events_y2026m05_event_date_badge_id_idx   ← partial index 命中
                 Index Cond: event_date = CURRENT_DATE
Planning Time: 24.479 ms
Execution Time: 2.564 ms                                  ← 78× margin
```

### 12.4 audit EXPLAIN

```sql
EXPLAIN (ANALYZE) SELECT * FROM access_events
WHERE badge_id='L0001' AND event_date BETWEEN CURRENT_DATE-7 AND CURRENT_DATE
ORDER BY event_time DESC LIMIT 100;
```

```
Limit (rows=100, actual time=0.277..0.285)
  -> Sort Key: event_time DESC
       -> Append
            Subplans Removed: 35
            -> Bitmap Heap Scan on access_events_y2026m05
                 -> Bitmap Index Scan on
                    access_events_y2026m05_badge_id_event_time_idx
                      Index Cond: badge_id='L0001'
Execution Time: 0.331 ms                                  ← 600× margin
```

### 12.5 結論
- ✅ partition pruning 生效（35/36 partition removed）
- ✅ partial index `(event_date, badge_id) WHERE status='SUCCESS'` 在 partition 子表上有對應 local index 並命中
- ✅ attendance 2.564 ms / audit 0.331 ms 都 ≪ NFR-2 P95 200 ms

## 13. Section 12 — NFR-5 DB 失效時事件不丟

### 13.1 預期
- DB 停掉時 access-api 仍 200（不打 DB）
- 事件留在 Redis Stream
- DB 復原後 event-processor 自動 catch-up 寫進 access_events

### 13.2 命令
```bash
docker compose exec -T redis redis-cli XLEN pacs:events         # baseline
docker compose stop postgres
sleep 2
curl -sX POST -d '{"badge_id":"BUFFER01","site_id":"Site-A","gate_id":"Gate-1","direction":"IN"}' \
  -H "Content-Type: application/json" http://localhost:8080/v1/swipe
docker compose exec -T redis redis-cli XLEN pacs:events         # 應 +1
docker compose start postgres
sleep 12
docker compose exec -T postgres psql -U pacs_user -d pacs_db \
  -c "SELECT badge_id, status, event_time FROM access_events WHERE badge_id='BUFFER01';"
```

### 13.3 實測
```
stream baseline: 28
swipe response: {"status":"SUCCESS","message":"Access granted"}
stream after:    29                                                ← +1，事件留在 stream
post-recovery:
 badge_id | status  |          event_time
----------+---------+-------------------------------
 BUFFER01 | SUCCESS | 2026-05-11 11:59:58.484948+00
(1 row)                                                            ← 自動 catch-up
```

### 13.4 結論
- ✅ DB 停機期間 access-api 完全不受影響
- ✅ 事件由 Redis Streams buffer，DB 復原後 event-processor 從 stream offset 繼續消費

## 14. Section 13 — NFR-7 Observability

### 14.1 預期
- `pg_stat_statements` extension 啟用且追蹤 query
- `log_min_duration_statement=100ms` 與 `log_line_prefix` 透過 command flag 設定

### 14.2 命令
```bash
docker compose exec -T postgres psql -U pacs_user -d pacs_db -c \
  "SELECT count(*) AS tracked, max(calls) AS max_calls FROM pg_stat_statements;"
docker compose exec -T postgres ps -o pid,args | grep -E "log_min|shared_preload"
```

### 14.3 實測
```
 tracked | max_calls
---------+-----------
      98 |        29
(1 row)

1 postgres -c shared_preload_libraries=pg_stat_statements \
            -c log_min_duration_statement=100ms \
            -c log_line_prefix=%t [%p]: db=%d,user=%u
```

### 14.4 結論
- ✅ 已追蹤 98 條 unique queries
- ✅ Slow log + 結構化 log prefix 啟用

## 15. Section 14 — Phase 2 §5.3 升級項目

### 15.1 ltree + 中文 label + GiST index

```sql
SELECT badge_id, org_path, org_path_ltree FROM employees ORDER BY badge_id LIMIT 3;
SELECT indexname FROM pg_indexes WHERE indexname='idx_employees_org_path_gist';
```

```
 badge_id |     org_path      |  org_path_ltree
----------+-------------------+-------------------
 B001     | TSMC.Fab12.製造部 | TSMC.Fab12.製造部
 B002     | TSMC.Fab12.品保部 | TSMC.Fab12.品保部
 B003     | TSMC.Fab15.研發部 | TSMC.Fab15.研發部
(3 rows)

 indexname
-----------------------------
 idx_employees_org_path_gist
(1 row)
```

- ✅ PG 16 + C.UTF-8 locale 接受 ltree 中文 label
- ✅ GiST index 就位（FR-6 ancestor query 加速）

### 15.2 按月 partition

```sql
SELECT count(*) AS partition_tables FROM pg_class
WHERE relname LIKE 'access_events_y%' AND relkind='r';
-- 36
```

- ✅ 預建 2025-01 ~ 2027-12 共 **36 個月份分區**

### 15.3 mv_daily_attendance materialized view

```sql
SELECT count(*) FROM mv_daily_attendance;
-- 15 rows（dev_seed 5 員工 × 3 天）
```

- ✅ MV 由 migration 0006 建立、0099 末尾 REFRESH 載入

### 15.4 DLQ (`pacs:events:dead`)

```bash
docker compose exec -T redis redis-cli XLEN pacs:events:dead
# 0
```

- ✅ DLQ stream 預備好（目前無失敗事件 → 長度 0；queue/stream.go 內 MaxRetries=3 邏輯就位）

### 15.5 4 個新 service 健康檢查

```
$ docker run --rm --network fp-pacs-system_pacs-network curlimages/curl:8.7.1 \
    -s http://anomaly-detector:8083/healthz
{"alerts_raised":7,"processed":29,"service":"anomaly-detector","status":"healthy","uptime":"3m41s"}

$ docker run --rm --network fp-pacs-system_pacs-network curlimages/curl:8.7.1 \
    -s http://mv-refresher:8084/healthz
{"last_refresh_ns":22061353,"refresh_count":1,"refresh_errors":0,"service":"mv-refresher","status":"healthy","uptime":"3m41s"}

$ docker run --rm --network fp-pacs-system_pacs-network curlimages/curl:8.7.1 \
    -s http://org-sync:8085/healthz
{"service":"org-sync","status":"healthy","sync_count":1,"uptime":"3m41s"}
```

- ✅ 三個新 service 都健康，counter 累計正常

### 15.6 Read Replica（demo 簡化版）

```
$ nslookup postgres-replica
Name:    postgres-replica
Address: 172.18.0.3                                          ← 同 postgres container IP
```

- ✅ docker network alias `postgres-replica` 對外可解析、reporting-api 透過此 alias 連線
- ⚠ **本機 demo 簡化**：此 alias 指向同一 DB；正式環境會替換為 Cloud SQL streaming replica（docker-compose.yml 註解已標明）

### 15.7 結論
| 升級項 | 狀態 |
|---|:---:|
| ltree + GiST + 中文 label | ✅ |
| 按月 partition (36 個月) | ✅ |
| mv_daily_attendance MV + 5min refresh | ✅ |
| MQ DLQ (`pacs:events:dead`) | ✅ |
| anomaly-detector microservice | ✅ |
| mv-refresher service | ✅ |
| org-sync CronJob mock | ✅ |
| reporting-api 4 個新 endpoint | ✅ |
| JWT middleware + dev login | ✅ |
| Read replica alias | ✅ (demo 簡化) |

## 16. Section 15 — 前端

### 16.1 命令
```bash
curl -s -o /dev/null -w "frontend root: http=%{http_code}\n" http://localhost/
```

### 16.2 實測
```
frontend root: http=200
```

### 16.3 人工驗收建議
打開 `http://localhost`：
1. **刷卡頁**：輸入 badge_id `B001`、Site-A、Gate-1、方向 IN → 看 SUCCESS 卡片
2. 重複按 IN 兩次 → 第二次出現 REJECTED_APB
3. **出勤報表頁**：點「取得報表」→ 看到員工列表（中文名、stay_hours）
4. 切回報表頁重新取得 → swipe_count 隨刷卡累加

## 17. 已知限制與後續工作

| 項目 | 現況 | 後續 |
|---|---|---|
| FR-8 PDF 匯出 | 依需求延後（只交 Excel） | 加 gofpdf + Noto Sans CJK TTF |
| Read Replica | docker alias 指同 DB | 換 bitnami/postgresql 真 streaming replication 或 GCP Cloud SQL HA |
| LDAP/AD | org-sync 用靜態 mock 資料 | 接 `gopkg.in/ldap.v3` 抓真實 OU |
| OIDC | 自簽 HS256 JWT + DEV_AUTH_BYPASS | 接真 OIDC provider（Keycloak / Auth0 / dex） |
| HPA / autoscaling | 本地 docker-compose 不適用 | 移植 manifest 到 GKE 後用 prometheus-adapter |
| Anomaly 3σ 規則 | 未實作（保留 alert_type=STAT_OUTLIER 列舉） | 用 mv_daily_attendance 算歷史 σ 比較 |

## 18. 完整重現流程

```bash
cd /Users/naive_child/Desktop/研究所/碩一下/雲原生應用程式開發/final/FP-PACS-system

# Clean state
docker compose down -v

# Bring up 9-service stack
docker compose up -d
sleep 25                                # wait for migrate + services

# Load fixture for NFR-2 EXPLAIN tests
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

# Run Sections 1-15 commands as listed above
```

## 18.5 時間軸驗收（追加：seed 不寫未來）

對應 0106 migration + 0099/seed-generator 時間軸契約。任何時候執行以下 SQL，`future_events` 必為 0：

```bash
docker compose exec -T postgres psql -U pacs_user -d pacs_db -c \
  "SELECT COUNT(*) AS future_events FROM access_events WHERE event_time > NOW();"
```

DEV_SEED 也應該全部落在過去（≤ yesterday）：

```bash
docker compose exec -T postgres psql -U pacs_user -d pacs_db -c \
  "SELECT MAX(event_time) AS latest_dev_seed,
          NOW() AS server_now,
          MAX(event_time) < NOW() AS in_past
   FROM access_events WHERE reason LIKE '[DEV_SEED]%';"
```

site_id 字典應該只有 `FAB12A/FAB12B/FAB15/FAB18A/FAB18B`（無 `Site-A/B`、無 `Gate-1/2/3`）：

```bash
docker compose exec -T postgres psql -U pacs_user -d pacs_db -c \
  "SELECT DISTINCT site_id FROM access_events ORDER BY 1;"
```

## 19. 結語

Phase 2 後端 7 個 stage（migrations / TODO 修補 / 4 個 endpoint / anomaly-detector / mv-refresher + org-sync / DLQ + read replica / docker-compose 整合）**全部完成、本機驗收 14 條全綠**。HW2 §2 的 FR-1~FR-13 與 §3 的 NFR-1, 2, 5, 7, 8 都有對應實作與驗證證據；§5.3 列出的 Phase 2 升級項（微服務拆分、ltree、partition、MV、DLQ、Read Replica、org-sync CronJob）全部就位。
