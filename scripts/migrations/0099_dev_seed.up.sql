-- ============================================================
-- 0099 dev_seed
-- Demo data so /v1/reports/attendance & /v1/audit have rows on
-- a fresh `docker compose up` without manual swipes.
-- All rows tagged with reason = '[DEV_SEED]' for traceability.
--
-- 時間軸契約：
--   * 所有事件嚴格落在 [CURRENT_DATE - 3, CURRENT_DATE - 1]（昨天到三天前）。
--   * 「今天」完全不種，由 access-api 即時 swipe 產生 → 展示 CQRS write path。
--   * 不再使用 `CURRENT_DATE + TIME ...`，避免未來時間汙染報表。
--
-- 資料字典：
--   * site_id 對齊 seed-generator + k6：FAB12A / FAB15 / FAB18A
--   * gate_id 對齊 seed-generator + k6：G-01 / G-02 / CR-01 / OFF-01
--   * badge_id 對齊 org-sync 內建主管：
--       B001 王小明  TSMC.Fab12.製造部 → FAB12A
--       B002 李大華  TSMC.Fab12.品保部 → FAB12A
--       B003 張美玲  TSMC.Fab15.研發部 → FAB15
--       B004 陳志偉  TSMC.Fab15.設備部 → FAB15
--       B005 林雅婷  TSMC.總部.人資部  → FAB18A
--
-- 時區：
--   * event_time 以「台北時間」語意撰寫，再 AT TIME ZONE 'Asia/Taipei'
--     轉成正確 UTC 儲存值。
--   * event_date 與「event_time 在台北時區的日期」對齊。
-- ============================================================

-- 30 SUCCESS rows: 5 employees × 3 days × (IN 09:00, OUT 18:00) 台北時區
-- day_offset 1/2/3 = 昨天 / 前天 / 大前天
INSERT INTO access_events (badge_id, site_id, gate_id, direction, status, reason, event_time, event_date)
SELECT
    e.badge_id,
    CASE
        WHEN e.badge_id IN ('B001','B002') THEN 'FAB12A'
        WHEN e.badge_id IN ('B003','B004') THEN 'FAB15'
        ELSE 'FAB18A'  -- B005
    END,
    CASE
        WHEN e.badge_id IN ('B001','B002') THEN 'G-01'
        WHEN e.badge_id IN ('B003','B004') THEN 'G-02'
        ELSE 'OFF-01'  -- B005
    END,
    d.direction,
    'SUCCESS',
    '[DEV_SEED]',
    (((CURRENT_DATE - days.day_offset) + d.t) AT TIME ZONE 'Asia/Taipei')::timestamptz,
    (CURRENT_DATE - days.day_offset)
FROM (VALUES ('B001'),('B002'),('B003'),('B004'),('B005')) AS e(badge_id)
CROSS JOIN (VALUES (1),(2),(3)) AS days(day_offset)
CROSS JOIN (VALUES ('IN', TIME '09:00'), ('OUT', TIME '18:00')) AS d(direction, t);

-- 5 REJECTED_APB rows for FR-2 demo (anti-passback violations).
-- 全部落在 [yesterday, 3 days ago]，避免未來時間。
INSERT INTO access_events (badge_id, site_id, gate_id, direction, status, reason, event_time, event_date)
VALUES
    ('B001','FAB12A','G-01','IN', 'REJECTED_APB','[DEV_SEED] same direction within 30s', (((CURRENT_DATE - 1) + TIME '09:01') AT TIME ZONE 'Asia/Taipei')::timestamptz, CURRENT_DATE - 1),
    ('B002','FAB12A','G-01','OUT','REJECTED_APB','[DEV_SEED]',                              (((CURRENT_DATE - 1) + TIME '18:01') AT TIME ZONE 'Asia/Taipei')::timestamptz, CURRENT_DATE - 1),
    ('B999','FAB12A','G-01','IN', 'REJECTED_APB','[DEV_SEED] unregistered badge',           (((CURRENT_DATE - 1) + TIME '02:14') AT TIME ZONE 'Asia/Taipei')::timestamptz, CURRENT_DATE - 1),
    ('B003','FAB15', 'G-02','IN', 'REJECTED_APB','[DEV_SEED]',                              (((CURRENT_DATE - 2) + TIME '09:01') AT TIME ZONE 'Asia/Taipei')::timestamptz, CURRENT_DATE - 2),
    ('B005','FAB18A','OFF-01','OUT','REJECTED_APB','[DEV_SEED]',                            (((CURRENT_DATE - 3) + TIME '18:05') AT TIME ZONE 'Asia/Taipei')::timestamptz, CURRENT_DATE - 3);

