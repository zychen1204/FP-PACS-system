# 系統架構 (System Architecture)

本文件歸檔了 PACS 系統在 Phase 2 升級後的完整微服務架構設計與資料庫結構。

## 系統架構圖

```text
                    ┌──────────────────┐
                    │  frontend (nginx)│  (Port 80)
                    │  SPA / 靜態頁面   │
                    └────────┬─────────┘
                             │
              Badge Readers / Browser
                             │
              ┌──────────────┴──────────────┐
              ▼                             ▼
      ┌─────────────┐              ┌────────────────────┐
      │ access-api  │ (Port 8080)  │   reporting-api    │ (Port 8081)
      │ POST /v1/swipe             │ /v1/reports/*      │
      │ GET /metrics│              │ /v1/reports/*/aggregated
      │ Anti-Passback              │ /v1/audit          │
      └──────┬──────┘              │ /v1/alerts         │
             │                     │ /v1/dev/login      │
  ┌──────────┼──────────┐         └────────┬───────────┘
  ▼                     ▼                  │ (via postgres-replica alias)
┌──────────┐     ┌──────────────┐          │
│  Redis   │     │ Redis Streams│          │
│  Cache   │     │ pacs:events  │          │
│  (APB)   │     └──────┬───────┘          │
└──────────┘  (named consumer groups)      │
                        │                  │
     ┌──────────────────┼────────────┐     │
     ▼                               ▼    │
┌────────────────┐           ┌──────────────────┐
│ event-processor│           │ anomaly-detector │
│  寫 access_events          │ 3 條規則 → alerts │
└────────┬───────┘           └────────┬─────────┘
         │                            │
         │ (DLQ: pacs:events:dead     │
         │  在重試 3 次後)             │
         ▼                            ▼
┌──────────────────────────────────────────────────────┐
│          PostgreSQL 16-alpine (:5432)                 │
│  access_events (append-only, 36 monthly partition)   │
│  employees (org_path + org_path_ltree + job_level)   │
│  alerts                                              │
│  mv_daily_attendance (materialized view)             │
└──────────┬───────────────────────┬───────────────────┘
           │                       │
     ┌─────▼──────┐         ┌──────▼──────┐
     │mv-refresher│         │  org-sync   │
     │ 5min REFRESH│        │ LDAP→DB 同步 │
     │  (:8084)    │        │  (:8085)    │
     └─────────────┘        └─────────────┘

──── Observability ────────────────────────────────────
  ┌──────────────┐        ┌──────────────┐
  │  Prometheus  │───────▶│   Grafana    │
  │  (:9090)     │        │  (:3000)     │
  └──────────────┘        └──────────────┘
        ▲
        │ scrape /metrics
        │
   access-api, reporting-api
```

## 全服務列表

`docker compose up -d` 啟動共 **12 個 service**（含 infra + one-shot migrate）：

### 後端微服務

| Service | Port | DB User | 角色 |
|---|---|---|---|
| `access-api` | 8080 | — (不連 DB) | 門禁寫入路徑，直接與 Redis 互動（低延遲 APB 驗證）；暴露 `GET /metrics` 供 Prometheus 抓取 |
| `event-processor` | (8082 health) | `pacs_user` | 消費 Stream 並持久化寫入 `access_events` 表格 |
| `reporting-api` | 8081 | `pacs_reporter` | 報表（含 aggregated）、稽核軌跡、警報、Excel 匯出、JWT 簽發 |
| `anomaly-detector` | (8083 health) | `pacs_user` | 規則引擎（OFF_HOURS_ENTRY / APB_BURST / TAILGATING），訂閱 Stream 寫 `alerts` |
| `mv-refresher` | (8084 health) | `pacs_user` | 每 5 分鐘 `REFRESH MATERIALIZED VIEW CONCURRENTLY mv_daily_attendance` |
| `org-sync` | (8085 health) | `pacs_user` | 模擬 LDAP / AD → UPSERT `employees` |

### 基礎設施

| Service | Port | 角色 |
|---|---|---|
| `postgres` | 5432 | PostgreSQL 16-alpine，network alias `postgres-replica` 供 reporting-api 讀 |
| `redis` | 6379 | Redis 7-alpine，APB state cache + Streams `pacs:events` |
| `migrate` | (one-shot) | golang-migrate，啟動時跑 `scripts/migrations/` 0001~0105 |
| `frontend` | 80 | nginx，serve SPA 靜態頁面（刷卡模擬、出勤報表、趨勢分析） |
| `prometheus` | 9090 | 抓取 access-api `/metrics`、各 service health |
| `grafana` | 3000 | 視覺化 dashboard（admin/admin），接 Prometheus 資料源 |

## 資料庫架構與設計理念

### 核心 Schema
- **`access_events`**：append-only 稽核日誌，採用不可變設計（FR-12 immutable），並按月進行 Partitioning（預建 36 個月份分區）。
- **`employees`**：員工主檔，包含 `org_path` (中文) 與 `org_path_ltree` (GiST 索引)，並使用 `job_level` (STAFF / MANAGER_L1 / MANAGER_L2) 控制層級權限以供主管視野 (FR-6/9) 查詢。
- **`alerts`**：FR-11 異常警報紀錄。
- **`mv_daily_attendance`**：Materialized View，預先聚合每日 first_in / last_out / swipe_count / stay_hours，供趨勢報表與主管視野查詢。
  - `stay_hours` 演算法（0105 修正）：IN/OUT counter pairing + Asia/Taipei 00:00 切片。同日多次進出（午休、會議外出）分別累加；跨午夜 visit 依日期切分到不同列；未配對 IN / OUT 自動忽略。
  - 由獨立 `mv-refresher` service 每 5 分鐘 `REFRESH MATERIALIZED VIEW CONCURRENTLY` 一次。

### 角色分工（最小權限原則）
| 角色 | 權限範圍 | 使用的微服務 |
|---|---|---|
| `pacs_user` | `SELECT, INSERT` (觸發器與角色設定禁止 UPDATE/DELETE 以保護日誌) | `event-processor`, `anomaly-detector`, `mv-refresher`, `org-sync` |
| `pacs_reporter` | Read Only (`SELECT` only) | `reporting-api` |

## 壓測工具分工

PACS 採雙工具壓測架構，避免「灌歷史」與「即時壓測」混淆：

| 工具 | 角色 | 路徑 | 主要驗證 |
|---|---|---|---|
| **seed-generator** (`scripts/seed-generator/`) | 一次性灌歷史 demo 資料 | 直接 SQL → `psql` 灌 `access_events` | dashboard 有畫面、EXPLAIN ANALYZE 看得到 index 效益 |
| **k6-load-test** (`scripts/k6-load-test/`) | 即時 HTTP 壓測 | `POST /v1/swipe` → access-api → Redis → Stream → event-processor | NFR-1 `p(99)<50ms`、NFR-2 `p(95)<200ms`、NFR-4 HPA 60s 擴展、spec「Shift Change spike」可視化 |

兩者**不互相取代**：seed-generator 走 SQL 直灌可保留真實時間戳供報表計算，
k6 走 HTTP 才能驗證 access-api / Redis APB / Stream / event-processor 完整鏈路效能。

詳細指南：
- [`SimulationGuide.md`](SimulationGuide.md) — seed-generator
- [`LoadTestGuide.md`](LoadTestGuide.md) — k6 三場景
