-- ============================================================
-- 0001 init_schema (consolidated baseline)
--
-- Single migration for the PACS schema:
--   * append-only audit log (access_events) with FR-12 immutability
--     enforced at BOTH role-privilege and trigger level
--   * employee registry with audit columns
--   * tuned indexes for NFR-2 (reporting P95 < 200 ms)
--   * least-privilege read-only role for reporting-api
--
-- pacs_user is provisioned by the postgres image (POSTGRES_USER env);
-- we never CREATE ROLE pacs_user, only adjust its privileges.
--
-- For a description of why each block exists (FR / NFR mapping),
-- see docs/database-spec.md and docs/database-erd.md.
-- ============================================================

-- ── Extensions ────────────────────────────────────────────────
-- Requires shared_preload_libraries=pg_stat_statements (set in compose).
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

-- ── Tables ────────────────────────────────────────────────────

-- Append-only audit log. event_date is a STORED generated column in
-- Asia/Taipei, so any (event_date, ...) index is directly usable by
-- queries that filter on local-day instead of UTC instant.
CREATE TABLE IF NOT EXISTS access_events (
    id          BIGSERIAL    PRIMARY KEY,
    badge_id    VARCHAR(50)  NOT NULL,
    site_id     VARCHAR(50)  NOT NULL,
    gate_id     VARCHAR(50)  NOT NULL,
    direction   VARCHAR(10)  NOT NULL CHECK (direction IN ('IN', 'OUT')),
    status      VARCHAR(20)  NOT NULL,
    reason      TEXT         DEFAULT '',
    event_time  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    event_date  DATE         GENERATED ALWAYS AS
                ((event_time AT TIME ZONE 'Asia/Taipei')::date) STORED
);
-- No FK to employees(badge_id) on purpose: an unregistered badge swipe
-- (e.g. a stranger at gate-3 at 02:14) is itself a security-relevant
-- audit record and must be persisted, not rejected.

CREATE TABLE IF NOT EXISTS employees (
    badge_id    VARCHAR(50)  PRIMARY KEY,
    name        VARCHAR(100) NOT NULL,
    org_path    VARCHAR(255) NOT NULL DEFAULT 'TSMC',
    is_active   BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- ── Indexes ───────────────────────────────────────────────────

-- General-purpose: badge × event_time, used by ORDER BY event_time DESC LIMIT N
-- (audit trail with explicit time ordering).
CREATE INDEX IF NOT EXISTS idx_events_badge_date
    ON access_events (badge_id, event_time);

-- Site-scoped time scans (e.g. per-Fab dashboards).
CREATE INDEX IF NOT EXISTS idx_events_site
    ON access_events (site_id, event_time);

-- attendance: filters on event_date AND status='SUCCESS', groups by badge.
-- Partial index keeps the footprint small (only successful swipes are in
-- attendance) and makes it the cheapest plan for the report query.
CREATE INDEX IF NOT EXISTS idx_events_status_date
    ON access_events (event_date, badge_id)
    WHERE status = 'SUCCESS';

-- audit_trail by date range: WHERE badge_id=$1 AND event_date BETWEEN $2 AND $3.
-- Reserved for queries that filter purely on event_date (no event_time order).
CREATE INDEX IF NOT EXISTS idx_events_badge_eventdate
    ON access_events (badge_id, event_date DESC);

-- ── FR-12 immutability: function + triggers ───────────────────

CREATE OR REPLACE FUNCTION protect_audit_log()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'Updates and deletes are not allowed on the access_events table (FR-12 compliance)';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_protect_audit ON access_events;
CREATE TRIGGER trg_protect_audit
    BEFORE UPDATE OR DELETE ON access_events
    FOR EACH STATEMENT
    EXECUTE FUNCTION protect_audit_log();

-- Row-level triggers don't fire on TRUNCATE; statement-level trigger closes
-- that bypass.
DROP TRIGGER IF EXISTS trg_protect_audit_truncate ON access_events;
CREATE TRIGGER trg_protect_audit_truncate
    BEFORE TRUNCATE ON access_events
    FOR EACH STATEMENT
    EXECUTE FUNCTION protect_audit_log();

-- Privilege layer (defence in depth alongside the triggers).
REVOKE UPDATE, DELETE ON access_events FROM pacs_user;

-- ── Read-only role for reporting-api ──────────────────────────

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'pacs_reporter') THEN
        CREATE ROLE pacs_reporter WITH LOGIN PASSWORD 'reporter_password';
    END IF;
END
$$;

GRANT CONNECT ON DATABASE pacs_db TO pacs_reporter;
GRANT USAGE  ON SCHEMA public    TO pacs_reporter;
GRANT SELECT ON access_events    TO pacs_reporter;
GRANT SELECT ON employees        TO pacs_reporter;

-- ── Seed (idempotent) ─────────────────────────────────────────

INSERT INTO employees (badge_id, name, org_path) VALUES
    ('B001', '王小明', 'TSMC.Fab12.製造部'),
    ('B002', '李大華', 'TSMC.Fab12.品保部'),
    ('B003', '張美玲', 'TSMC.Fab15.研發部'),
    ('B004', '陳志偉', 'TSMC.Fab15.設備部'),
    ('B005', '林雅婷', 'TSMC.總部.人資部')
ON CONFLICT (badge_id) DO NOTHING;
