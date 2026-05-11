-- ============================================================
-- 0005 down: 把 partitioned access_events 還原成單表
-- FR-12 immutability：不能 DELETE 已存事件，所以 down 同樣靠
-- INSERT INTO new SELECT 搬資料 + RENAME swap。
-- event_date 為普通 DATE 欄位（由呼叫端顯式提供）。
-- ============================================================

BEGIN;

CREATE TABLE access_events_unpart (
    id          BIGINT       NOT NULL DEFAULT nextval('access_events_id_seq'),
    badge_id    VARCHAR(50)  NOT NULL,
    site_id     VARCHAR(50)  NOT NULL,
    gate_id     VARCHAR(50)  NOT NULL,
    direction   VARCHAR(10)  NOT NULL CHECK (direction IN ('IN', 'OUT')),
    status      VARCHAR(20)  NOT NULL,
    reason      TEXT         DEFAULT '',
    event_time  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    event_date  DATE         NOT NULL,
    PRIMARY KEY (id)
);

ALTER SEQUENCE access_events_id_seq OWNED BY access_events_unpart.id;

INSERT INTO access_events_unpart
    (id, badge_id, site_id, gate_id, direction, status, reason, event_time, event_date)
SELECT id, badge_id, site_id, gate_id, direction, status, reason, event_time, event_date
FROM access_events;

CREATE INDEX idx_events_badge_date_unpart       ON access_events_unpart (badge_id, event_time);
CREATE INDEX idx_events_site_unpart             ON access_events_unpart (site_id, event_time);
CREATE INDEX idx_events_status_date_unpart      ON access_events_unpart (event_date, badge_id) WHERE status = 'SUCCESS';
CREATE INDEX idx_events_badge_eventdate_unpart  ON access_events_unpart (badge_id, event_date DESC);

CREATE TRIGGER trg_protect_audit
    BEFORE UPDATE OR DELETE ON access_events_unpart
    FOR EACH STATEMENT
    EXECUTE FUNCTION protect_audit_log();
CREATE TRIGGER trg_protect_audit_truncate
    BEFORE TRUNCATE ON access_events_unpart
    FOR EACH STATEMENT
    EXECUTE FUNCTION protect_audit_log();

GRANT SELECT, INSERT ON access_events_unpart TO pacs_user;
REVOKE UPDATE, DELETE ON access_events_unpart FROM pacs_user;
GRANT SELECT ON access_events_unpart TO pacs_reporter;

ALTER TABLE access_events        RENAME TO access_events_partitioned;
ALTER TABLE access_events_unpart RENAME TO access_events;

ALTER INDEX idx_events_badge_date_unpart      RENAME TO idx_events_badge_date;
ALTER INDEX idx_events_site_unpart            RENAME TO idx_events_site;
ALTER INDEX idx_events_status_date_unpart     RENAME TO idx_events_status_date;
ALTER INDEX idx_events_badge_eventdate_unpart RENAME TO idx_events_badge_eventdate;

DROP TABLE access_events_partitioned;

COMMIT;
