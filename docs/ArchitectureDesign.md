# 系統架構 (System Architecture)

本文件歸檔了 PACS 系統在 Phase 2 升級後的完整微服務架構設計與資料庫結構。

## 系統架構圖

```text
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

## 後端微服務列表

| Service | Port | 角色 |
|---|---|---|
| `access-api` | 8080 | 門禁寫入路徑，直接與 Redis 互動（低延遲防跟隨驗證），不打 DB |
| `event-processor` | (8082 health) | 消費 Stream 並持久化寫入 `access_events` 表格 |
| `reporting-api` | 8081 | 提供報表、警報、資料匯出查詢，以及 JWT 簽發 |
| `anomaly-detector` | (8083 health) | 規則引擎，訂閱 Stream 並判斷異常行為，寫入 `alerts` |
| `mv-refresher` | (8084 health) | 每 5 分鐘執行 `REFRESH MATERIALIZED VIEW CONCURRENTLY` 加速報表 |
| `org-sync` | (8085 health) | 模擬與 LDAP / AD 系統同步，更新 `employees` |

## 資料庫架構與設計理念

### 核心 Schema
- **`access_events`**：append-only 稽核日誌，採用不可變設計（FR-12 immutable），並按月進行 Partitioning（預建 36 個月份分區）。
- **`employees`**：員工主檔，包含 `org_path` (中文) 與 `org_path_ltree` (GiST 索引)，並使用 `job_level` (STAFF / MANAGER_L1 / MANAGER_L2) 控制層級權限以供主管視野 (FR-6/9) 查詢。
- **`alerts`**：FR-11 異常警報紀錄。
- **`mv_daily_attendance`**：Materialized View，預先聚合每日的出勤資料，確保趨勢報表與主管視野的查詢效能。

### 角色分工（最小權限原則）
| 角色 | 權限範圍 | 使用的微服務 |
|---|---|---|
| `pacs_user` | `SELECT, INSERT` (觸發器與角色設定禁止 UPDATE/DELETE 以保護日誌) | `event-processor`, `anomaly-detector`, `mv-refresher`, `org-sync` |
| `pacs_reporter` | Read Only (`SELECT` only) | `reporting-api` |
