-- ============================================================
-- 0006 partition_access_events
--
-- HW2 §5.3 Phase 2：access_events 依月份 PARTITION BY RANGE (event_date)。
--
-- ⚠ PG 限制：partition key 不能是 generated column。
-- 因此本 migration 把 event_date 從 GENERATED STORED 轉成普通 DATE 欄位 +
-- BEFORE INSERT row trigger 自動計算（取 event_time AT TIME ZONE Asia/Taipei
-- 的 date 部分），語意完全等價。trigger 對 INSERT 自動 fire；FR-12 已 REVOKE
-- UPDATE/DELETE，event_date 一旦寫入就不變。
--
-- 流程：
--   1. 建普通欄位版的 partition root 與 36 個月份分區
--   2. INSERT FROM 舊表（trigger 自動填 event_date — 與舊 generated 值一致）
--   3. 新表上掛 FR-12 trigger、index、grant
--   4. RENAME swap，DROP 舊表
-- ============================================================

BEGIN;

-- ── 0. fill_event_date trigger function ─────────────────────
CREATE OR REPLACE FUNCTION fill_event_date()
RETURNS TRIGGER AS $$
BEGIN
    NEW.event_date := (NEW.event_time AT TIME ZONE 'Asia/Taipei')::date;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ── 1. partition root（event_date 為普通 DATE 欄位） ─────────
CREATE TABLE access_events_new (
    id          BIGINT       NOT NULL DEFAULT nextval('access_events_id_seq'),
    badge_id    VARCHAR(50)  NOT NULL,
    site_id     VARCHAR(50)  NOT NULL,
    gate_id     VARCHAR(50)  NOT NULL,
    direction   VARCHAR(10)  NOT NULL CHECK (direction IN ('IN', 'OUT')),
    status      VARCHAR(20)  NOT NULL,
    reason      TEXT         DEFAULT '',
    event_time  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    event_date  DATE         NOT NULL,
    PRIMARY KEY (id, event_date)
) PARTITION BY RANGE (event_date);

ALTER SEQUENCE access_events_id_seq OWNED BY access_events_new.id;

-- BEFORE INSERT row trigger 自動填 event_date（在 root 上 PG11+ 會 propagate 到 partitions）
CREATE TRIGGER trg_fill_event_date
    BEFORE INSERT ON access_events_new
    FOR EACH ROW
    EXECUTE FUNCTION fill_event_date();

-- ── 2. 預建 36 個月份分區（2025-01 ~ 2027-12） ────────────────
DO $$
DECLARE
    start_date DATE := '2025-01-01';
    months_ahead INT := 36;
    i INT;
    partition_start DATE;
    partition_end   DATE;
    partition_name  TEXT;
BEGIN
    FOR i IN 0..(months_ahead - 1) LOOP
        partition_start := start_date + (i || ' months')::INTERVAL;
        partition_end   := start_date + ((i + 1) || ' months')::INTERVAL;
        partition_name  := format('access_events_y%sm%s',
                                  to_char(partition_start, 'YYYY'),
                                  to_char(partition_start, 'MM'));
        EXECUTE format(
            'CREATE TABLE IF NOT EXISTS %I PARTITION OF access_events_new '
            'FOR VALUES FROM (%L) TO (%L)',
            partition_name, partition_start, partition_end
        );
    END LOOP;
END
$$;

-- ── 3. 資料搬遷（trigger 會覆寫 event_date 為新計算值，與舊值一致） ─
INSERT INTO access_events_new
    (id, badge_id, site_id, gate_id, direction, status, reason, event_time)
SELECT id, badge_id, site_id, gate_id, direction, status, reason, event_time
FROM access_events;

-- ── 4. 索引 ────────────────────────────────────────────────
CREATE INDEX idx_events_badge_date_new
    ON access_events_new (badge_id, event_time);

CREATE INDEX idx_events_site_new
    ON access_events_new (site_id, event_time);

CREATE INDEX idx_events_status_date_new
    ON access_events_new (event_date, badge_id)
    WHERE status = 'SUCCESS';

CREATE INDEX idx_events_badge_eventdate_new
    ON access_events_new (badge_id, event_date DESC);

-- ── 5. FR-12 trigger 重掛到新 root ────────────────────────────
CREATE TRIGGER trg_protect_audit
    BEFORE UPDATE OR DELETE ON access_events_new
    FOR EACH STATEMENT
    EXECUTE FUNCTION protect_audit_log();

CREATE TRIGGER trg_protect_audit_truncate
    BEFORE TRUNCATE ON access_events_new
    FOR EACH STATEMENT
    EXECUTE FUNCTION protect_audit_log();

-- ── 6. 權限 ─────────────────────────────────────────────────
GRANT SELECT, INSERT ON access_events_new TO pacs_user;
REVOKE UPDATE, DELETE ON access_events_new FROM pacs_user;
GRANT SELECT ON access_events_new TO pacs_reporter;

-- ── 7. Rename swap + drop legacy ────────────────────────────
ALTER TABLE access_events     RENAME TO access_events_legacy;
ALTER TABLE access_events_new RENAME TO access_events;

-- 先 DROP 舊表，連帶釋放舊 index 名（避免 rename 衝突）。
-- DDL DROP 不會 fire FR-12 BEFORE DELETE / TRUNCATE trigger。
DROP TABLE access_events_legacy;

ALTER INDEX idx_events_badge_date_new      RENAME TO idx_events_badge_date;
ALTER INDEX idx_events_site_new            RENAME TO idx_events_site;
ALTER INDEX idx_events_status_date_new     RENAME TO idx_events_status_date;
ALTER INDEX idx_events_badge_eventdate_new RENAME TO idx_events_badge_eventdate;

COMMIT;
