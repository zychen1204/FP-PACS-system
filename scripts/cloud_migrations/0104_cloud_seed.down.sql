-- ============================================================
-- 0104_cloud_seed.down.sql
-- 回滾：移除雲端大規模播種的所有員工資料
-- ============================================================

-- 先清除相關 access_events
DELETE FROM access_events
WHERE badge_id IN (
    SELECT badge_id FROM employees
    WHERE badge_id BETWEEN 'B-000001' AND 'B-090000'
);

-- 移除員工
DELETE FROM employees
WHERE badge_id BETWEEN 'B-000001' AND 'B-090000';

-- 刷新 MV
REFRESH MATERIALIZED VIEW CONCURRENTLY mv_daily_attendance;

RAISE NOTICE '0104 雲端大規模播種已回滾完畢';
