-- ============================================================
-- 0005 mv_daily_attendance
--
-- FR-7：人力出勤趨勢報表（依日/週/月/季維度）。HW2 §5.3 列為 Phase 2
--       升級項：reporting-api 不再每次 GROUP BY，改讀 5 min refresh 的 MV。
--
-- MV 物件：
--   * mv_daily_attendance — 每員工每日聚合（first_in / last_out / swipe_count /
--     stay_hours）。週/月/季維度由 reporting-api 在 MV 上再聚合（GROUP BY
--     date_trunc('week', event_date) 等），避免多份 MV 重複同樣資料。
--   * UNIQUE INDEX 是 `REFRESH MATERIALIZED VIEW CONCURRENTLY` 的硬需求。
--
-- Refresh 機制：獨立 mv-refresher service 跑 ticker 5min `REFRESH MV CONCURRENTLY`
-- （見 backend/cmd/mv-refresher）。
-- ============================================================

CREATE MATERIALIZED VIEW IF NOT EXISTS mv_daily_attendance AS
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

-- REFRESH CONCURRENTLY 需要 UNIQUE 索引。
CREATE UNIQUE INDEX IF NOT EXISTS idx_mv_daily_attendance_pk
    ON mv_daily_attendance (badge_id, event_date);

-- 趨勢 query：按 org_path_ltree ancestor + event_date range
CREATE INDEX IF NOT EXISTS idx_mv_daily_attendance_org_date
    ON mv_daily_attendance USING GIST (org_path_ltree);

CREATE INDEX IF NOT EXISTS idx_mv_daily_attendance_event_date
    ON mv_daily_attendance (event_date);

GRANT SELECT ON mv_daily_attendance TO pacs_reporter;
-- mv-refresher 用 pacs_user 連線去 REFRESH（需要 owner 或 SELECT 權限即可）。
-- 因 MV 由 pacs_user 建立（migrate 跑時是 pacs_user），擁有者就是 pacs_user，無需另外 GRANT。
