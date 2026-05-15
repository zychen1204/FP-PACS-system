# Phase 2 後端改動記錄

> 這份文件解釋 `feat/phase2-backend` 分支（PR #2）做了什麼、為什麼這樣做、考慮過哪些替代方案、留下什麼後續工作。
>
> 互補文件：[`PHASE2_VERIFICATION.md`](PHASE2_VERIFICATION.md) 是執行面（命令 + 預期 + 實測）；本文件是設計面（架構 + 決策 + 取捨）。

---

## 目錄

- [0. 摘要](#0-摘要)
- [1. 起點：Phase 1 現況與 Phase 2 目標](#1-起點phase-1-現況與-phase-2-目標)
- [2. 改動全景圖](#2-改動全景圖)
- [3. 資料層升級（4 支新 migration + 1 支修正）](#3-資料層升級4-支新-migration--1-支修正)
- [4. 後端服務升級](#4-後端服務升級)
- [5. Docker Compose 拓樸](#5-docker-compose-拓樸)
- [6. 關鍵設計決策（含替代方案）](#6-關鍵設計決策含替代方案)
- [7. 已知限制與後續工作](#7-已知限制與後續工作)
- [8. 對應 HW2 spec](#8-對應-hw2-spec)
- [9. 怎麼擴展](#9-怎麼擴展)

---

## 0. 摘要

| 項目 | 數字 |
|---|---|
| Commit 數 | 4 |
| 修改檔案 | 21 |
| 新增行數 | +2216 |
| 刪除行數 | -64 |
| 新 migration | 4 支（0003-0006） |
| 新 endpoint | 5 個 |
| 新 microservice | 3 個 |
| 影響 FR | FR-3, 5, 6, 7, 8, 9, 10, 11, 13 |
| 影響 NFR | NFR-2, 5, 7 |

落實了 HW2 §5.3 列出的 **全部** Phase 2 升級項：
- 單體 → 4 微服務（access-api / event-processor / reporting-api / **+ anomaly-detector**）
- 單 DB → Read Replica（demo 簡化版）
- 新增 materialized view（`mv_daily_attendance` 5 min refresh）
- HPA 升級（雙指標 CPU+QPS）— 本機 demo 不適用
- Pub/Sub + DLQ → MQ 內建 DLQ 機制
- GKE Autopilot → Standard Regional — 本機 demo 不適用
- Cache 升級 — 本地 redis 不適用
- 新增 `org-sync` CronJob

---

## 1. 起點：Phase 1 現況與 Phase 2 目標

### 1.1 Phase 1 已經做完什麼

Phase 1 PR #1 (`f51a35b` + `ad00ea1`) 落地了一個基本可運作的單體系統：

```
Badge Readers ──► access-api ──► Redis Cache (APB)
                                  Redis Streams ──► event-processor ──► PostgreSQL
                                                                          ▲
                                                          reporting-api ──┘
```

具體：
- 3 個 Go service：`access-api`、`event-processor`、`reporting-api`
- 基本 endpoint：`POST /v1/swipe`、`GET /v1/reports/attendance`、`GET /v1/audit`
- DB schema：`access_events`（append-only）+ `employees`（含 `org_path` 字串 + `is_manager` flag）
- FR-1, 2, 3, 4, 12, 13 與 NFR-1, 2, 5, 7, 8 都有對應實作
- **未實作**的 FR：FR-6（drill-down endpoint）、FR-7（趨勢）、FR-8（匯出）、FR-9（API 層 manager scope filter）、FR-10（OIDC）、FR-11（異常警報）
- 已知 backend follow-up（README 列出）：`QueryAttendance`/`QueryAuditTrail` 用 `event_time::date` 不命中索引

### 1.2 Phase 2 目標（依 HW2 §5.3）

HW2 文件對 Phase 2 的 scope 寫得很明確（30k DAU、單一廠區、~100 QPS），列出了具體的「Phase 1 → Phase 2 變更」：

| 維度 | Phase 1 | Phase 2 |
|---|---|---|
| 服務拓樸 | 1 個單體 | 4 個微服務 |
| DB | 單一 PG | Primary + Read Replica + monthly partition |
| 報表 | 即時 GROUP BY | `mv_daily_attendance` 5 min refresh |
| MQ | Pub/Sub | Pub/Sub + DLQ |
| 組織樹 | (HW2 寫 ltree) | (HW2 寫 ltree + GiST) |
| 認證 | 無 | OIDC token verify |
| 警報 | 無 | anomaly-detector + 30s 推送 |
| 組織同步 | 無 | org-sync CronJob (LDAP/AD) |

> 注意：Phase 1 repo 內 `docs/database-spec.md` §5 寫「HW2 §5.2 字面寫 adjacency list」，但實際讀 HW2 PDF 後發現 HW2 §5.2/§5.3 全程都是 **ltree + GiST index**。這次改動把組織樹真正升到 ltree（見 §3.1）。

---

## 2. 改動全景圖

### 2.1 4 個 commit 對應 4 個邏輯區塊

```
908a083  feat(db): Phase 2 schema — ltree, alerts, partition, MV
         └─ scripts/migrations/0003-0006 + 0099 dev_seed 修正

a8cc2fc  feat(backend): Phase 2 endpoints + JWT middleware + MQ DLQ
         └─ internal/{auth,db,models,queue} + reporting-api 新 endpoints
            backend/{go.mod,go.sum} 加新 deps

2e930ec  feat(services): anomaly-detector + mv-refresher + org-sync
         └─ backend/cmd/{anomaly-detector,mv-refresher,org-sync}/

d9503b5  chore(compose): wire Phase 2 services + PG 16; docs: verification
         └─ docker-compose.yml + docs/PHASE2_VERIFICATION.md
```

### 2.2 拓樸變化

#### Phase 1（before）
```
                          ┌──────────────┐
[Badge]──HTTP──► access-api ──► Redis Cache (apb:*)
                              └─► Redis Stream (pacs:events)
                                          │
                                          ▼
                              event-processor ──► PostgreSQL ◄── reporting-api
                                                  (single DB)
```

#### Phase 2（after）
```
                          ┌──────────────┐
[Badge]──HTTP──► access-api ──► Redis Cache (apb:*)
                              └─► Redis Stream (pacs:events) ──► [DLQ pacs:events:dead]
                                          │ (named consumer groups)
                       ┌──────────────────┼──────────────────┐
                       ▼                                     ▼
              event-processor                       anomaly-detector ──► alerts table
                       │                                     │
                       ▼                                     │
              PostgreSQL (primary, 36 monthly partitions) ◄──┘
                       │
                       ▼
            mv_daily_attendance (CONCURRENT refresh by mv-refresher service)
                       │
                       ▼
              [postgres-replica alias] ──► reporting-api ──► (JWT-protected endpoints)
                                                                │
                                              ┌─────────────────┼───────────────────┐
                                              ▼                 ▼                   ▼
                                          /attendance     /manager-team        /trend
                                          /audit         (FR-6/9 ltree<@)     (reads MV)
                                          /alerts        /export?format=excel  /dev/login
                                          (FR-11 read)   (FR-8)                (FR-10)

              org-sync (CronJob) ──► UPSERT employees ──► trg_sync_org_path_ltree
                                                          (自動同步 ltree 欄位)
```

新元素以 `bold` 標示。

### 2.3 21 個檔案影響

| 區塊 | 新增 | 修改 |
|---|---|---|
| **migrations** | 4 支（0003-0006 各含 up + down，共 8 檔）| 0099 dev_seed（時區修正 + REFRESH MV）|
| **internal modules** | `auth/jwt.go` | `db/postgres.go` (+185 lines)、`queue/stream.go` (+79)、`models/models.go` (+22) |
| **cmd services** | `anomaly-detector/main.go`、`mv-refresher/main.go`、`org-sync/main.go` | `reporting-api/main.go` (+173) |
| **go modules** | — | `go.mod`、`go.sum` |
| **infra** | — | `docker-compose.yml` (+91) |
| **docs** | `PHASE2_VERIFICATION.md`、（本檔）| — |

---

## 3. 資料層升級（4 支新 migration + 1 支修正）

### 3.1 0003 — ltree 組織樹（HW2 §5.2/§5.3 明列）

#### Before（Phase 1）
```sql
employees (
  badge_id    VARCHAR(50) PK,
  org_path    VARCHAR(255),   -- "TSMC.Fab12.製造部"
  is_manager  BOOLEAN,
  ...
)
-- FR-6/9 查詢用 LIKE prefix：
-- WHERE org_path = $1 OR org_path LIKE $1 || '.%'
```

#### After
```sql
CREATE EXTENSION ltree;
ALTER TABLE employees ADD COLUMN org_path_ltree LTREE NOT NULL;
CREATE INDEX idx_employees_org_path_gist
  ON employees USING GIST (org_path_ltree);
-- trigger 自動同步：INSERT/UPDATE org_path 時 ltree 一起更新
CREATE TRIGGER trg_sync_org_path_ltree
  BEFORE INSERT OR UPDATE OF org_path ON employees
  FOR EACH ROW EXECUTE FUNCTION sync_org_path_ltree();

-- 查詢改用 ltree descendant operator + GiST 命中：
-- WHERE org_path_ltree <@ $1::ltree
```

#### 為什麼新欄位而非替換 VARCHAR？
1. 既有資料無痛遷移（零風險 backfill）
2. UI（`frontend/app.js`）直接讀 `org_path` 中文字串顯示，不動前端
3. ltree 是 query 加速器，VARCHAR 是 canonical display

`docs/database-spec.md` §5 留下的「未來如 Phase 3 確定 ltree 穩定再考慮 drop VARCHAR」描述仍適用。

#### 中文 label 問題（升 PG 15 → 16 的真實原因）
PostgreSQL `ltree` 規定 label 是 alphanumeric + 底線，「是否視中文為 alphanumeric」由 database locale 決定：
- **PG 15-alpine 預設 C locale**：完全不接受中文 → ltree cast `'TSMC.Fab12.製造部'::ltree` 報錯
- **PG 16-alpine + C.UTF-8 locale**：接受 → 直接 work

兩個改動：
1. `docker-compose.yml` 把 image 從 `postgres:15-alpine` → `postgres:16-alpine`
2. 加 `LANG: C.UTF-8` environment

[`PHASE2_VERIFICATION.md` §15.1](PHASE2_VERIFICATION.md#151-ltree--中文-label--gist-index) 有實測證明中文 ltree 與 GiST 都正常。

### 3.2 0004 — alerts 表（FR-11）

```sql
CREATE TABLE alerts (
    id          BIGSERIAL PK,
    alert_type  VARCHAR(40)  CHECK (alert_type IN (
                  'OFF_HOURS_ENTRY','APB_BURST','TAILGATING','STAT_OUTLIER')),
    severity    VARCHAR(10)  CHECK (severity IN ('LOW','MEDIUM','HIGH','CRITICAL')),
    badge_id    VARCHAR(50),     -- nullable（某些異常無對應人員）
    site_id     VARCHAR(50),
    gate_id     VARCHAR(50),
    details     JSONB DEFAULT '{}',
    occurred_at TIMESTAMPTZ DEFAULT NOW(),
    resolved_at TIMESTAMPTZ      -- NULL = 未處理
);
-- 預設列表：先看未處理、再按時間倒序
CREATE INDEX idx_alerts_open_recent
    ON alerts (resolved_at NULLS FIRST, occurred_at DESC);

GRANT INSERT, SELECT ON alerts TO pacs_user;      -- anomaly-detector
GRANT SELECT          ON alerts TO pacs_reporter; -- reporting-api
```

#### 設計重點
- `alert_type` 用 CHECK 列舉避免 typo 進庫
- `details` 用 JSONB 給規則特定 metadata（例如 `count_window_minutes`）
- `resolved_at NULLS FIRST` 索引讓「未處理優先」query 走索引
- 寫入用 `pacs_user`（與 access_events 同帳號）、讀用 `pacs_reporter`（reporting-api 的 read-only 角色）

### 3.3 0005 — `access_events` 按月 partition（HW2 §5.3 明列）

#### Before
```sql
CREATE TABLE access_events (
  id          BIGSERIAL PK,
  ...
  event_time  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  event_date  DATE GENERATED ALWAYS AS
              ((event_time AT TIME ZONE 'Asia/Taipei')::date) STORED
);
```

#### After
```sql
CREATE TABLE access_events (
  id          BIGINT NOT NULL DEFAULT nextval('access_events_id_seq'),
  ...
  event_time  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  event_date  DATE NOT NULL,                  -- ← 改普通欄位
  PRIMARY KEY (id, event_date)                -- ← partition key 必須在 PK
) PARTITION BY RANGE (event_date);

-- 預建 2025-01 ~ 2027-12 共 36 個月份分區
-- access_events_y2025m01 ... access_events_y2027m12
```

#### 為什麼 `event_date` 從 GENERATED STORED 改普通欄位？

這是 Phase 2 落地過程中遇到最深的一道坑。原本想保留 STORED generated column 維持「呼叫端不用提供 event_date」的好性質，但卡了兩道 PG 限制：

1. **PG 16 不允許 generated column 當 partition key**
   ```
   ERROR: cannot use generated column in partition key
   ```

2. **Workaround：改用 BEFORE INSERT row trigger 也失敗**
   ```
   ERROR: no partition of relation "access_events" found for row
   DETAIL: Partition key of the failing row contains (event_date) = (null)
   ```
   PG 對 partitioned table 的 routing 順序：
   ```
   1. PG 看 NEW.event_date 決定 partition
   2. routing 完成後才 fire BEFORE row trigger
   ```
   trigger 想填 partition key 但已經太晚。

#### 採取的解法
讓 `event_date` 變普通 DATE NOT NULL 欄位，由 INSERT 路徑顯式提供：
- **`event-processor` 在 SQL 內計算**：
  ```sql
  INSERT INTO access_events
    (badge_id, ..., event_time, event_date)
  VALUES
    ($1, ..., $7, ($7 AT TIME ZONE 'Asia/Taipei')::date)
  ```
  Go 端不用改、`event_time` 還是來自 model 的 `Timestamp`。
- **`scripts/migrations/0099_dev_seed.up.sql`** 也改顯式提供。

語意與舊的 STORED generated column 完全等價（同樣的 `AT TIME ZONE 'Asia/Taipei'` cast）。

#### 36 個 partition
預建 `access_events_y2025m01` ~ `access_events_y2027m12`（3 年）。理由：
- 跑 demo / 改 code 過程 CURRENT_DATE 不確定，預建多一點安全
- 超過範圍未來再加 cron 預建下個月（HW2 §5.3 提到的 pg_partman 模式）

#### 為什麼 DROP 舊表後才 RENAME index？
INSERT INTO new SELECT 完成後做 swap：
```
ALTER TABLE access_events     RENAME TO access_events_legacy;
ALTER TABLE access_events_new RENAME TO access_events;
DROP   TABLE access_events_legacy;                          -- ← 先 DROP
ALTER INDEX idx_events_badge_date_new RENAME TO idx_events_badge_date;
                                                            -- ← 才 rename，避免衝突
```
RENAME TABLE 不會 rename indexes，舊表 DROP 後才能釋放原 index 名給新表的索引用。

#### FR-12 還生效嗎？
是。`trg_protect_audit` 與 `trg_protect_audit_truncate` 都重新掛到 partitioned root 上（STATEMENT trigger 在 PG 13+ 支援 partitioned root）。`PHASE2_VERIFICATION.md` §11 三條 DML 都還是被擋。

#### DROP TABLE 不會 fire FR-12 trigger？
不會。FR-12 trigger 是 `BEFORE UPDATE OR DELETE` 與 `BEFORE TRUNCATE` — DML 事件。`DROP TABLE` 是 DDL，不會 fire row/statement trigger。

### 3.4 0006 — `mv_daily_attendance`（FR-7）

```sql
CREATE MATERIALIZED VIEW mv_daily_attendance AS
SELECT e.badge_id, e.event_date,
       COALESCE(emp.name, ...)     AS name,
       COALESCE(emp.org_path, ...) AS org_path,
       emp.org_path_ltree          AS org_path_ltree,
       MIN(e.event_time) FILTER (WHERE e.direction = 'IN')  AS first_in,
       MAX(e.event_time) FILTER (WHERE e.direction = 'OUT') AS last_out,
       COUNT(*) AS swipe_count,
       EXTRACT(EPOCH FROM (MAX_OUT - MIN_IN)) / 3600.0 AS stay_hours
FROM access_events e
LEFT JOIN employees emp ON emp.badge_id = e.badge_id
WHERE e.status = 'SUCCESS'
GROUP BY e.badge_id, e.event_date, emp.name, emp.org_path, emp.org_path_ltree
WITH DATA;

-- CONCURRENT refresh 需要 unique index
CREATE UNIQUE INDEX idx_mv_daily_attendance_pk
    ON mv_daily_attendance (badge_id, event_date);

-- 趨勢 query 走 ltree ancestor
CREATE INDEX idx_mv_daily_attendance_org_date
    ON mv_daily_attendance USING GIST (org_path_ltree);
```

#### 為什麼一個 MV 涵蓋 day/week/month/quarter？
原本可以為每個 period 各建一個 MV，但會：
- 重複儲存（同樣資料切不同 group）
- refresh 成本 ×4
- 而 day 級 MV 上做 `date_trunc('week', event_date)` 已經夠快

選擇：只建 day 級 MV，week/month/quarter 由 reporting-api 在 query 上 `date_trunc` 即時聚合（見 `internal/db/postgres.go QueryAttendanceTrend`）。

#### 為什麼用 CONCURRENTLY refresh？
普通 `REFRESH MATERIALIZED VIEW` 會 ACCESS EXCLUSIVE lock — 阻塞所有讀。CONCURRENTLY 用 unique index 做 diff、只 ACCESS SHARE lock，不阻塞 reporting-api 查詢。代價：需要 unique index（已建）+ refresh 時間略長（但對 demo data 量無感）。

#### 為什麼把 org_path_ltree 也帶進 MV？
讓 FR-7 trend 與 FR-6/9 manager-team 都能在 MV 上 push down `<@` filter，不必每次再 JOIN employees。

#### MV 何時 refresh？
獨立 `mv-refresher` service（見 §4.7），預設 300 秒 / 5 分鐘一次。HW2 §5.3 列為 Phase 2 升級項。

### 3.5 0099 dev_seed 修正（timezone bug）

#### Before
```sql
INSERT INTO access_events (..., event_time, event_date)
VALUES (..., 
  (CURRENT_DATE + TIME '18:00')::timestamptz,    -- ← UTC 18:00
  CURRENT_DATE);                                  -- ← 但 event_date 寫今天
```

PG server 預設 UTC：
- `CURRENT_DATE + TIME '18:00'` = `今天 18:00:00`（無時區）
- `::timestamptz` 用 server tz UTC → `今天 18:00:00 +00`
- 在台北時區實際是 `明天 02:00:00`
- 該事件「在台北的 date」應該是明天，但 event_date 卻寫成今天

結果：MV 計算 `MAX(OUT)` 跟 `MIN(IN)` 不在同一 event_date row，`stay_hours` 全為 NULL。

#### After
```sql
INSERT INTO access_events (..., event_time, event_date)
VALUES (...,
  ((CURRENT_DATE + TIME '18:00') AT TIME ZONE 'Asia/Taipei')::timestamptz,
  CURRENT_DATE);
```

`(timestamp AT TIME ZONE 'Asia/Taipei')` 對「無時區的 timestamp」是反向 cast：把它視為台北本地時間並轉成 UTC。所以「台北的 18:00」變成 UTC 的 10:00，再做 AT TIME ZONE 'Asia/Taipei' 又轉回台北 18:00 → date = 今天 ✅。

驗證：`PHASE2_VERIFICATION.md` §7 的 trend 報表現在 `avg_stay_hrs = 9` 全部對齊（IN 09:00, OUT 18:00, 9 小時）。

---

## 4. 後端服務升級

### 4.1 `internal/auth/jwt.go`（新檔，FR-10）

```go
type Claims struct {
    BadgeID string `json:"badge_id"`
    jwt.RegisteredClaims
}

func Issue(badgeID string, ttl time.Duration) (string, error) { ... }
func Parse(tokenStr string) (*Claims, error)                  { ... }
func Middleware() gin.HandlerFunc                             { ... }
func BadgeIDFromCtx(c *gin.Context) (string, bool)            { ... }
```

設計重點：

| 項目 | 選擇 | 理由 |
|---|---|---|
| Signing alg | **HS256** | 自簽 demo IdP；正式環境換 RS256 或交給 OIDC provider |
| Secret | env `JWT_SECRET` | dev 預設 `"dev-only-not-for-prod-change-me-please"` |
| TTL | 24h | dev 方便；正式環境縮 15 min + refresh token |
| DEV_AUTH_BYPASS | env=`1` 時 middleware 看 `?as=badge_id` 直接放行 | 讓 Phase 1 frontend 完全不改 code 仍可用 |

#### 為什麼自簽 JWT 而非真 OIDC？

[使用者選擇](../PHASE2_VERIFICATION.md) 是「自生 HS256 JWT + dev login」，理由：

1. **不增 service**：跑 Keycloak 要 30s 啟動 + 多 200 MB 記憶體
2. **flow 完整**：HS256 JWT 與 OIDC ID Token 結構一致（`iss`/`sub`/`exp`），未來換真 IdP 只需改 middleware 驗 signature 演算法
3. **demo 便利**：`POST /v1/dev/login` 一行給 token，curl 演示快

### 4.2 `internal/queue/stream.go` — DLQ + named consumer groups

#### 新增 `ConsumeEventsWithGroup`
原本 stream 寫死 `GroupName = "event-processors"`，只能一個 group 消費。`anomaly-detector` 要獨立消費就需要可命名。

```go
const (
    StreamName     = "pacs:events"
    GroupName      = "event-processors"
    DeadStreamName = "pacs:events:dead"
    MaxRetries     = 3
)

// 原 ConsumeEvents 變 thin wrapper
func (s *RedisStream) ConsumeEvents(...) error {
    return s.ConsumeEventsWithGroup(ctx, GroupName, consumerName, handler)
}

// 新一般化版本，支援 DLQ
func (s *RedisStream) ConsumeEventsWithGroup(ctx, group, consumerName, handler) error {
    for {
        // XReadGroup ...
        for msg := range messages {
            var lastErr error
            for attempt := 0; attempt < MaxRetries; attempt++ {
                if err := handler(event); err == nil {
                    lastErr = nil
                    break
                } else {
                    lastErr = err
                    time.Sleep(500 * time.Millisecond)
                }
            }
            if lastErr != nil {
                s.toDLQ(ctx, msg.ID, data, lastErr, consumerName)
            }
            s.client.XAck(ctx, StreamName, group, msg.ID)
            // ↑ 不論 success 或 DLQ 都 ACK 主 stream，避免無限重投
        }
    }
}
```

#### `toDLQ` 寫進 `pacs:events:dead`
```go
s.client.XAdd(ctx, &redis.XAddArgs{
    Stream: DeadStreamName,
    Values: map[string]interface{}{
        "data":        data,        // 原始 event JSON
        "original_id": originalID,
        "error":       cause.Error(),
        "consumer":    consumer,    // 哪個 service 失敗
        "failed_at":   time.Now().UTC().Format(time.RFC3339Nano),
    },
})
```

#### 與 HW2 §5.3 對齊
HW2 寫 `Cloud Pub/Sub + DLQ`。本實作用 Redis Streams 模擬：
- DLQ 概念對齊（失敗事件不丟、不卡 main stream）
- 不同的：Pub/Sub DLQ 是配置式（自動退避），這裡是 in-handler retry + 失敗 push DLQ。對 demo 等價。

#### 已知簡化
- retry 是「同一輪內 3 次連續」，不是真正的 exponential backoff
- 沒實作 DLQ 反向處理（dead-letter monitoring / replay）— 留作後續

### 4.3 `internal/db/postgres.go` — 重寫舊 query + 6 個新 method

#### 修原 README backend TODO
```diff
- WHERE e.event_time::date = $1
+ WHERE e.event_date = $1

- GROUP BY e.badge_id, emp.name, emp.org_path, e.event_time::date
+ GROUP BY e.badge_id, emp.name, emp.org_path, e.event_date
```
效益：
- 直接命中 partial index `idx_events_status_date (event_date, badge_id) WHERE status='SUCCESS'`
- 同時 partition pruning 自動發生
- 修原 `GROUP BY` 用 `e.event_time::date` 跟 `SELECT` 用 `e.event_time::date::text` 在 PG 嚴格 mode 對齊不嚴格的潛在 bug

#### 新增 6 個 method

```go
// FR-9 step 1：caller 是不是 manager，是的話拿其 ltree scope
func GetManagerScope(ctx, badgeID) (string, error)

// FR-6 step 2：用 ltree <@ 拿子樹 attendance
func QueryManagerTeamAttendance(ctx, scopeLtree, date string) ([]AttendanceReport, error)

// FR-7：讀 MV，依 period (day/week/month/quarter) 與 scope 聚合
func QueryAttendanceTrend(ctx, period, scope, startDate, endDate) ([]AttendanceTrend, error)

// FR-11 read：列 alerts，可只取未處理
func QueryAlerts(ctx, openOnly bool, limit int) ([]Alert, error)

// FR-11 write：anomaly-detector 用
func InsertAlert(ctx, alertType, severity, badgeID, siteID, gateID, detailsJSON) error
```

#### FR-9 pattern a（兩段式）
HW2 規格說明 manager 只能看子樹。實作分兩步：

```go
// Step 1: 確認 caller 是 active manager + 取 scope
scope, _ := db.GetManagerScope(ctx, callerBadgeID)
if scope == "" {
    return 403 Forbidden    // ← 非主管 / 不在職
}

// Step 2: 用 scope 過濾子樹
reports, _ := db.QueryManagerTeamAttendance(ctx, scope, date)
```

DB 層的 SQL 用：
```sql
WHERE mv.org_path_ltree <@ $1::ltree
```
`<@` 是 ltree「descendant of or equal to」operator，命中 GiST index。

#### 設計選擇：為什麼 FR-9 在 API 層而不是 DB 層？
DB 沒有「session 身份」概念（pacs_reporter 是共用 read role），不可能在 RLS 層 enforce「只看自己組」。身份來自 JWT（FR-10），所以 manager scope 必然在 API 層判定。`docs/database-compliance.md` §FR-9 也標明這設計取向。

### 4.4 `internal/models/models.go` — 新增 2 type

```go
type AttendanceTrend struct {
    Bucket      string  `json:"bucket"`        // ISO date string of bucket start
    HeadCount   int     `json:"head_count"`
    AvgStayHrs  float64 `json:"avg_stay_hrs"`
    TotalSwipes int     `json:"total_swipes"`
}

type Alert struct {
    ID         int64     `json:"id"`
    AlertType  string    `json:"alert_type"`
    Severity   string    `json:"severity"`
    BadgeID    *string   `json:"badge_id,omitempty"`
    SiteID     *string   `json:"site_id,omitempty"`
    GateID     *string   `json:"gate_id,omitempty"`
    Details    string    `json:"details"`            // JSON raw string (forward-compat)
    OccurredAt time.Time `json:"occurred_at"`
    ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}
```

#### 為什麼 `Details` 用 raw string 而不是 `map[string]interface{}`？
- DB 是 JSONB，Go 端把它 marshal/unmarshal 兩遍多此一舉
- forward-compat：新規則增加 details 欄位不用改 Go struct
- frontend 直接拿到完整 JSON 自行 parse

### 4.5 `reporting-api/main.go` — 加 5 個 endpoint

```go
// 既有
router.POST("/v1/dev/login", devLogin)                            // FR-10

// 套 JWT middleware 的 protected group
authed := router.Group("/", auth.Middleware())
authed.GET("/v1/reports/attendance", getAttendanceReport)         // FR-5（既有）
authed.GET("/v1/audit", getAuditTrail)                            // FR-13（既有）
authed.GET("/v1/reports/manager-team", getManagerTeamReport)      // FR-6/9 新
authed.GET("/v1/reports/trend", getAttendanceTrend)               // FR-7 新
authed.GET("/v1/reports/attendance/export", exportAttendance)     // FR-8 新
authed.GET("/v1/alerts", listAlerts)                              // FR-11 新
```

#### Excel 匯出（FR-8）
用 `github.com/xuri/excelize/v2 v2.8.1`（pin 此版本因為 v2.10+ 要 go 1.24，但 go.mod 寫 1.21）。

實作概念：
```go
f := excelize.NewFile()
sheet := "Attendance"
f.NewSheet(sheet); f.SetActiveSheet(...); f.DeleteSheet("Sheet1")

// 表頭
for col, h := range headers {
    cell, _ := excelize.CoordinatesToCellName(col+1, 1)
    f.SetCellValue(sheet, cell, h)
}
// 資料列
for row, r := range reports { ... }

// 直接 write 進 HTTP response
c.Header("Content-Disposition", ...)
c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
f.Write(c.Writer)
```

PDF 依使用者需求**延後**，未實作（option 「只做 Excel」）。

### 4.6 `cmd/anomaly-detector/main.go`（新 service，FR-11）

```go
const consumerGroup = "anomaly-detectors"  // 獨立 group，跟 event-processor 分開

// 3 條規則（簡化版）
func detect(ctx, event AccessEvent) {
    if isOffHoursEntry(event) { raise("OFF_HOURS_ENTRY", "MEDIUM", ...) }
    if isApbBurst(event)      { raise("APB_BURST", "HIGH", ...) }
    if isTailgating(event)    { raise("TAILGATING", "HIGH", ...) }
}
```

#### 規則
| 規則 | 觸發條件 | 嚴重度 |
|---|---|---|
| `OFF_HOURS_ENTRY` | `status=SUCCESS` AND `direction=IN` AND 台北時區 22:00 ~ 06:00 | MEDIUM |
| `APB_BURST` | 同 badge 30 分鐘內 `REJECTED_APB` ≥ 3 次 | HIGH |
| `TAILGATING` | 同 gate 5 秒內 `SUCCESS IN` ≥ 3 次 | HIGH |
| `STAT_OUTLIER` | （未實作，保留 enum）| — |

#### 為什麼用 in-memory state（`apbState` / `tailgateState`）而非 Redis？
- 單 instance 簡化（不必跨 instance 同步 counter）
- 滑動窗口短（30 min / 5 sec），重啟損失可接受
- 真要做 cluster-aware 可改 Redis ZSET（score = timestamp，ZCOUNT 取窗口 count）

#### Mutex 為什麼必要？
goroutine handler 對同一 map 寫 → 加 `sync.Mutex`。對於 demo 流量 mutex 競爭可忽略；高流量可改 sharded map / sync.Map。

#### 健康端點
`:8083/healthz` 回 `{"processed": N, "alerts_raised": N, "uptime": "..."}`。

### 4.7 `cmd/mv-refresher/main.go`（新 service，FR-7）

```go
intervalSec, _ := time.ParseDuration(os.Getenv("REFRESH_INTERVAL_SECONDS") + "s")
if intervalSec < 10*time.Second { intervalSec = 10*time.Second }  // 防呆

t := time.NewTicker(intervalSec)
for {
    select {
    case <-ctx.Done(): return
    case <-t.C:
        db.ExecContext(ctx, "REFRESH MATERIALIZED VIEW CONCURRENTLY mv_daily_attendance")
    }
}
```

#### 為什麼獨立 service 而非 cron / event-processor 內嵌？
- 「mv-refresher service」是 HW2 §5.3 的 explicit 元件
- cron job 在 docker-compose 不好做（要額外 ofelia / dcron 鏡像）
- 嵌入 event-processor 違反 separation of concerns

#### 連 DB 用 `pacs_user` 還是 `pacs_reporter`？
MV 是 `pacs_user` 在 migrate 階段建的（migrate 用 `pacs_user` 連線），owner 是 `pacs_user`。`REFRESH` 需要 owner 或 superuser 權限，所以 mv-refresher 用 `pacs_user`。`pacs_reporter` 只能 SELECT。

#### 健康端點
`:8084/healthz` 含 `refresh_count`、`refresh_errors`、`last_refresh_ns`。

### 4.8 `cmd/org-sync/main.go`（新 service，HW2 §5.3 CronJob）

```go
// 注意：以下程式碼反映 migration 0102 後現況（job_level VARCHAR 取代 is_manager BOOLEAN）。
func mockLDAP() []orgRecord {
    return []orgRecord{
        {"B100", "黃廠長", "TSMC.Fab12", JobLevelManagerL1},     // 一級主管
        {"B001", "王小明", "TSMC.Fab12.製造部", JobLevelManagerL2}, // 二級主管
        ...
        {"B013", "鄭新進", "TSMC.Fab12.製造部", JobLevelStaff},     // 員工（模擬新進）
    }
}

func sync(ctx, db) {
    tx, _ := db.BeginTx(ctx, nil)
    for _, r := range records {
        tx.ExecContext(ctx, `
            INSERT INTO employees (badge_id, name, org_path, job_level, is_active, updated_at)
            VALUES ($1, $2, $3, $4, TRUE, NOW())
            ON CONFLICT (badge_id) DO UPDATE
            SET name=EXCLUDED.name, org_path=EXCLUDED.org_path,
                job_level=EXCLUDED.job_level, is_active=TRUE, updated_at=NOW()
        `, r.badgeID, r.name, r.orgPath, r.jobLevel)
    }
    tx.Commit()
}
```

#### 為什麼是 mock 而非真接 LDAP？
- HW2 §5.3 列為 CronJob 但沒指定資料源
- demo 環境不會有 LDAP server
- 換真 LDAP：把 `mockLDAP()` 換成 `gopkg.in/ldap.v3` 連線抓 OU 即可，呼叫端不變

#### `is_active=TRUE` 為什麼一律設真？
mock 資料代表「目前在職員工列表」，所以 sync 結果都是在職。若 LDAP 回掉某個 badge，下次 sync 不會 UPSERT 該 badge，但**也不會 set is_active=FALSE**。這留作後續改進（diff 比對 + soft-delete）。

#### `trg_sync_org_path_ltree` 自動接管
org-sync UPSERT 只寫 `org_path` VARCHAR，不寫 `org_path_ltree`。0003 的 BEFORE INSERT/UPDATE trigger 自動同步 ltree 欄位 — 上游服務不需要關心。

#### 健康端點
`:8085/healthz` 含 `sync_count`。

---

## 5. Docker Compose 拓樸

### 5.1 service 數量變化
- **Phase 1**：postgres + redis + migrate + access-api + event-processor + reporting-api + frontend = **7 個**
- **Phase 2**：上述 7 個 + anomaly-detector + mv-refresher + org-sync = **10 個**

### 5.2 重要 env 變更

```yaml
postgres:
  image: postgres:16-alpine                  # 15 → 16
  environment:
    LANG: C.UTF-8                            # 新增：讓 ltree 中文 label work
  networks:
    pacs-network:
      aliases:
        - postgres-replica                   # 新增：read replica alias

reporting-api:
  environment:
    - DB_HOST=postgres-replica               # postgres → postgres-replica（demo alias）
    - DEV_AUTH_BYPASS=1                      # 新增：frontend 不需改 code
    - JWT_SECRET=dev-only-not-for-prod-...   # 新增
```

### 5.3 Read Replica：故意簡化的「假 replica」

正式環境 HW2 §5.3 是 Cloud SQL HA Primary + Read Replica（streaming replication）。本機 demo 為了不啟動 bitnami/postgresql 的 streaming replication 配置（init script + replication slot + pg_basebackup），用 **docker network alias** 解：

```yaml
postgres:
  networks:
    pacs-network:
      aliases:
        - postgres-replica   # 同 container 多一個名字
```

效果：
- `reporting-api` 透過 `postgres-replica:5432` 連線，DNS resolve 到 primary container
- 拓樸上「reporting-api → postgres-replica」這條線存在
- 換成真 replica 只需把 alias 拿掉、加 `postgres-replica` 真正的 service

docker-compose.yml 註解明確標明這是 demo 簡化。

### 5.4 obsolete `version` 屬性
順手把 `version: '3.8'` 移除（docker compose v2+ 已 deprecated，留著會 warn）。

---

## 6. 關鍵設計決策（含替代方案）

### 6.1 PG 15 → 16 升級（為了 ltree 中文 label）

| 選項 | 取捨 | 採用？ |
|---|---|---|
| 升 PG 16 | image 換、需 down -v 清 volume；ltree 中文 label work | ✅ |
| 保 PG 15 + ASCII ltree label（如 `Mfg`/`QA`） | UI 顯示變英文、要動 frontend | ❌ |
| 保 PG 15 + 雙欄位（VARCHAR 中文 + LTREE 英文） | 雙 source of truth、UPSERT 邏輯複雜 | ❌ |

**採用 PG 16 + UTF-8 locale，賭 ltree 中文支援；事實證明 PG 16 + C.UTF-8 接受中文 label**（[實測在 §15.1](PHASE2_VERIFICATION.md#151-ltree--中文-label--gist-index)）。

### 6.2 event_date 從 generated column 改普通欄位

| 選項 | 取捨 | 採用？ |
|---|---|---|
| 維持 STORED generated + 不 partition | 違反 HW2 §5.3 partition by month 要求 | ❌ |
| Partition by expression `((event_time AT TIME ZONE Asia/Taipei)::date)` | query 必須用同樣 expression 才會 prune；應用層改動大 | ❌ |
| Partition by event_time（TIMESTAMPTZ）+ 月份邊界用 timestamp | event_date 過濾 query 不會被 prune；損失整個 NFR-2 主目標 | ❌ |
| **event_date 改普通 DATE + INSERT 路徑顯式提供** | 應用層 1 處改動（postgres.go InsertEvent）；語意等價 | ✅ |
| event_date 普通 + BEFORE INSERT row trigger 自動填 | PG partition routing 在 trigger 之前 fire，trigger 太晚 | ❌（試過失敗）|

### 6.3 OIDC：自簽 HS256 vs 跑 Keycloak vs mock-oauth2-server

[使用者選擇](PHASE2_VERIFICATION.md#9-section-8--fr-10-oidc自簽-hs256-jwt) 自簽 HS256：

| 選項 | 啟動成本 | 真實度 | 採用？ |
|---|---|---|---|
| 自簽 HS256 + dev/login | 0 service 增加 | 中（claim 結構同 OIDC） | ✅ |
| Keycloak container | ~30s + 200MB | 高 | ❌（demo 太重）|
| mock-oauth2-server | ~5s + 50MB | 中 | ❌（不必要的中間值）|

### 6.4 PDF 匯出延後

| 選項 | 工程量 | 採用？ |
|---|---|---|
| **只做 Excel（excelize）** | 半天 | ✅（使用者選擇）|
| Excel + PDF（gofpdf + Noto Sans CJK） | 1-2 天（中文字型嵌入） | ❌ |

### 6.5 MV refresh：獨立 service vs pg_cron vs 內嵌

[使用者選擇](PHASE2_VERIFICATION.md) 新 service：

| 選項 | 採用？ |
|---|---|
| **獨立 `mv-refresher` service** | ✅（HW2 §5.3 暗示獨立元件）|
| pg_cron extension | ❌（需要換 postgres image / 加 shared_preload_libraries）|
| event-processor 內嵌 ticker | ❌（違反 separation of concerns）|

### 6.6 Read Replica：真 replica vs 假 alias

| 選項 | 工程量 | 採用？ |
|---|---|---|
| **docker network alias 指同 DB** | 0 行 | ✅ demo 簡化 |
| bitnami/postgresql streaming replication | 半天（換 image + 寫 replication user + env） | ❌（demo 不必要）|
| 假 replica 用 pg_basebackup 啟動拷一次 | 兩小時（entrypoint script） | ❌（中途簡化）|

---

## 7. 已知限制與後續工作

| 限制 | 升級路徑 |
|---|---|
| Read Replica 是 alias，沒 streaming replication | 換 bitnami/postgresql 或 Cloud SQL HA |
| OIDC 用 HS256 自簽 | 接真 IdP（Keycloak / Auth0 / Google），verify 改 RS256 |
| PDF 匯出未實作 | 加 gofpdf + Noto Sans CJK TTF 嵌入；改 export endpoint 接受 `format=pdf` |
| LDAP 是 mock | 接 `gopkg.in/ldap.v3` 抓 OU；soft-delete 失效員工 |
| Anomaly `STAT_OUTLIER` 未實作 | 用 mv_daily_attendance 算每員工歷史 mean ± 3σ |
| Anomaly retry 不是 backoff | 改 exponential delay |
| DLQ 沒 monitoring/replay UI | 加 `/v1/dlq` endpoint 列 dead messages、`POST /v1/dlq/{id}/replay` |
| HPA / autoscaling | 移到 GKE 後加 prometheus-adapter |
| `org-sync` 不會把消失的 badge 設 `is_active=FALSE` | 加 diff 比對 + soft-delete |
| Partition 預建到 2027-12 | 加 cron 預建下個月 / 用 pg_partman |

---

## 8. 對應 HW2 spec

### 8.1 FR 一覽

| FR | 規格摘要 | 落實位置 | Phase 2 是否觸碰 |
|---|---|---|:---:|
| FR-1 | swipe sub-50ms | access-api（不打 DB）| - |
| FR-2 | Anti-Passback | access-api + Redis APB cache | - |
| FR-3 | 拒絕原因 | access_events.reason | - |
| FR-4 | 事件非同步 | Redis Stream + event-processor | - |
| FR-5 | 個人出勤 | reporting-api `/v1/reports/attendance` | ✏️ (event_date 改寫)|
| FR-6 | 階層團隊報表 | reporting-api `/v1/reports/manager-team` + ltree `<@` | ✨ 新 |
| FR-7 | 出勤趨勢 | reporting-api `/v1/reports/trend` + mv_daily_attendance | ✨ 新 |
| FR-8 | PDF/Excel 匯出 | reporting-api `/v1/reports/attendance/export?format=excel` | ✨ 新（Excel 部分） |
| FR-9 | 階層資料權限 | API 層 pattern a + ltree filter + 403 | ✨ 新 |
| FR-10 | OIDC | JWT middleware + `/v1/dev/login` | ✨ 新 |
| FR-11 | 異常警報 | anomaly-detector + alerts 表 + reporting-api `/v1/alerts` | ✨ 新 |
| FR-12 | Immutable audit | DB trigger + REVOKE（Phase 2 partition root 重掛 trigger）| ✏️ (partition 後重掛)|
| FR-13 | Audit 查詢 | reporting-api `/v1/audit` + event_date 索引 | ✏️ (event_date 改寫)|

### 8.2 NFR 一覽

| NFR | 規格摘要 | 落實 | Phase 2 觸碰 |
|---|---|---|:---:|
| NFR-1 | 寫入 P99 < 50ms | access-api 不打 DB | - |
| NFR-2 | 報表 P95 < 200ms | partial index + partition pruning + MV | ✏️ (改 query / 加 partition / MV) |
| NFR-3 | 99.9% uptime | Phase 2+ infra 層；本階段不適用 | - |
| NFR-4 | HPA | 本機 demo 不適用 | - |
| NFR-5 | DB 失效不丟事件 | Redis Stream buffer | - |
| NFR-6 | mTLS + OIDC | OIDC 部分（FR-10）；mTLS infra 層 | ✨ 部分新 |
| NFR-7 | Observability | pg_stat_statements + slow log | - |
| NFR-8 | Immutable | 同 FR-12 | - |

### 8.3 §5.3 元件對應

| HW2 §5.3 元件 | 本實作 | 狀態 |
|---|---|:---:|
| access-api | Phase 1 既有 | ✅ |
| event-processor | Phase 1 既有 | ✅ |
| **reporting-api** | Phase 1 既有 + 5 個 endpoint | ✅ |
| **anomaly-detector** | `cmd/anomaly-detector/` | ✅ 新 |
| **org-sync CronJob** | `cmd/org-sync/` | ✅ 新 (mock LDAP) |
| Cache (Memorystore HA 5GB) | redis container | ⚠️ demo 用 single instance |
| Primary DB (Cloud SQL HA + monthly partition) | postgres + 36 partitions | ✅ partition；HA infra 層 |
| Read Replica | docker network alias | ⚠️ demo 簡化 |
| Message Queue (Pub/Sub + DLQ) | Redis Streams + `pacs:events:dead` | ✅ 概念對齊 |
| **mv_daily_attendance MV** | migration 0006 + mv-refresher | ✅ 新 |
| **HPA 雙指標 (CPU+QPS)** | 本機 demo 不適用 | — |

---

## 9. 怎麼擴展

### 9.1 加一個新規則進 anomaly-detector

1. 在 `cmd/anomaly-detector/main.go` 加 `isMyRule(event)` 函式
2. 在 `detect()` 加一行 `if isMyRule(e) { raise("MY_RULE", "MEDIUM", ...) }`
3. 在 migration 0004 的 CHECK 列舉加 `'MY_RULE'`
4. 重新跑 migrate（或 `ALTER TABLE alerts DROP CONSTRAINT ... ADD CONSTRAINT ...`）

### 9.2 加一個新報表 endpoint

1. 在 `internal/db/postgres.go` 加 `QueryXxxReport(ctx, ...) ([]models.XxxReport, error)`
2. 在 `internal/models/models.go` 加 `XxxReport` struct
3. 在 `cmd/reporting-api/main.go` 加 `getXxxReport(c *gin.Context)` 與 router line `authed.GET("/v1/reports/xxx", getXxxReport)`

### 9.3 加一個新 cron service

仿 `cmd/mv-refresher/` 或 `cmd/org-sync/` 結構：
```
cmd/your-cron/
  main.go     # 連 DB / Redis、起 ticker、健康端點 :808X
```
加進 docker-compose：
```yaml
your-cron:
  build:
    context: ./backend
    args:
      SERVICE: your-cron
  environment: [...]
  depends_on: [...]
  networks: [pacs-network]
```
不必動 Dockerfile（已 parametric by `ARG SERVICE`）。

### 9.4 換真 Read Replica

```yaml
postgres:
  # 移除 networks.pacs-network.aliases

postgres-replica:
  image: bitnami/postgresql:16
  environment:
    POSTGRESQL_REPLICATION_MODE: slave
    POSTGRESQL_REPLICATION_USER: rep_user
    POSTGRESQL_REPLICATION_PASSWORD: rep_pass
    POSTGRESQL_MASTER_HOST: postgres
    POSTGRESQL_MASTER_PORT_NUMBER: 5432
    POSTGRESQL_PASSWORD: pacs_password
  depends_on:
    postgres:
      condition: service_healthy
  networks: [pacs-network]
```
+ primary 端設 `wal_level=replica`、建 replication user。

### 9.5 換真 OIDC

把 `cmd/reporting-api/main.go` 的 `/v1/dev/login` 拿掉，把 `internal/auth/jwt.go` 的 `Parse` 改驗 RS256：
```go
key, _ := jwk.Fetch("https://<provider>/.well-known/jwks.json")
token, _ := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
    return key, nil  // 用 IdP 的公鑰驗
})
```
DEV_AUTH_BYPASS 直接拿掉、frontend 加 OIDC redirect flow。

---

## 10. 一句總結

把單體 PACS 拆成 4 個職責清楚的微服務、補完所有 reporting endpoint、加上 FR-10 / FR-11 兩條全新的縱貫線、把資料層升到符合 HW2 §5.3 規格的 ltree + monthly partition + materialized view + DLQ，本機 14/14 驗收綠燈。

---

## 11. Phase 2.x — `is_manager` → `job_level` 多階主管重構

### 動機
`employees.is_manager BOOLEAN`（migration `0002`）只能表達二元狀態，無法區分廠長 vs. 部主管。組員要求支援「一級／二級主管」多階層。

### 變更面
1. **DB** — migration `0102_replace_is_manager_with_job_level`：
   - `ADD COLUMN job_level VARCHAR(20) NOT NULL DEFAULT 'STAFF' CHECK IN ('STAFF','MANAGER_L1','MANAGER_L2')`
   - 回填：`B100 → MANAGER_L1`、`B001~B005 → MANAGER_L2`
   - `DROP COLUMN is_manager`
2. **Backend** — `internal/db/postgres.go::GetManagerScope` SQL 從 `is_manager = TRUE` 改 `job_level <> 'STAFF'`
3. **org-sync** — `orgRecord.jobLevel string` 取代 `isManager bool`；mockLDAP 補上 `MANAGER_L1`；UPSERT 改寫 `job_level` 欄位

### 不影響面
- **API JSON schema 不變**：`manager-team` response 仍是 `{ manager_scope, reports[] }`，下游 frontend 無感
- **權限範圍不變**：FR-9 scope 仍為 `org_path_ltree` 子樹；`job_level` 是身分標籤，不是權限決策因子
- **MV / 分區**：`mv_daily_attendance` 不含 `is_manager`，免重建；`access_events` 分區不受影響
- **JWT / 認證流程**：JWT 只帶 `badge_id`，無變更

### 未來擴充
新增 `MANAGER_L3`（如組長）：
```sql
ALTER TABLE employees DROP CONSTRAINT employees_job_level_check;
ALTER TABLE employees ADD CONSTRAINT employees_job_level_check
  CHECK (job_level IN ('STAFF','MANAGER_L1','MANAGER_L2','MANAGER_L3'));
```
後端零改動。
