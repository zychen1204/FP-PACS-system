# PACS 資料庫規範書

> 來源：`Distributed Physical Access Control System.pdf`（系統 spec）、
> `HW2_Architecture_Design_Group15.pdf`（組架構 HW，含 §5.3 Phase 2 設計）。
> 本文件**僅萃取與資料庫有關**的條款，避免重複整份系統 spec。
>
> **狀態**：當前實作對應 **Phase 2**（PR #2 / PR #3 已 merge）。
> Phase 2 全部改動的設計脈絡見
> [`PHASE2_CHANGES.md`](PHASE2_CHANGES.md)、驗收見
> [`PHASE2_VERIFICATION.md`](PHASE2_VERIFICATION.md)。

## 1. 背景

PACS（Distributed Physical Access Control System）是 TSMC 場域的分散式門禁系統，
採 **CQRS + Event-Driven** 架構：

```
Badge Reader ──► access-api ──► Redis Cache (Anti-Passback)
                                Redis Streams (+DLQ) ─► event-processor ─► PostgreSQL 16
                                                       └► anomaly-detector ─► alerts
                                                                     ▲
                                                  mv-refresher ──► mv_daily_attendance
                                                                     ▲
                                            reporting-api ───────────┘  (read-only)
```

寫入路徑（門禁決策）**不打 DB**，全靠 Redis 達 sub-50ms；DB 只在 audit / 報表這條
讀路徑出現。本資料庫即承擔下面四件事：

1. 寫不可變的事件稽核日誌（FR-12 規範強制）
2. 提供報表查詢支援（attendance / audit trail / trend / manager team）
3. 持有員工主檔與組織路徑（access-api 不讀此資料）
4. 持有 FR-11 異常警報紀錄（`alerts` 表，由 anomaly-detector 寫入）

並依 HW2 規劃 **Phase 1 → 2 → 3** 分階段擴容。**Phase 1 + Phase 2 已落地**。

## 2. 資料相關 FR

| ID | 規範摘要 | DB 上的具體要求 | 狀態 |
|---|---|---|:---:|
| FR-3 | 拒絕原因回讀 | `access_events.reason` 欄位記錄拒絕原因 | ✅ |
| FR-4 | 事件非同步持久化 | DB **不在 hot path**；event-processor 從 Redis Streams 消費後才寫入；DB 失效時事件留在 stream | ✅ |
| FR-5 | 個人出勤紀錄 | 每筆事件須含 `event_time` / `gate_id` / `direction`；`stay_hours` 由 reporting-api 用 IN/OUT 配對計算 | ✅ |
| FR-6 | 階層式組織報表（drill-down） | **Phase 2**：`employees.org_path_ltree LTREE` + GiST index；查詢用 `<@` ancestor operator 命中 index | ✅ |
| FR-7 | 出勤趨勢報表 | **Phase 2**：`mv_daily_attendance` materialized view + `mv-refresher` 每 5 min `REFRESH CONCURRENTLY` | ✅ |
| FR-9 | 階層式資料權限（manager 只看子樹） | DB 層提供 `org_path_ltree` 與 `job_level`（多階主管：`MANAGER_L1`/`MANAGER_L2`）；filter logic 由 reporting-api 處理（pattern a：先 `GetManagerScope`（`job_level <> 'STAFF'`）取 scope、空回 403；非空用 `<@` 限縮）。scope 語意仍為「自己 `org_path_ltree` 子樹」，不因 `job_level` 改變 | ✅ |
| FR-11 | 異常警報 | `alerts` 表（`alert_type` CHECK 列舉、`severity`、`details JSONB`），anomaly-detector 寫入、reporting-api 透過 `/v1/alerts` 讀 | ✅ |
| FR-12 | 不可變更稽核（Immutable Audit） | 雙層保護：(a) `REVOKE UPDATE, DELETE ON access_events FROM pacs_user` (b) `BEFORE UPDATE OR DELETE` 與 `BEFORE TRUNCATE` trigger（partition 後重掛到 partition root）| ✅ |
| FR-13 | Audit 查詢（badge × 日期範圍） | 索引 `idx_events_badge_eventdate` 支援 `WHERE badge_id = ? AND event_date BETWEEN ?` 高效查詢；partition pruning 自動裁剪非相關月份 | ✅ |

> FR-1（sub-50ms 開門/拒絕）、FR-2（Anti-Passback）、FR-8（PDF/Excel 匯出）、
> FR-10（OIDC）**不直接落在 DB 層**（由 access-api / reporting-api 處理）。
> FR-11 的 anomaly 偵測規則本身在 anomaly-detector service，DB 只負責持久化。

## 3. 資料相關 NFR

