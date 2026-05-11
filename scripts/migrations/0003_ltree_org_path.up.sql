-- ============================================================
-- 0003 ltree_org_path
--
-- HW2 §5.2/§5.3 明確要求組織樹採 ltree + GiST index。本 migration:
--   * 啟用 ltree extension
--   * 在 employees 加 org_path_ltree LTREE 欄位（與既有 org_path VARCHAR 並存）
--   * 自既有 org_path 同步 ltree 值（中文 label 依靠 PG16 + C.UTF-8 locale）
--   * 建 GiST index 支援 `<@`（descendant of）/`@>`（ancestor of）查詢
--   * trigger 自動同步：未來 INSERT/UPDATE org_path 時 org_path_ltree 一起更新
--
-- 為什麼保留 org_path VARCHAR 而非直接替換：
--   1. 既有資料移植零風險
--   2. UI（frontend/app.js）直接讀 org_path 字串，不需要動前端
--   3. ltree 是 query 層加速器，VARCHAR 是 display canonical
--   未來如 Phase 3 確定 ltree 穩定，再評估 drop VARCHAR。
-- ============================================================

CREATE EXTENSION IF NOT EXISTS ltree;

-- ── Schema ────────────────────────────────────────────────────
ALTER TABLE employees
    ADD COLUMN IF NOT EXISTS org_path_ltree LTREE;

-- ── Backfill：把現有 VARCHAR 路徑轉成 ltree ─────────────────────
-- ltree 用 `.` 作 separator 與 org_path 字串格式一致，cast 即可。
UPDATE employees
   SET org_path_ltree = org_path::ltree
 WHERE org_path_ltree IS NULL;

ALTER TABLE employees
    ALTER COLUMN org_path_ltree SET NOT NULL;

-- ── GiST index 支援 <@ / @> / @ lquery ────────────────────────
CREATE INDEX IF NOT EXISTS idx_employees_org_path_gist
    ON employees USING GIST (org_path_ltree);

-- ── Trigger：未來 INSERT/UPDATE org_path 時自動同步 ltree 欄位 ─
CREATE OR REPLACE FUNCTION sync_org_path_ltree()
RETURNS TRIGGER AS $$
BEGIN
    NEW.org_path_ltree := NEW.org_path::ltree;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_sync_org_path_ltree ON employees;
CREATE TRIGGER trg_sync_org_path_ltree
    BEFORE INSERT OR UPDATE OF org_path ON employees
    FOR EACH ROW
    EXECUTE FUNCTION sync_org_path_ltree();
