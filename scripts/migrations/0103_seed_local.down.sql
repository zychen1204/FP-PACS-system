-- ============================================================
-- 0103_seed_local.down.sql
-- 回滾：移除本地播種的所有員工資料
-- ============================================================

-- 先清除相關 access_events（避免 FK 衝突）
DELETE FROM access_events
WHERE badge_id IN (
    SELECT badge_id FROM employees
    WHERE badge_id BETWEEN 'B-000001' AND 'B-001000'
);

-- 移除員工
DELETE FROM employees
WHERE badge_id BETWEEN 'B-000001' AND 'B-001000';

-- 刷新 MV
REFRESH MATERIALIZED VIEW CONCURRENTLY mv_daily_attendance;

RAISE NOTICE '0103 本地播種已回滾完畢';
