-- ============================================================
-- 0102 replace_is_manager_with_job_level (down)
--
-- Reverse: restore the is_manager BOOLEAN column, derive its
-- value from job_level (any non-STAFF → TRUE), then drop the
-- new column + constraint.
-- ============================================================

ALTER TABLE employees
    ADD COLUMN is_manager BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE employees SET is_manager = TRUE WHERE job_level <> 'STAFF';

ALTER TABLE employees DROP CONSTRAINT IF EXISTS employees_job_level_check;
ALTER TABLE employees DROP COLUMN IF EXISTS job_level;
