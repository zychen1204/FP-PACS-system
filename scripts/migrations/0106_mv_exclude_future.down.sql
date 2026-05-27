-- ============================================================
-- 0106_mv_exclude_future.down.sql
-- 回退到 0105 的 MV 結構（不含 event_time <= NOW() guard）。
-- 完全重建 0105 的版本以保證 down 後狀態正確。
-- ============================================================

DROP MATERIALIZED VIEW IF EXISTS mv_daily_attendance;

CREATE MATERIALIZED VIEW mv_daily_attendance AS
WITH events_with_counter AS (
    SELECT
        e.badge_id,
        e.event_time,
        e.direction,
        SUM(CASE WHEN e.direction = 'IN' THEN 1 ELSE -1 END)
            OVER (PARTITION BY e.badge_id ORDER BY e.event_time
                  ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) AS inside_after
    FROM access_events e
    WHERE e.status = 'SUCCESS'
),
visit_start AS (
    SELECT
        badge_id,
        event_time AS enter_time,
        ROW_NUMBER() OVER (PARTITION BY badge_id ORDER BY event_time) AS visit_no
    FROM events_with_counter
    WHERE direction = 'IN' AND inside_after = 1
),
visit_end AS (
    SELECT
        badge_id,
        event_time AS exit_time,
        ROW_NUMBER() OVER (PARTITION BY badge_id ORDER BY event_time) AS visit_no
    FROM events_with_counter
    WHERE direction = 'OUT' AND inside_after = 0
),
visits AS (
    SELECT s.badge_id, s.enter_time, e.exit_time
    FROM visit_start s
    JOIN visit_end e USING (badge_id, visit_no)
),
sliced AS (
    SELECT
        v.badge_id,
        v.enter_time,
        v.exit_time,
        day_start,
        GREATEST(v.enter_time, day_start)                          AS slice_start,
        LEAST(v.exit_time, day_start + INTERVAL '1 day')           AS slice_end
    FROM visits v
    CROSS JOIN LATERAL (
        SELECT generate_series(
            date_trunc('day', v.enter_time AT TIME ZONE 'Asia/Taipei')
                AT TIME ZONE 'Asia/Taipei',
            date_trunc('day', v.exit_time  AT TIME ZONE 'Asia/Taipei')
                AT TIME ZONE 'Asia/Taipei',
            '1 day'::interval
        ) AS day_start
    ) d
),
stay_per_day AS (
    SELECT
        badge_id,
        (day_start AT TIME ZONE 'Asia/Taipei')::date AS event_date,
        SUM(EXTRACT(EPOCH FROM (slice_end - slice_start))) / 3600.0 AS stay_hours
    FROM sliced
    WHERE slice_end > slice_start
    GROUP BY badge_id, (day_start AT TIME ZONE 'Asia/Taipei')::date
),
per_day_summary AS (
    SELECT
        e.badge_id,
        e.event_date,
        MIN(e.event_time) FILTER (WHERE e.direction = 'IN')  AS first_in,
        MAX(e.event_time) FILTER (WHERE e.direction = 'OUT') AS last_out,
        COUNT(*)                                             AS swipe_count
    FROM access_events e
    WHERE e.status = 'SUCCESS'
    GROUP BY e.badge_id, e.event_date
)
SELECT
    p.badge_id,
    p.event_date,
    COALESCE(emp.name,     'Employee ' || p.badge_id) AS name,
    COALESCE(emp.org_path, 'Unknown')                 AS org_path,
    emp.org_path_ltree                                AS org_path_ltree,
    p.first_in,
    p.last_out,
    p.swipe_count,
    COALESCE(s.stay_hours, 0.0)                       AS stay_hours
FROM per_day_summary p
LEFT JOIN stay_per_day s USING (badge_id, event_date)
LEFT JOIN employees emp ON emp.badge_id = p.badge_id
WITH DATA;

CREATE UNIQUE INDEX idx_mv_daily_attendance_pk
    ON mv_daily_attendance (badge_id, event_date);

CREATE INDEX idx_mv_daily_attendance_org_date
    ON mv_daily_attendance USING GIST (org_path_ltree);

CREATE INDEX idx_mv_daily_attendance_event_date
    ON mv_daily_attendance (event_date);

GRANT SELECT ON mv_daily_attendance TO pacs_reporter;
