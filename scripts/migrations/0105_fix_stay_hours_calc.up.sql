-- ============================================================
-- 0105_fix_stay_hours_calc.up.sql
--
-- 修正 mv_daily_attendance.stay_hours 計算：
--   舊版（0006）：MAX(OUT) − MIN(IN) — 同日多次進出時錯把午休時間算進去；
--   新版：IN/OUT 配對累加 + Asia/Taipei 00:00 切分，跨日 visit 分配給對應日期。
--
-- 算法：
--   1. 每個 badge 維護 inside_after counter（IN +1，OUT −1）。
--   2. counter 0→1 視為 visit 開始，1→0 視為 visit 結束（用 ROW_NUMBER 配對）。
--   3. 每段 visit 沿 Asia/Taipei 00:00 切片，分別計入對應 event_date。
--   4. 未配對的 IN / OUT（orphan）自動忽略。
--
-- 注意：
--   * 同日多次進出（午休、會議外出 IN→OUT→IN→OUT）：各區段分別累加。
--   * 跨午夜 visit (IN 23:00 → OUT 02:00)：1h 計入 Day1、2h 計入 Day2。
--   * Tier-1/Tier-2 巢狀（IN1, IN2, OUT2, OUT1）視為一次 visit（counter 從 0→2→0）。
--
-- DROP + CREATE 而不是 ALTER：PG 不支援 ALTER MV 改 SELECT，而且舊資料完全重算才正確。
-- mv-refresher 之後的 REFRESH CONCURRENTLY 仍然可運作（owner 不變、UNIQUE 索引重建）。
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
    -- counter 0→1：IN 後 inside_after = 1
    SELECT
        badge_id,
        event_time AS enter_time,
        ROW_NUMBER() OVER (PARTITION BY badge_id ORDER BY event_time) AS visit_no
    FROM events_with_counter
    WHERE direction = 'IN' AND inside_after = 1
),
visit_end AS (
    -- counter 1→0：OUT 後 inside_after = 0
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
    -- 把每段 visit 沿 Asia/Taipei midnight 切片
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
    -- first_in / last_out / swipe_count 仍按事件本身的 event_date 分組（保留原語意）
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

-- REFRESH CONCURRENTLY 需要 UNIQUE 索引。
CREATE UNIQUE INDEX idx_mv_daily_attendance_pk
    ON mv_daily_attendance (badge_id, event_date);

-- 趨勢 / 主管視野 query：按 org_path_ltree ancestor + event_date range
CREATE INDEX idx_mv_daily_attendance_org_date
    ON mv_daily_attendance USING GIST (org_path_ltree);

CREATE INDEX idx_mv_daily_attendance_event_date
    ON mv_daily_attendance (event_date);

GRANT SELECT ON mv_daily_attendance TO pacs_reporter;
