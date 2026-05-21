-- ============================================================
-- 0105 fix stay_hours aggregation
--
-- 起因：spec FR-5「員工查看每日刷卡時間、門號、方向，以及系統計算的
--       『當日在廠停留時數』」。0006 baseline 用 `last_out - first_in` 頭尾相減，
--       午餐外出 1h 也會被算進廠內（8:00 IN / 12:00 OUT / 13:00 IN / 18:00 OUT = 10h，
--       語意應為 9h）。
--
-- 改動：重定義 mv_daily_attendance，用 LAG window function 配對 IN→OUT
--       時段並累加。其它欄位 (first_in / last_out / swipe_count / name / org_path)
--       保留原語意；只有 stay_hours 計算方式變更。
--
-- 安全性：DROP + CREATE 第一次 REFRESH 非 CONCURRENTLY 會短暫 lock，
--         但 mv_daily_attendance 無下游 view，CASCADE 安全。
--         索引與權限完整重建以對齊 0006。
-- ============================================================

DROP MATERIALIZED VIEW IF EXISTS mv_daily_attendance CASCADE;

CREATE MATERIALIZED VIEW mv_daily_attendance AS
WITH ordered AS (
    SELECT
        e.badge_id,
        e.event_date,
        e.event_time,
        e.direction,
        LAG(e.event_time) OVER (
            PARTITION BY e.badge_id, e.event_date
            ORDER BY e.event_time
        ) AS prev_time,
        LAG(e.direction) OVER (
            PARTITION BY e.badge_id, e.event_date
            ORDER BY e.event_time
        ) AS prev_dir
    FROM access_events e
    WHERE e.status = 'SUCCESS'
),
pairs AS (
    -- 取所有 IN→OUT 配對的時段（午餐外出時段 OUT→IN 不算）
    SELECT
        badge_id,
        event_date,
        EXTRACT(EPOCH FROM (event_time - prev_time)) / 3600.0 AS seg_hours
    FROM ordered
    WHERE prev_dir = 'IN' AND direction = 'OUT'
)
SELECT
    e.badge_id,
    e.event_date,
    COALESCE(emp.name,     'Employee ' || e.badge_id) AS name,
    COALESCE(emp.org_path, 'Unknown')                 AS org_path,
    emp.org_path_ltree                                AS org_path_ltree,
    MIN(e.event_time) FILTER (WHERE e.direction = 'IN')  AS first_in,
    MAX(e.event_time) FILTER (WHERE e.direction = 'OUT') AS last_out,
    COUNT(*)                                          AS swipe_count,
    COALESCE((
        SELECT SUM(p.seg_hours)
        FROM pairs p
        WHERE p.badge_id = e.badge_id AND p.event_date = e.event_date
    ), 0)::float8 AS stay_hours
FROM access_events e
LEFT JOIN employees emp ON emp.badge_id = e.badge_id
WHERE e.status = 'SUCCESS'
GROUP BY e.badge_id, e.event_date, emp.name, emp.org_path, emp.org_path_ltree
WITH DATA;

-- REFRESH CONCURRENTLY 需要 UNIQUE 索引（與 0006 一致）
CREATE UNIQUE INDEX idx_mv_daily_attendance_pk
    ON mv_daily_attendance (badge_id, event_date);

-- 趨勢 query：按 org_path_ltree ancestor
CREATE INDEX idx_mv_daily_attendance_org_date
    ON mv_daily_attendance USING GIST (org_path_ltree);

CREATE INDEX idx_mv_daily_attendance_event_date
    ON mv_daily_attendance (event_date);

GRANT SELECT ON mv_daily_attendance TO pacs_reporter;
