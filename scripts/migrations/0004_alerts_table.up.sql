-- ============================================================
-- 0004 alerts_table
--
-- FR-11：anomaly-detector 偵測到「非工時進入 / APB 連發 / tailgating /
--        3σ 偏差」等異常時寫入；reporting-api 提供 /v1/alerts 讀取。
--
-- 設計重點：
--   * `alert_type` 列舉常見類別（CHECK constraint 保證 typo 不入庫）
--   * `severity` 排序方便 UI 過濾
--   * `resolved_at` 留空表示未處理（UI 預設只顯示 unresolved）
--   * 索引 `(resolved_at NULLS FIRST, occurred_at DESC)` 對應「先看未處理、
--     再按時間排序」的列表 query
-- ============================================================

CREATE TABLE IF NOT EXISTS alerts (
    id            BIGSERIAL    PRIMARY KEY,
    alert_type    VARCHAR(40)  NOT NULL
                  CHECK (alert_type IN (
                      'OFF_HOURS_ENTRY',
                      'APB_BURST',
                      'TAILGATING',
                      'STAT_OUTLIER'
                  )),
    severity      VARCHAR(10)  NOT NULL DEFAULT 'MEDIUM'
                  CHECK (severity IN ('LOW','MEDIUM','HIGH','CRITICAL')),
    badge_id      VARCHAR(50),                   -- nullable: 部分異常無對應人員
    site_id       VARCHAR(50),
    gate_id       VARCHAR(50),
    details       JSONB        NOT NULL DEFAULT '{}'::jsonb,
    occurred_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    resolved_at   TIMESTAMPTZ
);

-- 預設列表：未處理優先、時間倒序
CREATE INDEX IF NOT EXISTS idx_alerts_open_recent
    ON alerts (resolved_at NULLS FIRST, occurred_at DESC);

-- 按 badge 查 alerts
CREATE INDEX IF NOT EXISTS idx_alerts_badge
    ON alerts (badge_id, occurred_at DESC)
    WHERE badge_id IS NOT NULL;

-- ── 權限 ─────────────────────────────────────────────────────
-- anomaly-detector 連線 user 暫共用 pacs_user：寫入。
-- reporting-api 用 pacs_reporter：只讀。
GRANT INSERT, SELECT ON alerts                    TO pacs_user;
GRANT USAGE          ON SEQUENCE alerts_id_seq    TO pacs_user;
GRANT SELECT         ON alerts                    TO pacs_reporter;
