# PACS — 分散式實體門禁控制系統

Cloud-Native Physical Access Control System

## 架構

```
Badge Readers / Frontend
        │
        ▼
  ┌─────────────┐     ┌───────┐     ┌──────────────┐
  │ access-api  │────▶│ Redis │────▶│    Redis      │
  │  (Port 8080)│     │ Cache │     │   Streams     │
  └─────────────┘     │  APB  │     │  (pacs:events)│
                      └───────┘     └──────┬───────┘
                                           │
                                           ▼
                                   ┌───────────────┐
                                   │event-processor│
                                   └───────┬───────┘
                                           │
                                           ▼
                                   ┌───────────────┐
                                   │  PostgreSQL   │
                                   │ (append-only) │
                                   └───────┬───────┘
                                           │
                                           ▼
                                   ┌───────────────┐
                                   │ reporting-api │
                                   │  (Port 8081)  │
                                   └───────────────┘
```

## 快速啟動

```bash
docker-compose up --build
```

- **前端介面**: http://localhost
- **Access API**: http://localhost:8080
- **Reporting API**: http://localhost:8081

## 技術棧

| 元件 | 技術 |
|------|------|
| 前端 | HTML5 + CSS3 + JavaScript + Nginx |
| 後端 | Go 1.21 + Gin Framework |
| 資料庫 | PostgreSQL 15 |
| 快取/MQ | Redis 7 (Cache + Streams) |
| 容器化 | Docker + Docker Compose |

## 資料庫

Schema 與 seed data 由 [golang-migrate](https://github.com/golang-migrate/migrate)
管理，所有變更檔案放在 `scripts/migrations/`，是 single source of truth。

### 啟動流程

`docker compose up` 會：

1. 起動 `postgres`，等待 `pg_isready` 健康檢查通過（並啟用
   `pg_stat_statements` 與 `log_min_duration_statement=100ms`）。
2. 起動 `migrate` 一次性 service，依序套用所有 `up` migrations 後退出。
3. `event-processor` 與 `reporting-api` 等待 `migrate` 退出 0 後才啟動。

### Schema 摘要

| Table          | 用途                              | 寫入者              | 讀取者          |
|----------------|-----------------------------------|---------------------|-----------------|
| `access_events` | append-only 稽核日誌（FR-12 immutable） | `event-processor`   | `reporting-api` |
| `employees`     | 員工主檔                          | (運維)              | `reporting-api` |

關鍵索引（baseline migration `0001`）：

- `idx_events_status_date`：`(event_date, badge_id) WHERE status='SUCCESS'` — attendance 報表
- `idx_events_badge_eventdate`：`(badge_id, event_date DESC)` — audit trail
- `event_date` 是以 `Asia/Taipei` 時區計算的 STORED generated column

### 角色分工（最小權限）

| 角色            | 權限                          | 由誰使用           |
|-----------------|-------------------------------|--------------------|
| `pacs_user`     | `SELECT, INSERT`（`UPDATE`/`DELETE` 已 revoke + trigger 阻擋） | `event-processor` |
| `pacs_reporter` | `SELECT` only                  | `reporting-api`   |

`access-api` 完全不連 PostgreSQL（走 Redis cache + Stream）。

### 手動備份

```bash
docker compose exec -T postgres pg_dump -U pacs_user pacs_db > backup-$(date +%Y%m%d).sql
```

### 詳細的 migration 規範與 Phase 2 partitioning playbook

請參閱 [`scripts/README.md`](scripts/README.md)。

### 詳細文件（規範書、ERD、合規對照）

| 文件 | 內容 |
|---|---|
| [`docs/database-spec.md`](docs/database-spec.md) | DB 範圍的 FR / NFR 規範蒸餾、容量估算、Phase 1/2/3 分階段目標 |
| [`docs/database-erd.md`](docs/database-erd.md) | Mermaid ERD、欄位字典、約束、索引、觸發器、角色權限、Phase 2 升級預留 |
| [`docs/database-compliance.md`](docs/database-compliance.md) | spec ↔ 實作 ↔ **實測輸出**對照矩陣（FR-12 三層、NFR-2 EXPLAIN 等） |

文件索引總覽：[`docs/README.md`](docs/README.md)。

### Backend follow-up TODO

`backend/internal/db/postgres.go` 的 `QueryAttendance` 與 `QueryAuditTrail`
目前仍以 `event_time::date` 過濾，導致新加的索引無法直接命中（功能正確，
但 Phase 2 規模下會超出 NFR-2 的 P95 < 200ms 預算）。建議改寫為：

- `WHERE event_date = $1` 取代 `WHERE event_time::date = $1`
- `GROUP BY badge_id, name, org_path, event_date` 取代以 `event_time::date` 分組
- 此 patch 同時修掉 `GROUP BY` 與 `SELECT` 對齊的既有 bug

由 backend owner 在後續 PR 處理；DB 層的 `event_date` 欄位已就位。

## 詳細測試流程

請參閱 [TESTING.md](TESTING.md)
