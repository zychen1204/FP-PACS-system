-- ============================================================
-- 0103_seed_local.up.sql
--
-- 本地開發 / 本地壓力測試用資料播種
-- 規模：1,000 人
--   - 1   廠長        (MANAGER_L1) : B-000001
--   - 10  部門經理    (MANAGER_L2) : B-000002 ~ B-000011
--   - 989 員工        (STAFF)      : B-000012 ~ B-001000
--
-- 組織樹：
--   TSMC
--   └─ 製造部_01 ~ 製造部_10  (10 個部門)
--      └─ (每部包含 1 位經理與約 99 位員工)
--
-- 用途：
--   docker-compose 自動執行 (migrate up)
--   防止本地端因資料量過大而過載
--
-- 雲端大規模播種請改用：
--   0104_cloud_seed.up.sql  (90,000 人，手動執行)
-- ============================================================

-- ── Phase 0: 確保環境（不強制清空） ────────────────────────
SET session_replication_role = 'replica';
-- 原本的 TRUNCATE 已移除，現在支援增量播種 (ON CONFLICT DO NOTHING)

-- ── Phase 1: 廠長 L1 ────────────────────────────────────────
INSERT INTO employees (badge_id, name, job_level, org_path, org_path_ltree, is_active)
VALUES (
    'B-000001',
    '廠長_總管',
    'MANAGER_L1',
    'TSMC',
    text2ltree('TSMC'),
    TRUE
) ON CONFLICT (badge_id) DO NOTHING;

-- ── Phase 2: 10 部門經理 L2 ─────────────────────────────────
INSERT INTO employees (badge_id, name, job_level, org_path, org_path_ltree, is_active)
SELECT
    'B-' || lpad(i::text, 6, '0'),
    '部經理_' || lpad((i - 1)::text, 2, '0'),
    'MANAGER_L2',
    'TSMC.製造部_' || lpad((i - 1)::text, 2, '0'),
    text2ltree('TSMC.製造部_' || lpad((i - 1)::text, 2, '0')),
    TRUE
FROM generate_series(2, 11) AS i
ON CONFLICT (badge_id) DO NOTHING;

-- ── Phase 3: 989 員工 STAFF ─────────────────────────────────
INSERT INTO employees (badge_id, name, job_level, org_path, org_path_ltree, is_active)
SELECT
    'B-' || lpad(i::text, 6, '0'),
    '員工_' || lpad(i::text, 6, '0'),
    'STAFF',
    'TSMC.製造部_' || lpad(((i - 12) % 10 + 1)::text, 2, '0'),
    text2ltree('TSMC.製造部_' || lpad(((i - 12) % 10 + 1)::text, 2, '0')),
    TRUE
FROM generate_series(12, 1000) AS i
ON CONFLICT (badge_id) DO NOTHING;

-- ── Phase 4: 恢復環境與刷新 ───────────────────────────────
SET session_replication_role = 'origin';
REFRESH MATERIALIZED VIEW CONCURRENTLY mv_daily_attendance;

-- ── 確認結果 ─────────────────────────────────────────────────
DO $$
DECLARE
    v_total    int;
    v_l1       int;
    v_l2       int;
    v_staff    int;
BEGIN
    SELECT COUNT(*) INTO v_total  FROM employees WHERE is_active;
    SELECT COUNT(*) INTO v_l1     FROM employees WHERE job_level = 'MANAGER_L1' AND is_active;
    SELECT COUNT(*) INTO v_l2     FROM employees WHERE job_level = 'MANAGER_L2' AND is_active;
    SELECT COUNT(*) INTO v_staff  FROM employees WHERE job_level = 'STAFF'      AND is_active;

    RAISE NOTICE '=== 0103 本地播種結果 ===';
    RAISE NOTICE '  廠長   (L1): %', v_l1;
    RAISE NOTICE '  部經理 (L2): %', v_l2;
    RAISE NOTICE '  員工 (STAFF): %', v_staff;
    RAISE NOTICE '  總計: % 人', v_total;

    IF v_total < 1000 THEN
        RAISE EXCEPTION '播種異常：預期至少 1000 人，實際僅 % 人', v_total;
    END IF;
END;
$$;

