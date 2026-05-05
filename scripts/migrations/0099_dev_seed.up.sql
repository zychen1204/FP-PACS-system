-- ============================================================
-- 0099 dev_seed
-- Demo data so /v1/reports/attendance & /v1/audit have rows on
-- a fresh `docker compose up` without manual swipes.
-- All rows tagged with reason = '[DEV_SEED]' for traceability.
-- ============================================================

-- 30 SUCCESS rows: 5 employees × 3 days × (IN 09:00, OUT 18:00).
INSERT INTO access_events (badge_id, site_id, gate_id, direction, status, reason, event_time)
SELECT
    e.badge_id,
    CASE WHEN e.badge_id IN ('B003','B004') THEN 'Site-B' ELSE 'Site-A' END,
    CASE WHEN e.badge_id IN ('B003','B004') THEN 'Gate-2' ELSE 'Gate-1' END,
    d.direction,
    'SUCCESS',
    '[DEV_SEED]',
    ((CURRENT_DATE - days.day_offset) + d.t)::timestamptz
FROM (VALUES ('B001'),('B002'),('B003'),('B004'),('B005')) AS e(badge_id)
CROSS JOIN (VALUES (0),(1),(2)) AS days(day_offset)
CROSS JOIN (VALUES ('IN', TIME '09:00'), ('OUT', TIME '18:00')) AS d(direction, t);

-- 5 REJECTED_APB rows for FR-2 demo (anti-passback violations).
INSERT INTO access_events (badge_id, site_id, gate_id, direction, status, reason, event_time)
VALUES
    ('B001','Site-A','Gate-1','IN', 'REJECTED_APB','[DEV_SEED] same direction within 30s', (CURRENT_DATE + TIME '09:01')::timestamptz),
    ('B002','Site-A','Gate-1','OUT','REJECTED_APB','[DEV_SEED]',                              (CURRENT_DATE + TIME '18:01')::timestamptz),
    ('B999','Site-A','Gate-1','IN', 'REJECTED_APB','[DEV_SEED] unregistered badge',           (CURRENT_DATE + TIME '02:14')::timestamptz),
    ('B003','Site-B','Gate-2','IN', 'REJECTED_APB','[DEV_SEED]',                              ((CURRENT_DATE - 1) + TIME '09:01')::timestamptz),
    ('B005','Site-A','Gate-3','OUT','REJECTED_APB','[DEV_SEED]',                              ((CURRENT_DATE - 2) + TIME '18:05')::timestamptz);

-- 10 extra cross-gate SUCCESS rows for variety in attendance reports.
INSERT INTO access_events (badge_id, site_id, gate_id, direction, status, reason, event_time)
VALUES
    ('B001','Site-A','Gate-3','IN', 'SUCCESS','[DEV_SEED]', (CURRENT_DATE + TIME '12:30')::timestamptz),
    ('B001','Site-A','Gate-3','OUT','SUCCESS','[DEV_SEED]', (CURRENT_DATE + TIME '13:30')::timestamptz),
    ('B002','Site-A','Gate-1','IN', 'SUCCESS','[DEV_SEED]', (CURRENT_DATE + TIME '14:00')::timestamptz),
    ('B002','Site-A','Gate-1','OUT','SUCCESS','[DEV_SEED]', (CURRENT_DATE + TIME '15:00')::timestamptz),
    ('B003','Site-B','Gate-1','IN', 'SUCCESS','[DEV_SEED]', ((CURRENT_DATE - 1) + TIME '13:00')::timestamptz),
    ('B003','Site-B','Gate-1','OUT','SUCCESS','[DEV_SEED]', ((CURRENT_DATE - 1) + TIME '14:00')::timestamptz),
    ('B004','Site-B','Gate-2','IN', 'SUCCESS','[DEV_SEED]', ((CURRENT_DATE - 1) + TIME '13:00')::timestamptz),
    ('B004','Site-B','Gate-2','OUT','SUCCESS','[DEV_SEED]', ((CURRENT_DATE - 1) + TIME '14:00')::timestamptz),
    ('B005','Site-A','Gate-1','IN', 'SUCCESS','[DEV_SEED]', ((CURRENT_DATE - 2) + TIME '14:00')::timestamptz),
    ('B005','Site-A','Gate-1','OUT','SUCCESS','[DEV_SEED]', ((CURRENT_DATE - 2) + TIME '15:00')::timestamptz);

-- Refresh updated_at on employees (no-op if column missing pre-0003).
UPDATE employees SET updated_at = NOW() WHERE updated_at IS NOT NULL;