| ID | 規範 | DB 對應策略 | 狀態 |
|---|---|---|:---:|
| NFR-1 | 寫入 P99 < 50 ms（門禁決策） | DB 完全不在寫路徑上；access-api 走 Redis | ✅ |
| NFR-2 | 報表 P95 < 200 ms | (a) partial index `idx_events_status_date WHERE status='SUCCESS'` (b) `idx_events_badge_eventdate` (c) 按月 partition + pruning (d) `mv_daily_attendance` 預聚合 | ✅ 實測 attendance 2.56 ms / audit 0.33 ms @ 10k rows |
| NFR-5 | DB 失效時事件不可丟 | Redis Streams 在 DB 之前 + DLQ；event-processor 拉不到 DB 時事件留在 stream，恢復後 catch-up | ✅ |
| NFR-7 | Observability | 啟用 `pg_stat_statements`、`log_min_duration_statement = 100ms` slow log；Phase 2 加 Prometheus + Grafana (PR #3) | ✅ |
| NFR-8 | Immutable audit | 同 FR-12 雙層保護 | ✅ |

> NFR-3（99.9% uptime）、NFR-4（HPA）、NFR-6（mTLS / 加密）**不在本階段 DB 範圍**，
> 由 infra / API gateway / cloud provider 負責。FR-10 OIDC 由 reporting-api 內建
> JWT middleware 滿足。

## 4. 容量與分階段目標

| 階段 | DAU | events/day | 年度資料量 | DB 配置 | 落地狀態 |
|---|---|---|---|---|:---:|
| Phase 1（試點 Fab12） | 1,000 | 6,000 | 210 MB | 單一 PostgreSQL、無 partitioning、`org_path` + `job_level`（多階主管，migration `0102`，取代早期 `is_manager` BOOLEAN）| ✅ baseline |
| Phase 2（全廠） | 30,000 | 300,000 | 10 GB | PG 16 + 按月 partitioning（36 個月）+ `mv_daily_attendance` + ltree + GiST + alerts + DLQ + read replica alias | ✅ 已落地 |
| Phase 3（全球） | 90,000 | 1,080,000 | 40 GB | AlloyDB（區域內）+ BigQuery（全球分析）+ archive > 24 個月 | 🔮 未來 |

當前實作對應 **Phase 2**。Phase 3 升級路徑見 §6。

## 5. HW2 已選型 / 已 commit 的設計

不可動搖（除非更新 HW2 設計）：

- **DBMS**：PostgreSQL 16（`postgres:16-alpine` + `LANG=C.UTF-8`，PG 16 + UTF-8 locale 接受中文 ltree label）
- **時間型別**：`TIMESTAMPTZ`（內部以 UTC 儲存）
- **Append-only**：`access_events` 嚴格 append-only，FR-12 強制
- **訊息佇列**：Redis Streams（`pacs:events`）+ DLQ（`pacs:events:dead`，重試 3 次後）
- **角色分離**：write role（`pacs_user`，含 event-processor / anomaly-detector / mv-refresher / org-sync）vs read-only role（`pacs_reporter`，reporting-api）
- **組織樹編碼**：**ltree + GiST index**（HW2 §5.2/§5.3 明列）。實作上保留 `org_path VARCHAR`（中文，給 UI 顯示）與 `org_path_ltree LTREE`（ASCII / 中文皆可，給 query），由 `trg_sync_org_path_ltree` trigger 自動同步
- **partitioning**：`access_events` 依 `event_date` `PARTITION BY RANGE`，預建 2025-01 ~ 2027-12 共 36 個月份分區
- **materialized view**：`mv_daily_attendance`，由 `mv-refresher` service 每 5 min `REFRESH CONCURRENTLY`

可在 Phase 2 內彈性決定：

- 索引組合細節
- DLQ retry policy（目前 3 次後丟 DLQ）
- 觀測工具（pg_stat_statements 已採用、PR #3 加 Prometheus + Grafana）

### 5.1 為何 event_date 從 GENERATED STORED 改普通 DATE 欄位

Phase 1 用 `event_date DATE GENERATED ALWAYS AS ((event_time AT TIME ZONE 'Asia/Taipei')::date) STORED`，但 Phase 2 partition 要求動了它：

1. **PG 不允許 generated column 當 partition key**：
   `ERROR: cannot use generated column in partition key`
2. **試 BEFORE INSERT row trigger 自動填也失敗**：partition tuple routing 在 BEFORE row trigger 之前 fire，trigger 太晚。

最終解法：`event_date` 改為普通 `DATE NOT NULL` 欄位，由呼叫端在 INSERT 顯式提供
`(event_time AT TIME ZONE 'Asia/Taipei')::date`。語意與舊 STORED generated 完全等價。
應用層改動只有 1 處（`backend/internal/db/postgres.go` `InsertEvent`）。

詳細討論：[`PHASE2_CHANGES.md` §3.3](PHASE2_CHANGES.md#33-0005--access_events-按月-partitionhw2-53-明列)。

## 6. Phase 3 升級預留（不在當前實作中）

| 升級項 | 觸發條件 | 工程量 |
|---|---|---|
| Read Replica 換真 streaming replication | 從 GCP 雲端部署起 | 改 image 為 `bitnami/postgresql:16` + replication env，或直接用 Cloud SQL HA |
| AlloyDB / 區域內 PG | 90k DAU 起 | infra 層遷移 |
| BigQuery 全球分析 | 跨 region 報表 | ETL pipeline + 雙寫 |
| Archive `access_events` > 24 個月 → Cloud Storage Parquet | 表 > 50 GB | 一次性 cron 任務 |
| pg_partman 自動預建下個月 partition | 接近預建範圍尾端（2027-12）| 加 extension + cron |
| Closure table 取代 ltree | 組織深度 > 5 或 GiST 效能下滑 | 1 sprint（很可能不會發生）|

## 7. 顯式不在本資料庫範圍

避免日後誤會，明確列出：

- 門禁決策延遲（FR-1 / NFR-1，由 access-api + Redis 負責）
- Anti-Passback 邏輯（FR-2，access-api + Redis Lua-like check）
- 異常檢測**規則**（FR-11，由 anomaly-detector service 跑規則；DB 只負責持久化結果）
- 報表匯出格式（FR-8，由 reporting-api 用 excelize 產 .xlsx）
- 身份驗證（FR-10，reporting-api 的 JWT middleware）
- 加密、mTLS（NFR-6，infra 層）
- 自動擴展（NFR-4，K8s HPA）

## 8. 驗證

每條 FR / NFR 的對應實作位置與實測證據見
[`database-compliance.md`](database-compliance.md)；完整 Phase 2 驗收劇本見
[`PHASE2_VERIFICATION.md`](PHASE2_VERIFICATION.md)（19 個 section、含命令 + 預期 + 實測 + 結論）。
