# PACS — 分散式實體門禁控制系統

Cloud-Native Physical Access Control System

> **狀態**：HW2 §5.3 Phase 2 後端已落地（PR #2 + #3）。Phase 2 改動詳述見
> [`docs/PHASE2_CHANGES.md`](docs/PHASE2_CHANGES.md)、驗收見
> [`docs/PHASE2_VERIFICATION.md`](docs/PHASE2_VERIFICATION.md)、前端整合指引見
> [`docs/FRONTEND_INTEGRATION.md`](docs/FRONTEND_INTEGRATION.md)。

## 架構（Phase 2）

```
                            Badge Readers / Frontend
                                      │
                                      ▼
                              ┌─────────────┐
                              │ access-api  │   (Port 8080, 不打 DB)
                              │  Anti-Passback│
                              └──────┬──────┘
                                     │
                       ┌─────────────┼─────────────┐
                       ▼                           ▼
                 ┌──────────┐               ┌──────────────┐
                 │  Redis   │               │ Redis Streams│
                 │  Cache   │               │ pacs:events  │
                 │  (APB)   │               └──────┬───────┘
                 └──────────┘    (named consumer groups)
                                                │
                       ┌────────────────────────┼──────────────────────┐
                       ▼                                               ▼
              ┌────────────────┐                              ┌──────────────────┐
              │ event-processor│                              │ anomaly-detector │
              │   寫 access_events                            │  3 條規則 → alerts│
              └────────┬───────┘                              └────────┬─────────┘
                       │                                               │
                       │  (DLQ: pacs:events:dead 在重試 3 次後)         │
                       ▼                                               ▼
              ┌──────────────────────────────────────────────────────────────┐
              │              PostgreSQL 16 (append-only, 36 monthly partition)│
              │  access_events  /  employees(org_path + org_path_ltree)       │
              │  alerts         /  mv_daily_attendance (materialized view)    │
              └─────────┬───────────────────────────────┬────────────────────┘
                        │                               │
                  ┌─────▼─────┐                  ┌──────▼──────┐
                  │mv-refresher│                 │  org-sync   │
                  │ 5min REFRESH│                │ LDAP→DB 同步 │
                  └────────────┘                 └─────────────┘
                        │
                        ▼  (network alias: postgres-replica)
                  ┌────────────────────┐
                  │   reporting-api    │  (Port 8081, JWT-protected)
                  │ /v1/reports/*      │
                  │ /v1/audit          │
                  │ /v1/alerts         │
                  │ /v1/dev/login      │
                  └────────────────────┘
```

## 快速啟動

```bash
docker compose up -d --build
sleep 25                          # 等 migrate + 各 service ready
```

- **前端介面**: <http://localhost>
- **Access API**: <http://localhost:8080>
- **Reporting API**: <http://localhost:8081>
- **完整驗收劇本**：[`docs/PHASE2_VERIFICATION.md`](docs/PHASE2_VERIFICATION.md)

## 技術棧

