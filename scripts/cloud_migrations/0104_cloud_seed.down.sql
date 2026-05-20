-- ============================================================
-- 0104_cloud_seed.down.sql
--
-- 回滾雲端大規模播種：
--   - access_events 是 append-only 稽核資料，不可 DELETE/TRUNCATE。
--   - 因此 rollback 只停用 B-000001 ~ B-090000 的 cloud seed employees。
-- ============================================================

UPDATE employees
SET is_active = FALSE,
    updated_at = NOW()
WHERE badge_id BETWEEN 'B-000001' AND 'B-090000';

REFRESH MATERIALIZED VIEW CONCURRENTLY mv_daily_attendance;

DO $$
BEGIN
    RAISE NOTICE '0104 雲端大規模播種已回滾：cloud seed employees 已停用，access_events 依 FR-12 保留。';
END;
$$;
