-- ============================================================
-- 0003 down: 移除 ltree 相關物件
-- ltree extension 不 DROP（其他 migration 可能引用）。
-- ============================================================
DROP TRIGGER IF EXISTS trg_sync_org_path_ltree ON employees;
DROP FUNCTION IF EXISTS sync_org_path_ltree();
DROP INDEX IF EXISTS idx_employees_org_path_gist;
ALTER TABLE employees DROP COLUMN IF EXISTS org_path_ltree;