| 元件 | 技術 |
|------|------|
| 前端 | HTML5 + CSS3 + JavaScript + Nginx |
| 後端 | Go 1.21 + Gin Framework + golang-jwt/v5 + xuri/excelize/v2 |
| 資料庫 | PostgreSQL 16 (C.UTF-8 locale, ltree, pg_stat_statements) |
| 快取/MQ | Redis 7 (Cache + Streams + DLQ) |
| 容器化 | Docker + Docker Compose |
| 觀測 | pg_stat_statements + slow log + Prometheus + Grafana (PR #3) |

### 後端 service 列表

| Service | Port | 角色 |
|---|---|---|
| `access-api` | 8080 | 門禁寫入路徑，不打 DB |
| `event-processor` | (8082 health) | 消費 stream 寫 `access_events` |
| `reporting-api` | 8081 | 報表 / 警報 / 匯出 / JWT 簽發 |
| `anomaly-detector` | (8083 health) | FR-11 規則引擎、寫 `alerts` |
| `mv-refresher` | (8084 health) | 每 5 min `REFRESH MV CONCURRENTLY` |
| `org-sync` | (8085 health) | LDAP / AD → `employees` upsert（mock）|

## 資料庫

Schema 與 seed data 由 [golang-migrate](https://github.com/golang-migrate/migrate)
管理，所有變更檔案放在 `scripts/migrations/`，是 single source of truth。

### 啟動流程

`docker compose up` 會：

1. 起動 `postgres`（PG 16 + C.UTF-8），等待 `pg_isready` 健康檢查（並啟用
   `pg_stat_statements` 與 `log_min_duration_statement=100ms`）。
2. 起動 `migrate` 一次性 service，依序套用所有 `up` migrations（0001~0006 + 0099 dev_seed）後退出。
3. `event-processor` / `reporting-api` / `anomaly-detector` / `mv-refresher` / `org-sync`
   等待 `migrate` 退出 0 後才啟動。

### Schema 摘要

| Table | 用途 | 寫入者 | 讀取者 |
|---|---|---|---|
| `access_events` | append-only 稽核日誌（FR-12 immutable，按月 partition）| `event-processor` | `reporting-api` |
| `employees` | 員工主檔（`org_path` 中文 + `org_path_ltree` GiST + `is_manager` flag）| `org-sync` / 運維 | `reporting-api` |
| `alerts` | FR-11 異常警報 | `anomaly-detector` | `reporting-api` |
| `mv_daily_attendance` (MV) | FR-7 趨勢報表預聚合 | `mv-refresher` REFRESH | `reporting-api` |

### FR-6 / FR-9 階層查詢實作

Phase 2 改用 **ltree + GiST index**（HW2 §5.3 明列規格）：
- `employees.org_path_ltree LTREE NOT NULL`，由 `trg_sync_org_path_ltree` 自動同步 `org_path`
- 查詢用 `org_path_ltree <@ $scope::ltree`（descendant of）命中 GiST
- API 層 pattern a：caller badge → `GetManagerScope` 取 scope，空回 403；用 scope filter 子樹

舊版的 `org_path` VARCHAR 仍保留供 UI 顯示中文。詳細查詢樣板見
[`docs/database-compliance.md`](docs/database-compliance.md) §FR-9。

### 關鍵索引

- `idx_events_status_date`：`(event_date, badge_id) WHERE status='SUCCESS'` — attendance 報表（partition-local 由 PG 自動傳播）
- `idx_events_badge_eventdate`：`(badge_id, event_date DESC)` — audit trail
- `idx_employees_org_path_gist`：employees 上的 GiST(`org_path_ltree`) — FR-6/9 ancestor
- `idx_mv_daily_attendance_pk` UNIQUE：MV 上必需 (供 REFRESH CONCURRENTLY)
- `idx_mv_daily_attendance_org_date` GiST：MV 上的 ltree filter
- `idx_alerts_open_recent`：未處理優先 + 時間倒序
- `event_date` 是普通 `DATE NOT NULL` 欄位（partition key 限制；呼叫端在
  INSERT 顯式提供 `(event_time AT TIME ZONE 'Asia/Taipei')::date`）

### 按月 partitioning

`access_events` 已依 `event_date` `PARTITION BY RANGE`，預建 2025-01 ~ 2027-12
共 **36 個月份分區**（`access_events_y2025m01` ~ `access_events_y2027m12`）。
細節與升級脈絡：[`docs/PHASE2_CHANGES.md`](docs/PHASE2_CHANGES.md) §3.3。

### 角色分工（最小權限）

| 角色 | 權限 | 由誰使用 |
|---|---|---|
| `pacs_user` | `SELECT, INSERT` on `access_events`、`alerts`（`UPDATE/DELETE` revoke + trigger 阻擋）；`MV` owner | `event-processor` / `anomaly-detector` / `mv-refresher` / `org-sync` / `migrate` |
| `pacs_reporter` | `SELECT` only on `access_events` / `employees` / `alerts` / `mv_daily_attendance` | `reporting-api` |

`access-api` 完全不連 PostgreSQL（走 Redis cache + Stream）。

### 手動備份

```bash
docker compose exec -T postgres pg_dump -U pacs_user pacs_db > backup-$(date +%Y%m%d).sql
```

### 詳細的 migration 規範

請參閱 [`scripts/README.md`](scripts/README.md)。Phase 2 已落地的 partition / MV / ltree
所有設計脈絡見 [`docs/PHASE2_CHANGES.md`](docs/PHASE2_CHANGES.md)。

### 詳細文件

| 文件 | 內容 |
|---|---|
| [`docs/database-spec.md`](docs/database-spec.md) | DB 範圍的 FR / NFR 規範蒸餾、容量估算、Phase 1/2/3 分階段目標 |
| [`docs/database-erd.md`](docs/database-erd.md) | Mermaid ERD、欄位字典、約束、索引、觸發器、角色權限 |
| [`docs/database-compliance.md`](docs/database-compliance.md) | spec ↔ 實作 ↔ **實測輸出**對照矩陣 |
| [`docs/PHASE2_CHANGES.md`](docs/PHASE2_CHANGES.md) | Phase 2 後端設計改動記錄（10 section、含替代方案對照）|
| [`docs/PHASE2_VERIFICATION.md`](docs/PHASE2_VERIFICATION.md) | Phase 2 完整驗收劇本（19 section、含實測命令 / 預期 / 結論）|
| [`docs/FRONTEND_INTEGRATION.md`](docs/FRONTEND_INTEGRATION.md) | 前端組員整合指引（API 字典、UI mockup、JS snippet）|

文件索引總覽：[`docs/README.md`](docs/README.md)。

## 詳細測試流程

請參閱 [TESTING.md](TESTING.md) 與 [`docs/PHASE2_VERIFICATION.md`](docs/PHASE2_VERIFICATION.md)。