-- 10 extra cross-gate SUCCESS rows for variety in attendance reports.
-- 全部落在 [yesterday, 3 days ago]，原本「今天 12:30/13:30/14:00/15:00」改為昨天。
INSERT INTO access_events (badge_id, site_id, gate_id, direction, status, reason, event_time, event_date)
VALUES
    ('B001','FAB12A','CR-01','IN', 'SUCCESS','[DEV_SEED]', (((CURRENT_DATE - 1) + TIME '12:30') AT TIME ZONE 'Asia/Taipei')::timestamptz, CURRENT_DATE - 1),
    ('B001','FAB12A','CR-01','OUT','SUCCESS','[DEV_SEED]', (((CURRENT_DATE - 1) + TIME '13:30') AT TIME ZONE 'Asia/Taipei')::timestamptz, CURRENT_DATE - 1),
    ('B002','FAB12A','G-02', 'IN', 'SUCCESS','[DEV_SEED]', (((CURRENT_DATE - 1) + TIME '14:00') AT TIME ZONE 'Asia/Taipei')::timestamptz, CURRENT_DATE - 1),
    ('B002','FAB12A','G-02', 'OUT','SUCCESS','[DEV_SEED]', (((CURRENT_DATE - 1) + TIME '15:00') AT TIME ZONE 'Asia/Taipei')::timestamptz, CURRENT_DATE - 1),
    ('B003','FAB15', 'G-01', 'IN', 'SUCCESS','[DEV_SEED]', (((CURRENT_DATE - 2) + TIME '13:00') AT TIME ZONE 'Asia/Taipei')::timestamptz, CURRENT_DATE - 2),
    ('B003','FAB15', 'G-01', 'OUT','SUCCESS','[DEV_SEED]', (((CURRENT_DATE - 2) + TIME '14:00') AT TIME ZONE 'Asia/Taipei')::timestamptz, CURRENT_DATE - 2),
    ('B004','FAB15', 'G-02', 'IN', 'SUCCESS','[DEV_SEED]', (((CURRENT_DATE - 2) + TIME '13:00') AT TIME ZONE 'Asia/Taipei')::timestamptz, CURRENT_DATE - 2),
    ('B004','FAB15', 'G-02', 'OUT','SUCCESS','[DEV_SEED]', (((CURRENT_DATE - 2) + TIME '14:00') AT TIME ZONE 'Asia/Taipei')::timestamptz, CURRENT_DATE - 2),
    ('B005','FAB18A','OFF-01','IN', 'SUCCESS','[DEV_SEED]', (((CURRENT_DATE - 3) + TIME '14:00') AT TIME ZONE 'Asia/Taipei')::timestamptz, CURRENT_DATE - 3),
    ('B005','FAB18A','OFF-01','OUT','SUCCESS','[DEV_SEED]', (((CURRENT_DATE - 3) + TIME '15:00') AT TIME ZONE 'Asia/Taipei')::timestamptz, CURRENT_DATE - 3);

-- Refresh updated_at on employees (no-op if column missing pre-0003).
UPDATE employees SET updated_at = NOW() WHERE updated_at IS NOT NULL;

-- Refresh MV 讓 dev_seed 資料立即可見於 mv_daily_attendance；
-- 否則需要等 mv-refresher service 第一次 tick。
REFRESH MATERIALIZED VIEW mv_daily_attendance;
