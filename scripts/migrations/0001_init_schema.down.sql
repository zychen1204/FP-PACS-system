-- ============================================================
-- 0001 init_schema (down)
-- Reverse order of the up file: role grants → triggers → tables → extension.
-- ============================================================

-- Drop the read-only role first so its grants don't dangle.
REVOKE ALL    ON access_events    FROM pacs_reporter;
REVOKE ALL    ON employees        FROM pacs_reporter;
REVOKE USAGE  ON SCHEMA public    FROM pacs_reporter;
REVOKE CONNECT ON DATABASE pacs_db FROM pacs_reporter;
DROP ROLE IF EXISTS pacs_reporter;

-- Triggers (drop with table is fine, but be explicit for clarity).
DROP TRIGGER IF EXISTS trg_protect_audit_truncate ON access_events;
DROP TRIGGER IF EXISTS trg_protect_audit          ON access_events;
DROP FUNCTION IF EXISTS protect_audit_log();

-- Indexes drop with their tables.
DROP TABLE IF EXISTS access_events;
DROP TABLE IF EXISTS employees;

DROP EXTENSION IF EXISTS pg_stat_statements;
