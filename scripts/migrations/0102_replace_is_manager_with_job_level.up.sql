-- ============================================================
-- 0102 replace_is_manager_with_job_level
--
-- Replace the binary `is_manager BOOLEAN` flag with a multi-tier
-- `job_level VARCHAR(20)` column so 一級主管 (e.g. 廠長) and
-- 二級主管 (e.g. 部主管) can be distinguished at the data layer.
--
-- Permission semantics are UNCHANGED: FR-6/FR-9 still scope a
-- manager's view to their own `org_path_ltree` subtree via the
-- ltree `<@` operator. `job_level` is an identity label, not a
-- visibility decision factor.
--
-- 不另建 index：低選擇性，且 GetManagerScope 已走 PK (badge_id)。
-- See docs/database-erd.md §3.4 for the rationale and query pattern.
-- ============================================================

-- 1) 新增 job_level，預設 STAFF，CHECK 限定值集合
ALTER TABLE employees
    ADD COLUMN job_level VARCHAR(20) NOT NULL DEFAULT 'STAFF'
    CONSTRAINT employees_job_level_check
    CHECK (job_level IN ('STAFF', 'MANAGER_L1', 'MANAGER_L2'));

-- 2) 回填：原 is_manager=TRUE 的主管依職等重新分派
--    B100 黃廠長        → MANAGER_L1（一級主管 / 廠長）
--    B001~B005 各部主管 → MANAGER_L2（二級主管 / 部主管）
UPDATE employees SET job_level = 'MANAGER_L1' WHERE badge_id = 'B100';
UPDATE employees SET job_level = 'MANAGER_L2'
WHERE badge_id IN ('B001', 'B002', 'B003', 'B004', 'B005');

-- 3) 刪除舊欄位
ALTER TABLE employees DROP COLUMN is_manager;
