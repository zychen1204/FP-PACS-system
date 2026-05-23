-- ============================================================
-- 0104_cloud_seed.up.sql
--
-- 雲端生產 / 壓力測試用大規模資料播種
-- 規模：90,000 人
--   - 1      廠長        (MANAGER_L1) : B-000001
--   - 150    部門經理    (MANAGER_L2) : B-000002  ~ B-000151
--   - 89,849 員工        (STAFF)      : B-000152  ~ B-090000
--
-- 組織樹：
--   TSMC
--   └─ 部002 ~ 部151  (150 個部門)
--      └─ 每部約 599 名員工
--
-- ⚠️  警告：此 migration 約需 2~5 分鐘，請手動執行，不納入自動 migrate！
--
-- 執行方式（GKE 環境）：
--   # 方式 A：透過 Cloud SQL Auth Proxy
--   gcloud sql connect <INSTANCE_NAME> \
--     --user=pacs_user \
--     --database=pacs_db \
--     < scripts/migrations/0104_cloud_seed.up.sql
--
--   # 方式 B：kubectl exec（需要先 port-forward Cloud SQL Proxy）
--   kubectl run psql-seeder --rm -it --image=postgres:16-alpine \
--     --env="PGPASSWORD=<DB_PASSWORD>" -- \
--     psql -h <CLOUD_SQL_PROXY_IP> -U pacs_user -d pacs_db \
--     -f /migrations/0104_cloud_seed.up.sql
--
-- 本地小規模播種請改用：
--   0103_seed_local.up.sql  (1,000 人，自動執行)
-- ============================================================

-- ── Phase 1: 廠長 L1 ────────────────────────────────────────
INSERT INTO employees (badge_id, name, job_level, org_path, org_path_ltree, is_active)
VALUES (
    'B-000001',
    '廠長_總管',
    'MANAGER_L1',
    'TSMC',
    text2ltree('TSMC'),
    TRUE
)
ON CONFLICT (badge_id) DO UPDATE
    SET name           = EXCLUDED.name,
        job_level      = EXCLUDED.job_level,
        org_path       = EXCLUDED.org_path,
        org_path_ltree = EXCLUDED.org_path_ltree,
        is_active      = EXCLUDED.is_active;

-- ── Phase 2: 150 部門經理 L2 ────────────────────────────────
-- badge: B-000002 ~ B-000151
-- org_path: TSMC.部002 ~ TSMC.部151
INSERT INTO employees (badge_id, name, job_level, org_path, org_path_ltree, is_active)
SELECT
    'B-' || lpad(i::text, 6, '0'),
    '部經理_' || lpad(i::text, 3, '0'),
    'MANAGER_L2',
    'TSMC.部' || lpad(i::text, 3, '0'),
    text2ltree('TSMC.部' || lpad(i::text, 3, '0')),
    TRUE
FROM generate_series(2, 151) AS i
ON CONFLICT (badge_id) DO UPDATE
    SET name           = EXCLUDED.name,
        job_level      = EXCLUDED.job_level,
        org_path       = EXCLUDED.org_path,
        org_path_ltree = EXCLUDED.org_path_ltree,
        is_active      = EXCLUDED.is_active;

-- ── Phase 3: 89,849 員工 STAFF ──────────────────────────────
-- badge: B-000152 ~ B-090000
-- 均勻分配到 150 個部門（每部約 599 人）
-- 部門索引 = ((i - 152) % 150) + 2  → 部002 ~ 部151
INSERT INTO employees (badge_id, name, job_level, org_path, org_path_ltree, is_active)
SELECT
    'B-' || lpad(i::text, 6, '0'),
    '員工_' || lpad(i::text, 6, '0'),
    'STAFF',
    'TSMC.部' || lpad((((i - 152) % 150) + 2)::text, 3, '0')
        || '.E_' || lpad(i::text, 6, '0'),
    text2ltree(
        'TSMC.部' || lpad((((i - 152) % 150) + 2)::text, 3, '0')
        || '.E_' || lpad(i::text, 6, '0')
    ),
    TRUE
FROM generate_series(152, 90000) AS i
ON CONFLICT (badge_id) DO UPDATE
    SET name           = EXCLUDED.name,
        job_level      = EXCLUDED.job_level,
        org_path       = EXCLUDED.org_path,
        org_path_ltree = EXCLUDED.org_path_ltree,
        is_active      = EXCLUDED.is_active;

-- ── Phase 4: 刷新 Materialized View ─────────────────────────
-- 注意：CONCURRENTLY 需要 mv_daily_attendance 至少有一個 UNIQUE INDEX
REFRESH MATERIALIZED VIEW CONCURRENTLY mv_daily_attendance;

-- ── Phase 5: 驗證播種結果 ────────────────────────────────────
DO $$
DECLARE
    v_total    int;
    v_l1       int;
    v_l2       int;
    v_staff    int;
BEGIN
    SELECT COUNT(*) INTO v_total
    FROM employees
    WHERE is_active
      AND badge_id BETWEEN 'B-000001' AND 'B-090000';

    SELECT COUNT(*) INTO v_l1
    FROM employees
    WHERE job_level = 'MANAGER_L1'
      AND is_active
      AND badge_id BETWEEN 'B-000001' AND 'B-090000';

    SELECT COUNT(*) INTO v_l2
    FROM employees
    WHERE job_level = 'MANAGER_L2'
      AND is_active
      AND badge_id BETWEEN 'B-000001' AND 'B-090000';

    SELECT COUNT(*) INTO v_staff
    FROM employees
    WHERE job_level = 'STAFF'
      AND is_active
      AND badge_id BETWEEN 'B-000001' AND 'B-090000';

    RAISE NOTICE '=== 0104 雲端大規模播種結果 ===';
    RAISE NOTICE '  廠長    (L1) : %',  v_l1;
    RAISE NOTICE '  部經理  (L2) : %',  v_l2;
    RAISE NOTICE '  員工 (STAFF) : %',  v_staff;
    RAISE NOTICE '  總計         : % 人', v_total;
    RAISE NOTICE '  部門數       : 150 個 (部002~部151)';
    RAISE NOTICE '  每部平均     : ~% 人', v_staff / 150;

    IF v_total != 90000 THEN
        RAISE EXCEPTION '播種異常：預期 90,000 人，實際 % 人', v_total;
    END IF;

    RAISE NOTICE '✅ 90,000 人播種完畢！可開始 GKE 壓力測試';
END;
$$;

-- ── 索引效能確認（選擇性執行）───────────────────────────────
-- ANALYZE employees;
-- ANALYZE access_events;
