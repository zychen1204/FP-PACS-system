-- ============================================================
-- 0099 dev_seed
-- Demo data so /v1/reports/attendance & /v1/audit have rows on
-- a fresh `docker compose up` without manual swipes.
-- All rows tagged with reason = '[DEV_SEED]' for traceability.
--
-- 注意時區：
--   * event_time 以「台北時間」語意撰寫，再 AT TIME ZONE 'Asia/Taipei'
--     轉成正確 UTC 儲存值。
--   * event_date 與「event_time 在台北時區的日期」對齊，避免 18:00 UTC
--     跨日到隔天台北的問題。
-- ============================================================

-- 30 SUCCESS rows: 5 employees × 3 days × (IN 09:00, OUT 18:00) 台北時區
INSERT INTO access_events (badge_id, site_id, gate_id, direction, status, reason, event_time, event_date)
SELECT
    e.badge_id,
    CASE WHEN e.badge_id IN ('B003','B004') THEN 'Site-B' ELSE 'Site-A' END,
    CASE WHEN e.badge_id IN ('B003','B004') THEN 'Gate-2' ELSE 'Gate-1' END,
    d.direction,
    'SUCCESS',
    '[DEV_SEED]',
    (((CURRENT_DATE - days.day_offset) + d.t) AT TIME ZONE 'Asia/Taipei')::timestamptz,
    (CURRENT_DATE - days.day_offset)
FROM (VALUES ('B001'),('B002'),('B003'),('B004'),('B005')) AS e(badge_id)
CROSS JOIN (VALUES (0),(1),(2)) AS days(day_offset)
CROSS JOIN (VALUES ('IN', TIME '09:00'), ('OUT', TIME '18:00')) AS d(direction, t);

-- 5 REJECTED_APB rows for FR-2 demo (anti-passback violations).
INSERT INTO access_events (badge_id, site_id, gate_id, direction, status, reason, event_time, event_date)
VALUES
    ('B001','Site-A','Gate-1','IN', 'REJECTED_APB','[DEV_SEED] same direction within 30s', ((CURRENT_DATE + TIME '09:01') AT TIME ZONE 'Asia/Taipei')::timestamptz, CURRENT_DATE),
    ('B002','Site-A','Gate-1','OUT','REJECTED_APB','[DEV_SEED]',                              ((CURRENT_DATE + TIME '18:01') AT TIME ZONE 'Asia/Taipei')::timestamptz, CURRENT_DATE),
    ('B999','Site-A','Gate-1','IN', 'REJECTED_APB','[DEV_SEED] unregistered badge',           ((CURRENT_DATE + TIME '02:14') AT TIME ZONE 'Asia/Taipei')::timestamptz, CURRENT_DATE),
    ('B003','Site-B','Gate-2','IN', 'REJECTED_APB','[DEV_SEED]',                              (((CURRENT_DATE - 1) + TIME '09:01') AT TIME ZONE 'Asia/Taipei')::timestamptz, CURRENT_DATE - 1),
    ('B005','Site-A','Gate-3','OUT','REJECTED_APB','[DEV_SEED]',                              (((CURRENT_DATE - 2) + TIME '18:05') AT TIME ZONE 'Asia/Taipei')::timestamptz, CURRENT_DATE - 2);

-- 10 extra cross-gate SUCCESS rows for variety in attendance reports.
INSERT INTO access_events (badge_id, site_id, gate_id, direction, status, reason, event_time, event_date)
VALUES
    ('B001','Site-A','Gate-3','IN', 'SUCCESS','[DEV_SEED]', ((CURRENT_DATE + TIME '12:30') AT TIME ZONE 'Asia/Taipei')::timestamptz, CURRENT_DATE),
    ('B001','Site-A','Gate-3','OUT','SUCCESS','[DEV_SEED]', ((CURRENT_DATE + TIME '13:30') AT TIME ZONE 'Asia/Taipei')::timestamptz, CURRENT_DATE),
    ('B002','Site-A','Gate-1','IN', 'SUCCESS','[DEV_SEED]', ((CURRENT_DATE + TIME '14:00') AT TIME ZONE 'Asia/Taipei')::timestamptz, CURRENT_DATE),
    ('B002','Site-A','Gate-1','OUT','SUCCESS','[DEV_SEED]', ((CURRENT_DATE + TIME '15:00') AT TIME ZONE 'Asia/Taipei')::timestamptz, CURRENT_DATE),
    ('B003','Site-B','Gate-1','IN', 'SUCCESS','[DEV_SEED]', (((CURRENT_DATE - 1) + TIME '13:00') AT TIME ZONE 'Asia/Taipei')::timestamptz, CURRENT_DATE - 1),
    ('B003','Site-B','Gate-1','OUT','SUCCESS','[DEV_SEED]', (((CURRENT_DATE - 1) + TIME '14:00') AT TIME ZONE 'Asia/Taipei')::timestamptz, CURRENT_DATE - 1),
    ('B004','Site-B','Gate-2','IN', 'SUCCESS','[DEV_SEED]', (((CURRENT_DATE - 1) + TIME '13:00') AT TIME ZONE 'Asia/Taipei')::timestamptz, CURRENT_DATE - 1),
    ('B004','Site-B','Gate-2','OUT','SUCCESS','[DEV_SEED]', (((CURRENT_DATE - 1) + TIME '14:00') AT TIME ZONE 'Asia/Taipei')::timestamptz, CURRENT_DATE - 1),
    ('B005','Site-A','Gate-1','IN', 'SUCCESS','[DEV_SEED]', (((CURRENT_DATE - 2) + TIME '14:00') AT TIME ZONE 'Asia/Taipei')::timestamptz, CURRENT_DATE - 2),
    ('B005','Site-A','Gate-1','OUT','SUCCESS','[DEV_SEED]', (((CURRENT_DATE - 2) + TIME '15:00') AT TIME ZONE 'Asia/Taipei')::timestamptz, CURRENT_DATE - 2);

-- Refresh updated_at on employees (no-op if column missing pre-0003).
UPDATE employees SET updated_at = NOW() WHERE updated_at IS NOT NULL;

-- Refresh MV 讓 dev_seed 資料立即可見於 mv_daily_attendance；
-- 否則需要等 mv-refresher service 第一次 tick。
REFRESH MATERIALIZED VIEW mv_daily_attendance;
