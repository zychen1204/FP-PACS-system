-- ============================================================
-- 0002 add_manager_flag
--
-- Closes the FR-6 (hierarchical team report) and FR-9 (hierarchical
-- data permission) DB-side gap. The schema before this migration could
-- describe which department an employee belongs to (employees.org_path)
-- but had no way to identify managers or their scope.
--
-- Approach: a single is_manager BOOLEAN flag. A manager's scope is
-- defined implicitly as "all rows whose org_path equals or extends
-- this manager's org_path", queried via B-tree LIKE prefix.
--
-- See docs/database-spec.md §FR-6/FR-9 and database-compliance.md
-- "Schema gap closure" for the design rationale (we evaluated
-- adjacency list per HW2 §5.2 and chose path enumeration as the
-- simpler fit for Phase 1).
-- ============================================================

-- ── Schema: one BOOLEAN flag, low-selectivity so no index. ────
ALTER TABLE employees
    ADD COLUMN IF NOT EXISTS is_manager BOOLEAN NOT NULL DEFAULT FALSE;

-- ── Seed: add a 廠長 + 2 部員 so subtree queries have meaningful
--    range (without these, every existing employee is alone in its
--    org_path and "scope" demos return only the manager themselves).
INSERT INTO employees (badge_id, name, org_path) VALUES
    ('B100', '黃廠長', 'TSMC.Fab12'),            -- Fab12 廠長: scope 'TSMC.Fab12'
    ('B011', '林員工', 'TSMC.Fab12.製造部'),     -- 製造部 部員 #1
    ('B012', '趙員工', 'TSMC.Fab12.製造部')      -- 製造部 部員 #2
ON CONFLICT (badge_id) DO NOTHING;

-- ── Mark managers (idempotent UPDATE):
--    B100 = 廠長 (scope = whole Fab12)
--    B001~B005 = 各部主管 (scope = own dept)
UPDATE employees SET is_manager = TRUE
WHERE badge_id IN ('B100', 'B001', 'B002', 'B003', 'B004', 'B005');
