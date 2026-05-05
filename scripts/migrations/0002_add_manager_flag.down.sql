-- ============================================================
-- 0002 add_manager_flag (down)
--
-- Full reverse: remove the 3 demo employees added by 0002 up,
-- then drop the is_manager column.
-- ============================================================

DELETE FROM employees WHERE badge_id IN ('B100', 'B011', 'B012');

ALTER TABLE employees DROP COLUMN IF EXISTS is_manager;
