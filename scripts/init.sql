-- ============================================
-- PACS Database Schema
-- Immutable Audit Log + Employee Registry
-- ============================================

-- Access Events (Append-Only Audit Log)
CREATE TABLE IF NOT EXISTS access_events (
    id          BIGSERIAL PRIMARY KEY,
    badge_id    VARCHAR(50)  NOT NULL,
    site_id     VARCHAR(50)  NOT NULL,
    gate_id     VARCHAR(50)  NOT NULL,
    direction   VARCHAR(10)  NOT NULL CHECK (direction IN ('IN', 'OUT')),
    status      VARCHAR(20)  NOT NULL,
    reason      TEXT         DEFAULT '',
    event_time  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Indexes for reporting queries
CREATE INDEX idx_events_badge_date ON access_events (badge_id, event_time);
CREATE INDEX idx_events_status     ON access_events (status);
CREATE INDEX idx_events_site       ON access_events (site_id, event_time);

-- Employees table
CREATE TABLE IF NOT EXISTS employees (
    badge_id   VARCHAR(50) PRIMARY KEY,
    name       VARCHAR(100) NOT NULL,
    org_path   VARCHAR(255) NOT NULL DEFAULT 'TSMC'
);

-- Seed sample employees
INSERT INTO employees (badge_id, name, org_path) VALUES
    ('B001', '王小明', 'TSMC.Fab12.製造部'),
    ('B002', '李大華', 'TSMC.Fab12.品保部'),
    ('B003', '張美玲', 'TSMC.Fab15.研發部'),
    ('B004', '陳志偉', 'TSMC.Fab15.設備部'),
    ('B005', '林雅婷', 'TSMC.總部.人資部')
ON CONFLICT (badge_id) DO NOTHING;

-- ============================================
-- IMMUTABLE AUDIT: Enforce no UPDATE/DELETE via Trigger
-- This ensures FR12 compliance even for the table owner
-- ============================================
CREATE OR REPLACE FUNCTION protect_audit_log()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'Updates and deletes are not allowed on the access_events table (FR12 compliance)';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_protect_audit
BEFORE UPDATE OR DELETE ON access_events
FOR EACH STATEMENT
EXECUTE FUNCTION protect_audit_log();

REVOKE UPDATE, DELETE ON access_events FROM pacs_user;

