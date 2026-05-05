# PACS 資料庫規範書

> 來源：`Distributed Physical Access Control System.pdf`（系統 spec）、
> `HW2_Group15.pdf`（組架構 HW，含已選型 schema 與分階段容量）。
> 本文件**僅萃取與資料庫有關**的條款，避免重複整份系統 spec。

## 1. 背景

PACS（Distributed Physical Access Control System）是 TSMC 場域的分散式門禁系統，
採 **CQRS + Event-Driven** 架構：

```
Badge Reader ──► access-api ──► Redis Cache (Anti-Passback)
                                Redis Streams ─► event-processor ─► PostgreSQL
                                                                    ▲
                                                    reporting-api ──┘ (read-only)
```

寫入路徑（門禁決策）**不打 DB**，全靠 Redis 達 sub-50ms；DB 只在 audit / 報表這條
讀路徑出現。本資料庫即承擔下面三件事：

1. 寫不可變的事件稽核日誌（FR-12 規範強制）
2. 提供報表查詢支援（attendance、audit trail）
3. 持有員工主檔與組織路徑（access-api 不讀此資料）

並依 HW2 規劃 **Phase 1 → 2 → 3** 分階段擴容。

## 2. 資料相關 FR

| ID | 規範摘要 | DB 上的具體要求 |
|---|---|---|
| FR-3 | 拒絕原因回讀 | `access_events` 需有 `reason` 欄位記錄拒絕原因（Anti-Passback、Offline 等） |
| FR-4 | 事件非同步持久化 | DB **不在 hot path**；event-processor 從 Redis Streams 消費後才寫入；DB 失效時事件留在 stream |
| FR-5 | 個人出勤紀錄 | 每筆事件須含 `event_time` / `gate_id` / `direction`；`stay_hours` 由 reporting-api 用 IN/OUT 配對計算 |
| FR-6 | 階層式組織報表（drill-down） | **Phase 1**：`employees.org_path` 字串路徑（如 `TSMC.Fab12.製造部`）支援 LIKE 前綴查詢 ／ **Phase 2**：升級為 closure table + materialized view |
| FR-7 | 出勤趨勢報表 | **Phase 1**：reporting-api 即時 GROUP BY 聚合 ／ **Phase 2**：`mv_daily_attendance` materialized view，5 分鐘 refresh |
| FR-9 | 階層式資料權限（manager 只看子樹） | reporting-api 層做 filter；DB 層只負責提供 `org_path` 欄位 |
| FR-12 | 不可變更稽核（Immutable Audit） | 雙層保護：(a) `REVOKE UPDATE, DELETE ON access_events FROM pacs_user` (b) `BEFORE UPDATE OR DELETE` 與 `BEFORE TRUNCATE` trigger |
| FR-13 | Audit 查詢（badge × 日期範圍） | 索引必須支援 `WHERE badge_id = ? AND event_date BETWEEN ? AND ?` 的高效查詢 |

> FR-1（sub-50ms 開門/拒絕）、FR-2（Anti-Passback）、FR-8（PDF/Excel 匯出）、
> FR-10（OIDC）、FR-11（30s 異常警報）**不直接落在 DB 層**。

## 3. 資料相關 NFR

| ID | 規範 | DB 對應策略 |
|---|---|---|
| NFR-1 | 寫入 P99 < 50 ms（門禁決策） | DB 完全不在寫路徑上；access-api 走 Redis |
| NFR-2 | 報表 P95 < 200 ms | 索引設計：`idx_events_status_date`（partial WHERE status='SUCCESS'）+ `idx_events_badge_eventdate (badge_id, event_date DESC)` |
| NFR-5 | DB 失效時事件不可丟 | Redis Streams 在 DB 之前；event-processor 拉不到 DB 時事件留在 stream，恢復後 catch-up |
| NFR-7 | Observability | 啟用 `pg_stat_statements`、`log_min_duration_statement = 100ms` slow log |
| NFR-8 | Immutable audit | 同 FR-12 雙層保護 |

> NFR-3（99.9% uptime）、NFR-4（HPA）、NFR-6（mTLS / 加密）**不在本階段 DB 範圍**，
> 由 infra / API gateway / cloud provider 負責。

## 4. 容量與分階段目標

| 階段 | DAU | events/day | 年度資料量 | DB 配置 |
|---|---|---|---|---|
| Phase 1（試點 Fab12） | 1,000 | 6,000 | 210 MB | 單一 PostgreSQL 15、無 partitioning、`org_path` 字串 |
| Phase 2（全廠） | 30,000 | 300,000 | 10 GB | 加上 read replica、按月 partitioning、closure table、`mv_daily_attendance` |
| Phase 3（全球） | 90,000 | 1,080,000 | 40 GB | AlloyDB（區域內）+ BigQuery（全球分析） |

當前實作對應 **Phase 1**。Phase 2 升級路徑寫在 [`scripts/README.md`](../scripts/README.md) 的
「Phase 2 partitioning playbook」與本文件第 6 節。

## 5. HW2 已選型 / 已 commit 的設計

不可動搖（除非更新 HW2 設計）：

- **DBMS**：PostgreSQL 15（`postgres:15-alpine`）
- **時間型別**：`TIMESTAMPTZ`（內部以 UTC 儲存）
- **Append-only**：`access_events` 嚴格 append-only，FR-12 強制
- **訊息佇列**：Redis Streams（`pacs:events`），不直接寫 DB
- **角色分離**：write role（event-processor）vs read-only role（reporting-api）

可在 Phase 1 內彈性決定：
- 索引組合
- generated column / functional column 形式
- 觀測工具（pg_stat_statements 已採用）

## 6. Phase 2 升級預留（不在當前實作中）

| 升級項 | 觸發條件 | 工程量 |
|---|---|---|
| 按月 partitioning（`access_events`） | 表大小 > 5 GB | 一次性 maintenance window，~30 min |
| Closure table 取代 `org_path` 字串 | 真實組織深度 > 5 層 或 hierarchical query 大量出現 | 1 sprint |
| `mv_daily_attendance` materialized view | reporting GROUP BY 開始拖累 P95 | 半 sprint |
| Read replica | 報表 QPS 與寫入互相干擾 | infra 層改動 |

playbook 細節：[`scripts/README.md`](../scripts/README.md) §"Phase 2 partitioning playbook"。

## 7. 顯式不在本資料庫範圍

避免日後誤會，明確列出：

- 門禁決策延遲（FR-1 / NFR-1）
- Anti-Passback 邏輯（FR-2，由 access-api + Redis 負責）
- 異常檢測 / 警報（FR-11）
- 報表匯出格式（FR-8，由 reporting-api 處理 PDF/Excel）
- 身份驗證（FR-10，gateway / OIDC provider）
- 加密、mTLS（NFR-6，infra 層）
- 自動擴展（NFR-4，K8s HPA）

## 8. 驗證

每條 FR / NFR 的對應實作位置與實測證據見
[database-compliance.md](database-compliance.md)。
