-- ============================================================
-- 0105 down — 還原為 0006 baseline 的 head-tail 計算
-- ============================================================

DROP MATERIALIZED VIEW IF EXISTS mv_daily_attendance CASCADE;

CREATE MATERIALIZED VIEW mv_daily_attendance AS
SELECT
    e.badge_id,
    e.event_date,
    COALESCE(emp.name,     'Employee ' || e.badge_id) AS name,
    COALESCE(emp.org_path, 'Unknown')                 AS org_path,
    emp.org_path_ltree                                AS org_path_ltree,
    MIN(e.event_time) FILTER (WHERE e.direction = 'IN')  AS first_in,
    MAX(e.event_time) FILTER (WHERE e.direction = 'OUT') AS last_out,
    COUNT(*)                                          AS swipe_count,
    EXTRACT(EPOCH FROM (
        MAX(e.event_time) FILTER (WHERE e.direction = 'OUT')
      - MIN(e.event_time) FILTER (WHERE e.direction = 'IN')
    )) / 3600.0                                       AS stay_hours
FROM access_events e
LEFT JOIN employees emp ON emp.badge_id = e.badge_id
WHERE e.status = 'SUCCESS'
GROUP BY e.badge_id, e.event_date, emp.name, emp.org_path, emp.org_path_ltree
WITH DATA;

CREATE UNIQUE INDEX idx_mv_daily_attendance_pk
    ON mv_daily_attendance (badge_id, event_date);

CREATE INDEX idx_mv_daily_attendance_org_date
    ON mv_daily_attendance USING GIST (org_path_ltree);

CREATE INDEX idx_mv_daily_attendance_event_date
    ON mv_daily_attendance (event_date);

GRANT SELECT ON mv_daily_attendance TO pacs_reporter;
